package heze_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/xmss"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

const (
	pqTestEpoch          = primitives.Epoch(10)
	pqTestFinalizedEpoch = primitives.Epoch(5)
)

// pqFixture is a Heze state with validators and builders whose BLS keys the test holds.
type pqFixture struct {
	st         state.BeaconState
	valKeys    []bls.SecretKey
	builderKey bls.SecretKey
}

// newPQFixture returns a Heze state at epoch 10 with 64 validators and 3 builders:
// builder 0 is active, builder 1 is not finalized yet, builder 2 has initiated an exit.
func newPQFixture(t *testing.T) *pqFixture {
	gloasSt, valKeys := util.DeterministicGenesisStateGloas(t, 64)
	builderKey, err := bls.RandKey()
	require.NoError(t, err)
	builderPub := builderKey.PublicKey().Marshal()
	newBuilder := func(depositEpoch, withdrawableEpoch primitives.Epoch) *ethpb.Builder {
		return &ethpb.Builder{
			Pubkey:            builderPub,
			Version:           []byte{0},
			ExecutionAddress:  make([]byte, 20),
			Balance:           1,
			DepositEpoch:      depositEpoch,
			WithdrawableEpoch: withdrawableEpoch,
		}
	}
	far := params.BeaconConfig().FarFutureEpoch
	require.NoError(t, gloasSt.SetBuilders([]*ethpb.Builder{
		newBuilder(1, far),
		newBuilder(pqTestFinalizedEpoch, far),
		newBuilder(1, 20),
	}))
	slot, err := params.BeaconConfig().SlotsPerEpoch.SafeMul(uint64(pqTestEpoch))
	require.NoError(t, err)
	require.NoError(t, gloasSt.SetSlot(slot))
	require.NoError(t, gloasSt.SetFinalizedCheckpoint(&ethpb.Checkpoint{Epoch: pqTestFinalizedEpoch, Root: make([]byte, 32)}))
	st, err := heze.UpgradeToHeze(gloasSt)
	require.NoError(t, err)
	return &pqFixture{st: st, valKeys: valKeys, builderKey: builderKey}
}

// register builds a registration of the leanXMSS key of seed for index, signed by sk and proven at leaf.
func register(
	t *testing.T,
	st state.ReadOnlyBeaconState,
	index primitives.ValidatorIndex,
	sk bls.SecretKey,
	seed byte,
	vrfRoot []byte,
	previous primitives.Epoch,
	leaf uint32,
) *ethpb.SignedPQKeyRegistration {
	xpk, possession := util.PQPossessionProof(t, st, sk.PublicKey().Marshal(), [32]byte{seed}, leaf)
	return signRegistration(t, st, sk, &ethpb.PQKeyRegistration{
		Index:               index,
		KeyMaterial:         &ethpb.PQKeyMaterial{XmssPublicKey: xpk[:], VrfRoot: vrfRoot},
		PreviousChangeEpoch: previous,
	}, possession)
}

func signRegistration(t *testing.T, st state.ReadOnlyBeaconState, sk bls.SecretKey, msg *ethpb.PQKeyRegistration, possession []byte) *ethpb.SignedPQKeyRegistration {
	return util.SignPQKeyRegistration(t, st, sk, msg, possession)
}

func vrf(b byte) []byte {
	r := make([]byte, 32)
	r[0] = b
	return r
}

func TestProcessPQKeyRegistration(t *testing.T) {
	f := newPQFixture(t)
	far := params.BeaconConfig().FarFutureEpoch
	builder0 := primitives.BuilderIndex(0).ToValidatorIndex()

	_, xpkA := xmss.KeyGen([32]byte{0xa})
	_, xpkB := xmss.KeyGen([32]byte{0xb})
	_, xpkC := xmss.KeyGen([32]byte{0xc})

	// Valid registrations, signed once and reused.
	regA := register(t, f.st, 0, f.valKeys[0], 0xa, vrf(1), far, 0)
	regC := register(t, f.st, builder0, f.builderKey, 0xc, make([]byte, 32), 1, 0)

	t.Run("registers keys for an unregistered validator", func(t *testing.T) {
		st := f.st.Copy()
		require.NoError(t, heze.ProcessPQKeyRegistration(st, regA))
		key, epoch, err := st.PQKeyEntry(0)
		require.NoError(t, err)
		require.DeepEqual(t, xpkA[:], key.XmssPublicKey)
		require.DeepEqual(t, vrf(1), key.VrfRoot)
		require.Equal(t, pqTestEpoch, epoch)
		inUse, err := st.IsPQPublicKeyInUse(xpkA)
		require.NoError(t, err)
		require.Equal(t, true, inUse)

		_, epoch, err = f.st.PQKeyEntry(0)
		require.NoError(t, err)
		require.Equal(t, far, epoch, "the copied-from state must not change")
	})

	t.Run("changes keys once MIN_PQ_KEY_CHANGE_DELAY has passed, keeping the VRF root", func(t *testing.T) {
		st := f.st.Copy()
		require.NoError(t, heze.ProcessPQKeyRegistration(st, regA))
		change := register(t, st, 0, f.valKeys[0], 0xb, vrf(1), pqTestEpoch, 0)

		early := st.Copy()
		require.NoError(t, early.SetSlot(early.Slot()+params.BeaconConfig().SlotsPerEpoch))
		require.ErrorContains(t, "cannot change again before epoch", heze.ProcessPQKeyRegistration(early, change))

		changeEpoch := pqTestEpoch + params.BeaconConfig().MinPQKeyChangeDelay
		slot, err := params.BeaconConfig().SlotsPerEpoch.SafeMul(uint64(changeEpoch))
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(slot))
		require.NoError(t, heze.ProcessPQKeyRegistration(st, change))
		key, epoch, err := st.PQKeyEntry(0)
		require.NoError(t, err)
		require.DeepEqual(t, xpkB[:], key.XmssPublicKey)
		require.DeepEqual(t, vrf(1), key.VrfRoot)
		require.Equal(t, changeEpoch, epoch)

		// The released key is no longer in use.
		inUse, err := st.IsPQPublicKeyInUse(xpkA)
		require.NoError(t, err)
		require.Equal(t, false, inUse)
	})

	t.Run("rejects a replayed registration", func(t *testing.T) {
		st := f.st.Copy()
		require.NoError(t, heze.ProcessPQKeyRegistration(st, regA))
		require.ErrorContains(t, "does not match stored change epoch", heze.ProcessPQKeyRegistration(st, regA))
	})

	t.Run("registers an active builder with a zero VRF root", func(t *testing.T) {
		st := f.st.Copy()
		require.NoError(t, heze.ProcessPQKeyRegistration(st, regC))
		key, epoch, err := st.PQKeyEntry(builder0)
		require.NoError(t, err)
		require.DeepEqual(t, xpkC[:], key.XmssPublicKey)
		require.Equal(t, pqTestEpoch, epoch)
		_, epoch, err = st.PQKeyEntry(0)
		require.NoError(t, err)
		require.Equal(t, far, epoch, "the validator registry must not change")
	})

	t.Run("rejects a builder registration that names FAR_FUTURE_EPOCH instead of the deposit epoch", func(t *testing.T) {
		r := signRegistration(t, f.st, f.builderKey, &ethpb.PQKeyRegistration{
			Index: builder0, KeyMaterial: regC.Message.KeyMaterial, PreviousChangeEpoch: far,
		}, regC.Possession)
		require.ErrorContains(t, "does not match stored change epoch", heze.ProcessPQKeyRegistration(f.st.Copy(), r))
	})

	t.Run("rejects a key registered for another index", func(t *testing.T) {
		st := f.st.Copy()
		require.NoError(t, heze.ProcessPQKeyRegistration(st, regA))
		// Validator 1 and builder 0 both try the key of validator 0.
		r := signRegistration(t, st, f.valKeys[1], &ethpb.PQKeyRegistration{
			Index: 1, KeyMaterial: regA.Message.KeyMaterial, PreviousChangeEpoch: far,
		}, regA.Possession)
		require.ErrorContains(t, "already registered", heze.ProcessPQKeyRegistration(st, r))
		r = signRegistration(t, st, f.builderKey, &ethpb.PQKeyRegistration{
			Index: builder0, KeyMaterial: &ethpb.PQKeyMaterial{XmssPublicKey: xpkA[:], VrfRoot: make([]byte, 32)}, PreviousChangeEpoch: 1,
		}, regA.Possession)
		require.ErrorContains(t, "already registered", heze.ProcessPQKeyRegistration(st, r))
	})

	t.Run("rejects a change to the registrant's own key", func(t *testing.T) {
		st := f.st.Copy()
		require.NoError(t, heze.ProcessPQKeyRegistration(st, regA))
		slot, err := params.BeaconConfig().SlotsPerEpoch.SafeMul(uint64(pqTestEpoch + params.BeaconConfig().MinPQKeyChangeDelay))
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(slot))
		r := signRegistration(t, st, f.valKeys[0], &ethpb.PQKeyRegistration{
			Index: 0, KeyMaterial: &ethpb.PQKeyMaterial{XmssPublicKey: xpkA[:], VrfRoot: vrf(2)}, PreviousChangeEpoch: pqTestEpoch,
		}, regA.Possession)
		require.ErrorContains(t, "already registered", heze.ProcessPQKeyRegistration(st, r))
	})

	t.Run("an unregistered all-zero entry is not in use for another key", func(t *testing.T) {
		inUse, err := f.st.IsPQPublicKeyInUse(xpkB)
		require.NoError(t, err)
		require.Equal(t, false, inUse)
	})

	// Each case fails at its own check; the expected error names that check.
	rejections := []struct {
		name string
		reg  func(t *testing.T, st state.BeaconState) *ethpb.SignedPQKeyRegistration
		want string
	}{
		{
			name: "nil registration",
			reg: func(*testing.T, state.BeaconState) *ethpb.SignedPQKeyRegistration {
				return &ethpb.SignedPQKeyRegistration{}
			},
			want: "nil pq key registration",
		},
		{
			name: "validator index outside the registry",
			reg: func(t *testing.T, st state.BeaconState) *ethpb.SignedPQKeyRegistration {
				return signRegistration(t, st, f.valKeys[0], &ethpb.PQKeyRegistration{Index: 64, KeyMaterial: regA.Message.KeyMaterial, PreviousChangeEpoch: far}, regA.Possession)
			},
			want: "out of range",
		},
		{
			name: "validator that has initiated an exit",
			reg: func(t *testing.T, st state.BeaconState) *ethpb.SignedPQKeyRegistration {
				v, err := st.ValidatorAtIndex(0)
				require.NoError(t, err)
				v.ExitEpoch = 50
				require.NoError(t, st.UpdateValidatorAtIndex(0, v))
				return regA
			},
			want: "has initiated an exit",
		},
		{
			name: "builder index outside the builder registry",
			reg: func(t *testing.T, st state.BeaconState) *ethpb.SignedPQKeyRegistration {
				return signRegistration(t, st, f.builderKey, &ethpb.PQKeyRegistration{Index: primitives.BuilderIndex(3).ToValidatorIndex(), KeyMaterial: regC.Message.KeyMaterial, PreviousChangeEpoch: 1}, regC.Possession)
			},
			want: "out of bounds",
		},
		{
			name: "builder that is not yet active",
			reg: func(t *testing.T, st state.BeaconState) *ethpb.SignedPQKeyRegistration {
				return signRegistration(t, st, f.builderKey, &ethpb.PQKeyRegistration{Index: primitives.BuilderIndex(1).ToValidatorIndex(), KeyMaterial: regC.Message.KeyMaterial, PreviousChangeEpoch: pqTestFinalizedEpoch}, regC.Possession)
			},
			want: "is not active",
		},
		{
			name: "builder that has initiated an exit",
			reg: func(t *testing.T, st state.BeaconState) *ethpb.SignedPQKeyRegistration {
				return signRegistration(t, st, f.builderKey, &ethpb.PQKeyRegistration{Index: primitives.BuilderIndex(2).ToValidatorIndex(), KeyMaterial: regC.Message.KeyMaterial, PreviousChangeEpoch: 1}, regC.Possession)
			},
			want: "is not active",
		},
		{
			name: "builder with a nonzero VRF root",
			reg: func(t *testing.T, st state.BeaconState) *ethpb.SignedPQKeyRegistration {
				return signRegistration(t, st, f.builderKey, &ethpb.PQKeyRegistration{Index: builder0, KeyMaterial: &ethpb.PQKeyMaterial{XmssPublicKey: xpkC[:], VrfRoot: vrf(1)}, PreviousChangeEpoch: 1}, regC.Possession)
			},
			want: "nonzero VRF root",
		},
		{
			name: "BLS signature by another key",
			reg: func(t *testing.T, st state.BeaconState) *ethpb.SignedPQKeyRegistration {
				return signRegistration(t, st, f.valKeys[1], regA.Message, regA.Possession)
			},
			want: "signature did not verify",
		},
		{
			name: "all-zero leanXMSS key",
			reg: func(t *testing.T, st state.BeaconState) *ethpb.SignedPQKeyRegistration {
				return signRegistration(t, st, f.valKeys[0], &ethpb.PQKeyRegistration{Index: 0, KeyMaterial: &ethpb.PQKeyMaterial{XmssPublicKey: make([]byte, 32), VrfRoot: vrf(1)}, PreviousChangeEpoch: far}, regA.Possession)
			},
			want: "zero leanXMSS public key",
		},
		{
			name: "tampered proof of possession",
			reg: func(t *testing.T, st state.BeaconState) *ethpb.SignedPQKeyRegistration {
				possession := append([]byte{}, regA.Possession...)
				possession[len(possession)-1] ^= 1
				return signRegistration(t, st, f.valKeys[0], regA.Message, possession)
			},
			want: "invalid proof of possession",
		},
		{
			name: "proof of possession for another BLS public key",
			reg: func(t *testing.T, st state.BeaconState) *ethpb.SignedPQKeyRegistration {
				// regA proves possession for validator 0's BLS key, not validator 1's.
				return signRegistration(t, st, f.valKeys[1], &ethpb.PQKeyRegistration{Index: 1, KeyMaterial: regA.Message.KeyMaterial, PreviousChangeEpoch: far}, regA.Possession)
			},
			want: "invalid proof of possession",
		},
		{
			name: "proof of possession at a leaf index other than 0",
			reg: func(t *testing.T, st state.BeaconState) *ethpb.SignedPQKeyRegistration {
				return register(t, st, 0, f.valKeys[0], 0xa, vrf(1), far, 1)
			},
			want: "invalid proof of possession",
		},
	}
	for _, tt := range rejections {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			st := f.st.Copy()
			require.ErrorContains(t, tt.want, heze.ProcessPQKeyRegistration(st, tt.reg(t, st)))
		})
	}
}

func TestUpgradeToHeze_PQKeyRegistry(t *testing.T) {
	f := newPQFixture(t)
	far := params.BeaconConfig().FarFutureEpoch
	container, err := state_native.ContainerFrom[*ethpb.BeaconStateHeze](f.st.ToContainer())
	require.NoError(t, err)

	require.Equal(t, len(container.Validators), len(container.PostQuantumKeyMaterial))
	require.Equal(t, len(container.Validators), len(container.PostQuantumKeyChangeEpochs))
	require.Equal(t, len(container.Builders), len(container.BuilderPostQuantumKeyMaterial))
	require.Equal(t, len(container.Builders), len(container.BuilderPostQuantumKeyChangeEpochs))
	for i := range container.Validators {
		require.DeepEqual(t, make([]byte, 32), container.PostQuantumKeyMaterial[i].XmssPublicKey)
		require.Equal(t, far, container.PostQuantumKeyChangeEpochs[i])
	}
	for i, b := range container.Builders {
		require.DeepEqual(t, make([]byte, 32), container.BuilderPostQuantumKeyMaterial[i].XmssPublicKey)
		require.Equal(t, b.DepositEpoch, container.BuilderPostQuantumKeyChangeEpochs[i])
	}

	// The native state root matches the SSZ root of its container.
	want, err := container.HashTreeRoot()
	require.NoError(t, err)
	got, err := f.st.HashTreeRoot(t.Context())
	require.NoError(t, err)
	require.Equal(t, want, got)
}
