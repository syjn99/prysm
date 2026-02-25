package beacon_chain_client_factory

import (
	beaconApi "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	nodeClientFactory "github.com/OffchainLabs/prysm/v7/validator/client/node-client-factory"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
)

func NewChainClient(validatorConn validatorHelpers.NodeConnection) iface.ChainClient {
	return beaconApi.NewBeaconApiChainClientWithFallback(validatorConn.GetRestHandler(), nil)
}

func NewPrysmChainClient(validatorConn validatorHelpers.NodeConnection) iface.PrysmChainClient {
	return beaconApi.NewPrysmChainClient(validatorConn.GetRestHandler(), nodeClientFactory.NewNodeClient(validatorConn))
}
