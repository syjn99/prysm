package xmss

import (
	"encoding/hex"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestConstants(t *testing.T) {
	// The identities of Constants.lean.
	assert.Equal(t, 8*digestLen, 2*numDigits)
	assert.Equal(t, 6, numDigits-numChains)
	assert.Equal(t, 68, numChains*(chainLength-1)-targetSum)
	assert.Equal(t, 2*layerHeight, 8*leafIndexSize)
	assert.Equal(t, "6c65616e584d5353", hex.EncodeToString(seedTag))
	assert.Equal(t, 32, pubKeySize)
	assert.Equal(t, 1200, layerSigSize)
	assert.Equal(t, 2404, sigSize)
}

func TestEncodeAddress(t *testing.T) {
	t.Run("layout", func(t *testing.T) {
		got := encodeAddress(addrTypeChain, 2, 0x00abcdef, 0x01020304)
		want := address{0x04, 0x03, 0x02, 0x01, 0xef, 0xcd, 0xab, 1 + 32*2}
		assert.Equal(t, want, got)
	})
	t.Run("top layer sets bit 23 of hi", func(t *testing.T) {
		got := addrNode(topLayer, 16, 7)
		want := address{7, 0, 0, 0, 16, 0, 0x80, addrTypeNode}
		assert.Equal(t, want, got)
	})
	t.Run("param", func(t *testing.T) {
		assert.Equal(t, address{7: addrTypeParam}, addrParam())
	})
}

func TestCodeword(t *testing.T) {
	t.Run("all zero is not a codeword", func(t *testing.T) {
		_, ok := codewordOf(digest{})
		assert.Equal(t, false, ok)
	})
	t.Run("digits sum to the target", func(t *testing.T) {
		// 35 chains at 3 and one chain at 1 sum to 106.
		var d digest
		for i := range 35 {
			d[i/4] |= 3 << (2 * (i % 4))
		}
		d[35/4] |= 1 << (2 * (35 % 4))
		x, ok := codewordOf(d)
		require.Equal(t, true, ok)
		assert.Equal(t, uint8(3), x[0])
		assert.Equal(t, uint8(1), x[35])
		assert.Equal(t, uint8(0), x[57])
	})
	t.Run("nonzero tail digit is not a codeword", func(t *testing.T) {
		var d digest
		for i := range 35 {
			d[i/4] |= 3 << (2 * (i % 4))
		}
		// Digit 63 is the top two bits of the last byte.
		d[15] |= 1 << 6
		_, ok := codewordOf(d)
		assert.Equal(t, false, ok)
	})
}

func TestTrialRandomizer(t *testing.T) {
	t.Run("carries into the high word", func(t *testing.T) {
		base := randomizer{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		assert.Equal(t, randomizer{8: 1}, trialRandomizer(&base, 1))
	})
	t.Run("wraps mod 2^128", func(t *testing.T) {
		var base randomizer
		for i := range base {
			base[i] = 0xff
		}
		assert.Equal(t, randomizer{}, trialRandomizer(&base, 1))
	})
}

func TestSignVerifyRoundTrip(t *testing.T) {
	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i)
	}
	sk, pk := KeyGen(seed)
	msg := [32]byte{1, 2, 3}

	for _, idx := range []uint32{0, 0xFFFF_FFFE} {
		sig, err := Sign(sk, idx, msg)
		require.NoError(t, err)
		require.Equal(t, sigSize, len(sig))

		t.Run("valid", func(t *testing.T) {
			assert.Equal(t, true, Verify(pk, msg, sig))
		})
		t.Run("leaf index in the first 4 bytes", func(t *testing.T) {
			assert.Equal(t, idx, uint32(sig[0])|uint32(sig[1])<<8|uint32(sig[2])<<16|uint32(sig[3])<<24)
		})
		t.Run("deterministic", func(t *testing.T) {
			again, err := Sign(sk, idx, msg)
			require.NoError(t, err)
			assert.DeepEqual(t, sig, again)
		})
		t.Run("tampered message", func(t *testing.T) {
			m := msg
			m[31] ^= 1
			assert.Equal(t, false, Verify(pk, m, sig))
		})
		t.Run("tampered public key root", func(t *testing.T) {
			p := pk
			p[0] ^= 1
			assert.Equal(t, false, Verify(p, msg, sig))
		})
		t.Run("tampered public parameter", func(t *testing.T) {
			p := pk
			p[31] ^= 1
			assert.Equal(t, false, Verify(p, msg, sig))
		})
		t.Run("tampered signature", func(t *testing.T) {
			// One byte in each field of each layer.
			for _, off := range []int{
				leafIndexSize,                                // bottom randomizer
				leafIndexSize + randomizerLen,                // bottom chain value
				leafIndexSize + layerSigSize - 1,             // bottom path
				leafIndexSize + layerSigSize,                 // top randomizer
				leafIndexSize + layerSigSize + randomizerLen, // top chain value
				sigSize - 1,                                  // top path
			} {
				s := append([]byte(nil), sig...)
				s[off] ^= 1
				assert.Equal(t, false, Verify(pk, msg, s), "offset %d", off)
			}
		})
		t.Run("wrong leaf index", func(t *testing.T) {
			s := append([]byte(nil), sig...)
			s[0] ^= 1
			assert.Equal(t, false, Verify(pk, msg, s))
		})
		t.Run("short signature", func(t *testing.T) {
			assert.Equal(t, false, Verify(pk, msg, sig[:sigSize-1]))
			assert.Equal(t, false, Verify(pk, msg, nil))
		})
		t.Run("long signature", func(t *testing.T) {
			assert.Equal(t, false, Verify(pk, msg, append(append([]byte(nil), sig...), 0)))
		})
	}
}

func FuzzVerify(f *testing.F) {
	pk := [32]byte(fromHex(f, kaPublicKeyBytes))
	msg := kaMessage()
	f.Add(pk[:], msg[:], fromHex(f, kaSignatureBytes))
	f.Add([]byte{}, []byte{}, []byte{})
	f.Add(pk[:], msg[:], make([]byte, sigSize))
	f.Fuzz(func(t *testing.T, pkBytes, msgBytes, sig []byte) {
		var p, m [32]byte
		copy(p[:], pkBytes)
		copy(m[:], msgBytes)
		Verify(p, m, sig)
	})
}

func BenchmarkKeyGen(b *testing.B) {
	seed := kaSeed()
	for b.Loop() {
		KeyGen(seed)
	}
}

func BenchmarkSign(b *testing.B) {
	sk := SecretKey{Seed: kaSeed()}
	msg := kaMessage()
	for b.Loop() {
		_, err := Sign(sk, kaLeafIndex, msg)
		require.NoError(b, err)
	}
}

func BenchmarkVerify(b *testing.B) {
	pk := [32]byte(fromHex(b, kaPublicKeyBytes))
	msg := kaMessage()
	sig := fromHex(b, kaSignatureBytes)
	b.ReportAllocs()
	for b.Loop() {
		require.Equal(b, true, Verify(pk, msg, sig))
	}
}
