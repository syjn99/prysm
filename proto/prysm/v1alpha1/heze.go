package eth

import (
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

// Copy creates a deep copy of ExecutionPayloadBidHeze.
func (b *ExecutionPayloadBidHeze) Copy() *ExecutionPayloadBidHeze {
	if b == nil {
		return nil
	}
	return &ExecutionPayloadBidHeze{
		ParentBlockHash:       bytesutil.SafeCopyBytes(b.ParentBlockHash),
		ParentBlockRoot:       bytesutil.SafeCopyBytes(b.ParentBlockRoot),
		BlockHash:             bytesutil.SafeCopyBytes(b.BlockHash),
		PrevRandao:            bytesutil.SafeCopyBytes(b.PrevRandao),
		FeeRecipient:          bytesutil.SafeCopyBytes(b.FeeRecipient),
		GasLimit:              b.GasLimit,
		BuilderIndex:          b.BuilderIndex,
		Slot:                  b.Slot,
		Value:                 b.Value,
		ExecutionPayment:      b.ExecutionPayment,
		BlobKzgCommitments:    bytesutil.SafeCopy2dBytes(b.BlobKzgCommitments),
		ExecutionRequestsRoot: bytesutil.SafeCopyBytes(b.ExecutionRequestsRoot),
		InclusionListBits:     bytesutil.SafeCopyBytes(b.InclusionListBits),
	}
}

// Copy creates a deep copy of SignedExecutionPayloadBidHeze.
func (sb *SignedExecutionPayloadBidHeze) Copy() *SignedExecutionPayloadBidHeze {
	if sb == nil {
		return nil
	}
	return &SignedExecutionPayloadBidHeze{
		Message:   sb.Message.Copy(),
		Signature: bytesutil.SafeCopyBytes(sb.Signature),
	}
}

// CopySignedBeaconBlockHeze copies the provided signed beacon block Heze object.
func CopySignedBeaconBlockHeze(sb *SignedBeaconBlockHeze) *SignedBeaconBlockHeze {
	if sb == nil {
		return nil
	}
	return &SignedBeaconBlockHeze{
		Block:     copyBeaconBlockHeze(sb.Block),
		Signature: bytesutil.SafeCopyBytes(sb.Signature),
	}
}

// copyBeaconBlockHeze copies the provided beacon block Heze object.
func copyBeaconBlockHeze(b *BeaconBlockHeze) *BeaconBlockHeze {
	if b == nil {
		return nil
	}
	return &BeaconBlockHeze{
		Slot:          b.Slot,
		ProposerIndex: b.ProposerIndex,
		ParentRoot:    bytesutil.SafeCopyBytes(b.ParentRoot),
		StateRoot:     bytesutil.SafeCopyBytes(b.StateRoot),
		Body:          copyBeaconBlockBodyHeze(b.Body),
	}
}

// copyBeaconBlockBodyHeze copies the provided beacon block body Heze object.
func copyBeaconBlockBodyHeze(body *BeaconBlockBodyHeze) *BeaconBlockBodyHeze {
	if body == nil {
		return nil
	}
	copied := &BeaconBlockBodyHeze{
		RandaoReveal:              bytesutil.SafeCopyBytes(body.RandaoReveal),
		Eth1Data:                  body.Eth1Data.Copy(),
		Graffiti:                  bytesutil.SafeCopyBytes(body.Graffiti),
		ProposerSlashings:         CopySlice(body.ProposerSlashings),
		AttesterSlashings:         CopySlice(body.AttesterSlashings),
		Attestations:              CopySlice(body.Attestations),
		Deposits:                  CopySlice(body.Deposits),
		VoluntaryExits:            CopySlice(body.VoluntaryExits),
		BlsToExecutionChanges:     CopySlice(body.BlsToExecutionChanges),
		SignedExecutionPayloadBid: body.SignedExecutionPayloadBid.Copy(),
		PayloadAttestations:       copyPayloadAttestations(body.PayloadAttestations),
		ParentExecutionRequests:   CopyExecutionRequestsGloas(body.ParentExecutionRequests),
		PqKeyRegistrations:        CopySlice(body.PqKeyRegistrations),
	}
	if body.SyncAggregate != nil {
		copied.SyncAggregate = body.SyncAggregate.Copy()
	}
	return copied
}

// Copy creates a deep copy of PQKeyMaterial.
func (k *PQKeyMaterial) Copy() *PQKeyMaterial {
	if k == nil {
		return nil
	}
	return &PQKeyMaterial{
		XmssPublicKey: bytesutil.SafeCopyBytes(k.XmssPublicKey),
		VrfRoot:       bytesutil.SafeCopyBytes(k.VrfRoot),
	}
}

// Copy creates a deep copy of SignedPQKeyRegistration.
func (r *SignedPQKeyRegistration) Copy() *SignedPQKeyRegistration {
	if r == nil {
		return nil
	}
	cp := &SignedPQKeyRegistration{
		Possession: bytesutil.SafeCopyBytes(r.Possession),
		Signature:  bytesutil.SafeCopyBytes(r.Signature),
	}
	if r.Message != nil {
		cp.Message = &PQKeyRegistration{
			Index:               r.Message.Index,
			KeyMaterial:         r.Message.KeyMaterial.Copy(),
			PreviousChangeEpoch: r.Message.PreviousChangeEpoch,
		}
	}
	return cp
}
