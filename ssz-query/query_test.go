package sszquery_test

import (
	"fmt"
	"testing"

	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	sszquery "github.com/OffchainLabs/prysm/v6/ssz-query"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ssz "github.com/prysmaticlabs/fastssz"
)

func TestPreCalculateSSZInfo(t *testing.T) {
	info, err := sszquery.PreCalculateSSZInfo(&ethpb.IndexedAttestationElectra{})
	if err != nil {
		t.Fatalf("PreCalculateSSZInfo failed: %v", err)
	}

	assert.NotNil(t, info, "Expected non-nil SSZ info")
	assert.Equal(t, uint64(228), info.FixedSize(), "Expected fixed size to be 228")
}

func TestCalculateOffset(t *testing.T) {
	// Target path: .data.target.root
	path, err := sszquery.ParsePath(".data.target.root")
	require.NoError(t, err, "ParsePath should not return an error")

	info, err := sszquery.PreCalculateSSZInfo(&ethpb.IndexedAttestationElectra{})
	require.NoError(t, err, "PreCalculateSSZInfo should not return an error")

	_, offset, length, err := sszquery.CalculateOffsetAndLength(info, path)
	if err != nil {
		t.Fatalf("ResolvePath failed: %v", err)
	}

	assert.Equal(t, uint64(100), offset, "Expected offset to be 100")
	assert.Equal(t, uint64(32), length, "Expected length to be 32")
}

func TestRoundTripSszInfo(t *testing.T) {
	// Start with a pointer to empty object and calculate SSZ info of `IndexedAttestationElectra`.
	info, err := sszquery.PreCalculateSSZInfo(&ethpb.IndexedAttestationElectra{})
	require.NoError(t, err)

	// Print the SSZ info for debugging.
	println(info.Print())

	// Construct IndexedAttestationElectra with dummy data.
	dummyRoot, err := hexutil.Decode("0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2")
	require.NoError(t, err)
	dummySignature, err := hexutil.Decode("0xc3a2f7d9e4a1b0c8d5e6f1a0b3c7d0e9f8a7b6c5d4e3f2a1b9c8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1a0b9c8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e28b7c6d5e4f3a2b1c0d9e8f7a6b5c")
	require.NoError(t, err)
	expectedTargetRoot, err := hexutil.Decode("0x4242424242424242424242424242424242424242424242424242424242424242")
	require.NoError(t, err)

	indexedAtt := &ethpb.IndexedAttestationElectra{
		AttestingIndices: []uint64{1, 2, 3},
		Data: &ethpb.AttestationData{
			Slot:            4,
			CommitteeIndex:  5,
			BeaconBlockRoot: dummyRoot,
			Source: &ethpb.Checkpoint{
				Epoch: 7,
				Root:  dummyRoot,
			},
			Target: &ethpb.Checkpoint{
				Epoch: 9,
				Root:  expectedTargetRoot,
			},
		},
		Signature: dummySignature,
	}
	marshalledIndexedAtt, err := indexedAtt.MarshalSSZ()
	require.NoError(t, err)

	tests := []struct {
		path     string
		expected any
	}{
		// Test paths for the fixed size types.
		{
			path:     ".data.target.root",
			expected: indexedAtt.Data.Target.Root,
		},
		{
			path:     ".data.target",
			expected: indexedAtt.Data.Target,
		},
		{
			path:     ".data",
			expected: indexedAtt.Data,
		},
		{
			path:     ".signature",
			expected: indexedAtt.Signature,
		},

		// Test paths for the variable size types.
		{
			path:     ".attesting_indices",
			expected: indexedAtt.AttestingIndices,
		},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			path, err := sszquery.ParsePath(test.path)
			require.NoError(t, err)

			info, offset, length, err := sszquery.CalculateOffsetAndLength(info, path)
			require.NoError(t, err)

			expectedRawBytes := marshalledIndexedAtt[offset : offset+length]
			if len(expectedRawBytes) != int(length) {
				t.Fatalf("Extracted target value length mismatch: got %d, want %d", len(expectedRawBytes), length)
			}

			// Compare bytes
			rawBytes, err := marshalAny(test.expected)
			require.NoError(t, err, "Marshalling expected value should not return an error")
			assert.DeepEqual(t, expectedRawBytes, rawBytes, "Extracted target value should match expected")

			// Compare unmarshalled value
			unmarshalledValue, err := info.UnmarshalFromSSZ(expectedRawBytes)
			require.NoError(t, err, "Unmarshalling extracted value should not return an error")
			assert.DeepEqual(t, test.expected, unmarshalledValue, "Unmarshalled value should match expected")
		})

	}
}

func marshalAny(value any) ([]byte, error) {
	switch v := value.(type) {
	case ssz.Marshaler:
		return v.MarshalSSZ()
	case []byte:
		return v, nil
	case []uint64:
		buf := make([]byte, len(v)*8)
		for i, val := range v {
			buf = ssz.MarshalUint64(buf[i*8:], val)
		}
		return buf, nil
	default:
		return nil, fmt.Errorf("unsupported type for SSZ marshalling: %T", value)
	}
}
