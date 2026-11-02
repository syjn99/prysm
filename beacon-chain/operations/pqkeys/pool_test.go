package pqkeys

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestPool(t *testing.T) {
	gloasSt, keys := util.DeterministicGenesisStateGloas(t, 64)
	st, err := heze.UpgradeToHeze(gloasSt)
	require.NoError(t, err)
	far := params.BeaconConfig().FarFutureEpoch

	// reg builds a valid registration of the leanXMSS key of seed for validator index.
	reg := func(index primitives.ValidatorIndex, seed byte) *ethpb.SignedPQKeyRegistration {
		pk, proof := util.PQPossessionProof(t, st, keys[index].PublicKey().Marshal(), [32]byte{seed}, 0)
		return util.SignPQKeyRegistration(t, st, keys[index], &ethpb.PQKeyRegistration{
			Index:               index,
			KeyMaterial:         &ethpb.PQKeyMaterial{XmssPublicKey: pk[:], VrfRoot: make([]byte, 32)},
			PreviousChangeEpoch: far,
		}, proof)
	}
	r0, r1, r2 := reg(0, 1), reg(1, 2), reg(2, 3)

	t.Run("keeps the first registration of a pair", func(t *testing.T) {
		p := NewPool()
		p.InsertRegistration(r1)
		p.InsertRegistration(r0)
		dup := util.SignPQKeyRegistration(t, st, keys[0], &ethpb.PQKeyRegistration{
			Index: 0, KeyMaterial: r2.Message.KeyMaterial, PreviousChangeEpoch: far,
		}, r2.Possession)
		p.InsertRegistration(dup)
		require.DeepEqual(t, []*ethpb.SignedPQKeyRegistration{r0, r1}, p.PendingRegistrations())

		p.MarkIncluded(r0)
		require.DeepEqual(t, []*ethpb.SignedPQKeyRegistration{r1}, p.PendingRegistrations())
	})

	t.Run("packs at most MAX_PQ_KEY_REGISTRATIONS valid registrations", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.MaxPQKeyRegistrations = 2
		params.OverrideBeaconConfig(cfg)

		p := NewPool()
		for _, r := range []*ethpb.SignedPQKeyRegistration{r0, r1, r2} {
			p.InsertRegistration(r)
		}
		got, err := p.RegistrationsForInclusion(st)
		require.NoError(t, err)
		require.DeepEqual(t, []*ethpb.SignedPQKeyRegistration{r0, r1}, got)
		require.Equal(t, 3, len(p.PendingRegistrations()))

		// The head state is not changed by the simulation.
		_, epoch, err := st.PQKeyEntry(0)
		require.NoError(t, err)
		require.Equal(t, far, epoch)
	})

	t.Run("skips a second registration of one key in the same block", func(t *testing.T) {
		p := NewPool()
		p.InsertRegistration(r0)
		// Validator 3 registers the key of r0; its proof is for validator 0, but the key check comes first.
		p.InsertRegistration(util.SignPQKeyRegistration(t, st, keys[3], &ethpb.PQKeyRegistration{
			Index: 3, KeyMaterial: r0.Message.KeyMaterial, PreviousChangeEpoch: far,
		}, r0.Possession))
		got, err := p.RegistrationsForInclusion(st)
		require.NoError(t, err)
		require.DeepEqual(t, []*ethpb.SignedPQKeyRegistration{r0}, got)
	})

	t.Run("drops registrations that can never apply", func(t *testing.T) {
		p := NewPool()
		applied := st.Copy()
		require.NoError(t, heze.ProcessPQKeyRegistration(applied, r0))
		p.InsertRegistration(r0)
		p.InsertRegistration(r1)
		got, err := p.RegistrationsForInclusion(applied)
		require.NoError(t, err)
		require.DeepEqual(t, []*ethpb.SignedPQKeyRegistration{r1}, got)
		require.DeepEqual(t, []*ethpb.SignedPQKeyRegistration{r1}, p.PendingRegistrations())
	})
}
