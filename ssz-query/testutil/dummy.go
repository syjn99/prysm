package testutil

import (
	"crypto/rand"
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

type DummyData struct {
	Root      []byte
	Signature []byte
}

func RandomDummyData(t *testing.T) *DummyData {
	dummyRoot := make([]byte, 32)
	_, err := rand.Read(dummyRoot)
	require.NoError(t, err)

	dummySignature := make([]byte, 96)
	_, err = rand.Read(dummySignature)
	require.NoError(t, err)

	return &DummyData{
		Root:      dummyRoot,
		Signature: dummySignature,
	}
}
