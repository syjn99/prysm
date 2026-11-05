package xmss

import (
	"encoding/binary"
	"math/bits"
	"runtime"
	"sync"
	"sync/atomic"
)

// Ots.lean: the WOTS+C one-time signature, 58 hash chains of 4 positions.
//
// The chains of key e on layer l, for i < 58 and mu < 3:
//
//	C_{i,0}      = H_{i mod 2}(P | A_prf(l, e, floor(i / 2)) | S)
//	C_{i,mu+1}   = Th(P, A_chain(l, e, mu), C_{i,mu} | LE_128(i))
//	leaf X_{0,e} = Th(P, A_leaf(l, e), C_{0,3} | ... | C_{57,3})

// chainStarts returns the 58 chain starts of key e, from 29 hashes of the scheme seed.
// Chains 2j and 2j + 1 start at the two halves of hash j.
func chainStarts(h *hasher, pp *publicParam, s *[seedLen]byte, l layer, e uint32) [numChains]digest {
	var starts [numChains]digest
	for j := range numChains / 2 {
		full := h.thFull(pp, addrPrf(l, e, uint32(j)), s[:])
		copy(starts[2*j][:], full[:digestLen])
		copy(starts[2*j+1][:], full[digestLen:])
	}
	return starts
}

// chainStep returns one step of chain i, from position mu to the next.
// The chain index follows the value, so every chain of a key shares the step's address.
func chainStep(h *hasher, pp *publicParam, l layer, e uint32, i int, mu uint8, value digest) digest {
	// LE_128(i): i is below 58, so only the low byte is nonzero.
	var index [16]byte
	index[0] = byte(i)
	return h.th(pp, addrChain(l, e, mu), value[:], index[:])
}

// chain walks chain i for steps steps, starting at position start.
// The caller keeps start + steps below chainLength.
func chain(h *hasher, pp *publicParam, l layer, e uint32, i int, start, steps uint8, value digest) digest {
	for s := range steps {
		value = chainStep(h, pp, l, e, i, start+s, value)
	}
	return value
}

// otsLeafHash returns the Merkle leaf of a one-time key, from the ends of its 58 chains.
func otsLeafHash(h *hasher, pp *publicParam, l layer, e uint32, ends *[numChains]digest) digest {
	var payload [numChains * digestLen]byte
	for i := range ends {
		copy(payload[i*digestLen:], ends[i][:])
	}
	return h.th(pp, addrLeaf(l, e), payload[:])
}

// otsPublicLeaf returns X_{0,e}: the leaf of key e, computed from the scheme seed.
func otsPublicLeaf(h *hasher, pp *publicParam, s *[seedLen]byte, l layer, e uint32) digest {
	ends := chainStarts(h, pp, s, l, e)
	// Every chain walked from position 0 to position 3.
	for i := range ends {
		ends[i] = chain(h, pp, l, e, i, 0, chainLength-1, ends[i])
	}
	return otsLeafHash(h, pp, l, e, &ends)
}

// messageDigest returns D = Th(P, A_msg(l, e), M | rho): the digest key e signs under one
// randomizer. M is the 32-byte message on the bottom layer, and a 16-byte root on the top.
func messageDigest(h *hasher, pp *publicParam, l layer, e uint32, msg []byte, rho *randomizer) digest {
	return h.th(pp, addrMsg(l, e), msg, rho[:])
}

// randomizerBase returns R_0 = Th(P, A_rnd(l, e), S | M): the base every randomizer trial
// adds to.
func randomizerBase(h *hasher, pp *publicParam, s *[seedLen]byte, l layer, e uint32, msg []byte) randomizer {
	return h.th(pp, addrRnd(l, e), s[:], msg)
}

// trialRandomizer returns rho_c = LE_128(LE^-1(R_0) + c mod 2^128): the randomizer of trial c.
func trialRandomizer(base *randomizer, c uint64) randomizer {
	lo := binary.LittleEndian.Uint64(base[0:8])
	hi := binary.LittleEndian.Uint64(base[8:16])
	lo, carry := bits.Add64(lo, c, 0)
	hi += carry
	var rho randomizer
	binary.LittleEndian.PutUint64(rho[0:8], lo)
	binary.LittleEndian.PutUint64(rho[8:16], hi)
	return rho
}

// trialBatch is the number of consecutive randomizer trials one worker claims at a time.
const trialBatch = 1 << 12

// findCodeword tries the trials c = 0, 1, ... below maxRandomizerTrials.
// It returns the randomizer of the least trial whose digest is a codeword, with the codeword.
//
// A digest is a codeword with probability about 2^-20, so it searches on every available CPU.
// Workers claim batches of trials in increasing order and stop at the least trial found so
// far. Every batch below that trial is searched in full, so the result is the least trial,
// whatever the number of workers.
func findCodeword(pp *publicParam, l layer, e uint32, msg []byte, base *randomizer) (randomizer, codeword, bool) {
	var next, best atomic.Uint64
	best.Store(maxRandomizerTrials)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			h := newHasher()
			for {
				start := next.Add(trialBatch) - trialBatch
				if start >= best.Load() {
					return
				}
				for c := start; c < start+trialBatch && c < best.Load(); c++ {
					rho := trialRandomizer(base, c)
					if _, ok := codewordOf(messageDigest(h, pp, l, e, msg, &rho)); ok {
						// Lower best to c, unless another worker found a lesser trial.
						for {
							cur := best.Load()
							if c >= cur || best.CompareAndSwap(cur, c) {
								break
							}
						}
						break
					}
				}
			}
		})
	}
	wg.Wait()
	c := best.Load()
	if c >= maxRandomizerTrials {
		return randomizer{}, codeword{}, false
	}
	rho := trialRandomizer(base, c)
	x, _ := codewordOf(messageDigest(newHasher(), pp, l, e, msg, &rho))
	return rho, x, true
}

// otsSign returns Ots.sign(l, e, M): the randomizer and the 58 chain values that sign M.
// It takes the least trial whose digest is a codeword x, and reveals chain i at position x_i.
// It returns false when no trial gives a codeword.
func otsSign(h *hasher, pp *publicParam, s *[seedLen]byte, l layer, e uint32, msg []byte) (randomizer, [numChains]digest, bool) {
	// Step 1: grind the randomizer until the digest is a codeword.
	base := randomizerBase(h, pp, s, l, e, msg)
	rho, x, ok := findCodeword(pp, l, e, msg, &base)
	if !ok {
		return randomizer{}, [numChains]digest{}, false
	}
	// Step 2: reveal each chain at the position its digit names.
	y := chainStarts(h, pp, s, l, e)
	for i := range y {
		y[i] = chain(h, pp, l, e, i, 0, x[i], y[i])
	}
	return rho, y, true
}

// otsLeaf returns Ots.leaf(l, e, M, rho, y): the leaf a claimed one-time signature leads to.
// It returns false when the digest is not a codeword.
func otsLeaf(h *hasher, pp *publicParam, l layer, e uint32, msg []byte, rho *randomizer, y *[numChains]digest) (digest, bool) {
	// Step 1: recompute the codeword.
	x, ok := codewordOf(messageDigest(h, pp, l, e, msg, rho))
	if !ok {
		return digest{}, false
	}
	// Step 2: walk chain i from position x_i to position 3.
	var ends [numChains]digest
	for i := range ends {
		ends[i] = chain(h, pp, l, e, i, x[i], chainLength-1-x[i], y[i])
	}
	// Step 3: hash the ends into the leaf.
	return otsLeafHash(h, pp, l, e, &ends), true
}
