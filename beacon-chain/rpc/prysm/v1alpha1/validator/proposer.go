package validator

import (
	"context"
	"fmt"
	"sync"
	"time"

	builderapi "github.com/OffchainLabs/prysm/v7/api/client/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	blockfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/block"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// eth1DataNotification is a latch to stop flooding logs with the same warning.
var eth1DataNotification bool

const (
	eth1dataTimeout           = 2 * time.Second
	defaultBuilderBoostFactor = primitives.Gwei(100)
)

var errOptimisticMode = fmt.Errorf("the node is currently optimistic and cannot serve validators")

func (vs *Server) optimisticStatus(ctx context.Context) error {
	if slots.ToEpoch(vs.TimeFetcher.CurrentSlot()) < params.BeaconConfig().BellatrixForkEpoch {
		return nil
	}
	optimistic, err := vs.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		return status.Errorf(codes.Internal, "Could not determine if the node is a optimistic node: %v", err)
	}
	if !optimistic {
		return nil
	}
	return status.Errorf(codes.Unavailable, "error=%v", errOptimisticMode)
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// GetBeaconBlock is called by a proposer during its assigned slot to request a block to sign
// by passing in the slot and the signed randao reveal of the slot.
// Delegates to BlockProducer.ProduceBlock for the actual block construction.
func (vs *Server) GetBeaconBlock(ctx context.Context, req *ethpb.BlockRequest) (*ethpb.GenericBeaconBlock, error) {
	log.Warn("This gRPC endpoint is deprecated and will be removed. Please migrate to the Beacon REST API.")
	builderBoostFactor := defaultBuilderBoostFactor
	if req.BuilderBoostFactor != nil {
		builderBoostFactor = primitives.Gwei(req.BuilderBoostFactor.Value)
	}
	return vs.BlockProducer.ProduceBlock(ctx, req.Slot, req.RandaoReveal, req.Graffiti, req.SkipMevBoost, builderBoostFactor)
}

// BuildBlockParallel delegates to BlockProducer.BuildBlockParallel.
func (vs *Server) BuildBlockParallel(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, skipMevBoost bool, builderBoostFactor primitives.Gwei) (*ethpb.GenericBeaconBlock, error) {
	return vs.BlockProducer.BuildBlockParallel(ctx, sBlk, head, skipMevBoost, builderBoostFactor)
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// ProposeBeaconBlock handles the proposal of beacon blocks.
func (vs *Server) ProposeBeaconBlock(ctx context.Context, req *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error) {
	log.Warn("This gRPC endpoint is deprecated and will be removed. Please migrate to the Beacon REST API.")
	var (
		blobSidecars       []*ethpb.BlobSidecar
		dataColumnSidecars []blocks.RODataColumn
	)

	ctx, span := trace.StartSpan(ctx, "ProposerServer.ProposeBeaconBlock")
	defer span.End()

	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	block, err := blocks.NewSignedBeaconBlock(req.Block)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s: %v", "decode block failed", err)
	}
	root, err := block.Block().HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not hash tree root: %v", err)
	}

	// For post-Fulu blinded blocks, submit to relay and return early
	if block.IsBlinded() && slots.ToEpoch(block.Block().Slot()) >= params.BeaconConfig().FuluForkEpoch {
		err := vs.BlockBuilder.SubmitBlindedBlockPostFulu(ctx, block)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not submit blinded block post-Fulu: %v", err)
		}
		return &ethpb.ProposeResponse{BlockRoot: root[:]}, nil
	}

	rob, err := blocks.NewROBlockWithRoot(block, root)
	if block.IsBlinded() {
		block, blobSidecars, err = vs.handleBlindedBlock(ctx, block)
		if errors.Is(err, builderapi.ErrBadGateway) {
			log.WithError(err).Info("Optimistically proposed block - builder relay temporarily unavailable, block may arrive over P2P")
			return &ethpb.ProposeResponse{BlockRoot: root[:]}, nil
		}
	} else if block.Version() >= version.Deneb {
		blobSidecars, dataColumnSidecars, err = vs.handleUnblindedBlock(rob, req)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%s: %v", "handle block failed", err)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	wg.Add(1)
	go func() {
		if err := vs.broadcastReceiveBlock(ctx, &wg, block, root); err != nil {
			errChan <- errors.Wrap(err, "broadcast/receive block failed")
			return
		}
		errChan <- nil
	}()

	wg.Wait()

	if err := vs.broadcastAndReceiveSidecars(ctx, block, root, blobSidecars, dataColumnSidecars); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not broadcast/receive sidecars: %v", err)
	}
	if err := <-errChan; err != nil {
		return nil, status.Errorf(codes.Internal, "Could not broadcast/receive block: %v", err)
	}

	return &ethpb.ProposeResponse{BlockRoot: root[:]}, nil
}

// broadcastAndReceiveSidecars broadcasts and receives sidecars.
func (vs *Server) broadcastAndReceiveSidecars(
	ctx context.Context,
	block interfaces.SignedBeaconBlock,
	root [fieldparams.RootLength]byte,
	blobSidecars []*ethpb.BlobSidecar,
	dataColumnSidecars []blocks.RODataColumn,
) error {
	if block.Version() >= version.Fulu {
		if err := vs.broadcastAndReceiveDataColumns(ctx, dataColumnSidecars); err != nil {
			return errors.Wrap(err, "broadcast and receive data columns")
		}
		return nil
	}

	if err := vs.broadcastAndReceiveBlobs(ctx, blobSidecars, root); err != nil {
		return errors.Wrap(err, "broadcast and receive blobs")
	}

	return nil
}

// handleBlindedBlock processes blinded beacon blocks (pre-Fulu only).
// Post-Fulu blinded blocks are handled directly in ProposeBeaconBlock.
func (vs *Server) handleBlindedBlock(ctx context.Context, block interfaces.SignedBeaconBlock) (interfaces.SignedBeaconBlock, []*ethpb.BlobSidecar, error) {
	if block.Version() < version.Bellatrix {
		return nil, nil, errors.New("pre-Bellatrix blinded block")
	}

	if vs.BlockBuilder == nil || !vs.BlockBuilder.Configured() {
		return nil, nil, errors.New("unconfigured block builder")
	}

	copiedBlock, err := block.Copy()
	if err != nil {
		return nil, nil, err
	}

	payload, bundle, err := vs.BlockBuilder.SubmitBlindedBlock(ctx, block)
	if err != nil {
		return nil, nil, errors.Wrap(err, "submit blinded block failed")
	}

	if err := copiedBlock.Unblind(payload); err != nil {
		return nil, nil, errors.Wrap(err, "unblind failed")
	}

	sidecars, err := unblindBlobsSidecars(copiedBlock, bundle)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unblind blobs sidecars: commitment value doesn't match block")
	}

	return copiedBlock, sidecars, nil
}

func (vs *Server) handleUnblindedBlock(
	block blocks.ROBlock,
	req *ethpb.GenericSignedBeaconBlock,
) ([]*ethpb.BlobSidecar, []blocks.RODataColumn, error) {
	rawBlobs, proofs, err := blobsAndProofs(req)
	if err != nil {
		return nil, nil, err
	}

	if block.Version() >= version.Fulu {
		// Compute cells and proofs from the blobs and cell proofs.
		cellsPerBlob, proofsPerBlob, err := peerdas.ComputeCellsAndProofsFromFlat(rawBlobs, proofs)
		if err != nil {
			return nil, nil, errors.Wrap(err, "compute cells and proofs")
		}

		// Construct data column sidecars from the signed block and cells and proofs.
		roDataColumnSidecars, err := peerdas.DataColumnSidecars(cellsPerBlob, proofsPerBlob, peerdas.PopulateFromBlock(block))
		if err != nil {
			return nil, nil, errors.Wrap(err, "data column sidcars")
		}

		return nil, roDataColumnSidecars, nil
	}

	blobSidecars, err := BuildBlobSidecars(block, rawBlobs, proofs)
	if err != nil {
		return nil, nil, errors.Wrap(err, "build blob sidecars")
	}

	return blobSidecars, nil, nil
}

// broadcastReceiveBlock broadcasts a block and handles its reception.
func (vs *Server) broadcastReceiveBlock(ctx context.Context, wg *sync.WaitGroup, block interfaces.SignedBeaconBlock, root [fieldparams.RootLength]byte) error {
	if err := vs.broadcastBlock(ctx, wg, block, root); err != nil {
		return errors.Wrap(err, "broadcast block")
	}

	vs.BlockNotifier.BlockFeed().Send(&feed.Event{
		Type: blockfeed.ReceivedBlock,
		Data: &blockfeed.ReceivedBlockData{SignedBlock: block},
	})

	if err := vs.BlockReceiver.ReceiveBlock(ctx, block, root, nil); err != nil {
		return errors.Wrap(err, "receive block")
	}

	return nil
}

func (vs *Server) broadcastBlock(ctx context.Context, wg *sync.WaitGroup, block interfaces.SignedBeaconBlock, root [fieldparams.RootLength]byte) error {
	defer wg.Done()

	protoBlock, err := block.Proto()
	if err != nil {
		return errors.Wrap(err, "protobuf conversion failed")
	}
	if err := vs.P2P.Broadcast(ctx, protoBlock); err != nil {
		return errors.Wrap(err, "broadcast failed")
	}

	log.WithFields(logrus.Fields{
		"slot": block.Block().Slot(),
		"root": fmt.Sprintf("%#x", root),
	}).Debug("Broadcasted block")

	return nil
}

// broadcastAndReceiveBlobs handles the broadcasting and reception of blob sidecars.
func (vs *Server) broadcastAndReceiveBlobs(ctx context.Context, sidecars []*ethpb.BlobSidecar, root [fieldparams.RootLength]byte) error {
	eg, eCtx := errgroup.WithContext(ctx)
	for subIdx, sc := range sidecars {
		eg.Go(func() error {
			if err := vs.P2P.BroadcastBlob(eCtx, uint64(subIdx), sc); err != nil {
				return errors.Wrap(err, "broadcast blob failed")
			}
			readOnlySc, err := blocks.NewROBlobWithRoot(sc, root)
			if err != nil {
				return errors.Wrap(err, "ROBlob creation failed")
			}
			verifiedBlob := blocks.NewVerifiedROBlob(readOnlySc)
			if err := vs.BlobReceiver.ReceiveBlob(ctx, verifiedBlob); err != nil {
				return errors.Wrap(err, "receive blob failed")
			}
			vs.OperationNotifier.OperationFeed().Send(&feed.Event{
				Type: operation.BlobSidecarReceived,
				Data: &operation.BlobSidecarReceivedData{Blob: &verifiedBlob},
			})
			return nil
		})
	}
	return eg.Wait()
}

// broadcastAndReceiveDataColumns handles the broadcasting and reception of data columns sidecars.
func (vs *Server) broadcastAndReceiveDataColumns(ctx context.Context, roSidecars []blocks.RODataColumn) error {
	// We built this block ourselves, so we can upgrade the read only data column sidecar into a verified one.
	verifiedSidecars := make([]blocks.VerifiedRODataColumn, 0, len(roSidecars))
	for _, sidecar := range roSidecars {
		verifiedSidecar := blocks.NewVerifiedRODataColumn(sidecar)
		verifiedSidecars = append(verifiedSidecars, verifiedSidecar)
	}

	// Broadcast sidecars (non blocking).
	if err := vs.P2P.BroadcastDataColumnSidecars(ctx, verifiedSidecars); err != nil {
		return errors.Wrap(err, "broadcast data column sidecars")
	}

	// In parallel, receive sidecars.
	if err := vs.DataColumnReceiver.ReceiveDataColumns(verifiedSidecars); err != nil {
		return errors.Wrap(err, "receive data columns")
	}

	return nil
}

// computeStateRoot computes the state root after a block has been processed through a state transition and
// returns it to the validator client.
func (vs *Server) computeStateRoot(ctx context.Context, block interfaces.SignedBeaconBlock) ([]byte, error) {
	beaconState, err := vs.StateGen.StateByRoot(ctx, block.Block().ParentRoot())
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve beacon state")
	}
	root, err := transition.CalculateStateRoot(
		ctx,
		beaconState,
		block,
	)
	if err != nil {
		return vs.handleStateRootError(ctx, block, err)
	}

	log.WithField("beaconStateRoot", fmt.Sprintf("%#x", root)).Debugf("Computed state root")
	return root[:], nil
}

type computeStateRootAttemptsKeyType string

const computeStateRootAttemptsKey = computeStateRootAttemptsKeyType("compute-state-root-attempts")
const maxComputeStateRootAttempts = 3

// handleStateRootError retries block construction in some error cases.
func (vs *Server) handleStateRootError(ctx context.Context, block interfaces.SignedBeaconBlock, err error) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, status.Errorf(codes.Canceled, "context error: %v", ctx.Err())
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
	return vs.computeStateRoot(ctx, block)
}

func blobsAndProofs(req *ethpb.GenericSignedBeaconBlock) ([][]byte, [][]byte, error) {
	switch {
	case req.GetDeneb() != nil:
		dbBlockContents := req.GetDeneb()
		return dbBlockContents.Blobs, dbBlockContents.KzgProofs, nil
	case req.GetElectra() != nil:
		dbBlockContents := req.GetElectra()
		return dbBlockContents.Blobs, dbBlockContents.KzgProofs, nil
	case req.GetFulu() != nil:
		dbBlockContents := req.GetFulu()
		return dbBlockContents.Blobs, dbBlockContents.KzgProofs, nil
	default:
		return nil, nil, errors.Errorf("unknown request type provided: %T", req)
	}
}
