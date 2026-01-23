package executionproofs

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/stretchr/testify/require"
)

func TestVerifyExecutionProof(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateFulu(t, 8)

	currentSlot := primitives.Slot(100)
	fcp := &ethpb.Checkpoint{
		Epoch: 1,
	}
	finalizedSlot, err := slots.EpochStart(fcp.Epoch)
	require.NoError(t, err)

	require.NoError(t, beaconState.SetSlot(currentSlot))
	require.NoError(t, beaconState.SetFinalizedCheckpoint(fcp))

	maxProofSize := params.BeaconConfig().MaxProofDataBytes

	tests := []struct {
		name    string
		setup   func() *ethpb.ExecutionProof
		wantErr string
	}{
		{
			name: "success: valid slot and data size",
			setup: func() *ethpb.ExecutionProof {
				return &ethpb.ExecutionProof{
					Slot:      currentSlot - 1, // Finalized < 99 < Current
					ProofData: make([]byte, 100),
				}
			},
			wantErr: "",
		},
		{
			name: "failure: future slot (ProofSlot > CurrentSlot)",
			setup: func() *ethpb.ExecutionProof {
				return &ethpb.ExecutionProof{
					Slot: currentSlot + 1,
				}
			},
			wantErr: fmt.Sprintf("execution proof slot %d is from the future", currentSlot+1),
		},
		{
			name: "failure: finalized slot (ProofSlot <= FinalizedSlot)",
			setup: func() *ethpb.ExecutionProof {
				return &ethpb.ExecutionProof{
					Slot: finalizedSlot,
				}
			},
			wantErr: fmt.Sprintf("execution proof slot %d is not above finalized slot %d", finalizedSlot, finalizedSlot),
		},
		{
			name: "failure: exceeds maximum allowed data size",
			setup: func() *ethpb.ExecutionProof {
				return &ethpb.ExecutionProof{
					Slot:      currentSlot,
					ProofData: make([]byte, maxProofSize+1),
				}
			},
			wantErr: "exceeds maximum allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proof := tt.setup()
			err := VerifyExecutionProof(beaconState, proof)

			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
