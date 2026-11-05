package state

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type readOnlyHezeFields interface {
	PQKeyEntry(registrant primitives.ValidatorIndex) (*ethpb.PQKeyMaterial, primitives.Epoch, error)
	IsPQPublicKeyInUse(key [fieldparams.RootLength]byte) (bool, error)
	PQKeyListsReadOnly() (keys []*ethpb.PQKeyMaterial, epochs []primitives.Epoch, builderKeys []*ethpb.PQKeyMaterial, builderEpochs []primitives.Epoch, err error)
}

type writeOnlyHezeFields interface {
	SetPQKeyEntry(registrant primitives.ValidatorIndex, key *ethpb.PQKeyMaterial, changeEpoch primitives.Epoch) error
}
