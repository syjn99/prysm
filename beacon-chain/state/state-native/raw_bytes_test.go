package state_native

import (
	"testing"

	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestValidatorRawContainer(t *testing.T) {
	pubkey := make([]byte, 48)
	for i := 0; i < 48; i++ {
		pubkey[i] = byte(i + 1)
	}

	wc := make([]byte, 32)
	for i := 0; i < 32; i++ {
		wc[i] = byte(i + 4)
	}

	v := &ethpb.Validator{
		PublicKey:                  pubkey,
		WithdrawalCredentials:      wc,
		EffectiveBalance:           42000000000,
		Slashed:                    false,
		ActivationEligibilityEpoch: 10,
		ActivationEpoch:            11,
		ExitEpoch:                  12,
		WithdrawableEpoch:          13,
	}

	// Serialized validator.
	vBytes, err := v.MarshalSSZ()
	require.NoError(t, err)

	// Wrap in ValidatorRawContainer.
	vc := &ethpb.ValidatorRawContainer{
		LeadingField:   make([]byte, 32),
		ValidatorBytes: vBytes,
		TrailingField:  make([]byte, 32),
	}

	containerBytes, err := vc.MarshalSSZ()
	require.NoError(t, err)

	// Unmarshal back to ValidatorRawContainer.
	var vc2 ethpb.ValidatorRawContainer
	err = vc2.UnmarshalSSZ(containerBytes)
	require.NoError(t, err)

	// Extract Validator from ValidatorRawContainer.
	var v2 ethpb.Validator
	err = v2.UnmarshalSSZ(vc2.ValidatorBytes)
	require.NoError(t, err)

	// Compare original and extracted Validator.
	require.Equal(t, v, &v2)
}
