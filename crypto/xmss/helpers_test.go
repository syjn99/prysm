package xmss

import (
	"bytes"
	"fmt"
	"testing"
)

// The tests use only the standard library, so the whole directory can be copied out of
// Prysm as is. These helpers stand in for Prysm's testing/assert and testing/require.

func describe(msg []any) string {
	if len(msg) == 0 {
		return ""
	}
	if format, ok := msg[0].(string); ok {
		return fmt.Sprintf(format, msg[1:]...) + ": "
	}
	return fmt.Sprint(msg...) + ": "
}

// equal reports an error when want != got, and the test goes on.
func equal[T comparable](t testing.TB, want, got T, msg ...any) {
	t.Helper()
	if want != got {
		t.Errorf("%swant %v, got %v", describe(msg), want, got)
	}
}

// mustEqual stops the test when want != got.
func mustEqual[T comparable](t testing.TB, want, got T, msg ...any) {
	t.Helper()
	if want != got {
		t.Fatalf("%swant %v, got %v", describe(msg), want, got)
	}
}

// equalBytes reports an error when two byte strings differ, and the test goes on.
func equalBytes(t testing.TB, want, got []byte, msg ...any) {
	t.Helper()
	if !bytes.Equal(want, got) {
		t.Errorf("%swant %x, got %x", describe(msg), want, got)
	}
}

// noError stops the test on a non-nil error.
func noError(t testing.TB, err error, msg ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf("%sunexpected error: %v", describe(msg), err)
	}
}
