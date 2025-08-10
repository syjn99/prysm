package sszquery_test

import (
	"testing"

	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	sszquery "github.com/OffchainLabs/prysm/v6/ssz-query"
	sszquery_testutil "github.com/OffchainLabs/prysm/v6/ssz-query/testutil"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestPreCalculateSSZInfo(t *testing.T) {
	info, err := sszquery.PreCalculateSSZInfo(&ethpb.IndexedAttestationElectra{})
	if err != nil {
		t.Fatalf("PreCalculateSSZInfo failed: %v", err)
	}

	assert.NotNil(t, info, "Expected non-nil SSZ info")
	assert.Equal(t, uint64(228), info.FixedSize(), "Expected fixed size to be 228")
}

func TestCalculateOffset(t *testing.T) {
	// Target path: .data.target.root
	path, err := sszquery.ParsePath(".data.target.root")
	require.NoError(t, err, "ParsePath should not return an error")

	info, err := sszquery.PreCalculateSSZInfo(&ethpb.IndexedAttestationElectra{})
	require.NoError(t, err, "PreCalculateSSZInfo should not return an error")

	_, offset, length, err := sszquery.CalculateOffsetAndLength(info, path)
	if err != nil {
		t.Fatalf("ResolvePath failed: %v", err)
	}

	assert.Equal(t, uint64(100), offset, "Expected offset to be 100")
	assert.Equal(t, uint64(32), length, "Expected length to be 32")
}

func TestRoundTripSszInfo(t *testing.T) {
	specs := []sszquery_testutil.TestSpec{
		getIndexedAttestationElectraSpec(t),
		getValidatorSpec(t),
		getBeaconBlockHeaderSpec(t),
	}

	for _, spec := range specs {
		sszquery_testutil.RunStructTest(t, spec)
	}
}

func createIndexedAttestationElectra(t *testing.T) any {
	randomData := sszquery_testutil.RandomDummyData(t)

	return &ethpb.IndexedAttestationElectra{
		AttestingIndices: []uint64{1, 2, 3},
		Data: &ethpb.AttestationData{
			Slot:            4,
			CommitteeIndex:  5,
			BeaconBlockRoot: randomData.Root,
			Source: &ethpb.Checkpoint{
				Epoch: 7,
				Root:  randomData.Root,
			},
			Target: &ethpb.Checkpoint{
				Epoch: 9,
				Root:  randomData.Root,
			},
		},
		Signature: randomData.Signature,
	}
}

func getIndexedAttestationElectraSpec(t *testing.T) sszquery_testutil.TestSpec {
	indexedAtt := createIndexedAttestationElectra(t).(*ethpb.IndexedAttestationElectra)

	return sszquery_testutil.TestSpec{
		Name:     "IndexedAttestationElectra",
		Type:     ethpb.IndexedAttestationElectra{},
		Instance: indexedAtt,
		PathTests: []sszquery_testutil.PathTest{
			{
				Path:     ".data.target.root",
				Expected: indexedAtt.Data.Target.Root,
			},
			{
				Path:     ".data.target",
				Expected: indexedAtt.Data.Target,
			},
			{
				Path:     ".data",
				Expected: indexedAtt.Data,
			},
			{
				Path:     ".signature",
				Expected: indexedAtt.Signature,
			},
			{
				Path:     ".attesting_indices",
				Expected: indexedAtt.AttestingIndices,
			},
		},
	}
}

func createValidator(t *testing.T) any {
	randomData := sszquery_testutil.RandomDummyData(t)

	return &ethpb.Validator{
		PublicKey:                  randomData.Pubkey,
		WithdrawalCredentials:      randomData.Root,
		EffectiveBalance:           32000000000, // 32 ETH in Gwei
		Slashed:                    false,
		ActivationEligibilityEpoch: 1,
		ActivationEpoch:            2,
		ExitEpoch:                  3,
		WithdrawableEpoch:          4,
	}
}

func getValidatorSpec(t *testing.T) sszquery_testutil.TestSpec {
	validator := createValidator(t).(*ethpb.Validator)

	return sszquery_testutil.TestSpec{
		Name:     "Validator",
		Type:     ethpb.Validator{},
		Instance: validator,
		PathTests: []sszquery_testutil.PathTest{
			{
				Path:     ".public_key",
				Expected: validator.PublicKey,
			},
			{
				Path:     ".withdrawal_credentials",
				Expected: validator.WithdrawalCredentials,
			},
			{
				Path:     ".effective_balance",
				Expected: validator.EffectiveBalance,
			},
			{
				Path:     ".slashed",
				Expected: validator.Slashed,
			},
			{
				Path:     ".activation_eligibility_epoch",
				Expected: validator.ActivationEligibilityEpoch,
			},
			{
				Path:     ".activation_epoch",
				Expected: validator.ActivationEpoch,
			},
			{
				Path:     ".exit_epoch",
				Expected: validator.ExitEpoch,
			},
			{
				Path:     ".withdrawable_epoch",
				Expected: validator.WithdrawableEpoch,
			},
		},
	}
}

func createBeaconBlockHeader(t *testing.T) any {
	randomData := sszquery_testutil.RandomDummyData(t)

	return &ethpb.BeaconBlockHeader{
		Slot:          1,
		ProposerIndex: 2,
		ParentRoot:    randomData.Root,
		StateRoot:     randomData.Root,
		BodyRoot:      randomData.Root,
	}
}

func getBeaconBlockHeaderSpec(t *testing.T) sszquery_testutil.TestSpec {
	header := createBeaconBlockHeader(t).(*ethpb.BeaconBlockHeader)

	return sszquery_testutil.TestSpec{
		Name:     "BeaconBlockHeader",
		Type:     ethpb.BeaconBlockHeader{},
		Instance: header,
		PathTests: []sszquery_testutil.PathTest{
			{
				Path:     ".slot",
				Expected: header.Slot,
			},
			{
				Path:     ".proposer_index",
				Expected: header.ProposerIndex,
			},
			{
				Path:     ".parent_root",
				Expected: header.ParentRoot,
			},
			{
				Path:     ".state_root",
				Expected: header.StateRoot,
			},
			{
				Path:     ".body_root",
				Expected: header.BodyRoot,
			},
		},
	}
}
