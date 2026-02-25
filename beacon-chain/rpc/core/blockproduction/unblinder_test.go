package blockproduction

import (
	"testing"

	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestUnblinder_UnblindBlobSidecars_InvalidBundle(t *testing.T) {
	wBlock, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockDeneb{
		Block: &ethpb.BeaconBlockDeneb{
			Body: &ethpb.BeaconBlockBodyDeneb{},
		},
		Signature: nil,
	})
	assert.NoError(t, err)
	_, err = UnblindBlobsSidecars(wBlock, nil)
	assert.NoError(t, err)

	wBlock, err = consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockDeneb{
		Block: &ethpb.BeaconBlockDeneb{
			Body: &ethpb.BeaconBlockBodyDeneb{
				BlobKzgCommitments: [][]byte{[]byte("a"), []byte("b")},
			},
		},
		Signature: nil,
	})
	assert.NoError(t, err)
	_, err = UnblindBlobsSidecars(wBlock, nil)
	assert.ErrorContains(t, "no valid bundle provided", err)
}

func TestUnblindBlobsSidecars_WithBlobsBundler(t *testing.T) {
	t.Run("Interface compatibility with BlobsBundle", func(t *testing.T) {
		wBlock, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{
			Block: &ethpb.BeaconBlockCapella{
				Body: &ethpb.BeaconBlockBodyCapella{},
			},
			Signature: nil,
		})
		require.NoError(t, err)

		bundle := &enginev1.BlobsBundle{
			KzgCommitments: [][]byte{make([]byte, 48)},
			Proofs:         [][]byte{make([]byte, 48)},
			Blobs:          [][]byte{make([]byte, 131072)},
		}

		sidecars, err := UnblindBlobsSidecars(wBlock, bundle)
		require.NoError(t, err)
		assert.Equal(t, true, sidecars == nil)
	})

	t.Run("Interface compatibility with BlobsBundleV2", func(t *testing.T) {
		wBlock, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{
			Block: &ethpb.BeaconBlockCapella{
				Body: &ethpb.BeaconBlockBodyCapella{},
			},
			Signature: nil,
		})
		require.NoError(t, err)

		bundleV2 := &enginev1.BlobsBundleV2{
			KzgCommitments: [][]byte{make([]byte, 48)},
			Proofs:         [][]byte{make([]byte, 48)},
			Blobs:          [][]byte{make([]byte, 131072)},
		}

		sidecars, err := UnblindBlobsSidecars(wBlock, bundleV2)
		require.NoError(t, err)
		assert.Equal(t, true, sidecars == nil)
	})

	t.Run("Function signature accepts BlobsBundler interface", func(t *testing.T) {
		wBlock, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{
			Block: &ethpb.BeaconBlockCapella{
				Body: &ethpb.BeaconBlockBodyCapella{},
			},
			Signature: nil,
		})
		require.NoError(t, err)

		var regularBundle enginev1.BlobsBundler = &enginev1.BlobsBundle{
			KzgCommitments: [][]byte{make([]byte, 48)},
			Proofs:         [][]byte{make([]byte, 48)},
			Blobs:          [][]byte{make([]byte, 131072)},
		}
		_, err = UnblindBlobsSidecars(wBlock, regularBundle)
		require.NoError(t, err)

		var bundleV2 enginev1.BlobsBundler = &enginev1.BlobsBundleV2{
			KzgCommitments: [][]byte{make([]byte, 48)},
			Proofs:         [][]byte{make([]byte, 48)},
			Blobs:          [][]byte{make([]byte, 131072)},
		}
		_, err = UnblindBlobsSidecars(wBlock, bundleV2)
		require.NoError(t, err)
	})
}

func TestUnblindBlobsSidecars_PreDenebBlock(t *testing.T) {
	wBlock, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{
		Block: &ethpb.BeaconBlockCapella{
			Body: &ethpb.BeaconBlockBodyCapella{},
		},
		Signature: nil,
	})
	require.NoError(t, err)

	bundle := &enginev1.BlobsBundle{
		KzgCommitments: [][]byte{make([]byte, 48)},
		Proofs:         [][]byte{make([]byte, 48)},
		Blobs:          [][]byte{make([]byte, 131072)},
	}

	sidecars, err := UnblindBlobsSidecars(wBlock, bundle)
	require.NoError(t, err)
	assert.Equal(t, true, sidecars == nil)

	bundleV2 := &enginev1.BlobsBundleV2{
		KzgCommitments: [][]byte{make([]byte, 48)},
		Proofs:         [][]byte{make([]byte, 48)},
		Blobs:          [][]byte{make([]byte, 131072)},
	}

	sidecars, err = UnblindBlobsSidecars(wBlock, bundleV2)
	require.NoError(t, err)
	assert.Equal(t, true, sidecars == nil)
}
