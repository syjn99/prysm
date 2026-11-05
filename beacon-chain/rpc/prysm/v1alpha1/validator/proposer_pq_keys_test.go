package validator

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/pqkeys"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestSetPQKeyRegistrations(t *testing.T) {
	gloasSt, keys := util.DeterministicGenesisStateGloas(t, 64)
	st, err := heze.UpgradeToHeze(gloasSt)
	require.NoError(t, err)
	pk, proof := util.PQPossessionProof(t, st, keys[0].PublicKey().Marshal(), [32]byte{1}, 0)
	reg := util.SignPQKeyRegistration(t, st, keys[0], &ethpb.PQKeyRegistration{
		Index:               0,
		KeyMaterial:         &ethpb.PQKeyMaterial{XmssPublicKey: pk[:], VrfRoot: make([]byte, 32)},
		PreviousChangeEpoch: params.BeaconConfig().FarFutureEpoch,
	}, proof)
	pool := pqkeys.NewPool()
	pool.InsertRegistration(reg)
	vs := &Server{PQKeyPool: pool}

	newBlock := func(t *testing.T) *blocks.SignedBeaconBlock {
		wsb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockHeze())
		require.NoError(t, err)
		return wsb.(*blocks.SignedBeaconBlock)
	}

	t.Run("packs pool registrations into a Heze block", func(t *testing.T) {
		blk := newBlock(t)
		vs.setPQKeyRegistrations(blk, st)
		got, err := blk.Block().Body().PQKeyRegistrations()
		require.NoError(t, err)
		require.Equal(t, 1, len(got))
		require.DeepSSZEqual(t, reg, got[0])
	})

	t.Run("state root retry drops registrations", func(t *testing.T) {
		blk := newBlock(t)
		vs.setPQKeyRegistrations(blk, st)
		// At the attempt limit the retry clears the field and then stops.
		ctx := context.WithValue(t.Context(), computeStateRootAttemptsKey, maxComputeStateRootAttempts)
		_, err := vs.handlePostBlockStateError(ctx, blk, transition.ErrProcessPQKeyRegistrationsFailed)
		require.ErrorContains(t, "attempted max compute state root attempts", err)
		got, err := blk.Block().Body().PQKeyRegistrations()
		require.NoError(t, err)
		require.Equal(t, 0, len(got))
	})
}
