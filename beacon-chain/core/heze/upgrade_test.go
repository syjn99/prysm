package heze_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestUpgradeToHeze(t *testing.T) {
	t.Run("carries every Gloas field over", func(t *testing.T) {
		st, _ := util.DeterministicGenesisStateGloas(t, params.BeaconConfig().MaxValidatorsPerCommittee)
		require.NoError(t, st.SetHistoricalRoots([][]byte{{1}}))
		require.NoError(t, st.SetPendingPartialWithdrawals([]*ethpb.PendingPartialWithdrawal{{Index: 1, Amount: 2}}))
		require.NoError(t, st.SetPendingConsolidations([]*ethpb.PendingConsolidation{{SourceIndex: 3, TargetIndex: 4}}))

		pre, ok := st.Copy().ToProtoUnsafe().(*ethpb.BeaconStateGloas)
		require.Equal(t, true, ok)

		post, err := heze.UpgradeToHeze(st)
		require.NoError(t, err)
		require.Equal(t, version.Heze, post.Version())

		got, ok := post.ToProtoUnsafe().(*ethpb.BeaconStateHeze)
		require.Equal(t, true, ok)

		require.Equal(t, pre.GenesisTime, got.GenesisTime)
		require.DeepEqual(t, pre.GenesisValidatorsRoot, got.GenesisValidatorsRoot)
		require.Equal(t, pre.Slot, got.Slot)
		require.DeepEqual(t, pre.LatestBlockHeader, got.LatestBlockHeader)
		require.DeepEqual(t, pre.BlockRoots, got.BlockRoots)
		require.DeepEqual(t, pre.StateRoots, got.StateRoots)
		require.DeepEqual(t, pre.HistoricalRoots, got.HistoricalRoots)
		require.DeepEqual(t, pre.Eth1Data, got.Eth1Data)
		require.DeepEqual(t, pre.Eth1DataVotes, got.Eth1DataVotes)
		require.Equal(t, pre.Eth1DepositIndex, got.Eth1DepositIndex)
		require.DeepEqual(t, pre.Validators, got.Validators)
		require.DeepEqual(t, pre.Balances, got.Balances)
		require.DeepEqual(t, pre.RandaoMixes, got.RandaoMixes)
		require.DeepEqual(t, pre.Slashings, got.Slashings)
		require.DeepEqual(t, pre.PreviousEpochParticipation, got.PreviousEpochParticipation)
		require.DeepEqual(t, pre.CurrentEpochParticipation, got.CurrentEpochParticipation)
		require.DeepEqual(t, pre.JustificationBits, got.JustificationBits)
		require.DeepEqual(t, pre.PreviousJustifiedCheckpoint, got.PreviousJustifiedCheckpoint)
		require.DeepEqual(t, pre.CurrentJustifiedCheckpoint, got.CurrentJustifiedCheckpoint)
		require.DeepEqual(t, pre.FinalizedCheckpoint, got.FinalizedCheckpoint)
		require.DeepEqual(t, pre.InactivityScores, got.InactivityScores)
		require.DeepEqual(t, pre.CurrentSyncCommittee, got.CurrentSyncCommittee)
		require.DeepEqual(t, pre.NextSyncCommittee, got.NextSyncCommittee)
		require.DeepEqual(t, pre.LatestBlockHash, got.LatestBlockHash)
		require.Equal(t, pre.NextWithdrawalIndex, got.NextWithdrawalIndex)
		require.Equal(t, pre.NextWithdrawalValidatorIndex, got.NextWithdrawalValidatorIndex)
		require.DeepEqual(t, pre.HistoricalSummaries, got.HistoricalSummaries)
		require.Equal(t, pre.DepositRequestsStartIndex, got.DepositRequestsStartIndex)
		require.Equal(t, pre.DepositBalanceToConsume, got.DepositBalanceToConsume)
		require.Equal(t, pre.ExitBalanceToConsume, got.ExitBalanceToConsume)
		require.Equal(t, pre.EarliestExitEpoch, got.EarliestExitEpoch)
		require.Equal(t, pre.ConsolidationBalanceToConsume, got.ConsolidationBalanceToConsume)
		require.Equal(t, pre.EarliestConsolidationEpoch, got.EarliestConsolidationEpoch)
		require.DeepEqual(t, pre.PendingDeposits, got.PendingDeposits)
		require.DeepEqual(t, pre.PendingPartialWithdrawals, got.PendingPartialWithdrawals)
		require.DeepEqual(t, pre.PendingConsolidations, got.PendingConsolidations)
		require.DeepEqual(t, pre.ProposerLookahead, got.ProposerLookahead)
		require.DeepEqual(t, pre.Builders, got.Builders)
		require.Equal(t, pre.NextWithdrawalBuilderIndex, got.NextWithdrawalBuilderIndex)
		require.DeepEqual(t, pre.ExecutionPayloadAvailability, got.ExecutionPayloadAvailability)
		require.DeepEqual(t, pre.BuilderPendingPayments, got.BuilderPendingPayments)
		require.DeepEqual(t, pre.BuilderPendingWithdrawals, got.BuilderPendingWithdrawals)
		require.DeepEqual(t, pre.PayloadExpectedWithdrawals, got.PayloadExpectedWithdrawals)
		require.DeepEqual(t, pre.PtcWindow, got.PtcWindow)
	})

	t.Run("sets the Heze fork version", func(t *testing.T) {
		st, _ := util.DeterministicGenesisStateGloas(t, params.BeaconConfig().MaxValidatorsPerCommittee)
		previous := st.Fork().CurrentVersion
		epoch := time.CurrentEpoch(st)

		post, err := heze.UpgradeToHeze(st)
		require.NoError(t, err)
		require.DeepSSZEqual(t, &ethpb.Fork{
			PreviousVersion: previous,
			CurrentVersion:  params.BeaconConfig().HezeForkVersion,
			Epoch:           epoch,
		}, post.Fork())
	})

	t.Run("copies the bid and adds empty inclusion list bits", func(t *testing.T) {
		st, _ := util.DeterministicGenesisStateGloas(t, params.BeaconConfig().MaxValidatorsPerCommittee)
		preBid, err := st.LatestExecutionPayloadBid()
		require.NoError(t, err)

		post, err := heze.UpgradeToHeze(st)
		require.NoError(t, err)

		got, ok := post.ToProtoUnsafe().(*ethpb.BeaconStateHeze)
		require.Equal(t, true, ok)

		parentBlockHash := preBid.ParentBlockHash()
		blockHash := preBid.BlockHash()
		prevRandao := preBid.PrevRandao()
		requestsRoot := preBid.ExecutionRequestsRoot()
		require.DeepSSZEqual(t, parentBlockHash[:], got.LatestExecutionPayloadBid.ParentBlockHash)
		require.DeepSSZEqual(t, blockHash[:], got.LatestExecutionPayloadBid.BlockHash)
		require.DeepSSZEqual(t, prevRandao[:], got.LatestExecutionPayloadBid.PrevRandao)
		require.DeepSSZEqual(t, requestsRoot[:], got.LatestExecutionPayloadBid.ExecutionRequestsRoot)
		require.Equal(t, preBid.GasLimit(), got.LatestExecutionPayloadBid.GasLimit)
		require.Equal(t, preBid.BuilderIndex(), got.LatestExecutionPayloadBid.BuilderIndex)
		require.Equal(t, preBid.Slot(), got.LatestExecutionPayloadBid.Slot)
		require.DeepSSZEqual(t,
			make([]byte, (fieldparams.InclusionListCommitteeSize+7)/8),
			got.LatestExecutionPayloadBid.InclusionListBits)
	})

	t.Run("rejects a pre-Gloas state", func(t *testing.T) {
		st, _ := util.DeterministicGenesisStateFulu(t, params.BeaconConfig().MaxValidatorsPerCommittee)
		_, err := heze.UpgradeToHeze(st)
		require.ErrorContains(t, "input is not a gloas beacon state", err)
	})

	t.Run("hashes the upgraded state", func(t *testing.T) {
		st, _ := util.DeterministicGenesisStateGloas(t, params.BeaconConfig().MaxValidatorsPerCommittee)
		post, err := heze.UpgradeToHeze(st)
		require.NoError(t, err)
		_, err = post.HashTreeRoot(t.Context())
		require.NoError(t, err)
	})
}
