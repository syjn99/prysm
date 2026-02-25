// Package validator defines a gRPC validator service implementation, providing
// critical endpoints for validator clients to submit blocks/attestations to the
// beacon node, receive assignments, and more.
package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache/depositsnapshot"
	blockfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/block"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/slashings"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/synccommittee"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/voluntaryexits"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core/blockproduction"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Server defines a server implementation of the gRPC Validator service,
// providing RPC endpoints for obtaining validator assignments per epoch, the slots
// and committees in which particular validators need to perform their responsibilities,
// and more.
type Server struct {
	ethpb.UnimplementedBeaconNodeValidatorServer
	Ctx                     context.Context
	PayloadIDCache          *cache.PayloadIDCache
	TrackedValidatorsCache  *cache.TrackedValidatorsCache
	HeadFetcher             blockchain.HeadFetcher
	ForkFetcher             blockchain.ForkFetcher
	ForkchoiceFetcher       blockchain.ForkchoiceFetcher
	GenesisFetcher          blockchain.GenesisFetcher
	FinalizationFetcher     blockchain.FinalizationFetcher
	TimeFetcher             blockchain.TimeFetcher
	BlockFetcher            execution.POWBlockFetcher
	DepositFetcher          cache.DepositFetcher
	ChainStartFetcher       execution.ChainStartFetcher
	Eth1InfoFetcher         execution.ChainInfoFetcher
	OptimisticModeFetcher   blockchain.OptimisticModeFetcher
	SyncChecker             sync.Checker
	StateNotifier           statefeed.Notifier
	BlockNotifier           blockfeed.Notifier
	P2P                     p2p.Broadcaster
	AttestationCache        *cache.AttestationCache
	AttPool                 attestations.Pool
	SlashingsPool           slashings.PoolManager
	ExitPool                voluntaryexits.PoolManager
	SyncCommitteePool       synccommittee.Pool
	BlockReceiver           blockchain.BlockReceiver
	BlobReceiver            blockchain.BlobReceiver
	DataColumnReceiver      blockchain.DataColumnReceiver
	MockEth1Votes           bool
	Eth1BlockFetcher        execution.POWBlockFetcher
	PendingDepositsFetcher  depositsnapshot.PendingDepositsFetcher
	OperationNotifier       opfeed.Notifier
	StateGen                stategen.StateManager
	ReplayerBuilder         stategen.ReplayerBuilder
	BeaconDB                db.HeadAccessDatabase
	ExecutionEngineCaller   execution.EngineCaller
	BlockBuilder            builder.BlockBuilder
	BLSChangesPool          blstoexec.PoolManager
	ClockWaiter             startup.ClockWaiter
	CoreService             *core.Service
	AttestationStateFetcher blockchain.AttestationStateFetcher
	GraffitiInfo            *execution.GraffitiInfo
	BlockProducer           *blockproduction.BlockProducer
}

