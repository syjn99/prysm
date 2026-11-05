package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

// pqKeyRegistrationKey names a registration for the first-seen rule: the registrant and the change it follows.
type pqKeyRegistrationKey struct {
	index         primitives.ValidatorIndex
	previousEpoch primitives.Epoch
}

// validatePQKeyRegistration runs the EIP-8450 pq_key_registration gossip checks, in the EIP's order.
func (s *Service) validatePQKeyRegistration(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Validation runs on publish (not just subscriptions), so we should approve any message from ourselves.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validatePQKeyRegistration")
	defer span.End()

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}
	signed, ok := m.(*ethpb.SignedPQKeyRegistration)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	if signed.Message == nil || signed.Message.KeyMaterial == nil {
		return pubsub.ValidationReject, errNilMessage
	}

	res, err := s.validatePQKeyRegistrationMessage(ctx, signed)
	if res != pubsub.ValidationAccept {
		return res, err
	}
	msg.ValidatorData = signed
	return pubsub.ValidationAccept, nil
}

// validatePQKeyRegistrationMessage runs the EIP-8450 gossip checks on a decoded registration,
// and records it as the first valid one for its (index, previous_change_epoch) pair.
func (s *Service) validatePQKeyRegistrationMessage(ctx context.Context, signed *ethpb.SignedPQKeyRegistration) (pubsub.ValidationResult, error) {
	registration := signed.Message
	keyMaterial := registration.KeyMaterial

	// [IGNORE] The current epoch, allowing for MAXIMUM_GOSSIP_CLOCK_DISPARITY, is at or after the fork epoch.
	currentEpoch := s.currentEpochWithClockDisparity()
	if currentEpoch < params.BeaconConfig().HezeForkEpoch {
		return pubsub.ValidationIgnore, errors.New("pq key registration before the Heze fork")
	}

	// [IGNORE] The head state has the registry lists of this EIP.
	st, err := s.cfg.chain.HeadStateReadOnly(ctx)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if st.Version() < version.Heze {
		return pubsub.ValidationIgnore, errors.New("head state is before the Heze fork")
	}

	// [REJECT] The index is a validator that has not initiated an exit, or a builder that has not initiated an exit.
	var pubkey []byte
	if registration.Index.IsBuilderIndex() {
		builder, err := st.Builder(registration.Index.ToBuilderIndex())
		if err != nil || builder == nil || builder.WithdrawableEpoch != params.BeaconConfig().FarFutureEpoch {
			return pubsub.ValidationReject, fmt.Errorf("builder %d cannot register keys", registration.Index.ToBuilderIndex())
		}
		// [IGNORE] The builder's deposit epoch is before the finalized epoch of the head state.
		if builder.DepositEpoch >= st.FinalizedCheckpointEpoch() {
			return pubsub.ValidationIgnore, fmt.Errorf("builder %d is not active yet", registration.Index.ToBuilderIndex())
		}
		// [REJECT] A builder registration has a zero VRF root.
		if !isZeroRoot(keyMaterial.VrfRoot) {
			return pubsub.ValidationReject, errors.New("builder registration has a nonzero VRF root")
		}
		pubkey = builder.Pubkey
	} else {
		if uint64(registration.Index) >= uint64(st.NumValidators()) {
			return pubsub.ValidationReject, fmt.Errorf("validator index %d out of range", registration.Index)
		}
		val, err := st.ValidatorAtIndexReadOnly(registration.Index)
		if err != nil {
			return pubsub.ValidationIgnore, err
		}
		if val.ExitEpoch() != params.BeaconConfig().FarFutureEpoch {
			return pubsub.ValidationReject, fmt.Errorf("validator %d has initiated an exit", registration.Index)
		}
		pk := val.PublicKey()
		pubkey = pk[:]
	}

	// [IGNORE] The registration is the first valid one received for (index, previous_change_epoch).
	seenKey := pqKeyRegistrationKey{index: registration.Index, previousEpoch: registration.PreviousChangeEpoch}
	if s.hasSeenPQKeyRegistration(seenKey) {
		return pubsub.ValidationIgnore, nil
	}

	// [REJECT] The previous change epoch is the stored change epoch of the index.
	stored, changeEpoch, err := st.PQKeyEntry(registration.Index)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if registration.PreviousChangeEpoch != changeEpoch {
		return pubsub.ValidationReject, fmt.Errorf("previous change epoch %d does not match stored change epoch %d", registration.PreviousChangeEpoch, changeEpoch)
	}

	// [IGNORE] A change waits MIN_PQ_KEY_CHANGE_DELAY epochs, allowing for MAXIMUM_GOSSIP_CLOCK_DISPARITY.
	if !isZeroRoot(stored.XmssPublicKey) {
		earliest, err := registration.PreviousChangeEpoch.SafeAddEpoch(params.BeaconConfig().MinPQKeyChangeDelay)
		if err != nil || currentEpoch < earliest {
			return pubsub.ValidationIgnore, errors.New("pq key change before MIN_PQ_KEY_CHANGE_DELAY")
		}
	}

	// [REJECT] The leanXMSS key is not all-zero.
	if isZeroRoot(keyMaterial.XmssPublicKey) {
		return pubsub.ValidationReject, errors.New("zero leanXMSS public key")
	}

	// [REJECT] The leanXMSS key is not registered for any index.
	inUse, err := st.IsPQPublicKeyInUse(bytesutil.ToBytes32(keyMaterial.XmssPublicKey))
	if err != nil {
		return pubsub.ValidationIgnore, err
	}
	if inUse {
		return pubsub.ValidationReject, errors.New("leanXMSS public key already registered")
	}

	// [REJECT] The BLS signature is valid for the registrant's public key.
	if err := heze.VerifyPQKeyRegistrationSignature(st, pubkey, signed); err != nil {
		return pubsub.ValidationReject, err
	}

	// [REJECT] The proof of possession is valid.
	if !heze.IsValidPQPossessionProof(st, pubkey, keyMaterial.XmssPublicKey, signed.Possession) {
		return pubsub.ValidationReject, errors.New("invalid proof of possession")
	}

	// Record the pair atomically: of two valid registrations validated at once, only the first is accepted.
	if seen, _ := s.seenPQKeyRegistrationCache.ContainsOrAdd(seenKey, true); seen {
		return pubsub.ValidationIgnore, nil
	}
	return pubsub.ValidationAccept, nil
}

// currentEpochWithClockDisparity returns the epoch at now + MAXIMUM_GOSSIP_CLOCK_DISPARITY.
func (s *Service) currentEpochWithClockDisparity() primitives.Epoch {
	now := s.cfg.clock.Now().Add(params.BeaconConfig().MaximumGossipClockDisparityDuration())
	return slots.ToEpoch(slots.At(s.cfg.clock.GenesisTime(), now))
}

func (s *Service) hasSeenPQKeyRegistration(k pqKeyRegistrationKey) bool {
	_, seen := s.seenPQKeyRegistrationCache.Get(k)
	return seen
}

func isZeroRoot(b []byte) bool {
	return bytesutil.ToBytes32(b) == [32]byte{}
}
