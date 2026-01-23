package executionproofs

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func VerifyExecutionProof(
	beaconstate state.ReadOnlyBeaconState,
	executionProof *ethpb.ExecutionProof,
) error {
	// 1. Verify proof is not from the future
	currentSlot := beaconstate.Slot()
	proofSlot := executionProof.Slot
	if proofSlot > currentSlot {
		return fmt.Errorf("execution proof slot %d is from the future (current slot %d)", proofSlot, currentSlot)
	}

	// 2. Verify proof slot is greater than finalized slot
	finalizedEpoch := beaconstate.FinalizedCheckpointEpoch()
	finalizedSlot, err := slots.EpochStart(finalizedEpoch)
	if err != nil {
		return fmt.Errorf("could not compute finalized slot from epoch %d: %w", finalizedEpoch, err)
	}

	if proofSlot <= finalizedSlot {
		return fmt.Errorf("execution proof slot %d is not above finalized slot %d", proofSlot, finalizedSlot)
	}

	// 4. Verify proof size limits
	proofLen := uint64(len(executionProof.ProofData))
	maxProofLen := params.BeaconConfig().MaxProofDataBytes
	if proofLen > maxProofLen {
		return fmt.Errorf("execution proof data size %d exceeds maximum allowed %d", proofLen, maxProofLen)
	}

	// 5. Run actual zkVM proof verification
	if err := verifyProof(executionProof); err != nil {
		return fmt.Errorf("could not verify execution proof: %w", err)
	}

	return nil
}

// verifyProof performs the actual verification of the execution proof.
func verifyProof(_ *ethpb.ExecutionProof) error {
	// For now, say all proof are valid.
	return nil
}
