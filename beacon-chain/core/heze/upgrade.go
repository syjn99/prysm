package heze

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// UpgradeToHeze upgrades a Gloas state to a Heze state with the Heze fork version.
func UpgradeToHeze(beaconState state.BeaconState) (state.BeaconState, error) {
	pre, err := state_native.ContainerFrom[*ethpb.BeaconStateGloas](beaconState.ToContainer())
	if err != nil {
		return nil, errors.Wrap(err, "could not convert state to Gloas protobuf")
	}
	bid := pre.LatestExecutionPayloadBid
	if bid == nil {
		return nil, errors.New("nil latest execution payload bid")
	}

	s := &ethpb.BeaconStateHeze{
		GenesisTime:           pre.GenesisTime,
		GenesisValidatorsRoot: pre.GenesisValidatorsRoot,
		Slot:                  pre.Slot,
		Fork: &ethpb.Fork{
			PreviousVersion: pre.Fork.CurrentVersion,
			CurrentVersion:  params.BeaconConfig().HezeForkVersion,
			Epoch:           time.CurrentEpoch(beaconState),
		},
		LatestBlockHeader:             pre.LatestBlockHeader,
		BlockRoots:                    pre.BlockRoots,
		StateRoots:                    pre.StateRoots,
		HistoricalRoots:               pre.HistoricalRoots,
		Eth1Data:                      pre.Eth1Data,
		Eth1DataVotes:                 pre.Eth1DataVotes,
		Eth1DepositIndex:              pre.Eth1DepositIndex,
		Validators:                    pre.Validators,
		Balances:                      pre.Balances,
		RandaoMixes:                   pre.RandaoMixes,
		Slashings:                     pre.Slashings,
		PreviousEpochParticipation:    pre.PreviousEpochParticipation,
		CurrentEpochParticipation:     pre.CurrentEpochParticipation,
		JustificationBits:             pre.JustificationBits,
		PreviousJustifiedCheckpoint:   pre.PreviousJustifiedCheckpoint,
		CurrentJustifiedCheckpoint:    pre.CurrentJustifiedCheckpoint,
		FinalizedCheckpoint:           pre.FinalizedCheckpoint,
		InactivityScores:              pre.InactivityScores,
		CurrentSyncCommittee:          pre.CurrentSyncCommittee,
		NextSyncCommittee:             pre.NextSyncCommittee,
		LatestBlockHash:               pre.LatestBlockHash,
		NextWithdrawalIndex:           pre.NextWithdrawalIndex,
		NextWithdrawalValidatorIndex:  pre.NextWithdrawalValidatorIndex,
		HistoricalSummaries:           pre.HistoricalSummaries,
		DepositRequestsStartIndex:     pre.DepositRequestsStartIndex,
		DepositBalanceToConsume:       pre.DepositBalanceToConsume,
		ExitBalanceToConsume:          pre.ExitBalanceToConsume,
		EarliestExitEpoch:             pre.EarliestExitEpoch,
		ConsolidationBalanceToConsume: pre.ConsolidationBalanceToConsume,
		EarliestConsolidationEpoch:    pre.EarliestConsolidationEpoch,
		PendingDeposits:               pre.PendingDeposits,
		PendingPartialWithdrawals:     pre.PendingPartialWithdrawals,
		PendingConsolidations:         pre.PendingConsolidations,
		ProposerLookahead:             pre.ProposerLookahead,
		Builders:                      pre.Builders,
		NextWithdrawalBuilderIndex:    pre.NextWithdrawalBuilderIndex,
		ExecutionPayloadAvailability:  pre.ExecutionPayloadAvailability,
		BuilderPendingPayments:        pre.BuilderPendingPayments,
		BuilderPendingWithdrawals:     pre.BuilderPendingWithdrawals,
		LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBidHeze{
			ParentBlockHash:       bid.ParentBlockHash,
			ParentBlockRoot:       bid.ParentBlockRoot,
			BlockHash:             bid.BlockHash,
			PrevRandao:            bid.PrevRandao,
			FeeRecipient:          bid.FeeRecipient,
			GasLimit:              bid.GasLimit,
			BuilderIndex:          bid.BuilderIndex,
			Slot:                  bid.Slot,
			Value:                 bid.Value,
			ExecutionPayment:      bid.ExecutionPayment,
			BlobKzgCommitments:    bid.BlobKzgCommitments,
			ExecutionRequestsRoot: bid.ExecutionRequestsRoot,
			InclusionListBits:     bitfield.NewBitvector16(),
		},
		PayloadExpectedWithdrawals: pre.PayloadExpectedWithdrawals,
		PtcWindow:                  pre.PtcWindow,
	}
	initPQKeyRegistry(s)
	return state_native.NewUnsafe(s)
}

// initPQKeyRegistry adds one unset EIP-8450 post-quantum key entry per validator and builder.
//
//	post_quantum_key_material=[PQKeyMaterial() for _ in range(len(pre.validators))],
//	post_quantum_key_change_epochs=[FAR_FUTURE_EPOCH for _ in range(len(pre.validators))],
//	builder_post_quantum_key_material=[PQKeyMaterial() for _ in range(len(pre.builders))],
//	builder_post_quantum_key_change_epochs=[builder.deposit_epoch for builder in pre.builders],
func initPQKeyRegistry(s *ethpb.BeaconStateHeze) {
	// All unset entries share one all-zero value: the state replaces entries and never mutates them.
	empty := &ethpb.PQKeyMaterial{XmssPublicKey: make([]byte, 32), VrfRoot: make([]byte, 32)}
	s.PostQuantumKeyMaterial = make([]*ethpb.PQKeyMaterial, len(s.Validators))
	s.PostQuantumKeyChangeEpochs = make([]primitives.Epoch, len(s.Validators))
	for i := range s.Validators {
		s.PostQuantumKeyMaterial[i] = empty
		s.PostQuantumKeyChangeEpochs[i] = params.BeaconConfig().FarFutureEpoch
	}
	s.BuilderPostQuantumKeyMaterial = make([]*ethpb.PQKeyMaterial, len(s.Builders))
	s.BuilderPostQuantumKeyChangeEpochs = make([]primitives.Epoch, len(s.Builders))
	for i, builder := range s.Builders {
		s.BuilderPostQuantumKeyMaterial[i] = empty
		if builder != nil {
			s.BuilderPostQuantumKeyChangeEpochs[i] = builder.DepositEpoch
		}
	}
}
