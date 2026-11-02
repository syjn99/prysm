package hdiff

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestHezeDiff(t *testing.T) {
	gloasSource, err := gloasState(t, 64)
	require.NoError(t, err)
	hezeSource, err := heze.UpgradeToHeze(gloasSource.Copy())
	require.NoError(t, err)
	require.Equal(t, version.Heze, hezeSource.Version())

	t.Run("round trip keeps inclusion list bits", func(t *testing.T) {
		target := hezeSource.Copy()
		require.NoError(t, target.SetSlot(hezeSource.Slot()+1))
		bid := &ethpb.ExecutionPayloadBidHeze{
			ParentBlockHash:       make([]byte, 32),
			ParentBlockRoot:       make([]byte, 32),
			BlockHash:             make([]byte, 32),
			PrevRandao:            make([]byte, 32),
			FeeRecipient:          make([]byte, 20),
			ExecutionRequestsRoot: make([]byte, 32),
			InclusionListBits:     []byte{0x03, 0x80},
		}
		wrapped, err := blocks.WrappedROExecutionPayloadBidHeze(bid)
		require.NoError(t, err)
		require.NoError(t, target.SetExecutionPayloadBid(wrapped))

		hdiffBytes, err := Diff(hezeSource, target)
		require.NoError(t, err)
		result, err := ApplyDiff(t.Context(), hezeSource.Copy(), hdiffBytes)
		require.NoError(t, err)
		requireEqualState(t, target, result)

		got, err := result.LatestExecutionPayloadBid()
		require.NoError(t, err)
		require.DeepEqual(t, bitfield.Bitvector16{0x03, 0x80}, got.InclusionListBits())
	})

	t.Run("round trip keeps post-quantum key entries", func(t *testing.T) {
		target := hezeSource.Copy()
		key := &ethpb.PQKeyMaterial{XmssPublicKey: bytes.Repeat([]byte{7}, 32), VrfRoot: bytes.Repeat([]byte{8}, 32)}
		require.NoError(t, target.SetPQKeyEntry(3, key, 12))
		// An appended validator entry, as add_validator_to_registry writes it.
		n := primitives.ValidatorIndex(target.NumValidators())
		require.NoError(t, target.AppendValidator(&ethpb.Validator{PublicKey: bytes.Repeat([]byte{9}, 48), WithdrawalCredentials: make([]byte, 32)}))
		require.NoError(t, target.AppendBalance(1))
		require.NoError(t, target.AppendInactivityScore(0))
		require.NoError(t, target.AppendPreviousParticipationBits(0))
		require.NoError(t, target.AppendCurrentParticipationBits(0))
		require.NoError(t, target.SetPQKeyEntry(n, key, 13))

		hdiffBytes, err := Diff(hezeSource, target)
		require.NoError(t, err)
		result, err := ApplyDiff(t.Context(), hezeSource.Copy(), hdiffBytes)
		require.NoError(t, err)
		requireEqualState(t, target, result)
		got, epoch, err := result.PQKeyEntry(n)
		require.NoError(t, err)
		require.DeepEqual(t, key.XmssPublicKey, got.XmssPublicKey)
		require.Equal(t, primitives.Epoch(13), epoch)
	})

	t.Run("cross fork from Gloas", func(t *testing.T) {
		hdiffBytes, err := Diff(gloasSource, hezeSource)
		require.NoError(t, err)
		result, err := ApplyDiff(t.Context(), gloasSource.Copy(), hdiffBytes)
		require.NoError(t, err)
		require.Equal(t, version.Heze, result.Version())
		requireEqualState(t, hezeSource, result)
	})
}
