package transition_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// TestProcessOperations_PQKeyRegistrations covers the EIP-8450 additions to process_operations:
// the length assert and the registrations, which run after the other operations.
func TestProcessOperations_PQKeyRegistrations(t *testing.T) {
	gloasSt, keys := util.DeterministicGenesisStateGloas(t, 64)
	st, err := heze.UpgradeToHeze(gloasSt)
	require.NoError(t, err)
	far := params.BeaconConfig().FarFutureEpoch

	pk, proof := util.PQPossessionProof(t, st, keys[0].PublicKey().Marshal(), [32]byte{1}, 0)
	valid := util.SignPQKeyRegistration(t, st, keys[0], &ethpb.PQKeyRegistration{
		Index:               0,
		KeyMaterial:         &ethpb.PQKeyMaterial{XmssPublicKey: pk[:], VrfRoot: make([]byte, 32)},
		PreviousChangeEpoch: far,
	}, proof)

	process := func(t *testing.T, regs []*ethpb.SignedPQKeyRegistration, exits []*ethpb.SignedVoluntaryExit) error {
		body := util.HydrateBeaconBlockBodyHeze(nil)
		body.PqKeyRegistrations = regs
		body.VoluntaryExits = exits
		signed, err := blocks.NewSignedBeaconBlock(util.HydrateSignedBeaconBlockHeze(&ethpb.SignedBeaconBlockHeze{
			Block: &ethpb.BeaconBlockHeze{Slot: st.Slot(), Body: body},
		}))
		require.NoError(t, err)
		_, err = transition.ProcessOperationsNoVerifyAttsSigs(t.Context(), st.Copy(), signed.Block(), st.Slot())
		return err
	}

	t.Run("applies a valid registration", func(t *testing.T) {
		require.NoError(t, process(t, []*ethpb.SignedPQKeyRegistration{valid}, nil))
	})

	t.Run("rejects more than MAX_PQ_KEY_REGISTRATIONS", func(t *testing.T) {
		regs := make([]*ethpb.SignedPQKeyRegistration, params.BeaconConfig().MaxPQKeyRegistrations+1)
		for i := range regs {
			regs[i] = valid
		}
		require.ErrorContains(t, "number of pq key registrations (9)", process(t, regs, nil))
	})

	t.Run("rejects a second registration of one key in the block", func(t *testing.T) {
		second := util.SignPQKeyRegistration(t, st, keys[1], &ethpb.PQKeyRegistration{
			Index: 1, KeyMaterial: valid.Message.KeyMaterial, PreviousChangeEpoch: far,
		}, proof)
		err := process(t, []*ethpb.SignedPQKeyRegistration{valid, second}, nil)
		require.ErrorIs(t, err, transition.ErrProcessPQKeyRegistrationsFailed)
		require.ErrorContains(t, "already registered", err)
	})

	t.Run("rejects a registration after an exit earlier in the block", func(t *testing.T) {
		// Exits need SHARD_COMMITTEE_PERIOD epochs of activity; check the ordering with a state that allows them.
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.ShardCommitteePeriod = 0
		params.OverrideBeaconConfig(cfg)
		exit := &ethpb.VoluntaryExit{Epoch: 0, ValidatorIndex: primitives.ValidatorIndex(0)}
		// EIP-7044: exits are signed for the Capella fork version.
		domain, err := signing.ComputeDomain(params.BeaconConfig().DomainVoluntaryExit, params.BeaconConfig().CapellaForkVersion, st.GenesisValidatorsRoot())
		require.NoError(t, err)
		root, err := signing.ComputeSigningRoot(exit, domain)
		require.NoError(t, err)
		signedExit := &ethpb.SignedVoluntaryExit{Exit: exit, Signature: keys[0].Sign(root[:]).Marshal()}
		require.NoError(t, process(t, nil, []*ethpb.SignedVoluntaryExit{signedExit}), "the exit alone is valid")
		err = process(t, []*ethpb.SignedPQKeyRegistration{valid}, []*ethpb.SignedVoluntaryExit{signedExit})
		require.ErrorIs(t, err, transition.ErrProcessPQKeyRegistrationsFailed)
		require.ErrorContains(t, "has initiated an exit", err)
	})
}
