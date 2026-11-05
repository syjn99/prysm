package state_native_test

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func pqKey(b byte) *ethpb.PQKeyMaterial {
	return &ethpb.PQKeyMaterial{XmssPublicKey: bytes.Repeat([]byte{b}, 32), VrfRoot: bytes.Repeat([]byte{b + 1}, 32)}
}

func TestPQKeyEntry(t *testing.T) {
	far := params.BeaconConfig().FarFutureEpoch
	st, err := util.NewBeaconStateHeze(func(s *ethpb.BeaconStateHeze) error {
		s.Validators = []*ethpb.Validator{{PublicKey: make([]byte, 48)}, {PublicKey: bytes.Repeat([]byte{1}, 48)}}
		s.Builders = []*ethpb.Builder{{Pubkey: make([]byte, 48), Version: []byte{0}, ExecutionAddress: make([]byte, 20), DepositEpoch: 3}}
		return nil
	})
	require.NoError(t, err)
	builder0 := primitives.BuilderIndex(0).ToValidatorIndex()

	t.Run("reads unset entries", func(t *testing.T) {
		key, epoch, err := st.PQKeyEntry(1)
		require.NoError(t, err)
		require.DeepEqual(t, make([]byte, 32), key.XmssPublicKey)
		require.Equal(t, far, epoch)
		key, epoch, err = st.PQKeyEntry(builder0)
		require.NoError(t, err)
		require.DeepEqual(t, make([]byte, 32), key.XmssPublicKey)
		require.Equal(t, primitives.Epoch(3), epoch)
	})

	t.Run("rejects an index outside the registry", func(t *testing.T) {
		_, _, err := st.PQKeyEntry(2)
		require.ErrorContains(t, "out of range", err)
		_, _, err = st.PQKeyEntry(primitives.BuilderIndex(1).ToValidatorIndex())
		require.ErrorContains(t, "out of range", err)
		require.ErrorContains(t, "out of range", st.Copy().SetPQKeyEntry(3, pqKey(1), 0))
	})

	t.Run("sets an entry without changing a copy", func(t *testing.T) {
		cp := st.Copy()
		require.NoError(t, cp.SetPQKeyEntry(1, pqKey(7), 9))
		require.NoError(t, cp.SetPQKeyEntry(builder0, pqKey(9), 11))

		key, epoch, err := cp.PQKeyEntry(1)
		require.NoError(t, err)
		require.DeepEqual(t, pqKey(7).XmssPublicKey, key.XmssPublicKey)
		require.Equal(t, primitives.Epoch(9), epoch)
		key, epoch, err = cp.PQKeyEntry(builder0)
		require.NoError(t, err)
		require.DeepEqual(t, pqKey(9).XmssPublicKey, key.XmssPublicKey)
		require.Equal(t, primitives.Epoch(11), epoch)

		_, epoch, err = st.PQKeyEntry(1)
		require.NoError(t, err)
		require.Equal(t, far, epoch)
		_, epoch, err = st.PQKeyEntry(builder0)
		require.NoError(t, err)
		require.Equal(t, primitives.Epoch(3), epoch)

		for _, k := range [][32]byte{[32]byte(pqKey(7).XmssPublicKey), [32]byte(pqKey(9).XmssPublicKey)} {
			inUse, err := cp.IsPQPublicKeyInUse(k)
			require.NoError(t, err)
			require.Equal(t, true, inUse)
			inUse, err = st.IsPQPublicKeyInUse(k)
			require.NoError(t, err)
			require.Equal(t, false, inUse)
		}
	})

	t.Run("matches the SSZ root after a change", func(t *testing.T) {
		cp := st.Copy()
		_, err := cp.HashTreeRoot(t.Context())
		require.NoError(t, err)
		require.NoError(t, cp.SetPQKeyEntry(0, pqKey(4), 5))
		require.NoError(t, cp.SetPQKeyEntry(builder0, pqKey(6), 7))
		got, err := cp.HashTreeRoot(t.Context())
		require.NoError(t, err)
		container, ok := cp.ToContainer().(*ethpb.BeaconStateHeze)
		require.Equal(t, true, ok)
		want, err := container.HashTreeRoot()
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("add_validator_to_registry appends an unset entry", func(t *testing.T) {
		cp := st.Copy()
		require.NoError(t, electra.AddValidatorToRegistry(cp, bytes.Repeat([]byte{2}, 48), make([]byte, 32), 1))
		key, epoch, err := cp.PQKeyEntry(2)
		require.NoError(t, err)
		require.DeepEqual(t, make([]byte, 32), key.XmssPublicKey)
		require.Equal(t, far, epoch)
	})

	t.Run("add_builder_to_registry resets a reused index to the new deposit epoch", func(t *testing.T) {
		cp, err := util.NewBeaconStateHeze(func(s *ethpb.BeaconStateHeze) error {
			s.Slot = 4 * params.BeaconConfig().SlotsPerEpoch
			// Builder 0 has exited and withdrawn, so its index is reusable.
			s.Builders = []*ethpb.Builder{{Pubkey: bytes.Repeat([]byte{3}, 48), Version: []byte{0}, ExecutionAddress: make([]byte, 20), DepositEpoch: 1, WithdrawableEpoch: 2}}
			return nil
		})
		require.NoError(t, err)
		require.NoError(t, cp.SetPQKeyEntry(builder0, pqKey(8), 2))

		var pubkey [48]byte
		pubkey[0] = 0x99
		require.NoError(t, cp.AddBuilderFromDeposit(pubkey, [32]byte{}, 1))
		b, err := cp.Builder(0)
		require.NoError(t, err)
		require.DeepEqual(t, pubkey[:], b.Pubkey)
		key, epoch, err := cp.PQKeyEntry(builder0)
		require.NoError(t, err)
		require.DeepEqual(t, make([]byte, 32), key.XmssPublicKey)
		require.Equal(t, primitives.Epoch(4), epoch)

		// A new index appends to both lists.
		pubkey[0] = 0x9a
		require.NoError(t, cp.AddBuilderFromDeposit(pubkey, [32]byte{}, 1))
		_, epoch, err = cp.PQKeyEntry(primitives.BuilderIndex(1).ToValidatorIndex())
		require.NoError(t, err)
		require.Equal(t, primitives.Epoch(4), epoch)
	})
}
