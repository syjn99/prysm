// Package pqkeys holds EIP-8450 post-quantum key registrations until a proposer includes them.
package pqkeys

import (
	"cmp"
	"slices"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// PoolManager keeps pending post-quantum key registrations for block inclusion.
type PoolManager interface {
	PendingRegistrations() []*ethpb.SignedPQKeyRegistration
	RegistrationsForInclusion(st state.BeaconState) []*ethpb.SignedPQKeyRegistration
	InsertRegistration(r *ethpb.SignedPQKeyRegistration)
	MarkIncluded(r *ethpb.SignedPQKeyRegistration)
}

// key is the pair that names one registration: the registrant and the change it follows.
type key struct {
	index         primitives.ValidatorIndex
	previousEpoch primitives.Epoch
}

func keyOf(r *ethpb.SignedPQKeyRegistration) key {
	return key{index: r.Message.Index, previousEpoch: r.Message.PreviousChangeEpoch}
}

// Pool is a PoolManager. The first registration inserted for a pair wins.
// ponytail: unbounded map; gossip validation bounds it to one entry per (index, change epoch). Add a size cap if REST spam shows up.
type Pool struct {
	lock sync.RWMutex
	m    map[key]*ethpb.SignedPQKeyRegistration
}

// NewPool returns an empty pool.
func NewPool() *Pool {
	return &Pool{m: make(map[key]*ethpb.SignedPQKeyRegistration)}
}

// PendingRegistrations returns every pending registration, ordered by index and previous change epoch.
func (p *Pool) PendingRegistrations() []*ethpb.SignedPQKeyRegistration {
	p.lock.RLock()
	defer p.lock.RUnlock()

	result := make([]*ethpb.SignedPQKeyRegistration, 0, len(p.m))
	for _, r := range p.m {
		result = append(result, r)
	}
	slices.SortFunc(result, func(a, b *ethpb.SignedPQKeyRegistration) int {
		return cmp.Or(cmp.Compare(a.Message.Index, b.Message.Index), cmp.Compare(a.Message.PreviousChangeEpoch, b.Message.PreviousChangeEpoch))
	})
	return result
}

// RegistrationsForInclusion returns up to MAX_PQ_KEY_REGISTRATIONS registrations that apply, in order, on a copy of st.
// It drops registrations whose previous change epoch no longer matches the state, since they can never apply.
func (p *Pool) RegistrationsForInclusion(st state.BeaconState) []*ethpb.SignedPQKeyRegistration {
	pending := p.PendingRegistrations()
	if len(pending) == 0 {
		return nil
	}
	sim := st.Copy()

	limit := params.BeaconConfig().MaxPQKeyRegistrations
	result := make([]*ethpb.SignedPQKeyRegistration, 0, min(uint64(len(pending)), limit))
	for _, r := range pending {
		if uint64(len(result)) == limit {
			break
		}
		_, changeEpoch, err := sim.PQKeyEntry(r.Message.Index)
		if err != nil || changeEpoch != r.Message.PreviousChangeEpoch {
			// Outside the registry, or a change has already been applied: it can never apply.
			p.MarkIncluded(r)
			continue
		}
		// Apply on the copy so that two registrations of one key in one block are caught.
		if err := heze.ProcessPQKeyRegistration(sim, r); err != nil {
			continue
		}
		result = append(result, r)
	}
	return result
}

// InsertRegistration adds a registration unless one is pending for the same pair.
func (p *Pool) InsertRegistration(r *ethpb.SignedPQKeyRegistration) {
	if r == nil || r.Message == nil {
		return
	}
	p.lock.Lock()
	defer p.lock.Unlock()

	k := keyOf(r)
	if _, ok := p.m[k]; ok {
		return
	}
	p.m[k] = r
}

// MarkIncluded removes the registration for the pair of r.
func (p *Pool) MarkIncluded(r *ethpb.SignedPQKeyRegistration) {
	if r == nil || r.Message == nil {
		return
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	delete(p.m, keyOf(r))
}
