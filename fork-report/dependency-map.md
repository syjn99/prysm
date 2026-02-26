# Cross-Cutting Dependency Map

> Where fork-specific code is referenced across the codebase, and what removing old forks would touch.

## Overview

Removing forks Phase0 through Deneb ("old forks") requires changes across **7 major subsystems**. This document maps each subsystem's fork dependencies and estimates the blast radius.

---

## 1. State Management

**Path**: `beacon-chain/state/state-native/`
**Old fork references**: 107

### What lives here

| Component | Fork Coupling | Files |
|-----------|---------------|-------|
| Field definitions (`state_trie.go`) | `phase0Fields`, `altairFields`, `bellatrixFields`, `capellaFields`, `denebFields` | 1 |
| InitializeFromProto* functions | One per fork (8 total, 5 are "old") | 1 |
| Getters/Setters | Phase0 attestation getters, per-fork execution payload header getters | ~10 |
| Hasher (`hasher.go`) | Version switches for field hashing | 1 |
| Proofs (`proofs.go`) | Version-aware proof generation | 1 |
| Spec parameters (`spec_parameters.go`) | Fork-based if-chains for penalty quotients | 1 |

### Blast radius if old forks removed

- Delete 5 of 8 `InitializeFromProtoUnsafe*` functions (~250 lines)
- Delete `phase0Fields`, simplify field lists
- Remove Phase0 attestation getters/setters (replaced by participation bits in Altair+)
- Simplify hasher and proof version switches
- Remove `LatestExecutionPayloadHeader` (Bellatrix), `...Capella`, `...Deneb` field indices (only `...Bid` remains for Gloas, or Electra/Fulu keep the Deneb header)

**Risk**: High. State is the core data structure. Every change here must be validated against spec tests.

---

## 2. Block Types & Factory

**Path**: `consensus-types/blocks/`
**Old fork references**: 246

### What lives here

| Component | Fork Coupling |
|-----------|---------------|
| `factory.go` | 30+ type-switch cases across `NewSignedBeaconBlock` and `NewBeaconBlock` |
| `getters.go` | Version checks for block body accessors |
| `setters.go` | Version checks for mutations |
| Per-fork init functions | `initSignedBlockFromProtoPhase0`, `...Altair`, `...Bellatrix`, `...Capella`, `...Deneb` |

### Blast radius

- Remove ~20 type-switch cases from factory functions
- Delete 10+ `initBlockFromProto*` / `initSignedBlockFromProto*` for old forks
- Remove blinded block variants for Bellatrix and Capella (blinding started at Bellatrix)
- Simplify `GenericSignedBeaconBlock` oneOf in proto (remove old fork variants)

**Risk**: Medium. Block type construction is well-tested, and removal is straightforward deletion.

---

## 3. Proto Definitions

**Path**: `proto/prysm/v1alpha1/`

### What lives here

| Proto File | Old Fork Content |
|-----------|-----------------|
| `beacon_state.proto` | `BeaconState` (Phase0), `BeaconStateAltair`, `BeaconStateBellatrix`, `BeaconStateCapella`, `BeaconStateDeneb` |
| `beacon_block.proto` | Phase0-Deneb block types + `GenericSignedBeaconBlock` oneOf variants |
| `attestation.proto` | Phase0/Altair attestation types (pre-Electra) |

### Blast radius

- Remove 5 of 8 BeaconState proto messages
- Remove ~600 lines from `beacon_block.proto`
- Regenerate SSZ code: eliminate ~17,000 lines of `*.ssz.go` (phase0 + altair + bellatrix + capella + deneb)
- Regenerate protobuf code: eliminate significant portions of `beacon_block.pb.go` and `beacon_state.pb.go`

**Risk**: Low (generated code), but requires re-running proto and SSZ generators.

---

## 4. State Transition (STF)

**Path**: `beacon-chain/core/transition/`
**Old fork references in transition/**: 25

### What lives here

| Component | Fork Coupling |
|-----------|---------------|
| `transition.go` | `ProcessEpoch()`: version dispatch (Phase0/Altair/Electra/Fulu) |
| `transition.go` | `UpgradeState()`: 7 sequential upgrade calls |
| `transition_no_verify_sig.go` | `processBlock()`: version-aware block processing |
| `state-bellatrix.go` | Bellatrix-specific merge transition handling |

### Blast radius

- `ProcessEpoch()`: remove Phase0 branch (`ProcessEpochPrecompute`)
- `UpgradeState()`: remove Altair/Bellatrix/Capella/Deneb upgrade calls (keep Electra+ only)
- Delete `state-bellatrix.go` (merge transition handling)
- Delete or simplify `processBlock()` version dispatching

**Cascading deletions**:
- `beacon-chain/core/altair/` (5,171 lines) - can be deleted if Altair epoch processing is no longer needed
- `beacon-chain/core/capella/` (202 lines) - upgrade only
- `beacon-chain/core/deneb/` (215 lines) - upgrade only
- `beacon-chain/core/execution/` (172 lines) - Bellatrix upgrade only
- `beacon-chain/core/epoch/` + `precompute/` - Phase0 epoch processing

**Risk**: High. The STF is the consensus-critical path. Removal must not alter behavior for Electra+ states.

---

## 5. Sync Subsystem

**Path**: `beacon-chain/sync/`

### What lives here

| Component | Fork Coupling |
|-----------|---------------|
| `checkpoint/api.go` | Downloads finalized state, auto-detects fork via `encoding/ssz/detect` |
| `initial-sync/service.go` | Round-robin block download, fork-aware verification |
| `backfill/service.go` | Historical block download, tracks `fuluStart`/`denebStart` boundaries |

### Blast radius

- **Checkpoint sync**: Minimal impact. If node starts from Electra+ checkpoint, old fork detection is unused.
- **Backfill**: Must still handle downloading old-fork blocks for historical data. This is the key integration point for the "legacy binary" approach.
- **Initial sync**: Fork-aware verification can be simplified.

**Risk**: Medium. Checkpoint sync is the enabler for removing old forks. Backfill is where the legacy binary becomes necessary.

---

## 6. Fork Choice

**Path**: `beacon-chain/forkchoice/doubly-linked-tree/`

### What lives here

| Component | Fork Coupling |
|-----------|---------------|
| `forkchoice.go` | Core fork choice algorithm (largely fork-agnostic) |
| `gloas.go` | Gloas-specific dual-node structure (empty/full payload) |

### Blast radius

- Fork choice is largely version-agnostic (operates on block roots and weights)
- Gloas introduces new node types but doesn't depend on old fork logic
- Minimal changes needed

**Risk**: Low.

---

## 7. RPC / API Layer

**Path**: `beacon-chain/rpc/`
**Old fork references**: 273

### What lives here

| Component | Fork Coupling |
|-----------|---------------|
| `eth/beacon/handlers_state.go` | Generic handlers via BeaconState interface |
| `prysm/v1alpha1/validator/` | Fork-specific proposer logic, attestation handling |
| `lookup/stater.go` | State fetching and regeneration |

### Blast radius

- API responses include version information; must handle "unknown old fork" gracefully
- Validator duties may reference old fork types in test fixtures
- State lookup/regeneration depends on being able to deserialize old-fork states from DB

**Risk**: Medium. API backward compatibility for queries about historical data.

---

## 8. Database

**Path**: `beacon-chain/db/`

### What lives here

- State stored as SSZ-encoded bytes with fork version auto-detection on read
- Blocks stored similarly with version-aware deserialization

### Blast radius

- Existing databases contain states/blocks from old forks
- If the node can't deserialize old-fork data, it can't read historical state
- **Two options**: (a) migrate DB on upgrade (expensive), or (b) leave old data unreadable and rely on checkpoint sync for the current state

**Risk**: High for option (a), Low for option (b) if checkpoint sync is mandatory.

---

## Summary: Blast Radius by Subsystem

| Subsystem | Old Fork Refs | Estimated Lines Affected | Risk |
|-----------|-------------:|------------------------:|------|
| State (`state-native/`) | 107 | ~1,500 | High |
| Blocks (`consensus-types/blocks/`) | 246 | ~800 | Medium |
| Proto (generated) | - | ~17,000 (SSZ) + ~5,000 (pb) | Low |
| STF (`core/transition/`) | 25 | ~300 + 5,700 cascading | High |
| Sync (`sync/`) | ~50 | ~200 | Medium |
| Fork Choice | ~5 | ~50 | Low |
| RPC/API | 273 | ~500 | Medium |
| Database | ~30 | ~200 | Medium-High |
| **Total** | **~736** | **~31,000+** | |
