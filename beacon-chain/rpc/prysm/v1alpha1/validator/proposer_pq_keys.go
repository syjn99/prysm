package validator

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// setPQKeyRegistrations packs up to MAX_PQ_KEY_REGISTRATIONS EIP-8450 registrations that apply on the head state.
// A registration that the block's other operations invalidate (an exit in the same block) fails the state root
// computation, which then retries without registrations.
func (vs *Server) setPQKeyRegistrations(sBlk interfaces.SignedBeaconBlock, head state.BeaconState) {
	if sBlk.Version() < version.Heze || vs.PQKeyPool == nil {
		return
	}
	registrations := vs.PQKeyPool.RegistrationsForInclusion(head)
	if registrations == nil {
		registrations = []*ethpb.SignedPQKeyRegistration{}
	}
	if err := sBlk.SetPQKeyRegistrations(registrations); err != nil {
		log.WithError(err).Error("Could not set pq key registrations")
	}
}
