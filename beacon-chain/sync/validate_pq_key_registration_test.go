package sync

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
)

func TestValidatePQKeyRegistration(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	const epoch = primitives.Epoch(10)
	far := params.BeaconConfig().FarFutureEpoch
	gloasSt, keys := util.DeterministicGenesisStateGloas(t, 64)
	builderKey, err := bls.RandKey()
	require.NoError(t, err)
	newBuilder := func(depositEpoch, withdrawableEpoch primitives.Epoch) *ethpb.Builder {
		return &ethpb.Builder{Pubkey: builderKey.PublicKey().Marshal(), Version: []byte{0}, ExecutionAddress: make([]byte, 20), Balance: 1, DepositEpoch: depositEpoch, WithdrawableEpoch: withdrawableEpoch}
	}
	// Builder 0 is active, builder 1 is not finalized yet, builder 2 has initiated an exit.
	require.NoError(t, gloasSt.SetBuilders([]*ethpb.Builder{newBuilder(1, far), newBuilder(5, far), newBuilder(1, 20)}))
	slot, err := params.BeaconConfig().SlotsPerEpoch.SafeMul(uint64(epoch))
	require.NoError(t, err)
	require.NoError(t, gloasSt.SetSlot(slot))
	require.NoError(t, gloasSt.SetFinalizedCheckpoint(&ethpb.Checkpoint{Epoch: 5, Root: make([]byte, 32)}))
	st, err := heze.UpgradeToHeze(gloasSt.Copy())
	require.NoError(t, err)

	// The wall clock is in the state's epoch.
	genesis := time.Now().Add(-time.Duration(uint64(slot)) * params.BeaconConfig().SlotDuration())
	newService := func(head state.ReadOnlyBeaconState) *Service {
		s := &Service{cfg: &config{
			p2p:         mockp2p.NewTestP2P(t),
			initialSync: &mockSync.Sync{IsSyncing: false},
			chain:       &mockChain.ChainService{State: head.(state.BeaconState)},
			clock:       startup.NewClock(genesis, [32]byte(st.GenesisValidatorsRoot())),
		}}
		s.seenPQKeyRegistrationCache = lruwrpr.New(10)
		return s
	}

	builder := func(i primitives.BuilderIndex) primitives.ValidatorIndex { return i.ToValidatorIndex() }
	xpkA, proofA := util.PQPossessionProof(t, st, keys[0].PublicKey().Marshal(), [32]byte{0xa}, 0)
	xpkB, proofB := util.PQPossessionProof(t, st, keys[0].PublicKey().Marshal(), [32]byte{0xb}, 0)
	material := func(xpk [32]byte, vrf byte) *ethpb.PQKeyMaterial {
		return &ethpb.PQKeyMaterial{XmssPublicKey: bytes.Clone(xpk[:]), VrfRoot: bytes.Repeat([]byte{vrf}, 32)}
	}
	sign := func(sk bls.SecretKey, index primitives.ValidatorIndex, km *ethpb.PQKeyMaterial, previous primitives.Epoch, possession []byte) *ethpb.SignedPQKeyRegistration {
		return util.SignPQKeyRegistration(t, st, sk, &ethpb.PQKeyRegistration{Index: index, KeyMaterial: km, PreviousChangeEpoch: previous}, possession)
	}
	valid := sign(keys[0], 0, material(xpkA, 1), far, proofA)

	// Validator 0 holds key A since epoch 10.
	registered := st.Copy()
	require.NoError(t, heze.ProcessPQKeyRegistration(registered, valid))

	tests := []struct {
		name string
		head state.ReadOnlyBeaconState
		fork primitives.Epoch
		reg  *ethpb.SignedPQKeyRegistration
		want pubsub.ValidationResult
	}{
		{name: "accepts a valid registration", reg: valid, want: pubsub.ValidationAccept},
		{name: "ignores before the fork epoch", fork: epoch + 2, reg: valid, want: pubsub.ValidationIgnore},
		{name: "ignores a head state before the fork", head: gloasSt, reg: valid, want: pubsub.ValidationIgnore},
		{name: "rejects a validator outside the registry", reg: sign(keys[0], 64, material(xpkA, 1), far, proofA), want: pubsub.ValidationReject},
		{name: "rejects a builder outside the registry", reg: sign(builderKey, builder(3), material(xpkA, 0), 1, proofA), want: pubsub.ValidationReject},
		{name: "rejects a builder that has initiated an exit", reg: sign(builderKey, builder(2), material(xpkA, 0), 1, proofA), want: pubsub.ValidationReject},
		{name: "ignores a builder that is not active yet", reg: sign(builderKey, builder(1), material(xpkA, 0), 5, proofA), want: pubsub.ValidationIgnore},
		{name: "rejects a builder with a nonzero VRF root", reg: sign(builderKey, builder(0), material(xpkA, 1), 1, proofA), want: pubsub.ValidationReject},
		{name: "rejects a stale previous change epoch", reg: sign(keys[0], 0, material(xpkA, 1), epoch, proofA), want: pubsub.ValidationReject},
		{name: "ignores a change before MIN_PQ_KEY_CHANGE_DELAY", head: registered, reg: sign(keys[0], 0, material(xpkB, 1), epoch, proofB), want: pubsub.ValidationIgnore},
		{name: "rejects the all-zero key", reg: sign(keys[0], 0, material([32]byte{}, 1), far, proofA), want: pubsub.ValidationReject},
		{name: "rejects a key in use", head: registered, reg: sign(keys[1], 1, material(xpkA, 1), far, proofA), want: pubsub.ValidationReject},
		{name: "rejects a BLS signature by another key", reg: sign(keys[1], 0, material(xpkA, 1), far, proofA), want: pubsub.ValidationReject},
		{name: "rejects a proof of possession for another key", reg: sign(keys[0], 0, material(xpkA, 1), far, proofB), want: pubsub.ValidationReject},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := params.BeaconConfig().Copy()
			c.HezeForkEpoch = tt.fork
			params.OverrideBeaconConfig(c)
			head := tt.head
			if head == nil {
				head = st
			}
			got, err := newService(head).validatePQKeyRegistrationMessage(t.Context(), tt.reg)
			require.Equal(t, tt.want, got, err)
		})
	}

	t.Run("ignores a second registration for a seen pair", func(t *testing.T) {
		s := newService(st)
		got, err := s.validatePQKeyRegistrationMessage(t.Context(), valid)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, got)
		other := sign(keys[0], 0, material(xpkB, 1), far, proofB)
		got, _ = s.validatePQKeyRegistrationMessage(t.Context(), other)
		require.Equal(t, pubsub.ValidationIgnore, got)
	})

	t.Run("accepts one of several valid registrations for a pair validated at once", func(t *testing.T) {
		regs := []*ethpb.SignedPQKeyRegistration{valid, sign(keys[0], 0, material(xpkB, 1), far, proofB)}
		for _, seed := range []byte{0xc, 0xd} {
			xpk, proof := util.PQPossessionProof(t, st, keys[0].PublicKey().Marshal(), [32]byte{seed}, 0)
			regs = append(regs, sign(keys[0], 0, material(xpk, 1), far, proof))
		}
		for range 5 {
			s := newService(st)
			var accepted atomic.Int32
			var wg sync.WaitGroup
			start := make(chan struct{})
			for _, r := range regs {
				wg.Go(func() {
					<-start
					if got, _ := s.validatePQKeyRegistrationMessage(t.Context(), r); got == pubsub.ValidationAccept {
						accepted.Add(1)
					}
				})
			}
			close(start)
			wg.Wait()
			require.Equal(t, int32(1), accepted.Load())
		}
	})

	t.Run("decodes a gossip message", func(t *testing.T) {
		s := newService(st)
		buf := new(bytes.Buffer)
		_, err := s.cfg.p2p.Encoding().EncodeGossip(buf, valid)
		require.NoError(t, err)
		digest := s.currentForkDigest()
		topic := s.addDigestToTopic(p2p.PQKeyRegistrationTopicFormat, digest) + "/" + encoder.ProtocolSuffixSSZSnappy
		msg := &pubsub.Message{Message: &pubsubpb.Message{Data: buf.Bytes(), Topic: &topic}}
		got, err := s.validatePQKeyRegistration(t.Context(), "peer", msg)
		require.NoError(t, err)
		require.Equal(t, pubsub.ValidationAccept, got)
		require.DeepEqual(t, valid, msg.ValidatorData)
	})
}
