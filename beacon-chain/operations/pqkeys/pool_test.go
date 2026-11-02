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

	t.Run("keeps one registration per pair, the latest", func(t *testing.T) {
		p := NewPool()
		p.InsertRegistration(r1)
		p.InsertRegistration(r0)
		dup := util.SignPQKeyRegistration(t, st, keys[0], &ethpb.PQKeyRegistration{
			Index: 0, KeyMaterial: r2.Message.KeyMaterial, PreviousChangeEpoch: far,
		}, r2.Possession)
		p.InsertRegistration(dup)
		require.DeepEqual(t, []*ethpb.SignedPQKeyRegistration{dup, r1}, p.PendingRegistrations())

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
		got := p.RegistrationsForInclusion(st)
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
		got := p.RegistrationsForInclusion(st)
		require.DeepEqual(t, []*ethpb.SignedPQKeyRegistration{r0}, got)
	})

	t.Run("drops a registration whose key another index took", func(t *testing.T) {
		// One leanXMSS key K, with leaf-0 proofs for validator 0 and for validator 3.
		k0 := reg(0, 9)
		pk3, proof3 := util.PQPossessionProof(t, st, keys[3].PublicKey().Marshal(), [32]byte{9}, 0)
		k3 := util.SignPQKeyRegistration(t, st, keys[3], &ethpb.PQKeyRegistration{
			Index: 3, KeyMaterial: &ethpb.PQKeyMaterial{XmssPublicKey: pk3[:], VrfRoot: make([]byte, 32)}, PreviousChangeEpoch: far,
		}, proof3)
		require.NoError(t, heze.ValidatePQKeyRegistration(st, k3))
		applied := st.Copy()
		require.NoError(t, heze.ProcessPQKeyRegistration(applied, k0))

		p := NewPool()
		p.InsertRegistration(k3)
		require.Equal(t, 0, len(p.RegistrationsForInclusion(applied)))
		require.Equal(t, 0, len(p.PendingRegistrations()))
	})

	t.Run("drops a registration for a validator that has exited", func(t *testing.T) {
		exited := st.Copy()
		v, err := exited.ValidatorAtIndex(2)
		require.NoError(t, err)
		v.ExitEpoch = 5
		require.NoError(t, exited.UpdateValidatorAtIndex(2, v))
		p := NewPool()
		p.InsertRegistration(r2)
		require.Equal(t, 0, len(p.RegistrationsForInclusion(exited)))
		require.Equal(t, 0, len(p.PendingRegistrations()))
	})

	t.Run("keeps a change that is not valid yet", func(t *testing.T) {
		registered := st.Copy()
		require.NoError(t, heze.ProcessPQKeyRegistration(registered, r0))
		pk, proof := util.PQPossessionProof(t, st, keys[0].PublicKey().Marshal(), [32]byte{8}, 0)
		change := util.SignPQKeyRegistration(t, st, keys[0], &ethpb.PQKeyRegistration{
			Index: 0, KeyMaterial: &ethpb.PQKeyMaterial{XmssPublicKey: pk[:], VrfRoot: make([]byte, 32)}, PreviousChangeEpoch: 0,
		}, proof)
		p := NewPool()
		p.InsertRegistration(change)
		require.Equal(t, 0, len(p.RegistrationsForInclusion(registered)))
		require.Equal(t, 1, len(p.PendingRegistrations()))
	})

	t.Run("a newer valid registration replaces a stale one for the pair", func(t *testing.T) {
		stale := util.SignPQKeyRegistration(t, st, keys[2], r1.Message, r1.Possession) // signed by the wrong key
		p := NewPool()
		p.InsertRegistration(stale)
		p.InsertRegistration(r1)
		got := p.RegistrationsForInclusion(st)
		require.Equal(t, 1, len(got))
		require.DeepSSZEqual(t, r1, got[0])
	})

	t.Run("drops registrations that can never apply", func(t *testing.T) {
		p := NewPool()
		applied := st.Copy()
		require.NoError(t, heze.ProcessPQKeyRegistration(applied, r0))
		p.InsertRegistration(r0)
		p.InsertRegistration(r1)
		got := p.RegistrationsForInclusion(applied)
		require.DeepEqual(t, []*ethpb.SignedPQKeyRegistration{r1}, got)
		require.DeepEqual(t, []*ethpb.SignedPQKeyRegistration{r1}, p.PendingRegistrations())
	})
}
