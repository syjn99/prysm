package xmss

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The data-driven cases of EthLib tests/cryptography/xmss, at ethereum/EthLib
// 6390c73ef040311e807b9047d99dca6b1be1c42f, copied to testdata/ with their SHA256SUMS.
// An output of null means that the input is rejected.

// parseCase reads one data.yaml file into flat keys such as "input.seed" and "output".
//
// The files use a small subset of YAML: one mapping "input" of scalars, and the scalar
// "output". Scalars are quoted hex strings, integers, booleans or null. A quoted value
// loses its quotes.
func parseCase(t *testing.T, data []byte) map[string]string {
	t.Helper()
	fields := make(map[string]string)
	prefix := ""
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		key, value, ok := strings.Cut(strings.TrimRight(line, " \r"), ":")
		mustEqual(t, true, ok, "bad line %q", line)
		value = strings.TrimSpace(value)
		indented := strings.HasPrefix(key, " ")
		key = strings.TrimSpace(key)
		switch {
		case value == "" && !indented:
			prefix = key + "."
			continue
		case !indented:
			prefix = ""
		}
		fields[prefix+key] = strings.Trim(value, "'")
	}
	return fields
}

// loadCases returns the parsed data.yaml file of each case of one operation, by case name.
func loadCases(t *testing.T, op string) map[string]map[string]string {
	files, err := filepath.Glob(filepath.Join("testdata", op, "*", "data.yaml"))
	noError(t, err)
	if len(files) == 0 {
		t.Fatalf("no %s cases", op)
	}
	cases := make(map[string]map[string]string, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		noError(t, err)
		cases[filepath.Base(filepath.Dir(f))] = parseCase(t, b)
	}
	return cases
}

func prefixedHex(t testing.TB, s string) []byte {
	t.Helper()
	mustEqual(t, true, strings.HasPrefix(s, "0x"), "missing 0x prefix: %s", s)
	b, err := hex.DecodeString(s[2:])
	noError(t, err)
	return b
}

func TestParseCase(t *testing.T) {
	got := parseCase(t, []byte("input:\n  seed: '0x01'\n  leaf_index: 7\noutput: null\n"))
	equal(t, 3, len(got))
	equal(t, "0x01", got["input.seed"])
	equal(t, "7", got["input.leaf_index"])
	equal(t, "null", got["output"])
}

func TestVectorChecksums(t *testing.T) {
	sums, err := os.ReadFile(filepath.Join("testdata", "SHA256SUMS"))
	noError(t, err)
	lines := strings.Split(strings.TrimSpace(string(sums)), "\n")
	mustEqual(t, 30, len(lines))
	for _, line := range lines {
		want, name, ok := strings.Cut(line, "  ")
		mustEqual(t, true, ok, "bad line: %s", line)
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(name)))
			noError(t, err)
			got := sha256.Sum256(b)
			equal(t, want, hex.EncodeToString(got[:]))
		})
	}
}

func TestKeyGenVectors(t *testing.T) {
	for name, c := range loadCases(t, "key_gen") {
		t.Run(name, func(t *testing.T) {
			seed := prefixedHex(t, c["input.seed"])
			if len(seed) != seedLen {
				// KeyGen takes a [32]byte, so the type rejects any other length.
				equal(t, "null", c["output"])
				return
			}
			_, pk := KeyGen([seedLen]byte(seed))
			equalBytes(t, prefixedHex(t, c["output"]), pk[:])
		})
	}
}

func TestSignVectors(t *testing.T) {
	for name, c := range loadCases(t, "sign") {
		t.Run(name, func(t *testing.T) {
			seed := prefixedHex(t, c["input.seed"])
			msg := prefixedHex(t, c["input.message"])
			if len(seed) != seedLen || len(msg) != messageLen {
				// Sign takes a [32]byte seed and message, so the type rejects any other length.
				equal(t, "null", c["output"])
				return
			}
			idx, err := strconv.ParseUint(c["input.leaf_index"], 10, 32)
			noError(t, err)
			sig, err := Sign(SecretKey{Seed: [seedLen]byte(seed)}, uint32(idx), [messageLen]byte(msg))
			noError(t, err)
			equalBytes(t, prefixedHex(t, c["output"]), sig)
		})
	}
}

func TestVerifyVectors(t *testing.T) {
	for name, c := range loadCases(t, "verify") {
		t.Run(name, func(t *testing.T) {
			pk := prefixedHex(t, c["input.public_key"])
			msg := prefixedHex(t, c["input.message"])
			sig := prefixedHex(t, c["input.signature"])
			if len(pk) != pubKeySize || len(msg) != messageLen {
				// Verify takes a [32]byte key and message, so the type rejects any other length.
				equal(t, "null", c["output"])
				return
			}
			got := Verify([pubKeySize]byte(pk), [messageLen]byte(msg), sig)
			if c["output"] == "null" {
				// A rejected input is an ordinary false.
				mustEqual(t, true, len(sig) != sigSize)
				equal(t, false, got)
				return
			}
			want, err := strconv.ParseBool(c["output"])
			noError(t, err)
			equal(t, want, got)
		})
	}
}
