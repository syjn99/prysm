package confirmation

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"testing"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestSupportMap_UpdateMatchesBuild replays random vote changes, balance sources and equivocations and checks
// the incrementally updated map against a fresh Build over the full vote set.
func TestSupportMap_UpdateMatchesBuild(t *testing.T) {
	const numValidators, steps = 80, 300
	for _, gloas := range []bool{false, true} {
		for seed := range uint64(10) {
			t.Run(fmt.Sprintf("gloas=%t/seed=%d", gloas, seed), func(t *testing.T) {
				if gloas {
					params.SetupTestConfigCleanup(t)
					cfg := params.BeaconConfig().Copy()
					cfg.GloasForkEpoch = 0
					params.OverrideBeaconConfig(cfg)
				}
				rng := rand.New(rand.NewPCG(seed, 1))
				roots := [][32]byte{{}, {1}, {2}, {3}, {4}, {5}}
				tables := []*epochSlotTable{randomTable(rng, 0, numValidators), randomTable(rng, 32, numValidators)}
				randomBalances := func() []uint64 {
					b := make([]uint64, numValidators-rng.IntN(5))
					for i := range b {
						b[i] = rng.Uint64N(3) * 16
					}
					return b
				}

				var latest []forkchoicetypes.VoteData
				pending := make(map[uint64]bool)
				balances, prevBalances := randomBalances(), randomBalances()
				equivocating := map[primitives.ValidatorIndex]bool{}
				sm := NewSupportMap()
				synced := false
				for step := range steps {
					switch op := rng.IntN(100); {
					case op < 60:
						i := rng.Uint64N(numValidators + 3)
						for uint64(len(latest)) <= i {
							latest = append(latest, forkchoicetypes.VoteData{})
						}
						latest[i] = forkchoicetypes.VoteData{Root: roots[rng.IntN(len(roots))], Slot: primitives.Slot(rng.IntN(64)), PayloadStatus: rng.IntN(2) == 0}
						pending[i] = true
					case op < 66:
						balances = randomBalances()
					case op < 69:
						equivocating = maps.Clone(equivocating)
						equivocating[primitives.ValidatorIndex(rng.IntN(numValidators))] = true
					default:
						var changes []forkchoicetypes.VoteChange
						for i, v := range latest {
							if !synced || pending[uint64(i)] {
								changes = append(changes, forkchoicetypes.VoteChange{Index: uint64(i), Vote: v})
							}
						}
						clear(pending)
						synced = true
						sm.Update(changes, len(latest), balances, nil, tables, equivocating)
						want := NewSupportMap()
						want.Build(slices.Clone(latest), balances, nil, tables, equivocating)
						requireSameSupport(t, want, sm, roots, step)

						prev, wantPrev := NewSupportMap(), NewSupportMap()
						prev.BuildShared(sm, prevBalances, nil, tables, equivocating)
						wantPrev.Build(slices.Clone(latest), prevBalances, nil, tables, equivocating)
						requireSameSupport(t, wantPrev, prev, roots, step)
					}
				}
			})
		}
	}
}

func randomTable(rng *rand.Rand, start primitives.Slot, n int) *epochSlotTable {
	offs := make([]uint8, n)
	for i := range offs {
		offs[i] = uint8(rng.IntN(int(params.BeaconConfig().SlotsPerEpoch)))
		if rng.IntN(10) == 0 {
			offs[i] = noAssignment
		}
	}
	return &epochSlotTable{start: start, slotOffset: offs}
}

func requireSameSupport(t *testing.T, want, got *SupportMap, roots [][32]byte, step int) {
	t.Helper()
	require.Equal(t, true, slices.Equal(want.votes, got.votes), "step %d votes", step)
	require.Equal(t, true, maps.Equal(want.rootWeights, got.rootWeights), "step %d root weights: want %v got %v", step, want.rootWeights, got.rootWeights)
	for root, voters := range got.voters {
		for pos, v := range voters {
			require.Equal(t, root, got.votes[v].Root, "step %d", step)
			require.Equal(t, uint32(pos), got.voterPos[v], "step %d", step)
		}
	}
	for _, root := range roots {
		for _, full := range []bool{false, true} {
			for _, rootSlot := range []primitives.Slot{0, 20, 40} {
				for _, r := range [][2]primitives.Slot{{0, 63}, {5, 9}, {33, 40}, {10, 10}} {
					require.Equal(t,
						want.NodeSupportBetweenSlots(root, rootSlot, full, r[0], r[1]),
						got.NodeSupportBetweenSlots(root, rootSlot, full, r[0], r[1]),
						"step %d root %x full %t slot %d range %v", step, root[0], full, rootSlot, r)
				}
			}
		}
	}
}

// BenchmarkSupportMap compares a full Build with an Update where 1/32 of the validators re-attest.
func BenchmarkSupportMap(b *testing.B) {
	for _, n := range []int{1_200_000, 2_200_000} {
		rng := rand.New(rand.NewPCG(1, 2))
		votes := make([]forkchoicetypes.VoteData, n)
		balances := make([]uint64, n)
		for i := range votes {
			votes[i] = forkchoicetypes.VoteData{Root: [32]byte{byte(rng.IntN(64)), 1}, Slot: primitives.Slot(i % 64)}
			balances[i] = params.BeaconConfig().MaxEffectiveBalance
		}
		chunk := n / int(params.BeaconConfig().SlotsPerEpoch)
		changes := make([]forkchoicetypes.VoteChange, chunk)
		b.Run(fmt.Sprintf("Build/validators=%d", n), func(b *testing.B) {
			sm := NewSupportMap()
			for b.Loop() {
				sm.Build(votes, balances, nil, nil, nil)
			}
		})
		b.Run(fmt.Sprintf("Update/validators=%d", n), func(b *testing.B) {
			sm := NewSupportMap()
			sm.Build(slices.Clone(votes), balances, nil, nil, nil)
			i := 0
			for b.Loop() {
				start := (i * chunk) % (n - chunk)
				for j := range changes {
					changes[j] = forkchoicetypes.VoteChange{Index: uint64(start + j), Vote: forkchoicetypes.VoteData{Root: [32]byte{byte(i % 64), 1}, Slot: primitives.Slot(64 + i)}}
				}
				sm.Update(changes, n, balances, nil, nil, nil)
				i++
			}
		})
	}
}
