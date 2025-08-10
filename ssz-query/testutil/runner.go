package testutil

import (
	"testing"

	sszquery "github.com/OffchainLabs/prysm/v6/ssz-query"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	ssz "github.com/ferranbt/fastssz"
)

func RunStructTest(t *testing.T, spec TestSpec) {
	t.Run(spec.Name, func(t *testing.T) {
		testInstance := spec.Instance

		info, err := sszquery.PreCalculateSSZInfo(testInstance)
		require.NoError(t, err)

		println("Before populating", info.Print())

		err = sszquery.PopulateFromValue(info, testInstance)
		require.NoError(t, err, "PopulateFromValue should not return an error")

		println("After populating", info.Print())

		marshaller, ok := testInstance.(ssz.Marshaler)
		if !ok {
			t.Fatalf("Test instance must implement ssz.Marshaler, got %T", testInstance)
		}

		marshalledData, err := marshaller.MarshalSSZ()
		require.NoError(t, err)

		for _, pathTest := range spec.PathTests {
			t.Run(pathTest.Path, func(t *testing.T) {
				path, err := sszquery.ParsePath(pathTest.Path)
				require.NoError(t, err)

				fieldInfo, offset, length, err := sszquery.CalculateOffsetAndLength(info, path)
				require.NoError(t, err)

				expectedRawBytes := marshalledData[offset : offset+length]
				if len(expectedRawBytes) != int(length) {
					t.Fatalf("Extracted value length mismatch: got %d, want %d", len(expectedRawBytes), length)
				}

				rawBytes, err := marshalAny(pathTest.Expected)
				require.NoError(t, err, "Marshalling expected value should not return an error")
				assert.DeepEqual(t, expectedRawBytes, rawBytes, "Extracted value should match expected")

				unmarshalledValue, err := fieldInfo.UnmarshalFromSSZ(expectedRawBytes)
				require.NoError(t, err, "Unmarshalling extracted value should not return an error")
				assert.DeepEqual(t, pathTest.Expected, unmarshalledValue, "Unmarshalled value should match expected")
			})
		}
	})
}
