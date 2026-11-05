package xmss

import (
	"hash"

	"golang.org/x/crypto/sha3"
)

// TweakableHash.lean: the hash every other definition is built from.
//
//	Th(P, A, M) = H_0(P | A | M)
//
// H is Keccak-256 with Keccak's original padding. H_0 and H_1 are the first and the last
// 16 bytes of its output.

// maxHashInput is the longest hash input: P | A, then the 58 chain ends of a leaf.
const maxHashInput = publicParamLen + addressLen + numChains*digestLen

// keccakState is the Keccak-256 state of golang.org/x/crypto/sha3. Its Read squeezes the
// digest without the copy that Sum makes.
type keccakState interface {
	hash.Hash
	Read([]byte) (int, error)
}

// hasher computes H. It reuses one Keccak state and one input buffer, so a hash call does
// not allocate. A hasher is not safe for concurrent use.
type hasher struct {
	state keccakState
	buf   [maxHashInput]byte
	out   [32]byte
}

func newHasher() *hasher {
	return &hasher{state: sha3.NewLegacyKeccak256().(keccakState)}
}

// hash returns H of the concatenation of parts.
// The parts must total at most maxHashInput bytes.
func (h *hasher) hash(parts ...[]byte) [32]byte {
	n := 0
	for _, p := range parts {
		n += copy(h.buf[n:], p)
	}
	h.state.Reset()
	// Keccak writes and reads never fail.
	_, _ = h.state.Write(h.buf[:n])
	_, _ = h.state.Read(h.out[:])
	return h.out
}

// thFull returns H(P | A | M), both halves.
func (h *hasher) thFull(pp *publicParam, a address, payload ...[]byte) [32]byte {
	n := copy(h.buf[:], pp[:])
	n += copy(h.buf[n:], a[:])
	for _, p := range payload {
		n += copy(h.buf[n:], p)
	}
	h.state.Reset()
	_, _ = h.state.Write(h.buf[:n])
	_, _ = h.state.Read(h.out[:])
	return h.out
}

// th returns Th(P, A, M) = H_0(P | A | M): hash a payload under one address of one key.
func (h *hasher) th(pp *publicParam, a address, payload ...[]byte) digest {
	full := h.thFull(pp, a, payload...)
	return digest(full[:digestLen])
}
