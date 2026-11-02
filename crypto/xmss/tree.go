package xmss

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// Tree.lean: the Merkle trees of one layer, whose leaves are the layer's one-time keys.
//
// On layer l, for 1 <= lambda <= 16:
//
//	X_{lambda,j} = Th(P, A_node(l, lambda, j), X_{lambda-1,2j} | X_{lambda-1,2j+1})
//
// The top layer is one tree, with root X_{16,0}. The bottom layer is 2^16 trees whose node
// indices are global: bottom tree b holds the nodes j with floor(j / 2^(16 - lambda)) = b.

// nodeHash returns the parent at index j on a level, from its two children.
func nodeHash(h *hasher, pp *publicParam, l layer, level uint32, j uint32, left, right *digest) digest {
	return h.th(pp, addrNode(l, level, j), left[:], right[:])
}

// leafChunk is the number of leaves one worker computes per claim.
const leafChunk = 64

// leaves returns the n one-time key leaves first, ..., first + n - 1 of a layer.
// The leaves are independent, so it computes them on every available CPU.
// The result does not depend on the number of workers.
func leaves(pp *publicParam, s *[seedLen]byte, l layer, first uint32, n int) []digest {
	out := make([]digest, n)
	workers := min(runtime.GOMAXPROCS(0), (n+leafChunk-1)/leafChunk)
	if workers <= 1 {
		h := newHasher()
		for k := range out {
			out[k] = otsPublicLeaf(h, pp, s, l, first+uint32(k))
		}
		return out
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			h := newHasher()
			for {
				start := int(next.Add(leafChunk)) - leafChunk
				if start >= n {
					return
				}
				for k := start; k < min(start+leafChunk, n); k++ {
					out[k] = otsPublicLeaf(h, pp, s, l, first+uint32(k))
				}
			}
		})
	}
	wg.Wait()
	return out
}

// node returns X_{level,j}: node j on a level of a layer.
// Computing it evaluates the 2^level leaves below it.
// The caller keeps level at most layerHeight and j below 2^(32 - level).
func node(pp *publicParam, s *[seedLen]byte, l layer, level uint32, j uint32) digest {
	// The leaves below node j on level `level` are j 2^level to (j + 1) 2^level - 1.
	xs := leaves(pp, s, l, j<<level, 1<<level)
	h := newHasher()
	for lv := uint32(1); lv <= level; lv++ {
		// The nodes on level lv below node j start at global index j 2^(level - lv).
		base := j << (level - lv)
		for k := range len(xs) / 2 {
			xs[k] = nodeHash(h, pp, l, lv, base+uint32(k), &xs[2*k], &xs[2*k+1])
		}
		xs = xs[:len(xs)/2]
	}
	return xs[0]
}

// siblingIndex returns the index of the sibling of key e's ancestor on a level:
// that ancestor with its low bit flipped.
func siblingIndex(e uint32, level uint32) uint32 {
	return (e >> level) ^ 1
}

// path returns the path of key e, from the leaf up:
//
//	pi_lambda = X_{lambda, floor(e / 2^lambda) xor 1}
func path(pp *publicParam, s *[seedLen]byte, l layer, e uint32) [layerHeight]digest {
	var pi [layerHeight]digest
	for level := range uint32(layerHeight) {
		pi[level] = node(pp, s, l, level, siblingIndex(e, level))
	}
	return pi
}

// fold returns Tree.fold(l, e, X, pi): the root a path leads to from a leaf.
func fold(h *hasher, pp *publicParam, l layer, e uint32, leaf digest, pi *[layerHeight]digest) digest {
	current := leaf
	for level := range uint32(layerHeight) {
		// The parent sits one level up, at the index with one more bit shifted out.
		parent := level + 1
		j := e >> parent
		// Bit `level` of e says which child the held node is.
		if (e>>level)%2 == 0 {
			current = nodeHash(h, pp, l, parent, j, &current, &pi[level])
		} else {
			current = nodeHash(h, pp, l, parent, j, &pi[level], &current)
		}
	}
	return current
}
