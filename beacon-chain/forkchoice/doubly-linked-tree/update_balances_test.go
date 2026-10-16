package doublylinkedtree

import (
	"fmt"
	"testing"

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
