package kv

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestAttCaches_hasSeenBit(t *testing.T) {
	c := NewAttCaches()

	seenA1 := util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000011}})
	seenA2 := util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b11100000}})
	require.NoError(t, c.insertSeenAtt(t, seenA1))
	require.NoError(t, c.insertSeenAtt(t, seenA2))
	tests := []struct {
		att  *ethpb.Attestation
		want bool
	}{
		{att: util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000000}}), want: true},
		{att: util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000001}}), want: true},
		{att: util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b11100000}}), want: true},
		{att: util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000011}}), want: true},
		{att: util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10001000}}), want: false},
		{att: util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b11110111}}), want: false},
	}
	for _, tt := range tests {
		got, err := c.hasSeenAtt(tt.att)
		require.NoError(t, err)
		if got != tt.want {
			t.Errorf("hasSeenBit() got = %v, want %v", got, tt.want)
		}
	}
}

func TestAttCaches_insertSeenBitDuplicates(t *testing.T) {
	c := NewAttCaches()
	att1 := util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000011}})
	id, err := attestation.NewId(att1, attestation.Data)
	require.NoError(t, err)
	require.NoError(t, c.insertSeenAtt(t, att1))
	require.Equal(t, 1, c.seenAtt.ItemCount())

	_, expirationTime1, ok := c.seenAtt.GetWithExpiration(id.String())
	require.Equal(t, true, ok)

	// Make sure that duplicates are not inserted, but expiration time gets updated.
	require.NoError(t, c.insertSeenAtt(t, att1))
	require.Equal(t, 1, c.seenAtt.ItemCount())
	_, expirationprysmTime, ok := c.seenAtt.GetWithExpiration(id.String())
	require.Equal(t, true, ok)
	require.Equal(t, true, expirationprysmTime.After(expirationTime1), "Expiration time is not updated")
}

// seenOf is the seen bits value insertSeenBit stores for one bitlist.
func seenOf(bits bitfield.Bitlist) seenBits {
	s := seenBits{union: bits}
	if bits.Count() > 1 {
		s.multi = []bitfield.Bitlist{bits}
	}
	return s
}

// TestSeenBits_MatchesListModel checks inserts and queries against the old semantics: a query is seen when one
// stored bitlist contains it, and an insert stores a bitlist unless one stored bitlist already contains it.
func TestSeenBits_MatchesListModel(t *testing.T) {
	const length = 24
	for seed := range uint64(20) {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewPCG(seed, 3))
			c := NewAttCaches()
			var model []bitfield.Bitlist
			modelHas := func(b bitfield.Bitlist) bool {
				for _, m := range model {
					if ok, err := m.Contains(b); err == nil && ok {
						return true
					}
				}
				return false
			}
			randomBits := func() bitfield.Bitlist {
				b := bitfield.NewBitlist(length)
				// Mostly single bits, as unaggregated attestations are, with some aggregates.
				n := 1
				if rng.IntN(4) == 0 {
					n = 2 + rng.IntN(6)
				}
				for range n {
					b.SetBitAt(uint64(rng.IntN(length)), true)
				}
				return b
			}
			for step := range 400 {
				att := util.HydrateAttestation(&ethpb.Attestation{AggregationBits: randomBits()})
				if rng.IntN(2) == 0 {
					require.NoError(t, c.insertSeenAtt(t, att))
					if !modelHas(att.AggregationBits) {
						model = append(model, att.AggregationBits)
					}
					continue
				}
				got, err := c.hasSeenAtt(att)
				require.NoError(t, err)
				require.Equal(t, modelHas(att.AggregationBits), got, "step %d bits %08b", step, []byte(att.AggregationBits))
			}
		})
	}
}

// BenchmarkSeenBits saves the single-bit attestations of a 512-member committee and checks each one again.
func BenchmarkSeenBits(b *testing.B) {
	const size = 512
	atts := make([]*ethpb.Attestation, size)
	for i := range atts {
		bits := bitfield.NewBitlist(size)
		bits.SetBitAt(uint64(i), true)
		atts[i] = util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bits})
	}
	for b.Loop() {
		c := NewAttCaches()
		for _, att := range atts {
			if _, err := c.hasSeenAtt(att); err != nil {
				b.Fatal(err)
			}
			if err := c.insertSeenAtt(b, att); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func TestInsertSeenBit_EmptyBits(t *testing.T) {
	for _, bits := range []bitfield.Bitlist{nil, {}} {
		t.Run(fmt.Sprintf("bits=%v", []byte(bits)), func(t *testing.T) {
			c := NewAttCaches()
			att := util.HydrateAttestation(&ethpb.Attestation{})
			att.AggregationBits = bits
			require.NoError(t, c.insertSeenAtt(t, att))
			require.NoError(t, c.insertSeenAtt(t, att))
		})
	}
	t.Run("single attestation", func(t *testing.T) {
		c := NewAttCaches()
		att := util.HydrateSingleAttestation(&ethpb.SingleAttestation{})
		require.NoError(t, c.SaveUnaggregatedAttestation(att))
		require.NoError(t, c.DeleteUnaggregatedAttestation(att))
	})
}

func TestInsertSeenBit_Concurrent(t *testing.T) {
	const size = 64
	c := NewAttCaches()
	var wg sync.WaitGroup
	for i := range size {
		wg.Go(func() {
			bits := bitfield.NewBitlist(size)
			bits.SetBitAt(uint64(i), true)
			assert.NoError(t, c.insertSeenAtt(t, util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bits})))
		})
	}
	wg.Wait()
	for i := range size {
		bits := bitfield.NewBitlist(size)
		bits.SetBitAt(uint64(i), true)
		seen, err := c.hasSeenAtt(util.HydrateAttestation(&ethpb.Attestation{AggregationBits: bits}))
		require.NoError(t, err)
		require.Equal(t, true, seen, "bit %d lost", i)
	}
}

// insertSeenAtt is insertSeenBit for tests that do not have the data ID.
func (c *AttCaches) insertSeenAtt(t testing.TB, att ethpb.Att) error {
	return c.insertSeenBit(dataID(t, att), att)
}

func dataID(t testing.TB, att ethpb.Att) attestation.Id {
	id, err := attestation.NewId(att, attestation.Data)
	require.NoError(t, err)
	return id
}
