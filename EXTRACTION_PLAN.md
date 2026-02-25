# Extraction Plan: Shared Block-Production Code

## Goal

Extract ~14,000 lines of shared block-production logic from the gRPC package
`beacon-chain/rpc/prysm/v1alpha1/validator/` into a standalone package so that:

1. The REST API calls the new package directly (no gRPC intermediary).
2. The gRPC `v1alpha1/validator/` package can be deleted entirely.
3. ~22,300 lines of gRPC-only code (handlers, tests, mocks, client adapter) become removable.

---

## Current Architecture

```
REST ProduceBlockV3 (eth/validator/handlers_block.go:104)
  └─ s.V1Alpha1Server.GetBeaconBlock(ctx, req)           ← calls through gRPC interface
       └─ v1alpha1.Server.GetBeaconBlock (proposer.go:63)
            ├─ vs.getParentState()
            ├─ getEmptyBlock()
            └─ vs.BuildBlockParallel()                    ← the shared core
                 ├─ vs.eth1DataMajorityVote()
                 ├─ vs.packDepositsAndAttestations()
                 ├─ vs.getSlashings()
                 ├─ vs.getExits()
                 ├─ vs.setSyncAggregate()
                 ├─ vs.setBlsToExecData()
                 ├─ vs.getLocalPayload()
                 ├─ vs.getBuilderPayloadAndBlobs()
                 ├─ setExecutionData()           (static)
                 ├─ vs.computeStateRoot()
                 └─ vs.constructGenericBeaconBlock()

REST PublishBlock (eth/beacon/handlers.go:882)
  └─ s.V1Alpha1ValidatorServer.ProposeBeaconBlock(ctx, blk)  ← gRPC-only broadcast path

REST broadcastSeenBlockSidecars (eth/beacon/handlers.go:1448)
  └─ validator.BuildBlobSidecars(b, blobs, kzgProofs)        ← direct import, static function
```

**Problem:** The REST API holds a `V1Alpha1Server eth.BeaconNodeValidatorServer`
field (in both `eth/validator/server.go:32` and `eth/beacon/server.go:45`) and
delegates block production through the full 26-method gRPC interface. This couples
the REST layer to the entire gRPC server struct.

---

## Phase 1: Create the `blockproduction` Package

### 1.1 New Package Location

```
beacon-chain/rpc/core/blockproduction/
├── block_builder.go          # BlockBuilder struct + ProduceBlock method
├── blob_sidecars.go          # BuildBlobSidecars (moved from proposer_deneb.go)
├── attestations.go           # packAttestations and helpers
├── attestations_electra.go   # computeOnChainAggregate
├── deposits.go               # packDepositsAndAttestations, deposits, depositTrie, etc.
├── eth1data.go               # eth1DataMajorityVote, canonicalEth1Data, etc.
├── execution_payload.go      # getLocalPayload, getBuilderPayloadAndBlobs, etc.
├── execution_data.go         # setExecutionData (static; from proposer_bellatrix.go)
├── builder_bid.go            # canUseBuilder, validatorRegistered, circuitBreakBuilder, getPayloadHeaderFromBuilder
├── slashings.go              # getSlashings
├── exits.go                  # getExits
├── sync_aggregate.go         # setSyncAggregate, getSyncAggregate, etc.
├── sync_contributions.go     # proposerSyncContributions type + filter/dedup methods
├── capella.go                # setBlsToExecData
├── empty_block.go            # getEmptyBlock (static)
├── generic_block.go          # constructGenericBeaconBlock + per-fork constructors
├── state_root.go             # computeStateRoot, handleStateRootError
├── parent_state.go           # getParentState, getParentStateFromReorgData, etc.
├── unblinder.go              # unblindBlobsSidecars (static)
├── log.go                    # Package logger
└── doc.go                    # Package documentation
```

**Rationale for `beacon-chain/rpc/core/blockproduction/`:**
- Follows the existing `beacon-chain/rpc/core/` pattern (already has `core.Service`).
- Clearly separates block-production domain logic from any RPC transport.
- Avoids polluting the existing `core.Service` struct, which serves a different purpose
  (lighter shared state for non-block endpoints).

### 1.2 The `BlockBuilder` Struct

The new struct replaces the gRPC `Server` as the receiver for block-production methods.
It contains only the 21 dependencies actually used by the block-production call tree.

```go
package blockproduction

type BlockBuilder struct {
    // Blockchain state
    HeadFetcher           blockchain.HeadFetcher
    ForkchoiceFetcher     blockchain.ForkchoiceFetcher
    TimeFetcher           blockchain.TimeFetcher
    FinalizationFetcher   blockchain.FinalizationFetcher
    OptimisticModeFetcher blockchain.OptimisticModeFetcher
    SyncChecker           sync.Checker

    // Execution layer
    Eth1InfoFetcher       execution.ChainInfoFetcher
    Eth1BlockFetcher      execution.POWBlockFetcher
    ChainStartFetcher     execution.ChainStartFetcher
    ExecutionEngineCaller execution.EngineCaller

    // Caches
    PayloadIDCache         *cache.PayloadIDCache
    TrackedValidatorsCache *cache.TrackedValidatorsCache
    AttestationCache       *cache.AttestationCache

    // Operation pools
    AttPool           attestations.Pool
    SlashingsPool     slashings.PoolManager
    ExitPool          voluntaryexits.PoolManager
    SyncCommitteePool synccommittee.Pool
    BLSChangesPool    blstoexec.PoolManager

    // Deposit handling
    DepositFetcher         cache.DepositFetcher
    PendingDepositsFetcher depositsnapshot.PendingDepositsFetcher

    // State generation
    StateGen stategen.StateManager

    // MEV builder
    BlockBuilderClient builder.BlockBuilder   // renamed from BlockBuilder to avoid shadowing

    // Misc
    MockEth1Votes bool
    GraffitiInfo  *execution.GraffitiInfo
}
```

**Fields deliberately excluded** (gRPC-only, used by ProposeBeaconBlock):
- `Ctx` — pass via function args
- `ForkFetcher` — only used by removed gRPC handlers (GetDuties)
- `GenesisFetcher` — only used by removed gRPC handlers (WaitForChainStart)
- `StateNotifier` — only used by removed gRPC handlers (WaitForActivation)
- `BlockNotifier` — only used by ProposeBeaconBlock broadcast
- `P2P` — only used by ProposeBeaconBlock broadcast
- `BlockReceiver` — only used by ProposeBeaconBlock broadcast
- `BlobReceiver` — only used by ProposeBeaconBlock broadcast
- `DataColumnReceiver` — only used by ProposeBeaconBlock broadcast
- `OperationNotifier` — only used by ProposeBeaconBlock broadcast
- `ReplayerBuilder` — not used by any proposer code
- `BeaconDB` — not used by block-production code
- `ClockWaiter` — only used by removed gRPC handlers
- `CoreService` — not used by block-production code
- `AttestationStateFetcher` — not used by block-production code
- `BlockFetcher` — duplicate of Eth1BlockFetcher

### 1.3 Primary API Surface

```go
// ProduceBlock builds a complete unsigned beacon block. This is the extracted
// equivalent of the old GetBeaconBlock gRPC handler, minus the gRPC error
// wrapping and deprecation logging.
func (b *BlockBuilder) ProduceBlock(
    ctx context.Context,
    slot primitives.Slot,
    randaoReveal []byte,
    graffiti []byte,
    skipMevBoost bool,
    builderBoostFactor primitives.Gwei,
) (*ethpb.GenericBeaconBlock, error)

// BuildBlockParallel fills in the consensus and execution data for a prepared
// block shell. Exported for callers that manage their own block initialization.
func (b *BlockBuilder) BuildBlockParallel(
    ctx context.Context,
    sBlk interfaces.SignedBeaconBlock,
    head state.BeaconState,
    skipMevBoost bool,
    builderBoostFactor primitives.Gwei,
) (*ethpb.GenericBeaconBlock, error)

// BuildBlobSidecars constructs blob sidecars from a signed block and raw blobs.
// This is a static function (no receiver needed).
func BuildBlobSidecars(
    blk interfaces.ReadOnlySignedBeaconBlock,
    blobs [][]byte,
    kzgProofs [][]byte,
) ([]*ethpb.BlobSidecar, error)
```

`ProduceBlock` encapsulates the full `GetBeaconBlock` flow:
1. Syncing check (`b.SyncChecker.Syncing()`)
2. Optimistic check (`b.OptimisticModeFetcher.IsOptimistic()`)
3. Parent state resolution (`b.getParentState()`)
4. Empty block creation (`getEmptyBlock()`)
5. Set slot, graffiti, randao, parent root, proposer index
6. Call `b.BuildBlockParallel()`

---

## Phase 2: Move Files (Mechanical Refactor)

### 2.1 File-by-File Migration

Each source file is moved from `beacon-chain/rpc/prysm/v1alpha1/validator/` to
`beacon-chain/rpc/core/blockproduction/`. The receiver type changes from
`(vs *Server)` to `(b *BlockBuilder)`.

| Source File | Destination File | Changes |
|-------------|-----------------|---------|
| `proposer.go` (lines 45-290 only) | `block_builder.go` + `parent_state.go` + `state_root.go` | Extract `optimisticStatus`, `GetBeaconBlock` → `ProduceBlock`, `BuildBlockParallel`, `getParentState*`, `computeStateRoot`, `handleStateRootError`. Drop gRPC `status.Errorf` wrappers → plain errors. |
| `proposer_deneb.go` | `blob_sidecars.go` | Move `BuildBlobSidecars` (static function, no receiver change). |
| `proposer_attestations.go` | `attestations.go` | Receiver `vs *Server` → `b *BlockBuilder`. |
| `proposer_attestations_electra.go` | `attestations_electra.go` | Static function, no receiver. |
| `proposer_deposits.go` | `deposits.go` | Receiver `vs *Server` → `b *BlockBuilder`. |
| `proposer_eth1data.go` | `eth1data.go` | Receiver `vs *Server` → `b *BlockBuilder`. |
| `proposer_execution_payload.go` | `execution_payload.go` | Receiver `vs *Server` → `b *BlockBuilder`. Split `getPayloadHeaderFromBuilder` into `builder_bid.go`. |
| `proposer_bellatrix.go` | `execution_data.go` + `builder_bid.go` | `setExecutionData` (static) → `execution_data.go`. `getPayloadHeaderFromBuilder`, `validateBuilderSignature`, etc. → `builder_bid.go`. |
| `proposer_builder.go` | `builder_bid.go` (append) | `canUseBuilder`, `validatorRegistered`, `circuitBreakBuilder`. |
| `proposer_altair.go` | `sync_aggregate.go` | Receiver change. |
| `proposer_sync_aggregate.go` | `sync_contributions.go` | Type methods on `proposerSyncContributions`. |
| `proposer_capella.go` | `capella.go` | Receiver change. |
| `proposer_exits.go` | `exits.go` | Receiver change. |
| `proposer_slashings.go` | `slashings.go` | Receiver change. |
| `proposer_empty_block.go` | `empty_block.go` | Static function. |
| `construct_generic_block.go` | `generic_block.go` | Receiver `vs *Server` → `b *BlockBuilder` (though no fields are accessed). |
| `unblinder.go` | `unblinder.go` | Static function. Only called by gRPC `ProposeBeaconBlock`; consider deferring move or marking gRPC-only for later deletion. |

### 2.2 Key Refactoring Rules

1. **Receiver rename:** `func (vs *Server)` → `func (b *BlockBuilder)`, with field
   references updated (e.g., `vs.HeadFetcher` → `b.HeadFetcher`).

2. **Drop gRPC status errors:** Replace `status.Errorf(codes.X, ...)` with plain
   `fmt.Errorf(...)` or `errors.Wrap(...)`. The REST handlers already translate
   errors to HTTP status codes themselves.

3. **Drop gRPC imports:** Remove `google.golang.org/grpc/codes` and
   `google.golang.org/grpc/status` from all migrated files.

4. **Field rename:** `BlockBuilder` → `BlockBuilderClient` (the field referencing
   the MEV builder interface) to avoid collision with the struct name.

5. **Package-level vars:** Move `eth1DataNotification`, `defaultBuilderBoostFactor`,
   `errOptimisticMode`, and Prometheus metrics (`builderGetPayloadMissCount`, etc.)
   to the new package.

### 2.3 Test File Migration

Tests for the shared block-production code are migrated alongside the source:

| Source Test File | Destination | Notes |
|-----------------|-------------|-------|
| `proposer_test.go` (partial) | `block_builder_test.go` | Only tests for `BuildBlockParallel`, `ProduceBlock`, `getParentState`, `computeStateRoot`. |
| `proposer_attestations_test.go` | `attestations_test.go` | Full move. |
| `proposer_attestations_electra_test.go` | `attestations_electra_test.go` | Full move. |
| `proposer_deposits_test.go` | `deposits_test.go` | Full move. |
| `proposer_bellatrix_test.go` | `execution_data_test.go` | Full move. |
| `proposer_builder_test.go` | `builder_bid_test.go` | Full move. |
| `proposer_altair_test.go` | `sync_aggregate_test.go` | Full move. |
| `proposer_sync_aggregate_test.go` | `sync_contributions_test.go` | Full move. |
| `proposer_execution_payload_test.go` | `execution_payload_test.go` | Full move. |
| `proposer_deneb_test.go` | `blob_sidecars_test.go` | Full move. |
| `proposer_deneb_bench_test.go` | `blob_sidecars_bench_test.go` | Full move. |
| `proposer_empty_block_test.go` | `empty_block_test.go` | Full move. |
| `proposer_exits_test.go` | `exits_test.go` | Full move. |
| `proposer_slashings_test.go` | `slashings_test.go` | Full move. |
| `construct_generic_block_test.go` | `generic_block_test.go` | Full move. |

Tests must be updated to construct `BlockBuilder` instead of `Server`, using the
same dependency injection pattern but with fewer fields.

---

## Phase 3: Rewire the REST API

### 3.1 REST Validator Server (`eth/validator/server.go`)

**Before:**
```go
type Server struct {
    // ...
    V1Alpha1Server eth.BeaconNodeValidatorServer   // full gRPC interface
    // ...
}
```

**After:**
```go
type Server struct {
    // ...
    BlockProducer *blockproduction.BlockBuilder     // narrower dependency
    // ...
}
```

### 3.2 REST Validator Handler (`eth/validator/handlers_block.go`)

**Before** (line 104):
```go
v1alpha1resp, err := s.V1Alpha1Server.GetBeaconBlock(ctx, v1alpha1req)
```

**After:**
```go
v1alpha1resp, err := s.BlockProducer.ProduceBlock(
    ctx,
    primitives.Slot(slot),
    randaoReveal,
    graffiti,
    req.SkipMevBoost,
    builderBoostFactor,
)
```

The `ProduceBlock` return type is `*ethpb.GenericBeaconBlock` — same as
`GetBeaconBlock`, so the downstream response handling (fork-specific serialization)
is unchanged.

### 3.3 REST Beacon Server (`eth/beacon/server.go`)

**Before:**
```go
type Server struct {
    // ...
    V1Alpha1ValidatorServer eth.BeaconNodeValidatorServer
    // ...
}
```

**After — Option A (inline the propose logic):**

The `proposeBlock` function (`handlers.go:881`) calls
`s.V1Alpha1ValidatorServer.ProposeBeaconBlock()` which does broadcast + receive.
The REST beacon server _already has_ the fields needed for block broadcast:
`BlockReceiver`, `BlockNotifier`, `Broadcaster`, `OperationNotifier`.

Replace the delegation with direct broadcast logic:
```go
func (s *Server) proposeBlock(ctx context.Context, w http.ResponseWriter, blk *eth.GenericSignedBeaconBlock) {
    block, err := blocks.NewSignedBeaconBlock(blk.Block)
    // ... validate, receive, broadcast using s.BlockReceiver, s.Broadcaster, etc.
}
```

This eliminates the last dependency on the gRPC server for block proposal.

**After — Option B (temporary BlockProposer interface):**

If inlining is too large a change for one PR, define a minimal interface:
```go
type BlockProposer interface {
    ProposeBeaconBlock(ctx context.Context, req *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error)
}
```

Wire the existing gRPC server through this interface temporarily, then inline in a
follow-up.

**Recommendation:** Option A is preferred — it removes the dependency entirely. The
broadcast logic in `ProposeBeaconBlock` is ~70 lines and the REST beacon server
already has all the needed dependencies.

### 3.4 REST Beacon Handler — `BuildBlobSidecars` (`handlers.go:1448`)

**Before:**
```go
import validator "github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/prysm/v1alpha1/validator"
scs, err := validator.BuildBlobSidecars(b, blobs, kzgProofs)
```

**After:**
```go
import "github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core/blockproduction"
scs, err := blockproduction.BuildBlobSidecars(b, blobs, kzgProofs)
```

### 3.5 Service Wiring (`beacon-chain/rpc/service.go`)

**Before** (lines 220-261): Creates `validatorv1alpha1.Server` and passes it to
both REST servers.

**After:**
```go
blockProd := &blockproduction.BlockBuilder{
    HeadFetcher:            s.cfg.HeadFetcher,
    ForkchoiceFetcher:      s.cfg.ForkchoiceFetcher,
    TimeFetcher:            s.cfg.GenesisTimeFetcher,
    FinalizationFetcher:    s.cfg.FinalizationFetcher,
    OptimisticModeFetcher:  s.cfg.OptimisticModeFetcher,
    SyncChecker:            s.cfg.SyncService,
    Eth1InfoFetcher:        s.cfg.ExecutionChainService,
    Eth1BlockFetcher:       s.cfg.ExecutionChainService,
    ChainStartFetcher:      s.cfg.ChainStartFetcher,
    ExecutionEngineCaller:  s.cfg.ExecutionEngineCaller,
    PayloadIDCache:         s.cfg.PayloadIDCache,
    TrackedValidatorsCache: s.cfg.TrackedValidatorsCache,
    AttestationCache:       s.cfg.AttestationCache,
    AttPool:                s.cfg.AttestationsPool,
    SlashingsPool:          s.cfg.SlashingsPool,
    ExitPool:               s.cfg.ExitPool,
    SyncCommitteePool:      s.cfg.SyncCommitteeObjectPool,
    BLSChangesPool:         s.cfg.BLSChangesPool,
    DepositFetcher:         s.cfg.DepositFetcher,
    PendingDepositsFetcher: s.cfg.PendingDepositFetcher,
    StateGen:               s.cfg.StateGen,
    BlockBuilderClient:     s.cfg.BlockBuilder,
    MockEth1Votes:          s.cfg.MockEth1Votes,
    GraffitiInfo:           s.cfg.GraffitiInfo,
}
```

Pass `blockProd` to REST endpoint constructors instead of `validatorServer`.

### 3.6 Endpoint Signatures (`beacon-chain/rpc/endpoints.go`)

**Before** (line 86):
```go
func (s *Service) endpoints(..., validatorServer *validatorv1alpha1.Server, ...) []endpoint
```

**After:**
```go
func (s *Service) endpoints(..., blockProd *blockproduction.BlockBuilder, ...) []endpoint
```

Update `validatorEndpoints()` (line 93) and `beaconEndpoints()` (line 95) signatures
and the server struct construction within each.

---

## Phase 4: Delete gRPC-Only Code

Once the REST API no longer references `v1alpha1/validator`, delete in this order:

### 4.1 gRPC Handler Implementations

Delete the remaining methods from `beacon-chain/rpc/prysm/v1alpha1/validator/proposer.go`:
- `GetBeaconBlock` (now `ProduceBlock` in new package)
- `ProposeBeaconBlock` (broadcast logic inlined in REST)
- `broadcastReceiveBlock`, `broadcastBlock`
- `broadcastAndReceiveSidecars`, `broadcastAndReceiveBlobs`, `broadcastAndReceiveDataColumns`
- `handleBlindedBlock`, `handleUnblindedBlock`
- `blobsAndProofs`
- `optimisticStatus` (moved to new package)

### 4.2 Shared Block-Production Files (Now Moved)

Delete from `beacon-chain/rpc/prysm/v1alpha1/validator/`:
- `proposer_altair.go`
- `proposer_attestations.go`
- `proposer_attestations_electra.go`
- `proposer_bellatrix.go`
- `proposer_builder.go`
- `proposer_capella.go`
- `proposer_deneb.go`
- `proposer_deposits.go`
- `proposer_empty_block.go`
- `proposer_eth1data.go`
- `proposer_execution_payload.go`
- `proposer_exits.go`
- `proposer_slashings.go`
- `proposer_sync_aggregate.go`
- `construct_generic_block.go`
- `unblinder.go`

### 4.3 Test Files

Delete all test files from `beacon-chain/rpc/prysm/v1alpha1/validator/`:
- `proposer_test.go` (~3,742 lines)
- `proposer_bellatrix_test.go` (~1,420 lines)
- `proposer_attestations_test.go` (~944 lines)
- `proposer_sync_aggregate_test.go` (~468 lines)
- `proposer_execution_payload_test.go` (~402 lines)
- `proposer_altair_test.go` (~293 lines)
- `proposer_deneb_bench_test.go` (~214 lines)
- `proposer_deposits_test.go` (~213 lines)
- `proposer_builder_test.go` (~190 lines)
- `construct_generic_block_test.go` (~177 lines)
- `proposer_attestations_electra_test.go` (~163 lines)
- `unblinder_test.go` (~150 lines)
- `proposer_empty_block_test.go` (~92 lines)
- `proposer_slashings_test.go` (~48 lines)
- `proposer_exits_test.go` (~37 lines)
- `proposer_deneb_test.go` (~36 lines)
- `validator_test.go` (~21 lines)
- All remaining `*_test.go` files already gutted in previous commits

### 4.4 Server Struct and Package Remains

Delete or gut:
- `server.go` — Remove `Server` struct entirely. The package can be deleted.
- `proposer.go` — All remaining code (everything was either moved or is gRPC-only).
- `log.go` — Package logger (no longer needed).

### 4.5 Mock Files

Delete from `testing/mock/`:
- `beacon_validator_server_mock.go` (~876 lines)
- `beacon_validator_client_mock.go` (~1,053 lines)
- `beacon_altair_validator_server_mock.go` (~133 lines)
- `beacon_altair_validator_client_mock.go` (~137 lines)

Update `hack/update-mockgen.sh` (lines 12-21): remove the four `mockgen` commands
that generate these files.

### 4.6 gRPC Service Registration

Delete from `beacon-chain/rpc/service.go:329`:
```go
ethpbv1alpha1.RegisterBeaconNodeValidatorServer(s.grpcServer, validatorServer)
```

Also delete the `validatorServer` construction (lines 220-261) and
`s.validatorServer` assignment (line 262).

### 4.7 Validator Client gRPC Adapter

Delete from `validator/client/grpc-api/`:
- `grpc_validator_client.go` (~393 lines)
- `grpc_validator_client_test.go` (~348 lines)

### 4.8 REST Handler Tests

Update tests in `beacon-chain/rpc/eth/`:
- `validator/handlers_block_test.go` — Replace `mock.NewMockBeaconNodeValidatorServer`
  with direct `blockproduction.BlockBuilder` instances. (~60 test cases affected)
- `beacon/handlers_test.go` — Replace `mock.NewMockBeaconNodeValidatorServer` with
  direct broadcast testing. (~28 test cases using `V1Alpha1ValidatorServer`)

---

## Phase 5: Cleanup

### 5.1 BUILD Files

Run `bazel run //:gazelle -- fix` after each phase to regenerate BUILD files.

### 5.2 Proto Deprecation

The following proto types are marked `option deprecated = true` but still used by
the validator REST client internally. They are NOT deleted in this plan:
- `GenericBeaconBlock`, `GenericSignedBeaconBlock` — used by `ProduceBlock` return type
- `BlockRequest` — used by REST handler to construct request
- `ProposeResponse` — used by REST propose path

These become candidates for replacement in a future proto cleanup pass.

### 5.3 Import Audit

After deletion, grep for any remaining imports of the old package:
```
rg "beacon-chain/rpc/prysm/v1alpha1/validator" --type go
```

All hits should be in test helpers or the new package's re-exports. Fix any stragglers.

---

## Execution Order (PR Sequence)

### PR 1: Create `blockproduction` package (extractive, additive only)

**No deletions.** Copy code into new package with new receiver type.
All existing code continues to work — this PR is purely additive.

Files created:
- All files listed in Section 1.1
- Corresponding test files

Verify: `bazel build //...` and `bazel test //beacon-chain/rpc/core/blockproduction/...`

### PR 2: Rewire REST validator server to use `blockproduction`

- Change `eth/validator/server.go`: `V1Alpha1Server` → `BlockProducer`
- Update `eth/validator/handlers_block.go`: call `s.BlockProducer.ProduceBlock()`
- Update `rpc/endpoints.go`: change `validatorEndpoints()` signature
- Update `rpc/service.go`: construct `BlockBuilder`, pass to REST validator server
- Update `eth/validator/handlers_block_test.go`: use real `BlockBuilder` or test helper

Verify: `bazel test //beacon-chain/rpc/eth/validator/...`

### PR 3: Rewire REST beacon server to remove `ProposeBeaconBlock` delegation

- Inline block broadcast logic in `eth/beacon/handlers.go`
- Change `eth/beacon/server.go`: remove `V1Alpha1ValidatorServer` field,
  add direct `BlockReceiver`, `Broadcaster`, etc. fields (most already present)
- Update `eth/beacon/handlers.go:1448`: import `blockproduction.BuildBlobSidecars`
- Update `rpc/endpoints.go`: change `beaconEndpoints()` signature
- Update `rpc/service.go`: stop passing `validatorServer` to beacon endpoints
- Update `eth/beacon/handlers_test.go`: remove mock server setup

Verify: `bazel test //beacon-chain/rpc/eth/beacon/...`

### PR 4: Delete gRPC `v1alpha1/validator` package

- Delete entire `beacon-chain/rpc/prysm/v1alpha1/validator/` directory
- Delete mock files from `testing/mock/`
- Delete `RegisterBeaconNodeValidatorServer` call from `service.go`
- Remove `validatorServer` construction from `service.go`
- Update `hack/update-mockgen.sh`
- Run `gazelle fix`

Verify: `bazel build //...` and full test suite

### PR 5: Delete validator client gRPC adapter

- Delete `validator/client/grpc-api/grpc_validator_client.go`
- Delete `validator/client/grpc-api/grpc_validator_client_test.go`
- Verify validator client still works via `beacon-api` adapter

Verify: `bazel test //validator/...`

---

## Dependency Graph: BuildBlockParallel Call Tree

```
BuildBlockParallel
├── [goroutine: consensus fields]
│   ├── eth1DataMajorityVote ─────────── HeadFetcher, TimeFetcher, Eth1InfoFetcher,
│   │   ├── slotStartTime                Eth1BlockFetcher, DepositFetcher,
│   │   ├── canonicalEth1Data             ChainStartFetcher, MockEth1Votes
│   │   ├── mockETH1DataVote
│   │   └── randomETH1DataVote
│   │
│   ├── packDepositsAndAttestations ──── AttPool, AttestationCache, ForkchoiceFetcher,
│   │   ├── deposits                      HeadFetcher, DepositFetcher,
│   │   │   ├── depositTrie               PendingDepositsFetcher, Eth1InfoFetcher
│   │   │   │   ├── rebuildDepositTrie
│   │   │   │   ├── validateDepositTrie
│   │   │   │   └── shouldRebuildTrie
│   │   │   └── constructMerkleProof
│   │   └── packAttestations
│   │       ├── onChainAggregates
│   │       ├── sortSlotAttestations
│   │       ├── attestationFields
│   │       ├── computeOnChainAggregate (electra)
│   │       ├── validateAndDeleteAttsInPool
│   │       ├── filterAttestationBySignature
│   │       ├── filterCurrentEpochAttByForkchoice
│   │       ├── filterCurrentEpochAttByTarget
│   │       └── filterPreviousEpochAttByTarget
│   │
│   ├── getSlashings ─────────────────── SlashingsPool
│   ├── getExits ─────────────────────── ExitPool
│   ├── setSyncAggregate ─────────────── SyncCommitteePool
│   │   └── getSyncAggregate
│   │       └── aggregateSyncSubcommitteeMessages
│   └── setBlsToExecData ────────────── BLSChangesPool
│
├── [main thread: execution payload]
│   ├── getLocalPayload ─────────────── PayloadIDCache, TrackedValidatorsCache,
│   │   └── getLocalPayloadFromEngine    ExecutionEngineCaller, FinalizationFetcher,
│   │       ├── setFeeRecipientIfBurnAddr Eth1BlockFetcher, HeadFetcher, TimeFetcher
│   │       ├── warnIfFeeRecipientDiffers
│   │       ├── getTerminalBlockHashIfExists
│   │       ├── getParentBlockHash
│   │       ├── activationEpochNotReached
│   │       └── emptyPayload*
│   │
│   ├── getBuilderPayloadAndBlobs ───── BlockBuilderClient, ForkchoiceFetcher,
│   │   ├── canUseBuilder                HeadFetcher, TimeFetcher
│   │   │   ├── validatorRegistered
│   │   │   └── circuitBreakBuilder
│   │   └── getPayloadHeaderFromBuilder
│   │       └── validateBuilderSignature
│   │
│   └── setExecutionData (static) ───── (no fields)
│       ├── matchingWithdrawalsRoot
│       ├── setLocalExecution
│       ├── setBuilderExecution
│       ├── setExecution
│       ├── expectedGasLimit
│       └── isVersionCompatible
│
├── computeStateRoot ─────────────────── StateGen
│   └── handleStateRootError
│
└── constructGenericBeaconBlock ──────── (no fields)
    ├── constructPhase0Block
    ├── constructAltairBlock
    ├── constructBellatrixBlock
    ├── constructCapellaBlock
    ├── constructDenebBlock
    ├── constructElectraBlock
    └── constructFuluBlock
```

---

## Server Struct Field Usage Matrix

| BlockBuilder Field | eth1data | deposits | attestations | slashings | exits | sync_agg | bls_exec | local_payload | builder_bid | state_root | parent_state | generic_block |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| HeadFetcher | X | | X | | | | | X | X | | X | |
| ForkchoiceFetcher | | | X | | | | | | X | | X | |
| TimeFetcher | X | | | | | | | X | X | | X | |
| FinalizationFetcher | | | | | | | | X | | | | |
| OptimisticModeFetcher | | | | | | | | | | | X | |
| SyncChecker | | | | | | | | | | | X | |
| Eth1InfoFetcher | X | X | | | | | | | | | | |
| Eth1BlockFetcher | X | | | | | | | X | | | | |
| ChainStartFetcher | X | | | | | | | | | | | |
| ExecutionEngineCaller | | | | | | | | X | | | | |
| PayloadIDCache | | | | | | | | X | | | | |
| TrackedValidatorsCache | | | | | | | | X | | | | |
| AttestationCache | | | X | | | | | | | | | |
| AttPool | | | X | | | | | | | | | |
| SlashingsPool | | | | X | | | | | | | | |
| ExitPool | | | | | X | | | | | | | |
| SyncCommitteePool | | | | | | X | | | | | | |
| BLSChangesPool | | | | | | | X | | | | | |
| DepositFetcher | X | X | | | | | | | | | | |
| PendingDepositsFetcher | | X | | | | | | | | | | |
| StateGen | | | | | | | | | | X | | |
| BlockBuilderClient | | | | | | | | | X | | | |
| MockEth1Votes | X | X | | | | | | | | | | |
| GraffitiInfo | | | | | | | | | | | X | |

---

## Risk Assessment

| Risk | Mitigation |
|------|-----------|
| Test helper `Server` construction breaks across hundreds of test files | Phase 2-3 only update REST test files; gRPC tests are deleted in Phase 4, not modified. |
| `GenericBeaconBlock` proto type still couples REST to v1alpha1 protos | Acceptable — proto types are generated code and used as data containers. Can be replaced with native types in a future pass. |
| `setExecutionData` is a static function but lives in `proposer_bellatrix.go` with non-static functions | Split file during move: static functions → `execution_data.go`, receiver methods → `builder_bid.go`. |
| `computeStateRoot` → `handleStateRootError` strips block fields on retry; may behave differently outside gRPC context | The retry logic is state-transition-level, not transport-level. No behavioral change expected. |
| `unblinder.go` is only called from gRPC `ProposeBeaconBlock` | Move it anyway for completeness, or leave it in the gRPC package and delete with Phase 4. Either works — leaving it avoids moving dead code. |

---

## Line Count Summary

| Category | Lines | Disposition |
|----------|-------|-------------|
| Shared block-production code (extracted) | ~14,000 | Moved to `core/blockproduction/` |
| Shared block-production tests (extracted) | ~4,900 | Moved to `core/blockproduction/` |
| gRPC-only handler code (deleted) | ~5,500 | Deleted in Phase 4 |
| gRPC-only test code (deleted) | ~9,000 | Deleted in Phase 4 |
| Mock files (deleted) | ~2,199 | Deleted in Phase 4 |
| Validator client gRPC adapter (deleted) | ~741 | Deleted in Phase 5 |
| **Total lines removed from gRPC package** | **~22,300** | |
