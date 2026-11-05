package xmss

import "fmt"

// Sign.lean: signing.
//
// Signing is deterministic: the same key, leaf index and message give the same signature.
// Signing is stateless, so tracking spent leaf indices is the caller's job.

// signLayer returns one layer's signature by key e of a message: the one-time signature and
// the path of the key. It fails when no randomizer trial gives a codeword, which happens
// with probability below exp(-2^11).
func signLayer(pp *publicParam, s *[seedLen]byte, l layer, e uint32, msg []byte) (layerSignature, error) {
	rho, y, ok := otsSign(newHasher(), pp, s, l, e, msg)
	if !ok {
		return layerSignature{}, fmt.Errorf("xmss: no codeword within the trial limit for key %d of layer %d", e, l)
	}
	return layerSignature{randomizer: rho, chainValues: y, path: path(pp, s, l, e)}, nil
}

// Sign returns Sig(sk, idx, m): the 2404-byte signature of msg at leaf index idx.
//
//  1. The bottom key idx signs the message.
//  2. The top key b = floor(idx / 2^16) signs the root of bottom tree b.
//
// Never sign two different messages at one leaf index with one secret key. Sign does not
// record which leaf indices it has used; the caller must.
//
// Sign builds the paths of both keys, about one tree of 2^16 one-time keys on each layer,
// on every available CPU. It returns an error only when a one-time key finds no codeword.
func Sign(sk SecretKey, idx uint32, msg [32]byte) ([]byte, error) {
	h := newHasher()
	s, pp := sk.derive(h)
	// Step 1: the bottom key signs the message, with its path in bottom tree b.
	bottom, err := signLayer(&pp, &s, bottomLayer, idx, msg[:])
	if err != nil {
		return nil, err
	}
	// Step 2: the root of bottom tree b, from the key's leaf and its path.
	// The path leads to that root, so this costs one leaf and 16 hashes, not a whole tree.
	leaf := otsPublicLeaf(h, &pp, &s, bottomLayer, idx)
	bottomRoot := fold(h, &pp, bottomLayer, idx, leaf, &bottom.path)
	// Step 3: the top key b signs that root, with its path in the top tree.
	top, err := signLayer(&pp, &s, topLayer, bottomTree(idx), bottomRoot[:])
	if err != nil {
		return nil, err
	}
	sig := signature{leafIndex: idx, bottom: bottom, top: top}
	return encodeSignature(&sig), nil
}
