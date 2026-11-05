// Package pqkeys holds EIP-8450 post-quantum key registrations until a proposer includes them.
package pqkeys

import (
	"cmp"
	"errors"
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
	InsertRegistration(r *ethpb.SignedPQKeyRegistration) bool
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

// MaxPendingRegistrations caps the pool: 16,384 registrations of 2,580 bytes, about 42 MB,
// or 2,048 full blocks of backlog.
const MaxPendingRegistrations = 1 << 14

// Pool is a PoolManager. The latest registration inserted for a pair wins.
type Pool struct {
	lock  sync.RWMutex
	m     map[key]*ethpb.SignedPQKeyRegistration
	limit int
}

// NewPool returns an empty pool that holds at most MaxPendingRegistrations registrations.
func NewPool() *Pool {
	return &Pool{m: make(map[key]*ethpb.SignedPQKeyRegistration), limit: MaxPendingRegistrations}
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
// It removes registrations that fail for a permanent reason (exited registrant, index out of range, key in use,
// stale previous change epoch, bad signature or proof) and keeps those that are only too early.
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
		// Apply on the copy so that two registrations of one key in one block are caught.
		if err := heze.ProcessPQKeyRegistration(sim, r); err != nil {
			if !errors.Is(err, heze.ErrPQKeyRegistrationNotYetValid) {
				p.remove(r)
			}
			continue
		}
		result = append(result, r)
	}
	return result
}

// InsertRegistration adds a registration and reports whether the pool holds it. It replaces a pending
// registration for the same pair, so a newer valid registration takes the place of one that became
// invalid. A full pool refuses a new pair. Callers validate r first.
func (p *Pool) InsertRegistration(r *ethpb.SignedPQKeyRegistration) bool {
	if r == nil || r.Message == nil {
		return false
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	k := keyOf(r)
	if _, ok := p.m[k]; !ok && len(p.m) >= p.limit {
		return false
	}
	p.m[k] = r
	return true
}

// remove deletes r, unless a newer registration has replaced it for its pair.
func (p *Pool) remove(r *ethpb.SignedPQKeyRegistration) {
	p.lock.Lock()
	defer p.lock.Unlock()
	k := keyOf(r)
	if p.m[k] == r {
		delete(p.m, k)
	}
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
