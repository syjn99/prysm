package heze

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// UpgradeToHeze upgrades a Gloas state to a Heze state.
//
//	<spec fn="upgrade_to_heze" fork="heze">
//	def upgrade_to_heze(pre: gloas.BeaconState) -> BeaconState:
//	    epoch = gloas.get_current_epoch(pre)
//	    latest_execution_payload_bid = ExecutionPayloadBid(
//	        parent_block_hash=pre.latest_execution_payload_bid.parent_block_hash,
//	        parent_block_root=pre.latest_execution_payload_bid.parent_block_root,
//	        block_hash=pre.latest_execution_payload_bid.block_hash,
//	        prev_randao=pre.latest_execution_payload_bid.prev_randao,
//	        fee_recipient=pre.latest_execution_payload_bid.fee_recipient,
//	        gas_limit=pre.latest_execution_payload_bid.gas_limit,
//	        builder_index=pre.latest_execution_payload_bid.builder_index,
//	        slot=pre.latest_execution_payload_bid.slot,
//	        value=pre.latest_execution_payload_bid.value,
//	        execution_payment=pre.latest_execution_payload_bid.execution_payment,
//	        blob_kzg_commitments=pre.latest_execution_payload_bid.blob_kzg_commitments,
//	        execution_requests_root=pre.latest_execution_payload_bid.execution_requests_root,
//	        # [New in Heze:EIP7805]
//	        inclusion_list_bits=InclusionListBits(),
//	    )
//
//	    post = BeaconState(
//	        genesis_time=pre.genesis_time,
//	        genesis_validators_root=pre.genesis_validators_root,
//	        slot=pre.slot,
//	        fork=Fork(
//	            previous_version=pre.fork.current_version,
//	            # [Modified in Heze]
//	            current_version=HEZE_FORK_VERSION,
//	            epoch=epoch,
//	        ),
//	        latest_block_header=pre.latest_block_header,
//	        block_roots=pre.block_roots,
//	        state_roots=pre.state_roots,
//	        historical_roots=pre.historical_roots,
//	        eth1_data=pre.eth1_data,
//	        eth1_data_votes=pre.eth1_data_votes,
//	        eth1_deposit_index=pre.eth1_deposit_index,
//	        validators=pre.validators,
//	        balances=pre.balances,
//	        randao_mixes=pre.randao_mixes,
//	        slashings=pre.slashings,
//	        previous_epoch_participation=pre.previous_epoch_participation,
//	        current_epoch_participation=pre.current_epoch_participation,
//	        justification_bits=pre.justification_bits,
//	        previous_justified_checkpoint=pre.previous_justified_checkpoint,
//	        current_justified_checkpoint=pre.current_justified_checkpoint,
//	        finalized_checkpoint=pre.finalized_checkpoint,
//	        inactivity_scores=pre.inactivity_scores,
//	        current_sync_committee=pre.current_sync_committee,
//	        next_sync_committee=pre.next_sync_committee,
//	        latest_block_hash=pre.latest_block_hash,
//	        next_withdrawal_index=pre.next_withdrawal_index,
//	        next_withdrawal_validator_index=pre.next_withdrawal_validator_index,
//	        historical_summaries=pre.historical_summaries,
//	        deposit_requests_start_index=pre.deposit_requests_start_index,
//	        deposit_balance_to_consume=pre.deposit_balance_to_consume,
//	        exit_balance_to_consume=pre.exit_balance_to_consume,
//	        earliest_exit_epoch=pre.earliest_exit_epoch,
//	        consolidation_balance_to_consume=pre.consolidation_balance_to_consume,
//	        earliest_consolidation_epoch=pre.earliest_consolidation_epoch,
//	        pending_deposits=pre.pending_deposits,
//	        pending_partial_withdrawals=pre.pending_partial_withdrawals,
//	        pending_consolidations=pre.pending_consolidations,
//	        proposer_lookahead=pre.proposer_lookahead,
//	        builders=pre.builders,
//	        next_withdrawal_builder_index=pre.next_withdrawal_builder_index,
//	        execution_payload_availability=pre.execution_payload_availability,
//	        builder_pending_payments=pre.builder_pending_payments,
//	        builder_pending_withdrawals=pre.builder_pending_withdrawals,
//	        # [Modified in Heze:EIP7805]
//	        latest_execution_payload_bid=latest_execution_payload_bid,
//	        payload_expected_withdrawals=pre.payload_expected_withdrawals,
//	        ptc_window=pre.ptc_window,
//	    )
//
//	    return post
//	</spec>
func UpgradeToHeze(beaconState state.BeaconState) (state.BeaconState, error) {
	pre, ok := beaconState.ToProtoUnsafe().(*ethpb.BeaconStateGloas)
	if !ok {
		return nil, errors.New("input is not a gloas beacon state")
	}

	post := &ethpb.BeaconStateHeze{
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
		LatestExecutionPayloadBid:     upgradeBid(pre.LatestExecutionPayloadBid),
		PayloadExpectedWithdrawals:    pre.PayloadExpectedWithdrawals,
		PtcWindow:                     pre.PtcWindow,
	}

	return state_native.InitializeFromProtoUnsafeHeze(post)
}

// upgradeBid copies a Gloas bid into its Heze shape with empty inclusion list bits.
func upgradeBid(pre *ethpb.ExecutionPayloadBid) *ethpb.ExecutionPayloadBidHeze {
	if pre == nil {
		return nil
	}
	return &ethpb.ExecutionPayloadBidHeze{
		ParentBlockHash:       pre.ParentBlockHash,
		ParentBlockRoot:       pre.ParentBlockRoot,
		BlockHash:             pre.BlockHash,
		PrevRandao:            pre.PrevRandao,
		FeeRecipient:          pre.FeeRecipient,
		GasLimit:              pre.GasLimit,
		BuilderIndex:          pre.BuilderIndex,
		Slot:                  pre.Slot,
		Value:                 pre.Value,
		ExecutionPayment:      pre.ExecutionPayment,
		BlobKzgCommitments:    pre.BlobKzgCommitments,
		ExecutionRequestsRoot: pre.ExecutionRequestsRoot,
		InclusionListBits:     make([]byte, (fieldparams.InclusionListCommitteeSize+7)/8),
	}
}
