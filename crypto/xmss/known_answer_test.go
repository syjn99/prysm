package xmss

import (
	"encoding/hex"
	"testing"
)

// The known answer of EthLib/Cryptography/Proofs/Xmss/KnownAnswer*.lean:
//
//	seed        S_0[i] = 37 i + 11 mod 256,   i < 32
//	message     m[i]   = 7 i + 3,             i < 32
//	leaf index  0xA5C37E19, in bottom tree 0xA5C3
//
// Each subtest names the Lean theorem it ports.

const kaLeafIndex uint32 = 0xA5C37E19

func kaSeed() [seedLen]byte {
	var s [seedLen]byte
	for i := range s {
		s[i] = byte(37*i + 11)
	}
	return s
}

func kaMessage() [messageLen]byte {
	var m [messageLen]byte
	for i := range m {
		m[i] = byte(7*i + 3)
	}
	return m
}

func fromHex(t testing.TB, s string) []byte {
	b, err := hex.DecodeString(s)
	noError(t, err)
	return b
}

func hexDigest(t testing.TB, s string) digest {
	b := fromHex(t, s)
	mustEqual(t, digestLen, len(b))
	return digest(b)
}

func hexDigests(t testing.TB, s string) [numChains]digest {
	b := fromHex(t, s)
	mustEqual(t, numChains*digestLen, len(b))
	var out [numChains]digest
	for i := range out {
		copy(out[i][:], b[i*digestLen:])
	}
	return out
}

func hexPath(t testing.TB, s string) [layerHeight]digest {
	b := fromHex(t, s)
	mustEqual(t, layerHeight*digestLen, len(b))
	var out [layerHeight]digest
	for i := range out {
		copy(out[i][:], b[i*digestLen:])
	}
	return out
}

// kaPublicKey is the Lean `publicKey`, field by field.
func kaPublicKey(t testing.TB) publicKey {
	return publicKey{root: hexDigest(t, kaPublicKeyRoot), publicParam: hexDigest(t, kaPublicKeyParam)}
}

// kaSignature is the Lean `signature`, field by field.
func kaSignature(t testing.TB) signature {
	return signature{
		leafIndex: kaLeafIndex,
		bottom: layerSignature{
			randomizer:  hexDigest(t, kaSigBottomRandomizer),
			chainValues: hexDigests(t, kaSigBottomChains),
			path:        hexPath(t, kaSigBottomPath),
		},
		top: layerSignature{
			randomizer:  hexDigest(t, kaSigTopRandomizer),
			chainValues: hexDigests(t, kaSigTopChains),
			path:        hexPath(t, kaSigTopPath),
		},
	}
}

// chainEnd walks the revealed value of chain i from its codeword digit to the chain's end.
func chainEnd(h *hasher, pp *publicParam, l layer, e uint32, x codeword, y *[numChains]digest, i int) digest {
	return chain(h, pp, l, e, i, x[i], chainLength-1-x[i], y[i])
}

// TestKnownAnswer ports EthLib/Cryptography/Proofs/Xmss/KnownAnswer.lean.
func TestKnownAnswer(t *testing.T) {
	pk := kaPublicKey(t)
	sig := kaSignature(t)
	msg := kaMessage()
	seed := kaSeed()
	h := newHasher()

	t.Run("encode_publicKey", func(t *testing.T) {
		enc := encodePublicKey(&pk)
		equalBytes(t, fromHex(t, kaPublicKeyBytes), enc[:])
	})
	t.Run("encode_signature", func(t *testing.T) {
		equalBytes(t, fromHex(t, kaSignatureBytes), encodeSignature(&sig))
	})
	t.Run("decode_publicKey", func(t *testing.T) {
		got := decodePublicKey([pubKeySize]byte(fromHex(t, kaPublicKeyBytes)))
		equal(t, pk, got)
	})
	t.Run("decode_signature", func(t *testing.T) {
		got, ok := decodeSignature(fromHex(t, kaSignatureBytes))
		mustEqual(t, true, ok)
		equal(t, sig, got)
	})
	t.Run("schemeSeed_known_answer", func(t *testing.T) {
		s := schemeSeed(h, &seed)
		equalBytes(t, fromHex(t, kaSchemeSeed), s[:])
	})
	t.Run("publicParam_known_answer", func(t *testing.T) {
		s := schemeSeed(h, &seed)
		equal(t, pk.publicParam, publicParamOf(h, &s))
	})
	t.Run("verify_wrong_message", func(t *testing.T) {
		m := msg
		m[0] = 0
		equal(t, false, verify(h, &pk, &m, &sig))
	})
	t.Run("verify_wrong_leafIndex", func(t *testing.T) {
		s := sig
		s.leafIndex = kaLeafIndex + 1
		equal(t, false, verify(h, &pk, &msg, &s))
	})
}

// testLayerKnownAnswer ports the layer files KnownAnswerBottom.lean and KnownAnswerTop.lean.
func testLayerKnownAnswer(t *testing.T, l layer, e uint32, msg []byte, sig *layerSignature, wantCodeword codeword, endsHex, leafHex string, wantRoot digest, name string) {
	pk := kaPublicKey(t)
	pp := &pk.publicParam
	h := newHasher()
	ends := hexDigests(t, endsHex)
	leaf := hexDigest(t, leafHex)

	t.Run(name+"Codeword_known_answer", func(t *testing.T) {
		x, ok := codewordOf(messageDigest(h, pp, l, e, msg, &sig.randomizer))
		mustEqual(t, true, ok)
		equal(t, wantCodeword, x)
	})
	t.Run(name+"Chains", func(t *testing.T) {
		for i := range numChains {
			equal(t, ends[i], chainEnd(h, pp, l, e, wantCodeword, &sig.chainValues, i), "chain %d", i)
		}
	})
	t.Run(name+"Leaf_known_answer", func(t *testing.T) {
		equal(t, leaf, otsLeafHash(h, pp, l, e, &ends))
	})
	t.Run(name+"Fold_known_answer", func(t *testing.T) {
		equal(t, wantRoot, fold(h, pp, l, e, leaf, &sig.path))
	})
	t.Run(name+"LayerRoot_known_answer", func(t *testing.T) {
		root, ok := layerRoot(h, pp, l, e, msg, sig)
		mustEqual(t, true, ok)
		equal(t, wantRoot, root)
	})
}

// TestKnownAnswerBottom ports EthLib/Cryptography/Proofs/Xmss/KnownAnswerBottom.lean.
func TestKnownAnswerBottom(t *testing.T) {
	sig := kaSignature(t)
	msg := kaMessage()
	testLayerKnownAnswer(t, bottomLayer, kaLeafIndex, msg[:], &sig.bottom, kaBottomCodeword,
		kaBottomEnds, kaBottomLeaf, hexDigest(t, kaBottomRoot), "bottom")
}

// TestKnownAnswerTop ports EthLib/Cryptography/Proofs/Xmss/KnownAnswerTop.lean.
func TestKnownAnswerTop(t *testing.T) {
	sig := kaSignature(t)
	bottomRoot := hexDigest(t, kaBottomRoot)
	testLayerKnownAnswer(t, topLayer, bottomTree(kaLeafIndex), bottomRoot[:], &sig.top, kaTopCodeword,
		kaTopEnds, kaTopLeaf, kaPublicKey(t).root, "top")
}

// testKeyKnownAnswer ports the key files KnownAnswerKeyBottom.lean and KnownAnswerKeyTop.lean.
func testKeyKnownAnswer(t *testing.T, l layer, e uint32, startsHex, endsHex, leafHex, name string) {
	pk := kaPublicKey(t)
	pp := &pk.publicParam
	h := newHasher()
	seed := kaSeed()
	s := schemeSeed(h, &seed)
	starts := hexDigests(t, startsHex)
	ends := hexDigests(t, endsHex)

	t.Run(name+"Starts_known_answer", func(t *testing.T) {
		equal(t, starts, chainStarts(h, pp, &s, l, e))
	})
	t.Run(name+"KeyChains", func(t *testing.T) {
		for i := range numChains {
			equal(t, ends[i], chain(h, pp, l, e, i, 0, chainLength-1, starts[i]), "chain %d", i)
		}
	})
	t.Run(name+"PublicLeaf_known_answer", func(t *testing.T) {
		equal(t, hexDigest(t, leafHex), otsPublicLeaf(h, pp, &s, l, e))
	})
}

// TestKnownAnswerKeyBottom ports EthLib/Cryptography/Proofs/Xmss/KnownAnswerKeyBottom.lean.
func TestKnownAnswerKeyBottom(t *testing.T) {
	testKeyKnownAnswer(t, bottomLayer, kaLeafIndex, kaBottomStarts, kaBottomEnds, kaBottomLeaf, "bottom")
}

// TestKnownAnswerKeyTop ports EthLib/Cryptography/Proofs/Xmss/KnownAnswerKeyTop.lean.
func TestKnownAnswerKeyTop(t *testing.T) {
	testKeyKnownAnswer(t, topLayer, bottomTree(kaLeafIndex), kaTopStarts, kaTopEnds, kaTopLeaf, "top")
}

// TestKnownAnswerVerify ports EthLib/Cryptography/Proofs/Xmss/KnownAnswerVerify.lean.
func TestKnownAnswerVerify(t *testing.T) {
	pk := kaPublicKey(t)
	sig := kaSignature(t)
	msg := kaMessage()
	h := newHasher()
	tampered := sig.top
	tampered.path[15] = digest{}

	t.Run("verify_signature", func(t *testing.T) {
		equal(t, true, verify(h, &pk, &msg, &sig))
	})
	t.Run("verify_known_answer", func(t *testing.T) {
		pkBytes := [pubKeySize]byte(fromHex(t, kaPublicKeyBytes))
		equal(t, true, Verify(pkBytes, msg, fromHex(t, kaSignatureBytes)))
	})
	t.Run("tamperedFold_known_answer", func(t *testing.T) {
		root := fold(h, &pk.publicParam, topLayer, bottomTree(kaLeafIndex), hexDigest(t, kaTopLeaf), &tampered.path)
		equal(t, hexDigest(t, kaTamperedRoot), root)
	})
	t.Run("verify_tampered_path", func(t *testing.T) {
		s := sig
		s.top = tampered
		equal(t, false, verify(h, &pk, &msg, &s))
	})
}
