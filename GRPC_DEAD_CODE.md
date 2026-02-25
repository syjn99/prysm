# gRPC-Only Dependencies: Dead Code Audit

Identifies code that exists solely to support the deprecated v1alpha1 validator
gRPC API and could be removed along with it.

---

## Critical Architectural Constraint

The REST API's `ProduceBlockV3` handler delegates block production to the gRPC
`Server` struct:

```
beacon-chain/rpc/eth/validator/handlers_block.go:104
    v1alpha1resp, err := s.V1Alpha1Server.GetBeaconBlock(ctx, v1alpha1req)
```

The REST `Server` holds a `V1Alpha1Server` field (see `eth/validator/server.go:32`).
This means the entire block-production pipeline is **shared code** that cannot be
deleted without first extracting it into a standalone package. This is the single
biggest obstacle to removing the gRPC validator API.

---

## 1. Helper Functions (gRPC-only)

All unexported functions in `beacon-chain/rpc/prysm/v1alpha1/validator/` are
package-private and called only from gRPC handlers (directly or transitively).
They become dead code if the package is removed.

| Symbol | File:Line | gRPC-Only? | Notes |
|--------|-----------|------------|-------|
| `computeDomainData` | `server.go:183` | yes | Helper for `DomainData` RPC only |
| `activationStatus` | `status.go:228` | yes | Helper for `WaitForActivation` stream |
| `optimisticStatus` | `status.go:269` | yes | Guards multiple gRPC handlers |
| `validatorStatus` | `status.go:285` | yes | Shared by `ValidatorStatus`, `MultipleValidatorStatus`, `WaitForActivation` |
| `checkValidatorsAreRecent` | `status.go:378` | yes | Helper for `CheckDoppelGanger` |
| `statusForPubKey` | `status.go:404` | yes | Helper for `validatorStatus` |
| `assignmentStatus` | `status.go:415` | yes | Used by duties and status paths |
| `depositStatus` | `status.go:442` | yes | Used by `assignmentStatus` and `validatorStatus` |
| `duties` | `duties.go:34` | yes | Core impl behind `GetDuties` |
| `dutiesv2` | `duties_v2.go:34` | yes | Core impl behind `GetDutiesV2` |
| `stateForEpoch` | `duties_v2.go:135` | yes | Helper for `dutiesv2` |
| `loadDutiesMetadata` | `duties_v2.go:167` | yes | Helper for `dutiesv2` |
| `loadMetadata` | `duties_v2.go:187` | yes | Helper for `loadDutiesMetadata` |
| `findValidatorIndexInCommittee` | `duties_v2.go:210` | yes | Helper for `getValidatorAssignment` |
| `getValidatorAssignment` | `duties_v2.go:220` | yes | Helper for `buildValidatorDuty` |
| `buildValidatorDuty` | `duties_v2.go:234` | yes | Helper for `dutiesv2` |
| `populateCommitteeFields` | `duties_v2.go:296` | yes | Helper for `buildValidatorDuty` |
| `proposeAtt` | `attester.go:177` | yes | Shared by `ProposeAttestation` and `ProposeAttestationElectra` |
| `processAggregateSelection` | `aggregator.go:101` | yes | Shared by both aggregate selection RPCs |
| `bestAggregate` | `aggregator.go:184` | yes | Helper for aggregate selection |
| `sendVerifiedBlocks` | `blocks.go:113` | yes | Helper for `StreamBlocksAltair` |
| `sendBlocks` | `blocks.go:209` | yes | Helper for `StreamBlocksAltair` |
| `constructGenericBeaconBlock` | `construct_generic_block.go:16` | **no** | Called by `BuildBlockParallel`; REST API reaches this via `V1Alpha1Server.GetBeaconBlock` |
| `constructPhase0Block` .. `constructFuluBlock` | `construct_generic_block.go` | **no** | Same reason — reachable from REST path |
| `getParentState` | `proposer.go:188` | **no** | Called by `GetBeaconBlock`; reachable from REST |
| `getParentStateFromReorgData` | `proposer.go:166` | **no** | Called by `getParentState` |
| `handleSuccesfulReorgAttempt` | `proposer.go:130` | **no** | Called by `getParentStateFromReorgData` |
| `logFailedReorgAttempt` | `proposer.go:144` | **no** | Called by `getParentStateFromReorgData` |
| `getHeadNoReorg` | `proposer.go:153` | **no** | Called by `getParentStateFromReorgData` |
| `broadcastReceiveBlock` | `proposer.go:445` | yes | Called by `ProposeBeaconBlock` (REST uses `PublishBlockV2` instead) |
| `broadcastBlock` | `proposer.go:462` | yes | Called by `broadcastReceiveBlock` |
| `broadcastAndReceiveSidecars` | `proposer.go:357` | yes | Called by `ProposeBeaconBlock` |
| `broadcastAndReceiveBlobs` | `proposer.go:482` | yes | Called by `broadcastAndReceiveSidecars` |
| `broadcastAndReceiveDataColumns` | `proposer.go:508` | yes | Called by `broadcastAndReceiveSidecars` |
| `handleBlindedBlock` | `proposer.go:380` | yes | Called by `ProposeBeaconBlock` |
| `handleUnblindedBlock` | `proposer.go:411` | yes | Called by `ProposeBeaconBlock` |
| `computeStateRoot` | `proposer.go:615` | **no** | Called by `BuildBlockParallel`; reachable from REST |
| `handleStateRootError` | `proposer.go:639` | **no** | Called by `computeStateRoot` |
| `blobsAndProofs` | `proposer.go:708` | yes | Called by `handleUnblindedBlock` |
| `getEmptyBlock` | `proposer_empty_block.go:14` | **no** | Called by `GetBeaconBlock`; reachable from REST |
| `eth1DataMajorityVote` | `proposer_eth1data.go:36` | **no** | Called by `BuildBlockParallel` |
| `slotStartTime` | `proposer_eth1data.go:98` | **no** | Called by `canonicalEth1Data` |
| `canonicalEth1Data` | `proposer_eth1data.go:104` | **no** | Called by `eth1DataMajorityVote` |
| `mockETH1DataVote` | `proposer_eth1data.go:136` | **no** | Called by `eth1DataMajorityVote` |
| `randomETH1DataVote` | `proposer_eth1data.go:166` | **no** | Called by `eth1DataMajorityVote` |
| `packDepositsAndAttestations` | `proposer_deposits.go:24` | **no** | Called by `BuildBlockParallel` |
| `deposits` | `proposer_deposits.go:78` | **no** | Called by `packDepositsAndAttestations` |
| `depositTrie` | `proposer_deposits.go:174` | **no** | Called by `deposits` |
| `rebuildDepositTrie` | `proposer_deposits.go:217` | **no** | Called by `depositTrie` |
| `validateDepositTrie` | `proposer_deposits.go:244` | **no** | Called by `depositTrie` |
| `constructMerkleProof` | `proposer_deposits.go:261` | **no** | Called by `deposits` |
| `shouldRebuildTrie` | `proposer_deposits.go:274` | **no** | Called by `depositTrie` |
| `packAttestations` | `proposer_attestations.go:32` | **no** | Called by `BuildBlockParallel` |
| `onChainAggregates` | `proposer_attestations.go:130` | **no** | Called by `packAttestations` |
| `sortSlotAttestations` | `proposer_attestations.go:319` | **no** | Called by `packAttestations` |
| `attestationFields` | `proposer_attestations.go:637` | **no** | Called by `packAttestations` |
| `computeOnChainAggregate` | `proposer_attestations_electra.go:42` | **no** | Called by `packAttestations` |
| `setSyncAggregate` | `proposer_altair.go:24` | **no** | Called by `BuildBlockParallel` |
| `getSyncAggregate` | `proposer_altair.go:51` | **no** | Called by `setSyncAggregate` |
| `aggregateSyncSubcommitteeMessages` | `proposer_altair.go:200` | **no** | Called by `getSyncAggregate` |
| `setExecutionData` | `proposer_bellatrix.go:55` | **no** | Called by `BuildBlockParallel` |
| `validateBuilderSignature` | `proposer_bellatrix.go:324` | **no** | Called by builder path |
| `matchingWithdrawalsRoot` | `proposer_bellatrix.go:344` | **no** | Called by builder path |
| `setLocalExecution` | `proposer_bellatrix.go:370` | **no** | Called by `setExecutionData` |
| `setBuilderExecution` | `proposer_bellatrix.go:385` | **no** | Called by `setExecutionData` |
| `setExecution` | `proposer_bellatrix.go:391` | **no** | Called by `set*Execution` |
| `expectedGasLimit` | `proposer_bellatrix.go:446` | **no** | Called by builder path |
| `isVersionCompatible` | `proposer_bellatrix.go:465` | **no** | Called by builder path |
| `getLocalPayload` | `proposer_execution_payload.go:51` | **no** | Called by `BuildBlockParallel` |
| `getLocalPayloadFromEngine` | `proposer_execution_payload.go:68` | **no** | Called by `getLocalPayload` |
| `setFeeRecipientIfBurnAddress` | `proposer_execution_payload.go:44` | **no** | Called by `getLocalPayloadFromEngine` |
| `warnIfFeeRecipientDiffers` | `proposer_execution_payload.go:199` | **no** | Called by `getLocalPayloadFromEngine` |
| `getTerminalBlockHashIfExists` | `proposer_execution_payload.go:222` | **no** | Called by `getLocalPayloadFromEngine` |
| `getBuilderPayloadAndBlobs` | `proposer_execution_payload.go:240` | **no** | Called by `BuildBlockParallel` |
| `getParentBlockHash` | `proposer_execution_payload.go:275` | **no** | Called by `getLocalPayloadFromEngine` |
| `activationEpochNotReached` | `proposer_execution_payload.go:337` | **no** | Called by `getLocalPayloadFromEngine` |
| `emptyPayload` .. `emptyPayloadDeneb` | `proposer_execution_payload.go` | **no** | Called by `getLocalPayloadFromEngine` |
| `getExits` | `proposer_exits.go:9` | **no** | Called by `BuildBlockParallel` |
| `getSlashings` | `proposer_slashings.go:13` | **no** | Called by `BuildBlockParallel` |
| `setBlsToExecData` | `proposer_capella.go:11` | **no** | Called by `BuildBlockParallel` |
| `canUseBuilder` | `proposer_builder.go:19` | **no** | Called by builder path |
| `validatorRegistered` | `proposer_builder.go:39` | **no** | Called by `canUseBuilder` |
| `circuitBreakBuilder` | `proposer_builder.go:54` | **no** | Called by `canUseBuilder` |
| `unblindBlobsSidecars` | `unblinder.go:15` | yes | Called only by `handleBlindedBlock` (gRPC propose path) |

---

## 2. Exported Symbols

| Symbol | File:Line | gRPC-Only? | Notes |
|--------|-----------|------------|-------|
| `Server` (struct) | `server.go:46` | **no** | REST `eth/validator` holds it as `V1Alpha1Server` |
| `BuildBlockParallel` | `proposer.go:198` | **no** | Called by `GetBeaconBlock` which REST delegates to |
| `BuildBlobSidecars` | `proposer_deneb.go:13` | **no** | Called by REST handler `beacon/handlers.go:1448` |

No exported functions or types in this package are gRPC-only. The `Server` struct
and its `BuildBlockParallel` / `BuildBlobSidecars` methods are shared with REST.

---

## 3. Proto Message Types (gRPC-only on the server side)

These proto types are used by gRPC server handlers AND by the validator client
(which also has a `beacon-api` REST adapter). The types themselves live in
generated `.pb.go` files and cannot be deleted without removing the proto
definitions. They are listed here as proto definitions that become dead once both
the gRPC server AND the validator client gRPC adapter are removed.

| Proto Type | Server Usage | Also Used By Validator Client? | gRPC-Only? |
|------------|-------------|-------------------------------|------------|
| `ValidatorActivationRequest` | `server.go` | no (REST client polls `GetValidators`) | **yes** |
| `ValidatorActivationResponse` | `server.go` | no | **yes** |
| `ChainStartResponse` | `server.go` | no (REST client polls `/eth/v1/beacon/genesis`) | **yes** |
| `ValidatorIndexRequest` | `server.go` | `grpc_validator_client.go`, `index.go` | no |
| `ValidatorIndexResponse` | `server.go` | `grpc_validator_client.go`, `index.go` | no |
| `DomainRequest` | `server.go` | `grpc_validator_client.go`, `domain_data.go` | no |
| `DomainResponse` | `server.go` | `grpc_validator_client.go`, `domain_data.go` | no |
| `DutiesResponse` | `duties.go` | `grpc_validator_client.go`, `duties.go` | no |
| `DutiesV2Response` | `duties_v2.go` | `grpc_validator_client.go` | no |
| `StreamBlocksRequest` | `blocks.go` | `stream_blocks.go` | no |
| `StreamBlocksResponse` | `blocks.go` | `stream_blocks.go` | no |
| `StreamSlotsRequest` | `blocks.go` | `grpc_validator_client_test.go` only | **yes** |
| `StreamSlotsResponse` | `blocks.go` | `grpc_validator_client_test.go` only | **yes** |
| `AggregateSelectionResponse` | `aggregator.go` | `submit_aggregate_selection_proof.go` | no |
| `AggregateSelectionElectraResponse` | `aggregator.go` | `submit_aggregate_selection_proof.go` | no |
| `SignedAggregateSubmitRequest` | `aggregator.go` | `grpc_validator_client.go` | no |
| `SignedAggregateSubmitElectraRequest` | `aggregator.go` | `grpc_validator_client.go` | no |
| `SignedAggregateSubmitResponse` | `aggregator.go` | `grpc_validator_client.go` | no |
| `DoppelGangerRequest` | `status.go` | `grpc_validator_client.go`, `doppelganger.go` | no |
| `DoppelGangerResponse` | `status.go` | `grpc_validator_client.go`, `doppelganger.go` | no |
| `GenericBeaconBlock` | `proposer.go` | `propose.go`, `get_beacon_block.go`, `blocks/factory.go` | no |
| `GenericSignedBeaconBlock` | `proposer.go` | `propose.go`, `propose_beacon_block.go` | no |
| `BlockRequest` | `proposer.go` | `grpc_validator_client.go`, `get_beacon_block.go` | no |
| `ProposeResponse` | `proposer.go` | `grpc_validator_client.go`, `propose_beacon_block.go` | no |
| `FeeRecipientByPubKeyRequest` | `proposer.go` | `grpc_validator_client.go` | no |
| `FeeRecipientByPubKeyResponse` | `proposer.go` | `grpc_validator_client.go` | no |
| `AssignValidatorToSubnetRequest` | `duties.go` | `grpc_validator_client.go` | no |
| `AggregatedSigAndAggregationBitsRequest` | `sync_committee.go` | used internally by REST `ProduceSyncCommitteeContribution` | no |
| `AggregatedSigAndAggregationBitsResponse` | `sync_committee.go` | no direct external usage | **yes** |

---

## 4. Mock Files (gRPC-only)

Auto-generated mocks for the `BeaconNodeValidator` gRPC interface. These become
dead code when the gRPC service is removed.

| File | Lines | gRPC-Only? | Notes |
|------|-------|------------|-------|
| `testing/mock/beacon_validator_server_mock.go` | 876 | **yes** | MockGen for `BeaconNodeValidatorServer` |
| `testing/mock/beacon_validator_client_mock.go` | 1,053 | **yes** | MockGen for `BeaconNodeValidatorClient` |
| `testing/mock/beacon_altair_validator_server_mock.go` | 133 | **yes** | MockGen for Altair streaming server |
| `testing/mock/beacon_altair_validator_client_mock.go` | 137 | **yes** | MockGen for Altair streaming client |
| **Total** | **2,199** | | |

Generator config: `hack/update-mockgen.sh` lines 12–21.

**Note:** The REST handler tests (`eth/validator/handlers_block_test.go`) use these
mocks because they test the `V1Alpha1Server` delegation. Once block production is
extracted from the gRPC `Server` struct, these mocks become unnecessary.

---

## 5. Test Files (gRPC-only)

All test files in `beacon-chain/rpc/prysm/v1alpha1/validator/` test the gRPC
handlers. They become dead code when the package is removed.

| File | Lines | Notes |
|------|-------|-------|
| `proposer_test.go` | 3,742 | Largest test file |
| `status_test.go` | 1,298 | |
| `proposer_bellatrix_test.go` | 1,420 | |
| `proposer_attestations_test.go` | 944 | |
| `attester_test.go` | 655 | |
| `aggregator_test.go` | 622 | |
| `duties_v2_test.go` | 593 | |
| `duties_test.go` | 513 | |
| `blocks_test.go` | 472 | |
| `proposer_sync_aggregate_test.go` | 468 | |
| `proposer_execution_payload_test.go` | 402 | |
| `server_test.go` | 380 | |
| `proposer_altair_test.go` | 293 | |
| `proposer_deneb_bench_test.go` | 214 | |
| `proposer_deposits_test.go` | 213 | |
| `proposer_builder_test.go` | 190 | |
| `construct_generic_block_test.go` | 177 | |
| `proposer_attestations_electra_test.go` | 163 | |
| `unblinder_test.go` | 150 | |
| `exit_test.go` | 150 | |
| `sync_committee_test.go` | 211 | |
| `server_mainnet_test.go` | 113 | |
| `status_mainnet_test.go` | 101 | |
| `attester_mainnet_test.go` | 98 | |
| `proposer_empty_block_test.go` | 92 | |
| `proposer_slashings_test.go` | 48 | |
| `proposer_exits_test.go` | 37 | |
| `proposer_deneb_test.go` | 36 | |
| `validator_test.go` | 21 | Test main with minimal config |
| **Total** | **13,885** | |

---

## 6. Validator Client gRPC Adapter (gRPC-only)

The validator client has both a gRPC adapter and a REST (`beacon-api`) adapter.
The gRPC adapter becomes dead code once the server-side gRPC API is removed.

| File | Lines | gRPC-Only? | Notes |
|------|-------|------------|-------|
| `validator/client/grpc-api/grpc_validator_client.go` | 393 | **yes** | Wraps gRPC stubs |
| `validator/client/grpc-api/grpc_validator_client_test.go` | 348 | **yes** | Tests for above |
| **Total** | **741** | | |

---

## 7. Service Registration (gRPC-only)

| Symbol | File:Line | gRPC-Only? | Notes |
|--------|-----------|------------|-------|
| `RegisterBeaconNodeValidatorServer(...)` | `beacon-chain/rpc/service.go:329` | **yes** | Single registration point |

---

## Summary

| Category | Lines | gRPC-Only Lines | Notes |
|----------|-------|----------------|-------|
| Server implementation (non-test `.go`) | 19,589 | ~5,500 | ~14,000 lines are shared block-production logic reachable from REST |
| Server tests (`*_test.go`) | 13,885 | 13,885 | All tests are gRPC-specific |
| Mock files (`testing/mock/`) | 2,199 | 2,199 | All mock files are gRPC-specific |
| Validator client gRPC adapter | 741 | 741 | Replaced by `beacon-api` adapter |
| Mock generation config | ~10 | ~10 | `hack/update-mockgen.sh` lines 12–21 |
| Service registration | 1 | 1 | `beacon-chain/rpc/service.go:329` |
| **Estimated total removable** | | **~22,300** | |

### Prerequisite: Extract Shared Block-Production Logic

Before the gRPC validator API can be removed, the following must be extracted
into a standalone package (not inside the gRPC handler directory):

1. `BuildBlockParallel` and its full call tree (~14,000 lines across
   `proposer*.go`, `construct_generic_block.go`, `proposer_deposits.go`,
   `proposer_attestations*.go`, `proposer_altair.go`, `proposer_bellatrix.go`,
   `proposer_capella.go`, `proposer_deneb.go`, `proposer_execution_payload.go`,
   `proposer_exits.go`, `proposer_slashings.go`, `proposer_sync_aggregate.go`,
   `proposer_builder.go`, `proposer_empty_block.go`, `proposer_eth1data.go`)
2. `BuildBlobSidecars` (already exported, but defined inside the gRPC package)
3. The `Server` struct fields that these functions depend on (can become a
   `BlockBuilder` service or similar)

Once extracted, the remaining ~5,500 lines of gRPC-only handler code plus
~16,800 lines of tests/mocks can be cleanly deleted.
