package xmss

import "encoding/binary"

// Serialization.lean: the byte encodings of the public key and the signature.
//
// Every field has a fixed size. So an encoding is the fields concatenated, with no offsets
// and no length prefixes.

// publicKey is what a verifier knows about a signer.
type publicKey struct {
	// root is X^1_{16,0}, the root of the top tree.
	root digest
	// publicParam is the public parameter every hash of this key is made under.
	publicParam publicParam
}

// layerSignature is one layer's signature: a one-time signature and the path of its key.
type layerSignature struct {
	randomizer  randomizer
	chainValues [numChains]digest
	path        [layerHeight]digest
}

// signature is a leaf index and one signature per layer.
type signature struct {
	leafIndex uint32
	bottom    layerSignature
	top       layerSignature
}

// encodePublicKey encodes a public key as 32 bytes:
//
//	[ root: 16 | public parameter: 16 ]
func encodePublicKey(pk *publicKey) [pubKeySize]byte {
	var out [pubKeySize]byte
	copy(out[:digestLen], pk.root[:])
	copy(out[digestLen:], pk.publicParam[:])
	return out
}

// decodePublicKey decodes a public key from 32 bytes. Every 32-byte string is a public key.
func decodePublicKey(b [pubKeySize]byte) publicKey {
	var pk publicKey
	copy(pk.root[:], b[:digestLen])
	copy(pk.publicParam[:], b[digestLen:])
	return pk
}

// appendLayerSignature appends one layer's signature as 1200 bytes:
//
//	[ randomizer: 16 | 58 chain values: 928 | 16 siblings: 256 ]
func appendLayerSignature(out []byte, sig *layerSignature) []byte {
	out = append(out, sig.randomizer[:]...)
	for i := range sig.chainValues {
		out = append(out, sig.chainValues[i][:]...)
	}
	for i := range sig.path {
		out = append(out, sig.path[i][:]...)
	}
	return out
}

// encodeSignature encodes a signature as 2404 bytes:
//
//	[ leaf index: 4 | bottom layer: 1200 | top layer: 1200 ]
//
// The leaf index is a little-endian 32-bit word.
func encodeSignature(sig *signature) []byte {
	out := make([]byte, 0, sigSize)
	out = binary.LittleEndian.AppendUint32(out, sig.leafIndex)
	out = appendLayerSignature(out, &sig.bottom)
	return appendLayerSignature(out, &sig.top)
}

// decodeLayerSignature reads one layer's signature from exactly layerSigSize bytes.
func decodeLayerSignature(b []byte) layerSignature {
	var sig layerSignature
	off := copy(sig.randomizer[:], b)
	for i := range sig.chainValues {
		off += copy(sig.chainValues[i][:], b[off:])
	}
	for i := range sig.path {
		off += copy(sig.path[i][:], b[off:])
	}
	return sig
}

// decodeSignature decodes a signature from 2404 bytes. Length is the only check: every
// 2404-byte string decodes. Decoding does not verify the signature.
func decodeSignature(b []byte) (signature, bool) {
	if len(b) != sigSize {
		return signature{}, false
	}
	return signature{
		leafIndex: binary.LittleEndian.Uint32(b[:leafIndexSize]),
		bottom:    decodeLayerSignature(b[leafIndexSize : leafIndexSize+layerSigSize]),
		top:       decodeLayerSignature(b[leafIndexSize+layerSigSize:]),
	}, true
}
