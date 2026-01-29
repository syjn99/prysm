package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/executionproofs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
)

func (s *Service) validateExecutionProof(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	// Always accept messages our own messages.
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	// Ignore messages during initial sync.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	// Reject messages with a nil topic.
	if msg.Topic == nil {
		return pubsub.ValidationReject, p2p.ErrInvalidTopic
	}

	// Decode the message, reject if it fails.
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		log.WithError(err).Error("Failed to decode message")
		return pubsub.ValidationReject, err
	}

	// Reject messages that are not of the expected type.
	executionProof, ok := m.(*ethpb.ExecutionProof)
	if !ok {
		log.WithField("message", m).Error("Message is not of type *ethpb.ExecutionProof")
		return pubsub.ValidationReject, errWrongMessage
	}

	// Check if the proof is already in the DA checker cache (execution proof pool)
	// If it exists in the cache, we know it has already passed validation.
	blockRoot := bytesutil.ToBytes32(executionProof.BlockRoot)
	if s.cfg.execProofPool.Exists(blockRoot, executionProof.ProofId) {
		return pubsub.ValidationIgnore, nil
	}

	st, err := s.cfg.chain.HeadState(ctx)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	st, err = transition.ProcessSlotsIfPossible(ctx, st, executionProof.Slot)
	if err != nil {
		return pubsub.ValidationIgnore, err
	}

	if err := executionproofs.VerifyExecutionProof(st, executionProof); err != nil {
		return pubsub.ValidationReject, err
	}

	// Validation successful, return accept
	msg.ValidatorData = executionProof
	return pubsub.ValidationAccept, nil
}
