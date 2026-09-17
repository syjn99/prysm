package util

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// DeterministicGenesisStateHeze returns a genesis state in Heze format made
// using the deterministic deposits.
func DeterministicGenesisStateHeze(t testing.TB, numValidators uint64) (state.BeaconState, []bls.SecretKey) {
	t.Helper()

	gloasState, privKeys := DeterministicGenesisStateGloas(t, numValidators)
	beaconState, err := heze.UpgradeToHeze(gloasState)
	if err != nil {
		t.Fatal(errors.Wrapf(err, "failed to upgrade genesis beacon state of %d validators to heze", numValidators))
	}
	resetCache()
	return beaconState, privKeys
}

// NewBeaconStateHeze creates a beacon state with minimum marshalable fields.
func NewBeaconStateHeze(options ...func(state *ethpb.BeaconStateHeze) error) (state.BeaconState, error) {
	gs, err := NewBeaconStateGloas()
	if err != nil {
		return nil, err
	}
	hs, err := heze.UpgradeToHeze(gs)
	if err != nil {
		return nil, err
	}
	seed, ok := hs.ToProtoUnsafe().(*ethpb.BeaconStateHeze)
	if !ok {
		return nil, errors.New("upgraded state is not a heze beacon state")
	}
	// Match NewBeaconStateGloas, which leaves the fork zeroed.
	seed.Fork = &ethpb.Fork{
		PreviousVersion: make([]byte, fieldparams.VersionLength),
		CurrentVersion:  make([]byte, fieldparams.VersionLength),
	}

	for _, opt := range options {
		if err := opt(seed); err != nil {
			return nil, err
		}
	}

	return state_native.InitializeFromProtoUnsafeHeze(seed)
}

// NewBeaconBlockHeze creates a beacon block with minimum marshalable fields.
func NewBeaconBlockHeze() *ethpb.SignedBeaconBlockHeze {
	return HydrateSignedBeaconBlockHeze(&ethpb.SignedBeaconBlockHeze{})
}

// HydrateSignedBeaconBlockHeze hydrates a signed beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateSignedBeaconBlockHeze(b *ethpb.SignedBeaconBlockHeze) *ethpb.SignedBeaconBlockHeze {
	if b == nil {
		b = &ethpb.SignedBeaconBlockHeze{}
	}
	if b.Signature == nil {
		b.Signature = make([]byte, fieldparams.BLSSignatureLength)
	}
	b.Block = HydrateBeaconBlockHeze(b.Block)
	return b
}

// HydrateBeaconBlockHeze hydrates a beacon block with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconBlockHeze(b *ethpb.BeaconBlockHeze) *ethpb.BeaconBlockHeze {
	if b == nil {
		b = &ethpb.BeaconBlockHeze{}
	}
	if b.ParentRoot == nil {
		b.ParentRoot = make([]byte, fieldparams.RootLength)
	}
	if b.StateRoot == nil {
		b.StateRoot = make([]byte, fieldparams.RootLength)
	}
	b.Body = HydrateBeaconBlockBodyHeze(b.Body)
	return b
}

// HydrateBeaconBlockBodyHeze hydrates a beacon block body with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateBeaconBlockBodyHeze(b *ethpb.BeaconBlockBodyHeze) *ethpb.BeaconBlockBodyHeze {
	if b == nil {
		b = &ethpb.BeaconBlockBodyHeze{}
	}
	if b.RandaoReveal == nil {
		b.RandaoReveal = make([]byte, fieldparams.BLSSignatureLength)
	}
	if b.Graffiti == nil {
		b.Graffiti = make([]byte, fieldparams.RootLength)
	}
	if b.Eth1Data == nil {
		b.Eth1Data = &ethpb.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		}
	}
	if b.SyncAggregate == nil {
		b.SyncAggregate = &ethpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		}
	}
	b.SignedExecutionPayloadBid = HydrateSignedExecutionPayloadBidHeze(b.SignedExecutionPayloadBid)
	if b.PayloadAttestations == nil {
		b.PayloadAttestations = make([]*ethpb.PayloadAttestation, 0)
	}
	if b.ParentExecutionRequests == nil {
		b.ParentExecutionRequests = &enginev1.ExecutionRequestsGloas{}
	}
	return b
}

// HydrateSignedExecutionPayloadBidHeze hydrates a signed execution payload bid with correct field
// length sizes to comply with fssz marshalling and unmarshalling rules.
func HydrateSignedExecutionPayloadBidHeze(b *ethpb.SignedExecutionPayloadBidHeze) *ethpb.SignedExecutionPayloadBidHeze {
	if b == nil {
		b = &ethpb.SignedExecutionPayloadBidHeze{}
	}
	if b.Signature == nil {
		b.Signature = make([]byte, fieldparams.BLSSignatureLength)
	}
	b.Message = HydrateExecutionPayloadBidHeze(b.Message)
	return b
}

// HydrateExecutionPayloadBidHeze hydrates an execution payload bid with correct field length sizes
// to comply with fssz marshalling and unmarshalling rules.
func HydrateExecutionPayloadBidHeze(b *ethpb.ExecutionPayloadBidHeze) *ethpb.ExecutionPayloadBidHeze {
	if b == nil {
		b = &ethpb.ExecutionPayloadBidHeze{}
	}
	if b.ParentBlockHash == nil {
		b.ParentBlockHash = make([]byte, fieldparams.RootLength)
	}
	if b.ParentBlockRoot == nil {
		b.ParentBlockRoot = make([]byte, fieldparams.RootLength)
	}
	if b.BlockHash == nil {
		b.BlockHash = make([]byte, fieldparams.RootLength)
	}
	if b.PrevRandao == nil {
		b.PrevRandao = make([]byte, fieldparams.RootLength)
	}
	if b.FeeRecipient == nil {
		b.FeeRecipient = make([]byte, fieldparams.FeeRecipientLength)
	}
	if b.BlobKzgCommitments == nil {
		b.BlobKzgCommitments = make([][]byte, 0)
	}
	if b.ExecutionRequestsRoot == nil {
		b.ExecutionRequestsRoot = make([]byte, fieldparams.RootLength)
	}
	if b.InclusionListBits == nil {
		b.InclusionListBits = make([]byte, (fieldparams.InclusionListCommitteeSize+7)/8)
	}
	return b
}
