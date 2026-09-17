package eth

import (
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

// Copy creates a deep copy of ExecutionPayloadBidHeze.
func (bid *ExecutionPayloadBidHeze) Copy() *ExecutionPayloadBidHeze {
	if bid == nil {
		return nil
	}
	return &ExecutionPayloadBidHeze{
		ParentBlockHash:       bytesutil.SafeCopyBytes(bid.ParentBlockHash),
		ParentBlockRoot:       bytesutil.SafeCopyBytes(bid.ParentBlockRoot),
		BlockHash:             bytesutil.SafeCopyBytes(bid.BlockHash),
		PrevRandao:            bytesutil.SafeCopyBytes(bid.PrevRandao),
		FeeRecipient:          bytesutil.SafeCopyBytes(bid.FeeRecipient),
		GasLimit:              bid.GasLimit,
		BuilderIndex:          bid.BuilderIndex,
		Slot:                  bid.Slot,
		Value:                 bid.Value,
		ExecutionPayment:      bid.ExecutionPayment,
		BlobKzgCommitments:    bytesutil.SafeCopy2dBytes(bid.BlobKzgCommitments),
		ExecutionRequestsRoot: bytesutil.SafeCopyBytes(bid.ExecutionRequestsRoot),
		InclusionListBits:     bytesutil.SafeCopyBytes(bid.InclusionListBits),
	}
}
