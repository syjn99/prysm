# gRPC Validator API Removal Log

## Summary

Removed **7,503 lines** of deprecated gRPC validator API code from
`beacon-chain/rpc/prysm/v1alpha1/validator/` while keeping the project buildable.

**Approach:** Embedded `UnimplementedBeaconNodeValidatorServer` in the `Server` struct,
enabling incremental deletion of handler methods that fall back to unimplemented stubs.

## What Was Removed

### Handler Implementations (28 of 32 methods removed)

| File | Methods Removed | Lines |
|------|----------------|-------|
| `exit.go` | ProposeExit | 60 |
| `blocks.go` | StreamBlocksAltair, StreamSlots + sendVerifiedBlocks, sendBlocks | 262 |
| `sync_committee.go` | GetSyncMessageBlockRoot, SubmitSyncMessage, GetSyncSubcommitteeIndex, GetSyncCommitteeContribution, SubmitSignedContributionAndProof, AggregatedSigAndAggregationBits | 144 |
| `attester.go` | GetAttestationData, ProposeAttestation, ProposeAttestationElectra, SubscribeCommitteeSubnets + proposeAtt | 232 |
| `aggregator.go` | SubmitAggregateSelectionProof, SubmitAggregateSelectionProofElectra, SubmitSignedAggregateSelectionProof, SubmitSignedAggregateSelectionProofElectra + processAggregateSelection, bestAggregate | 204 |
| `status.go` | ValidatorStatus, MultipleValidatorStatus, CheckDoppelGanger + activationStatus, validatorStatus, assignmentStatus, statusForPubKey, depositStatus, checkValidatorsAreRecent, optimisticStatus (moved to proposer.go) | 448 |
| `duties.go` | GetDuties, AssignValidatorToSubnet + duties helper | 191 |
| `duties_v2.go` | GetDutiesV2 + dutiesv2, stateForEpoch, loadDutiesMetadata, etc. | 304 |
| `server.go` | WaitForActivation, ValidatorIndex, DomainData, WaitForChainStart + computeDomainData | 150 |
| `proposer.go` | PrepareBeaconProposer, GetFeeRecipientByPubKey, SubmitValidatorRegistrations | 107 |

**Handler subtotal: ~2,102 lines**

### Test Files (13 files gutted)

| File | Lines Removed |
|------|--------------|
| `exit_test.go` | 149 |
| `blocks_test.go` | 471 |
| `sync_committee_test.go` | 210 |
| `attester_test.go` | 654 |
| `attester_mainnet_test.go` | 97 |
| `aggregator_test.go` | 621 |
| `status_test.go` | 1,297 |
| `status_mainnet_test.go` | 100 |
| `duties_test.go` | 512 |
| `duties_v2_test.go` | 592 |
| `server_test.go` | 379 |
| `server_mainnet_test.go` | 112 |
| `proposer_test.go` (partial) | 208 |

**Test subtotal: ~5,402 lines**

## What Was Kept (and Why)

### Methods Kept (4 of 32)

| Method | File | Reason |
|--------|------|--------|
| `GetBeaconBlock` | proposer.go | REST API delegates to it via `s.V1Alpha1Server.GetBeaconBlock()` in `eth/beacon/handlers.go:882` and `eth/validator/handlers_block.go:104` |
| `ProposeBeaconBlock` | proposer.go | REST API delegates to it via `s.V1Alpha1ValidatorServer.ProposeBeaconBlock()` in `eth/beacon/handlers.go:882` |
| `BuildBlockParallel` | proposer.go | Exported helper called by REST `ProduceBlockV3` handler |
| `optimisticStatus` | proposer.go | Called by GetBeaconBlock (moved from status.go) |

### Shared Block-Production Code (~14,000 lines)

These files contain helpers reachable from `GetBeaconBlock` → `BuildBlockParallel`:

- `proposer.go` (remaining: getParentState, computeStateRoot, handleStateRootError, broadcastReceiveBlock, etc.)
- `proposer_altair.go` (8,033 lines)
- `proposer_attestations.go` (21,808 lines)
- `proposer_attestations_electra.go` (3,082 lines)
- `proposer_bellatrix.go` (18,227 lines)
- `proposer_builder.go` (3,703 lines)
- `proposer_capella.go` (969 lines)
- `proposer_deneb.go` - `BuildBlobSidecars` called from `eth/beacon/handlers.go:1448`
- `proposer_deposits.go` (11,200 lines)
- `proposer_empty_block.go` (2,817 lines)
- `proposer_eth1data.go` (8,012 lines)
- `proposer_execution_payload.go` (14,846 lines)
- `proposer_exits.go` (499 lines)
- `proposer_slashings.go` (1,887 lines)
- `proposer_sync_aggregate.go` (3,019 lines)
- `construct_generic_block.go` (5,790 lines)
- `unblinder.go` (2,290 lines)

**To remove these, the block-production logic must first be extracted from the gRPC package
into a shared library that both REST and gRPC can use.**

### Server Struct

The `Server` struct in `server.go` is kept because the REST API holds a reference to it
via `V1Alpha1Server eth.BeaconNodeValidatorServer` interface in `eth/validator/server.go:32`.

### Other Kept Files

- `log.go` - Package logger, still needed
- `validator_test.go` - Contains `TestMain`, still needed by remaining proposer tests
- All `proposer_*_test.go` files - Tests for shared block-production code

## Surprises

1. **ProposeBeaconBlock is called from REST** (`eth/beacon/handlers.go:882`) - was initially
   classified as removable but the REST beacon API block submission handler delegates to it.

2. **No Gazelle changes needed** - Despite removing code from many files and changing imports,
   `bazel run //:gazelle -- fix` produced no BUILD file changes.

3. **No `mustEmbedUnimplementedBeaconNodeValidatorServer`** - The proto codegen uses the older
   gRPC style without the embed requirement, making the `UnimplementedBeaconNodeValidatorServer`
   embedding straightforward.

## Commits

1. `68e0667a18` - refactor(server.go): embed UnimplementedBeaconNodeValidatorServer
2. `7d0d0dd0c5` - remove(grpc): delete isolated gRPC handler implementations (exit, blocks, sync_committee, attester, aggregator)
3. `1216981f40` - remove(grpc): delete status, duties, and server handler implementations
4. `35e7360452` - remove(grpc): delete PrepareBeaconProposer, GetFeeRecipientByPubKey, SubmitValidatorRegistrations
5. `a087fad2b8` - remove(grpc): delete tests for removed gRPC handler implementations

## Next Steps

To remove the remaining ~14,000 lines of shared block-production code:

1. **Extract `BuildBlockParallel` and its call tree** into a shared package (e.g., `beacon-chain/rpc/core/blockbuilder/`)
2. Update both REST and gRPC server to import from the shared package
3. Remove the remaining proposer_*.go files from the v1alpha1 validator package
4. Remove the gRPC server registration in `beacon-chain/rpc/service.go:329`
5. Clean up mock files in `testing/mock/` (4 files, ~2,199 lines)
6. Consider removing the validator client gRPC adapter (`validator/client/grpc-api/`, ~741 lines)
