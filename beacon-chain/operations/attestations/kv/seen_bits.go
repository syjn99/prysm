package kv

import (
	"bytes"

	"github.com/OffchainLabs/go-bitfield"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
)

// seenBits holds the aggregation bits seen for one attestation data. A single-bit query is contained in some
// seen bitlist exactly when union has its bit, so only multi-bit bitlists are kept for multi-bit queries.
type seenBits struct {
	union bitfield.Bitlist
	multi []bitfield.Bitlist
}

// contains reports whether one seen bitlist contains every bit of b.
func (s seenBits) contains(b bitfield.Bitlist) (bool, error) {
	if s.union.Len() != b.Len() {
		return false, bitfield.ErrBitlistDifferentLength
	}
	switch b.Count() {
	case 0:
		return true, nil
	case 1:
		return s.union.BitAt(uint64(b.BitIndices()[0])), nil
	}
	for _, m := range s.multi {
		if c, err := m.Contains(b); err != nil || c {
			return c, err
		}
	}
	return false, nil
}

func (c *AttCaches) insertSeenBit(att ethpb.Att) error {
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}
	bits := att.GetAggregationBits()

	// Concurrent inserts for one attestation data must not drop each other's bits.
	c.seenAttLock.Lock()
	defer c.seenAttLock.Unlock()

	seen, err := c.seenBitsFor(id)
	if err != nil {
		return err
	}
	if seen == nil {
		// Start from a copy of the bits, Or would index past nil or empty bits.
		next := seenBits{union: bitfield.Bitlist(bytes.Clone(bits))}
		if bits.Count() > 1 {
			next.multi = []bitfield.Bitlist{bits}
		}
		c.seenAtt.Set(id.String(), next, cache.DefaultExpiration /* one epoch */)
		return nil
	}
	if ok, err := seen.contains(bits); err != nil {
		return err
	} else if ok {
		c.seenAtt.Set(id.String(), *seen, cache.DefaultExpiration /* one epoch */)
		return nil
	}
	// Build a new value, readers may hold the old one.
	union, err := seen.union.Or(bits)
	if err != nil {
		return err
	}
	next := seenBits{union: union, multi: seen.multi}
	if bits.Count() > 1 {
		next.multi = append(seen.multi[:len(seen.multi):len(seen.multi)], bits)
	}
	c.seenAtt.Set(id.String(), next, cache.DefaultExpiration /* one epoch */)
	return nil
}

func (c *AttCaches) hasSeenBit(att ethpb.Att) (bool, error) {
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return false, errors.Wrap(err, "could not create attestation ID")
	}
	seen, err := c.seenBitsFor(id)
	if err != nil || seen == nil {
		return false, err
	}
	return seen.contains(att.GetAggregationBits())
}

func (c *AttCaches) seenBitsFor(id attestation.Id) (*seenBits, error) {
	v, ok := c.seenAtt.Get(id.String())
	if !ok {
		return nil, nil
	}
	seen, ok := v.(seenBits)
	if !ok {
		return nil, errors.New("could not convert to seen bits type")
	}
	return &seen, nil
}
