# gRPC vs REST Endpoint Parity Audit

Audit of every RPC method in the deprecated `BeaconNodeValidator` gRPC service
(`proto/prysm/v1alpha1/validator.proto`, lines 40–442) against the REST handlers
in `beacon-chain/rpc/eth/`.

All 32 RPC methods in the service carry `option deprecated = true`.

File path prefix (gRPC): `beacon-chain/rpc/prysm/v1alpha1/validator/`
File path prefix (REST): `beacon-chain/rpc/eth/`

---

## Endpoint Parity Table

| # | gRPC Method | gRPC File:Line | REST Equivalent | REST File:Line | Status |
|---|-------------|---------------|-----------------|---------------|--------|
| 1 | `GetDuties` | `duties.go:24` | `GetAttesterDuties` + `GetProposerDuties` | `validator/handlers.go:853` + `validator/handlers.go:987` | covered |
| 2 | `GetDutiesV2` | `duties_v2.go:24` | `GetAttesterDuties` + `GetProposerDuties` | `validator/handlers.go:853` + `validator/handlers.go:987` | covered |
| 3 | `DomainData` | `server.go:161` | — | — | **gap** |
| 4 | `WaitForChainStart` (stream) | `server.go:195` | `GetGenesis` (poll) | `beacon/handlers.go:1414` | partial |
| 5 | `WaitForActivation` (stream) | `server.go:95` | `GetValidators` (poll) | `beacon/handlers_validator.go:30` | partial |
| 6 | `ValidatorIndex` | `server.go:142` | `GetValidator` (by pubkey) | `beacon/handlers_validator.go:179` | covered |
| 7 | `ValidatorStatus` | `status.go:44` | `GetValidator` / `GetValidators` | `beacon/handlers_validator.go:179` / `:30` | covered |
| 8 | `MultipleValidatorStatus` | `status.go:61` | `GetValidators` (POST) | `beacon/handlers_validator.go:30` | covered |
| 9 | `GetBeaconBlock` | `proposer.go:54` | `ProduceBlockV3` | `validator/handlers_block.go:44` | covered |
| 10 | `ProposeBeaconBlock` | `proposer.go:285` | `PublishBlockV2` | `beacon/handlers.go:586` | covered |
| 11 | `PrepareBeaconProposer` | `proposer.go:530` | `PrepareBeaconProposer` | `validator/handlers.go:800` | covered |
| 12 | `GetFeeRecipientByPubKey` | `proposer.go:575` | — | — | **gap** |
| 13 | `GetAttestationData` | `attester.go:29` | `GetAttestationData` | `validator/handlers.go:620` | covered |
| 14 | `ProposeAttestation` | `attester.go:51` | `SubmitAttestationsV2` | `beacon/handlers_pool.go:129` | covered |
| 15 | `ProposeAttestationElectra` | `attester.go:84` | `SubmitAttestationsV2` | `beacon/handlers_pool.go:129` | covered |
| 16 | `SubmitAggregateSelectionProof` | `aggregator.go:25` | `GetAggregateAttestationV2` | `validator/handlers.go:48` | covered |
| 17 | `SubmitAggregateSelectionProofElectra` | `aggregator.go:63` | `GetAggregateAttestationV2` | `validator/handlers.go:48` | covered |
| 18 | `SubmitSignedAggregateSelectionProof` | `aggregator.go:156` | `SubmitAggregateAndProofsV2` | `validator/handlers.go:298` | covered |
| 19 | `SubmitSignedAggregateSelectionProofElectra` | `aggregator.go:170` | `SubmitAggregateAndProofsV2` | `validator/handlers.go:298` | covered |
| 20 | `ProposeExit` | `exit.go:18` | `SubmitVoluntaryExit` | `beacon/handlers_pool.go:425` | covered |
| 21 | `SubscribeCommitteeSubnets` | `attester.go:126` | `SubmitBeaconCommitteeSubscription` | `validator/handlers.go:533` | covered |
| 22 | `CheckDoppelGanger` | `status.go:110` | — | — | **gap** |
| 23 | `GetSyncMessageBlockRoot` | `sync_committee.go:18` | `GetBlockRoot` (head) | `beacon/handlers.go:1041` | covered |
| 24 | `SubmitSyncMessage` | `sync_committee.go:41` | `SubmitSyncCommitteeSignatures` | `beacon/handlers_pool.go:483` | covered |
| 25 | `GetSyncSubcommitteeIndex` | `sync_committee.go:52` | `GetSyncCommitteeDuties` | `validator/handlers.go:1097` | covered |
| 26 | `GetSyncCommitteeContribution` | `sync_committee.go:70` | `ProduceSyncCommitteeContribution` | `validator/handlers.go:676` | covered |
| 27 | `SubmitSignedContributionAndProof` | `sync_committee.go:113` | `SubmitContributionAndProofs` | `validator/handlers.go:222` | covered |
| 28 | `StreamSlots` (stream) | `blocks.go:55` | `StreamEvents` (SSE, topic `slot`) | `events/events.go:167` | partial |
| 29 | `StreamBlocksAltair` (stream) | `blocks.go:20` | `StreamEvents` (SSE, topic `block`) | `events/events.go:167` | partial |
| 30 | `SubmitValidatorRegistrations` | `proposer.go:691` | `RegisterValidator` | `validator/handlers.go:759` | covered |
| 31 | `AssignValidatorToSubnet` | `duties.go:188` | — | — | **gap** |
| 32 | `AggregatedSigAndAggregationBits` | `sync_committee.go:127` | (internal to `ProduceSyncCommitteeContribution`) | `validator/handlers.go:735` | partial |

---

## Status Definitions

| Status | Meaning |
|--------|---------|
| **covered** | A direct REST equivalent exists in the standard Beacon API. |
| **partial** | REST provides the functionality through a different mechanism (polling, SSE, or as an internal component of another endpoint). |
| **gap** | No REST equivalent exists. The functionality is missing or is Prysm-specific with no standard Beacon API counterpart. |

---

## Gap Analysis

### Hard Gaps (no REST equivalent at all)

1. **`DomainData`** — Returns the signing domain for a given epoch and domain type.
   Prysm-specific helper. REST validator clients are expected to compute the
   signing domain locally using data from `GET /eth/v1/beacon/genesis` and
   `GET /eth/v1/config/fork_schedule`. Not part of the Beacon API standard.

2. **`GetFeeRecipientByPubKey`** — Returns the fee recipient address for a
   validator public key. Prysm-specific endpoint backed by the proposer
   settings cache. No standard Beacon API equivalent exists; the
   `PrepareBeaconProposer` endpoint only *sets* fee recipients but does not
   provide a read-back.

3. **`CheckDoppelGanger`** — Checks whether a validator was recently active to
   detect duplicate instances. Entirely Prysm-specific; there is no standard
   Beacon API endpoint for doppelganger detection.

4. **`AssignValidatorToSubnet`** — Assigns a validator to an attestation subnet.
   Prysm-specific internal mechanism. In the REST flow, subnet assignment is
   handled as a side effect of `SubmitBeaconCommitteeSubscription` and
   `SubmitSyncCommitteeSubscription`.

### Partial Coverage

5. **`WaitForChainStart`** (server-streaming) — In REST, clients poll
   `GET /eth/v1/beacon/genesis` until it returns successfully. No push
   notification equivalent.

6. **`WaitForActivation`** (server-streaming) — In REST, clients poll
   `GET /eth/v1/beacon/states/head/validators` periodically. No push
   notification equivalent.

7. **`StreamSlots`** (server-streaming) — Replaced by the SSE endpoint
   `GET /eth/v1/events?topics=slot`. Semantics differ: SSE is a single
   multiplexed stream vs. dedicated gRPC stream.

8. **`StreamBlocksAltair`** (server-streaming) — Replaced by the SSE endpoint
   `GET /eth/v1/events?topics=block`. Same SSE vs. gRPC stream difference.

9. **`AggregatedSigAndAggregationBits`** — Not exposed as a standalone REST
   endpoint. The functionality is used internally by
   `ProduceSyncCommitteeContribution` (line 735 in `validator/handlers.go`).
   External callers use `ProduceSyncCommitteeContribution` directly.

---

## Summary

| Category | Count |
|----------|-------|
| Total gRPC methods | 32 |
| **Covered** (direct REST equivalent) | 23 |
| **Partial** (alternative REST mechanism) | 5 |
| **Gap** (no REST equivalent) | 4 |

### Breakdown

- **23 covered**: Standard Beacon API has a direct equivalent endpoint.
- **5 partial**: Functionality exists via polling or SSE or as an internal
  component of another REST endpoint, but not as a 1:1 standalone endpoint.
- **4 gaps**: Prysm-specific endpoints (`DomainData`, `GetFeeRecipientByPubKey`,
  `CheckDoppelGanger`, `AssignValidatorToSubnet`) with no Beacon API standard
  counterpart. These require either:
  - A Prysm-specific REST extension, or
  - Migration of the logic into the validator client (e.g., local domain
    computation), or
  - Deprecation of the feature itself.

### Migration Risk Assessment

| Gap | Risk | Suggested Resolution |
|-----|------|---------------------|
| `DomainData` | **Low** | Client computes locally from genesis + fork schedule data |
| `GetFeeRecipientByPubKey` | **Medium** | Add Prysm-specific REST extension or extend PrepareBeaconProposer |
| `CheckDoppelGanger` | **Medium** | Add Prysm-specific REST extension (`/eth/v1/prysm/validators/doppelganger`) |
| `AssignValidatorToSubnet` | **Low** | Already handled as side effect of subscription endpoints |
