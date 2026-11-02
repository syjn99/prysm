package xmss

// KeyGen.lean: key generation.
//
// The secret key is a 32-byte seed S_0, nothing more. Every secret derives from the scheme
// seed S = H("leanXMSS" | S_0). So the seed alone regenerates every chain, every node and
// the public key.

// SecretKey is a leanXMSS secret key: the seed S_0.
//
// The seed must be 32 uniform bytes, kept secret. A secret key must never sign two different
// messages at one leaf index. Nothing in this package records which leaf indices a key has
// signed, so the caller must prevent reuse.
type SecretKey struct {
	// Seed holds the 32 uniform bytes every secret derives from.
	Seed [seedLen]byte
}

// schemeSeed returns S = H("leanXMSS" | S_0): the seed every secret derives from.
func schemeSeed(h *hasher, seed *[seedLen]byte) [seedLen]byte {
	return h.hash(seedTag, seed[:])
}

// publicParamOf returns P = Th(0^128, A_param, S): the public parameter.
// The all-zero parameter stands in for the one being derived.
func publicParamOf(h *hasher, s *[seedLen]byte) publicParam {
	var zero publicParam
	return h.th(&zero, addrParam(), s[:])
}

// derive returns the scheme seed S and the public parameter P of a secret key.
func (sk *SecretKey) derive(h *hasher) ([seedLen]byte, publicParam) {
	s := schemeSeed(h, &sk.Seed)
	return s, publicParamOf(h, &s)
}

// publicKey returns the public key: the root of the top tree, with the public parameter.
// Computing it builds the whole top tree: 2^16 one-time keys.
func (sk *SecretKey) publicKey() publicKey {
	s, pp := sk.derive(newHasher())
	return publicKey{
		root:        node(&pp, &s, topLayer, layerHeight, 0),
		publicParam: pp,
	}
}

// KeyGen returns the key pair grown from a seed: the secret key and the 32-byte public key.
//
// It builds the whole top tree, 2^16 one-time keys, on every available CPU.
// The seed must be 32 uniform bytes, kept secret.
func KeyGen(seed [32]byte) (SecretKey, [32]byte) {
	sk := SecretKey{Seed: seed}
	pk := sk.publicKey()
	return sk, encodePublicKey(&pk)
}
