package doublylinkedtree

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// BenchmarkUpdateBalances measures one Head-time balance update after 1/32 of the validators re-attest.
func BenchmarkUpdateBalances(b *testing.B) {
	for _, n := range []uint64{1_200_000, 2_200_000} {
		b.Run(fmt.Sprintf("validators=%d", n), func(b *testing.B) {
			ctx := b.Context()
			f := setup(1, 1)
			rootA, rootB := [32]byte{'a'}, [32]byte{'b'}
			st, blk, err := prepareForkchoiceState(ctx, 1, rootA, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 1, 1)
			require.NoError(b, err)
			require.NoError(b, f.InsertNode(ctx, st, blk))
			st, blk, err = prepareForkchoiceState(ctx, 2, rootB, rootA, [32]byte{'B'}, 1, 1)
			require.NoError(b, err)
			require.NoError(b, f.InsertNode(ctx, st, blk))

			f.justifiedBalances = make([]uint64, n)
			indices := make([]uint64, n)
			for i := range indices {
				f.justifiedBalances[i] = params.BeaconConfig().MaxEffectiveBalance
				indices[i] = uint64(i)
			}
			f.ProcessAttestation(ctx, indices, rootA, 1, false)
			require.NoError(b, f.updateBalances())

			chunk := n / uint64(params.BeaconConfig().SlotsPerEpoch)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				start := (uint64(i) * chunk) % (n - chunk)
				root := rootA
				if i%2 == 0 {
					root = rootB
				}
				f.ProcessAttestation(ctx, indices[start:start+chunk], root, primitives.Slot(uint64(i+1)*32), false)
				b.StartTimer()
				if err := f.updateBalances(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestIndexSet(t *testing.T) {
	var s indexSet
	t.Run("empty", func(t *testing.T) {
		require.Equal(t, false, s.has(0))
		require.Equal(t, false, s.has(1<<40))
	})
	t.Run("add grows", func(t *testing.T) {
		for _, i := range []uint64{0, 63, 64, 1000, 2_200_000} {
			s.add(i)
		}
		for _, i := range []uint64{0, 63, 64, 1000, 2_200_000} {
			require.Equal(t, true, s.has(i), i)
		}
		for _, i := range []uint64{1, 62, 65, 999, 2_199_999, 2_200_001} {
			require.Equal(t, false, s.has(i), i)
		}
	})
}

// TestUpdateBalances_DirtyMatchesFullScan replays random votes, balances, slashings, blocks and
// payloads on two stores and checks the dirty-vote path against a forced full scan.
func TestUpdateBalances_DirtyMatchesFullScan(t *testing.T) {
	const numValidators, numBlocks, steps = 96, 12, 400
	for seed := range uint64(20) {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			ctx := t.Context()
			rng := rand.New(rand.NewPCG(seed, 0))
			dirty, full := setupGloas(t, 1, 1), setupGloas(t, 1, 1)
			driftGenesisTime(dirty, 4*steps, 0)
			driftGenesisTime(full, 4*steps, 0)

			roots := make([][32]byte, numBlocks)
			parents := make([][32]byte, numBlocks)
			for i := range roots {
				roots[i] = indexToHash(uint64(i + 1))
				parents[i] = params.BeaconConfig().ZeroHash
				if i > 0 {
					parents[i] = roots[rng.IntN(i)]
				}
			}
			inserted, withPayload := 0, 0
			insertBlock := func() {
				for _, f := range []*ForkChoice{dirty, full} {
					st, blk, err := prepareGloasForkchoiceState(ctx, primitives.Slot(inserted+1), roots[inserted], parents[inserted], indexToHash(uint64(100+inserted)), params.BeaconConfig().ZeroHash, 1, 1)
					require.NoError(t, err)
					require.NoError(t, f.InsertNode(ctx, st, blk))
				}
				inserted++
			}
			insertPayload := func() {
				for _, f := range []*ForkChoice{dirty, full} {
					pe, err := prepareGloasForkchoicePayload(roots[withPayload])
					require.NoError(t, err)
					require.NoError(t, f.InsertPayload(pe))
				}
				withPayload++
			}
			newBalances := func() {
				b := make([]uint64, numValidators-rng.IntN(8))
				for i := range b {
					b[i] = rng.Uint64N(4) * 8
				}
				dirty.justifiedBalances, full.justifiedBalances = b, slices.Clone(b)
			}
			for range numBlocks / 3 {
				insertBlock()
			}
			newBalances()
			// A VoteChanges consumer must always mirror the latest votes.
			var mirror []forkchoicetypes.VoteData
			var changes []forkchoicetypes.VoteChange
			synced := false

			for step := range steps {
				switch op := rng.IntN(100); {
				case op < 55:
					indices := make([]uint64, 1+rng.IntN(16))
					for i := range indices {
						indices[i] = rng.Uint64N(numValidators + 4)
					}
					// Unknown roots exercise votes whose block arrives later.
					root := roots[rng.IntN(numBlocks)]
					slot := primitives.Slot(1 + step*rng.IntN(5))
					status := rng.IntN(2) == 0
					dirty.ProcessAttestation(ctx, indices, root, slot, status)
					full.ProcessAttestation(ctx, indices, root, slot, status)
				case op < 63:
					newBalances()
				case op < 66:
					idx := primitives.ValidatorIndex(rng.Uint64N(numValidators))
					dirty.InsertSlashedIndex(ctx, idx)
					full.InsertSlashedIndex(ctx, idx)
				case op < 72 && inserted < numBlocks:
					insertBlock()
				case op < 76 && withPayload < inserted:
					insertPayload()
				default:
					// Reference: the pre-dirty-tracking full scan, then Head finds nothing dirty.
					for i := range full.votes {
						full.updateVoteBalance(i, full.justifiedBalances)
					}
					full.balances = full.justifiedBalances
					clear(full.dirtyVotes)
					dh, err := dirty.Head(ctx)
					require.NoError(t, err)
					fh, err := full.Head(ctx)
					require.NoError(t, err)
					require.Equal(t, fh, dh, "step %d", step)
					requireSameWeights(t, full, dirty, step)

					var n int
					changes, n = dirty.VoteChanges(changes[:0], !synced)
					synced = true
					mirror = append(mirror, make([]forkchoicetypes.VoteData, n-len(mirror))...)
					for _, c := range changes {
						mirror[c.Index] = c.Vote
					}
					for i, v := range dirty.votes {
						require.Equal(t, forkchoicetypes.VoteData{Root: v.nextRoot, Slot: v.nextSlot, PayloadStatus: v.nextPayloadStatus}, mirror[i], "step %d vote %d", step, i)
					}
				}
			}
		})
	}
}

func requireSameWeights(t *testing.T, want, got *ForkChoice, step int) {
	t.Helper()
	require.Equal(t, true, slices.Equal(want.votes, got.votes), "step %d votes", step)
	require.Equal(t, len(want.store.emptyNodeByRoot), len(got.store.emptyNodeByRoot))
	for root, w := range want.store.emptyNodeByRoot {
		g := got.store.emptyNodeByRoot[root]
		require.NotNil(t, g)
		require.Equal(t, w.balance, g.balance, "step %d empty balance %#x", step, root)
		require.Equal(t, w.weight, g.weight, "step %d empty weight %#x", step, root)
		require.Equal(t, w.node.balance, g.node.balance, "step %d node balance %#x", step, root)
		require.Equal(t, w.node.weight, g.node.weight, "step %d node weight %#x", step, root)
	}
	require.Equal(t, len(want.store.fullNodeByRoot), len(got.store.fullNodeByRoot))
	for root, w := range want.store.fullNodeByRoot {
		g := got.store.fullNodeByRoot[root]
		require.NotNil(t, g)
		require.Equal(t, w.balance, g.balance, "step %d full balance %#x", step, root)
		require.Equal(t, w.weight, g.weight, "step %d full weight %#x", step, root)
	}
}

// BenchmarkVoteChanges compares copying every latest vote with copying the 1/32 that changed.
func BenchmarkVoteChanges(b *testing.B) {
	for _, n := range []uint64{1_200_000, 2_200_000} {
		f := setup(1, 1)
		indices := make([]uint64, n)
		for i := range indices {
			indices[i] = uint64(i)
		}
		f.ProcessAttestation(b.Context(), indices, [32]byte{'a'}, 1, false)
		chunk := n / uint64(params.BeaconConfig().SlotsPerEpoch)
		var buf []forkchoicetypes.VoteChange
		b.Run(fmt.Sprintf("all/validators=%d", n), func(b *testing.B) {
			for b.Loop() {
				buf, _ = f.VoteChanges(buf[:0], true)
			}
		})
		b.Run(fmt.Sprintf("changed/validators=%d", n), func(b *testing.B) {
			i := 0
			for b.Loop() {
				b.StopTimer()
				start := (uint64(i) * chunk) % (n - chunk)
				f.ProcessAttestation(b.Context(), indices[start:start+chunk], [32]byte{'b'}, primitives.Slot(uint64(i+1)*32), false)
				i++
				b.StartTimer()
				buf, _ = f.VoteChanges(buf[:0], false)
			}
		})
	}
}
