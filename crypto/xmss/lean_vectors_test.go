package xmss

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// testdata/lean_vectors.json holds vectors computed by the compiled Lean model:
// keyGenExport, signExport and verifyExport of EthLib.Cryptography.Exports.Xmss at
// ethereum/EthLib 6390c73ef040311e807b9047d99dca6b1be1c42f, built with Lean v4.35.0.
//
// Each verify case takes the inputs of one sign case, with its public key, and flips some
// bits of one input. The target "none" leaves the inputs as signed.

type leanVectors struct {
	Source string `json:"source"`
	KeyGen []struct {
		Seed      string `json:"seed"`
		PublicKey string `json:"public_key"`
	} `json:"key_gen"`
	Sign []struct {
		Seed      string `json:"seed"`
		LeafIndex uint32 `json:"leaf_index"`
		Message   string `json:"message"`
		Signature string `json:"signature"`
	} `json:"sign"`
	Verify []struct {
		SignCase int    `json:"sign_case"`
		Target   string `json:"target"`
		Offset   int    `json:"offset"`
		Xor      byte   `json:"xor"`
		Valid    bool   `json:"valid"`
	} `json:"verify"`
}

func loadLeanVectors(t *testing.T) *leanVectors {
	b, err := os.ReadFile(filepath.Join("testdata", "lean_vectors.json"))
	noError(t, err)
	var v leanVectors
	noError(t, json.Unmarshal(b, &v))
	if len(v.KeyGen) == 0 || len(v.Sign) == 0 || len(v.Verify) == 0 {
		t.Fatal("empty Lean vectors")
	}
	return &v
}

func TestLeanVectors(t *testing.T) {
	v := loadLeanVectors(t)
	publicKeys := make(map[string][pubKeySize]byte, len(v.KeyGen))

	t.Run("key_gen", func(t *testing.T) {
		for _, c := range v.KeyGen {
			t.Run(c.Seed, func(t *testing.T) {
				_, pk := KeyGen([seedLen]byte(fromHex(t, c.Seed)))
				equalBytes(t, fromHex(t, c.PublicKey), pk[:])
			})
		}
	})
	for _, c := range v.KeyGen {
		publicKeys[c.Seed] = [pubKeySize]byte(fromHex(t, c.PublicKey))
	}

	t.Run("sign", func(t *testing.T) {
		if testing.Short() {
			t.Skip("signing builds two trees per case")
		}
		for _, c := range v.Sign {
			t.Run(fmt.Sprintf("%s/%d", c.Seed[:8], c.LeafIndex), func(t *testing.T) {
				sig, err := Sign(SecretKey{Seed: [seedLen]byte(fromHex(t, c.Seed))}, c.LeafIndex, [messageLen]byte(fromHex(t, c.Message)))
				noError(t, err)
				equalBytes(t, fromHex(t, c.Signature), sig)
			})
		}
	})

	t.Run("verify", func(t *testing.T) {
		for i, c := range v.Verify {
			t.Run(fmt.Sprintf("%d/%s/%d", c.SignCase, c.Target, c.Offset), func(t *testing.T) {
				mustEqual(t, true, c.SignCase < len(v.Sign), "case %d", i)
				s := v.Sign[c.SignCase]
				pk, ok := publicKeys[s.Seed]
				mustEqual(t, true, ok, "no public key for seed %s", s.Seed)
				msg := [messageLen]byte(fromHex(t, s.Message))
				sig := fromHex(t, s.Signature)
				switch c.Target {
				case "none":
				case "public_key":
					pk[c.Offset] ^= c.Xor
				case "message":
					msg[c.Offset] ^= c.Xor
				case "signature":
					sig[c.Offset] ^= c.Xor
				default:
					t.Fatalf("unknown target %q", c.Target)
				}
				equal(t, c.Valid, Verify(pk, msg, sig))
			})
		}
	})
}
