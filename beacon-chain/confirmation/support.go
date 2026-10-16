package confirmation

import (
	"context"
	"fmt"
	"maps"
	"slices"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// noAssignment marks a validator with no attestation duty in an epoch.
const noAssignment = uint8(0xFF)

// epochSlotTable maps every validator to its assigned attestation slot within one
// epoch. This makes membership queries O(1).
type epochSlotTable struct {
	start      primitives.Slot
	seed       [32]byte // attester shuffling seed
	slotOffset []uint8  // val index -> slot offset
}

// newEpochSlotTable builds the table for one epoch
func newEpochSlotTable(ctx context.Context, committees CommitteeAccessor, epoch primitives.Epoch, seed [32]byte, sizeHint int) (*epochSlotTable, error) {
	start, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, fmt.Errorf("failed to get epoch start: %w", err)
	}

	offs := make([]uint8, sizeHint)
	for i := range offs {
		offs[i] = noAssignment
	}

	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	for slot := start; slot < start+spe; slot++ {
		members, err := committees.Committee(ctx, slot)
		if err != nil {
			return nil, fmt.Errorf("failed to get committee for slot %d: %w", slot, err)
		}

		for _, v := range members {
			if uint64(v) >= uint64(len(offs)) {
				grown := make([]uint8, v+1)
				copy(grown, offs)
				for i := len(offs); i < len(grown); i++ {
					grown[i] = noAssignment
				}
				offs = grown
			}

			offs[v] = uint8(slot - start)
		}
	}
	return &epochSlotTable{start: start, seed: seed, slotOffset: offs}, nil
}

// assignedSlot computes the attestation slot assigned to a validator in this epoch, if any.
// Caller should check the boolean return value before using the slot.
func (t *epochSlotTable) assignedSlot(v primitives.ValidatorIndex) (primitives.Slot, bool) {
	if uint64(v) >= uint64(len(t.slotOffset)) {
		return 0, false
	}

	if t.slotOffset[v] == noAssignment {
		return 0, false
	}

	return t.start + primitives.Slot(t.slotOffset[v]), true
}

// SupportMap pre-aggregates vote support for the slot-range queries FCR needs.
// totalSupport backs get_attestation_score (a vote credits every ancestor);
type SupportMap struct {
	votes           []forkchoicetypes.VoteData
	voters          map[[32]byte][]uint32 // validator indices by voted root
	voterPos        []uint32              // position of each validator in voters[votes[i].Root]
	balances        []uint64
	slashedBalances map[primitives.ValidatorIndex]uint64
	equivocating    map[primitives.ValidatorIndex]bool
	tables          []*epochSlotTable
	rootWeights     map[[32]byte]uint64
	totalSupport    map[[32]byte]uint64
	weighed         bool // rootWeights match votes, balances and equivocating
}

func NewSupportMap() *SupportMap {
	return &SupportMap{
		voters:       make(map[[32]byte][]uint32),
		rootWeights:  make(map[[32]byte]uint64),
		totalSupport: make(map[[32]byte]uint64),
	}
}

// Build snapshots the query inputs and aggregates per-root vote weights, it reads no forkchoice state so it can run off the lock.
func (s *SupportMap) Build(
	votes []forkchoicetypes.VoteData,
	balances []uint64,
	slashedBalances map[primitives.ValidatorIndex]uint64,
	tables []*epochSlotTable,
	equivocating map[primitives.ValidatorIndex]bool,
) {
	s.votes = votes
	clear(s.voters)
	s.voterPos = slices.Grow(s.voterPos[:0], len(votes))[:len(votes)]
	for i, vote := range votes {
		s.addVoter(uint32(i), vote.Root)
	}
	s.reweigh(balances, slashedBalances, tables, equivocating)
}

// BuildShared aggregates weights for other balances over the votes of base, which must not change while s is in use.
func (s *SupportMap) BuildShared(
	base *SupportMap,
	balances []uint64,
	slashedBalances map[primitives.ValidatorIndex]uint64,
	tables []*epochSlotTable,
	equivocating map[primitives.ValidatorIndex]bool,
) {
	s.votes, s.voters, s.voterPos = base.votes, base.voters, base.voterPos
	s.reweigh(balances, slashedBalances, tables, equivocating)
}

// Update applies the latest vote changes and moves only their weight. A different balances slice or
// equivocating set reweighs every vote. Callers must not mutate either in place once passed in.
func (s *SupportMap) Update(
	changes []forkchoicetypes.VoteChange,
	numVotes int,
	balances []uint64,
	slashedBalances map[primitives.ValidatorIndex]uint64,
	tables []*epochSlotTable,
	equivocating map[primitives.ValidatorIndex]bool,
) {
	incremental := s.weighed && sameBacking(s.balances, balances) && maps.Equal(s.equivocating, equivocating)
	s.growVotes(uint64(numVotes))
	for _, c := range changes {
		s.growVotes(c.Index + 1)
		old := s.votes[c.Index]
		if incremental {
			if w := s.voteWeight(c.Index, old.Root); w > 0 {
				if s.rootWeights[old.Root] -= w; s.rootWeights[old.Root] == 0 {
					delete(s.rootWeights, old.Root)
				}
			}
			if w := s.voteWeight(c.Index, c.Vote.Root); w > 0 {
				s.rootWeights[c.Vote.Root] += w
			}
		}
		s.removeVoter(uint32(c.Index), old.Root)
		s.addVoter(uint32(c.Index), c.Vote.Root)
		s.votes[c.Index] = c.Vote
	}
	if incremental {
		s.slashedBalances, s.tables = slashedBalances, tables
		return
	}
	s.reweigh(balances, slashedBalances, tables, equivocating)
}

func (s *SupportMap) reweigh(
	balances []uint64,
	slashedBalances map[primitives.ValidatorIndex]uint64,
	tables []*epochSlotTable,
	equivocating map[primitives.ValidatorIndex]bool,
) {
	s.balances = balances
	s.slashedBalances = slashedBalances
	s.tables = tables
	s.equivocating = equivocating
	clear(s.rootWeights)
	for i, vote := range s.votes {
		if w := s.voteWeight(uint64(i), vote.Root); w > 0 {
			s.rootWeights[vote.Root] += w
		}
	}
	s.weighed = true
}

// voteWeight is the balance validator i adds to root, zero when the vote does not count.
func (s *SupportMap) voteWeight(i uint64, root [32]byte) uint64 {
	if root == ([32]byte{}) || i >= uint64(len(s.balances)) {
		return 0
	}
	if len(s.equivocating) > 0 && s.equivocating[primitives.ValidatorIndex(i)] {
		return 0
	}
	return s.balances[i]
}

func (s *SupportMap) growVotes(n uint64) {
	if n > uint64(len(s.votes)) {
		s.votes = append(s.votes, make([]forkchoicetypes.VoteData, n-uint64(len(s.votes)))...)
		s.voterPos = append(s.voterPos, make([]uint32, n-uint64(len(s.voterPos)))...)
	}
}

func (s *SupportMap) addVoter(i uint32, root [32]byte) {
	if root == ([32]byte{}) {
		return
	}
	s.voterPos[i] = uint32(len(s.voters[root]))
	s.voters[root] = append(s.voters[root], i)
}

func (s *SupportMap) removeVoter(i uint32, root [32]byte) {
	if root == ([32]byte{}) {
		return
	}
	list := s.voters[root]
	last := list[len(list)-1]
	list[s.voterPos[i]] = last
	s.voterPos[last] = s.voterPos[i]
	if len(list) == 1 {
		delete(s.voters, root)
		return
	}
	s.voters[root] = list[:len(list)-1]
}

// sameBacking reports whether a and b are the same slice.
func sameBacking(a, b []uint64) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

func (s *SupportMap) Accumulate(fc ForkchoiceReader) {
	clear(s.totalSupport)
	for root, bal := range s.rootWeights {
		r := root
		for {
			s.totalSupport[r] += bal
			parent, err := fc.ParentRoot(r)
			if err != nil || parent == ([32]byte{}) {
				break
			}
			r = parent
		}
	}
}

// NodeSupportBetweenSlots implements get_node_support_between_slots.
// The spec sums over a participant set union, so a validator sitting in committees
// of several slots in the range counts once.
//
//	<spec fn="get_node_support_between_slots" fork="phase0">
//
//	participants = set()
//	for slot in range(start_slot, end_slot + 1):
//	    participants.update(get_slot_committee(store, slot))
//	return sum(balance[i] for i in participants
//	    if get_supported_node(store, latest_messages[i]) == node
//	    and i not in equivocating_indices)
//	</spec>
func (s *SupportMap) NodeSupportBetweenSlots(root [32]byte, rootSlot primitives.Slot, full bool, startSlot, endSlot primitives.Slot) uint64 {
	if root == ([32]byte{}) {
		return 0
	}
	gloas := slots.ToEpoch(rootSlot) >= params.BeaconConfig().GloasForkEpoch
	hasEquivocating := len(s.equivocating) > 0
	total := uint64(0)
	for _, i := range s.voters[root] {
		vote := s.votes[i]
		// Gloas votes support the full or empty variant only when cast after the block's slot.
		if gloas && (vote.Slot <= rootSlot || vote.PayloadStatus != full) {
			continue
		}
		if uint64(i) >= uint64(len(s.balances)) || s.balances[i] == 0 {
			continue
		}
		if hasEquivocating && s.equivocating[primitives.ValidatorIndex(i)] {
			continue
		}
		for _, t := range s.tables {
			if sl, ok := t.assignedSlot(primitives.ValidatorIndex(i)); ok && sl >= startSlot && sl <= endSlot {
				total += s.balances[i]
				break
			}
		}
	}
	return total
}

// AttestationScore implements get_attestation_score, votes for descendants count as support.
//
//	<spec fn="get_attestation_score" fork="phase0">
//
//	return sum(balance[i] for i in active_unslashed
//	    if i in latest_messages and i not in equivocating_indices
//	    and get_ancestor(store, latest_messages[i].root, store.blocks[root].slot) == root)
//	</spec>
func (s *SupportMap) AttestationScore(root [32]byte) uint64 {
	return s.totalSupport[root]
}

// EquivocationScore implements the spec's get_equivocation_score.
func (s *SupportMap) EquivocationScore(startSlot, endSlot primitives.Slot) uint64 {
	total := uint64(0)
	for idx, isEquivocating := range s.equivocating {
		if !isEquivocating || uint64(idx) >= uint64(len(s.balances)) {
			continue
		}
		bal := s.balances[idx] + s.slashedBalances[idx]
		if bal == 0 {
			continue
		}
		for _, t := range s.tables {
			if sl, ok := t.assignedSlot(idx); ok && sl >= startSlot && sl <= endSlot {
				total += bal
				break
			}
		}
	}
	return total
}
