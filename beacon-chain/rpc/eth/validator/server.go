package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/synccommittee"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/rewards"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// BlockProducer is the interface for producing beacon blocks.
type BlockProducer interface {
	ProduceBlock(ctx context.Context, slot primitives.Slot, randaoReveal []byte, graffiti []byte, skipMevBoost bool, builderBoostFactor primitives.Gwei) (*eth.GenericBeaconBlock, error)
}

// Server defines a server implementation of the gRPC Validator service,
// providing RPC endpoints intended for validator clients.
type Server struct {
	HeadFetcher            blockchain.HeadFetcher
	TimeFetcher            blockchain.TimeFetcher
	SyncChecker            sync.Checker
	AttestationCache       *cache.AttestationCache
	AttestationsPool       attestations.Pool
	PeerManager            p2p.PeerManager
	Broadcaster            p2p.Broadcaster
	Stater                 lookup.Stater
	OptimisticModeFetcher  blockchain.OptimisticModeFetcher
	SyncCommitteePool      synccommittee.Pool
	V1Alpha1Server         eth.BeaconNodeValidatorServer
	ChainInfoFetcher       blockchain.ChainInfoFetcher
	BeaconDB               db.HeadAccessDatabase
	BlockBuilder           builder.BlockBuilder
	OperationNotifier      operation.Notifier
	CoreService            *core.Service
	BlockRewardFetcher     rewards.BlockRewardsFetcher
	TrackedValidatorsCache *cache.TrackedValidatorsCache
	PayloadIDCache         *cache.PayloadIDCache
	BlockProducer          BlockProducer
}
