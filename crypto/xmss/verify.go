package xmss

// Verify.lean: signature verification.
//
//	X  = Tree.fold(0, idx, Ots.leaf(0, idx, m, rho^0, y^0), pi^0)
//	X' = Tree.fold(1, b,   Ots.leaf(1, b,   X, rho^1, y^1), pi^1)
//	accept  iff  neither Ots.leaf fails and X' = root
//
// b = floor(idx / 2^16) is the bottom tree of leaf index idx. Verification costs a constant
// 186 Keccak-f[1600] permutations. It reads only public values, so it need not be constant time.

// Verify reports whether sig is a valid leanXMSS signature of msg under pubkey.
//
// The signature carries its leaf index in its first 4 bytes, little-endian. Verify returns
// false for a signature of any length other than 2404 bytes, and never panics.
func Verify(pubkey [32]byte, msg [32]byte, sig []byte) bool {
	s, ok := decodeSignature(sig)
	if !ok {
		return false
	}
	pk := decodePublicKey(pubkey)
	return verify(newHasher(), &pk, &msg, &s)
}

// layerRoot returns the root one layer's signature leads to, from the message it signs.
// It returns false when the signed digest is not a codeword.
// e is the signing key's index on its layer. msg is the 32-byte message on the bottom
// layer, and a 16-byte root on the top.
func layerRoot(h *hasher, pp *publicParam, l layer, e uint32, msg []byte, sig *layerSignature) (digest, bool) {
	// Recover the key's leaf from the one-time signature.
	leaf, ok := otsLeaf(h, pp, l, e, msg, &sig.randomizer, &sig.chainValues)
	if !ok {
		return digest{}, false
	}
	// Climb the key's path to its tree's root.
	return fold(h, pp, l, e, leaf, &sig.path), true
}

// verify returns Ver(pk, m, sigma): whether a signature signs a message under a public key.
func verify(h *hasher, pk *publicKey, msg *[messageLen]byte, sig *signature) bool {
	pp := &pk.publicParam
	// Step 1: the bottom key signs the message, so its path reaches bottom tree b's root.
	bottomRoot, ok := layerRoot(h, pp, bottomLayer, sig.leafIndex, msg[:], &sig.bottom)
	if !ok {
		return false
	}
	// Step 2: the top key b signs that root, so its path reaches the top root.
	topRoot, ok := layerRoot(h, pp, topLayer, bottomTree(sig.leafIndex), bottomRoot[:], &sig.top)
	if !ok {
		return false
	}
	// Step 3: neither layer failed, and the top root is the key's.
	return topRoot == pk.root
}
