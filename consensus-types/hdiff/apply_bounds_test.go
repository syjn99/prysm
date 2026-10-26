package hdiff

import (
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// TestApplyDiffRejectsOutOfRangeIndices proves that a diff whose prefix-drop index or builder index exceeds
// the source length is rejected with an error. Before the bound checks, MaxUint64 cast to int became -1 and
// panicked with a slice-bounds error (even on the pending deposits path, whose int comparison let -1 through),
// and a large builder index appended one nil builder per missing index.
func testBuilder() *ethpb.Builder {
	return &ethpb.Builder{Pubkey: make([]byte, 48), Version: []byte{0}, ExecutionAddress: make([]byte, 20)}
}

func TestApplyDiffRejectsOutOfRangeIndices(t *testing.T) {
	electra := func(t *testing.T) state.BeaconState {
		st, _ := util.DeterministicGenesisStateElectra(t, 8)
		return st
	}
	gloasSt := func(t *testing.T) state.BeaconState {
		st, err := gloasState(t, 64)
		require.NoError(t, err)
		return st
	}
	for _, tc := range []struct {
		name    string
		source  func(*testing.T) state.BeaconState
		mutate  func(*stateDiff)
		wantErr string
	}{
		{"pending_deposits_max", electra, func(s *stateDiff) { s.pendingDepositIndex = math.MaxUint64 }, "pending deposit index"},
		{"pending_deposits_plus_one", electra, func(s *stateDiff) { s.pendingDepositIndex = 1 }, "pending deposit index"},
		{"pending_partial_withdrawals_max", electra, func(s *stateDiff) { s.pendingPartialWithdrawalsIndex = math.MaxUint64 }, "pending partial withdrawals index"},
		{"pending_partial_withdrawals_plus_one", electra, func(s *stateDiff) { s.pendingPartialWithdrawalsIndex = 1 }, "pending partial withdrawals index"},
		{"pending_consolidations_max", electra, func(s *stateDiff) { s.pendingConsolidationsIndex = math.MaxUint64 }, "pending consolidations index"},
		{"pending_consolidations_plus_one", electra, func(s *stateDiff) { s.pendingConsolidationsIndex = 1 }, "pending consolidations index"},
		{"builder_pending_withdrawals_max", gloasSt, func(s *stateDiff) { s.builderPendingWithdrawalsIndex = math.MaxUint64 }, "builder pending withdrawals index"},
		{"builder_pending_withdrawals_plus_one", gloasSt, func(s *stateDiff) { s.builderPendingWithdrawalsIndex = 1 }, "builder pending withdrawals index"},
		{"builder_index_max", gloasSt, func(s *stateDiff) {
			s.builderDiffs = []builderDiff{{index: math.MaxUint32, builder: testBuilder()}}
		}, "builder index"},
		{"builder_index_plus_one", gloasSt, func(s *stateDiff) {
			s.builderDiffs = []builderDiff{{index: 1, builder: testBuilder()}}
		}, "builder index"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.source(t)
			target := source.Copy()
			require.NoError(t, target.SetSlot(source.Slot()+1))
			h, err := diffInternal(source, target)
			require.NoError(t, err)
			tc.mutate(h.stateDiff)
			diff, err := h.serialize()
			require.NoError(t, err)
			_, err = ApplyDiff(t.Context(), source, diff)
			require.ErrorContains(t, tc.wantErr, err)
			require.ErrorContains(t, "exceeds source length", err)
		})
	}
}
