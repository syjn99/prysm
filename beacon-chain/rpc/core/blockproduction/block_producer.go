package blockproduction

import (
	"context"
	"fmt"
	"sync"
	"time"

	builderapi "github.com/OffchainLabs/prysm/v7/api/client/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache/depositsnapshot"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/slashings"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/synccommittee"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/voluntaryexits"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	ethsync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// eth1DataNotification is a latch to stop flooding logs with the same warning.
var eth1DataNotification bool

const (
	eth1dataTimeout = 2 * time.Second
	// DefaultBuilderBoostFactor is the default builder boost factor used when none is specified.
	DefaultBuilderBoostFactor = primitives.Gwei(100)
)

var errOptimisticMode = fmt.Errorf("the node is currently optimistic and cannot serve validators")

// BlockProducer holds all dependencies needed for block production.
// It replaces the gRPC Server struct as the receiver for shared block-production logic.
type BlockProducer struct {
	// Blockchain state
	HeadFetcher           blockchain.HeadFetcher
	ForkchoiceFetcher     blockchain.ForkchoiceFetcher
	TimeFetcher           blockchain.TimeFetcher
	FinalizationFetcher   blockchain.FinalizationFetcher
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
	SyncChecker           ethsync.Checker

	// Execution layer
	Eth1InfoFetcher       execution.ChainInfoFetcher
	Eth1BlockFetcher      execution.POWBlockFetcher
	ChainStartFetcher     execution.ChainStartFetcher
	ExecutionEngineCaller execution.EngineCaller

	// Caches
	PayloadIDCache         *cache.PayloadIDCache
	TrackedValidatorsCache *cache.TrackedValidatorsCache
	AttestationCache       *cache.AttestationCache

	// Operation pools
	AttPool           attestations.Pool
	SlashingsPool     slashings.PoolManager
	ExitPool          voluntaryexits.PoolManager
	SyncCommitteePool synccommittee.Pool
	BLSChangesPool    blstoexec.PoolManager

	// Deposit handling
	DepositFetcher         cache.DepositFetcher
	PendingDepositsFetcher depositsnapshot.PendingDepositsFetcher

	// State generation
	StateGen stategen.StateManager

	// MEV builder client (external block builder in the PBS pipeline)
	BlockBuilderClient builder.BlockBuilder

	// Misc
	MockEth1Votes bool
	GraffitiInfo  *execution.GraffitiInfo

	// Block proposal (broadcasting/receiving) deps
	Proposer *BlockProposerDeps
}

func (b *BlockProducer) optimisticStatus(ctx context.Context) error {
	if slots.ToEpoch(b.TimeFetcher.CurrentSlot()) < params.BeaconConfig().BellatrixForkEpoch {
		return nil
	}
	optimistic, err := b.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		return fmt.Errorf("could not determine if the node is a optimistic node: %v", err)
	}
	if !optimistic {
		return nil
	}
	return fmt.Errorf("error=%v", errOptimisticMode)
}

// ProduceBlock builds a complete unsigned beacon block. This is the extracted
// equivalent of the old GetBeaconBlock gRPC handler, minus the gRPC error
// wrapping and deprecation logging.
func (b *BlockProducer) ProduceBlock(ctx context.Context, slot primitives.Slot, randaoReveal []byte, graffiti []byte, skipMevBoost bool, builderBoostFactor primitives.Gwei) (*ethpb.GenericBeaconBlock, error) {
	ctx, span := trace.StartSpan(ctx, "BlockProducer.ProduceBlock")
	defer span.End()
	span.SetAttributes(trace.Int64Attribute("slot", int64(slot))) // lint:ignore uintcast -- OK for tracing.

	t, err := slots.StartTime(b.TimeFetcher.GenesisTime(), slot)
	if err != nil {
		log.WithError(err).Error("Could not convert slot to time")
	}

	log := log.WithField("slot", slot)
	log.WithField("sinceSlotStartTime", time.Since(t)).Info("Begin building block")

	// A syncing validator should not produce a block.
	if b.SyncChecker.Syncing() {
		log.Error("Fail to build block: node is syncing")
		return nil, fmt.Errorf("syncing to latest head, not ready to respond")
	}
	// An optimistic validator MUST NOT produce a block (i.e., sign across the DOMAIN_BEACON_PROPOSER domain).
	if slots.ToEpoch(slot) >= params.BeaconConfig().BellatrixForkEpoch {
		if err := b.optimisticStatus(ctx); err != nil {
			log.WithError(err).Error("Fail to build block: node is optimistic")
			return nil, fmt.Errorf("validator is not ready to propose: %v", err)
		}
	}

	head, parentRoot, err := b.getParentState(ctx, slot)
	if err != nil {
		log.WithError(err).Error("Fail to build block: could not get parent state")
		return nil, err
	}
	sBlk, err := getEmptyBlock(slot)
	if err != nil {
		log.WithError(err).Error("Fail to build block: could not get empty block")
		return nil, fmt.Errorf("could not prepare block: %v", err)
	}
	// Set slot, graffiti, randao reveal, and parent root.
	sBlk.SetSlot(slot)
	// Generate graffiti with client version info using flexible standard
	if b.GraffitiInfo != nil {
		g := b.GraffitiInfo.GenerateGraffiti(graffiti)
		sBlk.SetGraffiti(g[:])
	} else {
		sBlk.SetGraffiti(graffiti)
	}
	sBlk.SetRandaoReveal(randaoReveal)
	sBlk.SetParentRoot(parentRoot[:])

	// Set proposer index.
	idx, err := helpers.BeaconProposerIndex(ctx, head)
	if err != nil {
		return nil, fmt.Errorf("could not calculate proposer index %w", err)
	}
	sBlk.SetProposerIndex(idx)

	resp, err := b.BuildBlockParallel(ctx, sBlk, head, skipMevBoost, builderBoostFactor)
	log = log.WithFields(logrus.Fields{
		"sinceSlotStartTime": time.Since(t),
		"validator":          sBlk.Block().ProposerIndex(),
	})

	if err != nil {
		log.WithError(err).Error("Finished building block")
		return nil, errors.Wrap(err, "could not build block in parallel")
	}

	log.Info("Finished building block")
	return resp, nil
}

// BuildBlockParallel fills in the consensus and execution data for a prepared
// block shell. Exported for callers that manage their own block initialization.
func (b *BlockProducer) BuildBlockParallel(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, skipMevBoost bool, builderBoostFactor primitives.Gwei) (*ethpb.GenericBeaconBlock, error) {
	// Build consensus fields in background
	var wg sync.WaitGroup
	wg.Go(func() {

		// Set eth1 data.
		eth1Data, err := b.eth1DataMajorityVote(ctx, head)
		if err != nil {
			eth1Data = &ethpb.Eth1Data{DepositRoot: params.BeaconConfig().ZeroHash[:], BlockHash: params.BeaconConfig().ZeroHash[:]}
			log.WithError(err).Error("Could not get eth1data")
		}
		sBlk.SetEth1Data(eth1Data)

		// Set deposit and attestation.
		deposits, atts, err := b.packDepositsAndAttestations(ctx, head, sBlk.Block().Slot(), eth1Data) // TODO: split attestations and deposits
		if err != nil {
			sBlk.SetDeposits([]*ethpb.Deposit{})
			if err := sBlk.SetAttestations([]ethpb.Att{}); err != nil {
				log.WithError(err).Error("Could not set attestations on block")
			}
			log.WithError(err).Error("Could not pack deposits and attestations")
		} else {
			sBlk.SetDeposits(deposits)
			if err := sBlk.SetAttestations(atts); err != nil {
				log.WithError(err).Error("Could not set attestations on block")
			}
		}

		// Set slashings.
		validProposerSlashings, validAttSlashings := b.getSlashings(ctx, head)
		sBlk.SetProposerSlashings(validProposerSlashings)
		if err := sBlk.SetAttesterSlashings(validAttSlashings); err != nil {
			log.WithError(err).Error("Could not set attester slashings on block")
		}

		// Set exits.
		sBlk.SetVoluntaryExits(b.getExits(head, sBlk.Block().Slot()))

		// Set sync aggregate. New in Altair.
		b.setSyncAggregate(ctx, sBlk, head)

		// Set bls to execution change. New in Capella.
		b.setBlsToExecData(sBlk, head)
	})

	winningBid := primitives.ZeroWei()
	var bundle enginev1.BlobsBundler
	if sBlk.Version() >= version.Bellatrix {
		local, err := b.getLocalPayload(ctx, sBlk.Block(), head)
		if err != nil {
			return nil, fmt.Errorf("could not get local payload: %v", err)
		}

		// There's no reason to try to get a builder bid if local override is true.
		var builderBid builderapi.Bid
		if !(local.OverrideBuilder || skipMevBoost) {
			latestHeader, err := head.LatestExecutionPayloadHeader()
			if err != nil {
				return nil, fmt.Errorf("could not get latest execution payload header: %v", err)
			}
			parentGasLimit := latestHeader.GasLimit()
			builderBid, err = b.getBuilderPayloadAndBlobs(ctx, sBlk.Block().Slot(), sBlk.Block().ProposerIndex(), parentGasLimit)
			if err != nil {
				builderGetPayloadMissCount.Inc()
				log.WithError(err).Error("Could not get builder payload")
			}
		}

		winningBid, bundle, err = setExecutionData(ctx, sBlk, local, builderBid, builderBoostFactor)
		if err != nil {
			return nil, fmt.Errorf("could not set execution data: %v", err)
		}
	}

	wg.Wait()

	sr, err := b.computeStateRoot(ctx, sBlk)
	if err != nil {
		return nil, fmt.Errorf("could not compute state root: %v", err)
	}
	sBlk.SetStateRoot(sr)

	return b.constructGenericBeaconBlock(sBlk, bundle, winningBid)
}

func (b *BlockProducer) handleSuccesfulReorgAttempt(ctx context.Context, slot primitives.Slot, parentRoot, _ [32]byte) (state.BeaconState, error) {
	// Try to get the state from the NSC
	head := transition.NextSlotState(parentRoot[:], slot)
	if head != nil {
		return head, nil
	}
	// cache miss
	head, err := b.StateGen.StateByRoot(ctx, parentRoot)
	if err != nil {
		return nil, fmt.Errorf("could not obtain head state")
	}
	return head, nil
}

func logFailedReorgAttempt(slot primitives.Slot, oldHeadRoot, headRoot [32]byte) {
	blockchain.LateBlockAttemptedReorgCount.Inc()
	log.WithFields(logrus.Fields{
		"slot":        slot,
		"oldHeadRoot": fmt.Sprintf("%#x", oldHeadRoot),
		"headRoot":    fmt.Sprintf("%#x", headRoot),
	}).Warn("Late block attempted reorg failed")
}

func (b *BlockProducer) getHeadNoReorg(ctx context.Context, slot primitives.Slot, parentRoot [32]byte) (state.BeaconState, error) {
	// Try to get the state from the NSC
	head := transition.NextSlotState(parentRoot[:], slot)
	if head != nil {
		return head, nil
	}
	head, err := b.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get head state: %v", err)
	}
	return head, nil
}

func (b *BlockProducer) GetParentStateFromReorgData(ctx context.Context, slot primitives.Slot, oldHeadRoot, parentRoot, headRoot [32]byte) (head state.BeaconState, err error) {
	if parentRoot != headRoot {
		head, err = b.handleSuccesfulReorgAttempt(ctx, slot, parentRoot, headRoot)
	} else {
		if oldHeadRoot != headRoot {
			logFailedReorgAttempt(slot, oldHeadRoot, headRoot)
		}
		head, err = b.getHeadNoReorg(ctx, slot, parentRoot)
	}
	if err != nil {
		return nil, err
	}
	if head.Slot() >= slot {
		return head, nil
	}
	head, err = transition.ProcessSlotsUsingNextSlotCache(ctx, head, parentRoot[:], slot)
	if err != nil {
		return nil, fmt.Errorf("could not process slots up to %d: %v", slot, err)
	}
	return head, nil
}

func (b *BlockProducer) getParentState(ctx context.Context, slot primitives.Slot) (state.BeaconState, [32]byte, error) {
	// process attestations and update head in forkchoice
	oldHeadRoot := b.ForkchoiceFetcher.CachedHeadRoot()
	b.ForkchoiceFetcher.UpdateHead(ctx, b.TimeFetcher.CurrentSlot())
	headRoot := b.ForkchoiceFetcher.CachedHeadRoot()
	parentRoot := b.ForkchoiceFetcher.GetProposerHead()
	head, err := b.GetParentStateFromReorgData(ctx, slot, oldHeadRoot, parentRoot, headRoot)
	return head, parentRoot, err
}

// computeStateRoot computes the state root after a block has been processed through a state transition and
// returns it to the validator client.
func (b *BlockProducer) computeStateRoot(ctx context.Context, block interfaces.SignedBeaconBlock) ([]byte, error) {
	beaconState, err := b.StateGen.StateByRoot(ctx, block.Block().ParentRoot())
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve beacon state")
	}
	root, err := transition.CalculateStateRoot(
		ctx,
		beaconState,
		block,
	)
	if err != nil {
		return b.handleStateRootError(ctx, block, err)
	}

	log.WithField("beaconStateRoot", fmt.Sprintf("%#x", root)).Debugf("Computed state root")
	return root[:], nil
}

type computeStateRootAttemptsKeyType string

const computeStateRootAttemptsKey = computeStateRootAttemptsKeyType("compute-state-root-attempts")
const maxComputeStateRootAttempts = 3

// handleStateRootError retries block construction in some error cases.
func (b *BlockProducer) handleStateRootError(ctx context.Context, block interfaces.SignedBeaconBlock, err error) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, fmt.Errorf("context error: %v", ctx.Err())
	}
	switch {
	case errors.Is(err, transition.ErrAttestationsSignatureInvalid),
		errors.Is(err, transition.ErrProcessAttestationsFailed):
		log.WithError(err).Warn("Retrying block construction without attestations")
		if err := block.SetAttestations([]ethpb.Att{}); err != nil {
			return nil, errors.Wrap(err, "could not set attestations")
		}
	case errors.Is(err, transition.ErrProcessBLSChangesFailed), errors.Is(err, transition.ErrBLSToExecutionChangesSignatureInvalid):
		log.WithError(err).Warn("Retrying block construction without BLS to execution changes")
		if err := block.SetBLSToExecutionChanges([]*ethpb.SignedBLSToExecutionChange{}); err != nil {
			return nil, errors.Wrap(err, "could not set BLS to execution changes")
		}
	case errors.Is(err, transition.ErrProcessProposerSlashingsFailed):
		log.WithError(err).Warn("Retrying block construction without proposer slashings")
		block.SetProposerSlashings([]*ethpb.ProposerSlashing{})
	case errors.Is(err, transition.ErrProcessAttesterSlashingsFailed):
		log.WithError(err).Warn("Retrying block construction without attester slashings")
		if err := block.SetAttesterSlashings([]ethpb.AttSlashing{}); err != nil {
			return nil, errors.Wrap(err, "could not set attester slashings")
		}
	case errors.Is(err, transition.ErrProcessVoluntaryExitsFailed):
		log.WithError(err).Warn("Retrying block construction without voluntary exits")
		block.SetVoluntaryExits([]*ethpb.SignedVoluntaryExit{})
	case errors.Is(err, transition.ErrProcessSyncAggregateFailed):
		log.WithError(err).Warn("Retrying block construction without sync aggregate")
		emptySig := [96]byte{0xC0}
		emptyAggregate := &ethpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, params.BeaconConfig().SyncCommitteeSize/8),
			SyncCommitteeSignature: emptySig[:],
		}
		if err := block.SetSyncAggregate(emptyAggregate); err != nil {
			log.WithError(err).Error("Could not set sync aggregate")
		}

	default:
		return nil, errors.Wrap(err, "could not compute state root")
	}
	// prevent deep recursion by limiting max attempts.
	if v, ok := ctx.Value(computeStateRootAttemptsKey).(int); !ok {
		ctx = context.WithValue(ctx, computeStateRootAttemptsKey, int(1))
	} else if v >= maxComputeStateRootAttempts {
		return nil, fmt.Errorf("attempted max compute state root attempts %d", maxComputeStateRootAttempts)
	} else {
		ctx = context.WithValue(ctx, computeStateRootAttemptsKey, v+1)
	}
	// recursive call to compute state root again
	return b.computeStateRoot(ctx, block)
}
