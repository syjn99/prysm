package validator_client_factory

import (
	beaconApi "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
)

func NewValidatorClient(
	validatorConn validatorHelpers.NodeConnection,
	opt ...beaconApi.ValidatorClientOpt,
) iface.ValidatorClient {
	return beaconApi.NewBeaconApiValidatorClient(validatorConn.GetRestConnectionProvider(), opt...)
}
