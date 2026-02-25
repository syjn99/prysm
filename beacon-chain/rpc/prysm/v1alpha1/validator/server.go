// Package validator defines the remaining gRPC validator service types.
// Most handler implementations have been removed — only ProposeBeaconBlock
// remains, used by the REST PublishBlockV2 endpoint.
package validator

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/builder"
	blockfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/block"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core/blockproduction"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Server defines the remaining gRPC validator service server.
// Only ProposeBeaconBlock is still implemented; all other methods
// fall through to UnimplementedBeaconNodeValidatorServer.
type Server struct {
	ethpb.UnimplementedBeaconNodeValidatorServer
	BlockNotifier      blockfeed.Notifier
	P2P                p2p.Broadcaster
	BlockReceiver      blockchain.BlockReceiver
	BlobReceiver       blockchain.BlobReceiver
	DataColumnReceiver blockchain.DataColumnReceiver
	OperationNotifier  opfeed.Notifier
	BlockBuilder       builder.BlockBuilder
	BlockProducer      *blockproduction.BlockProducer
}
