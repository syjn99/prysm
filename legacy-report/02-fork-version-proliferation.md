# Problem: Fork Version Proliferation

## Summary

Prysm supports 8 fork versions (Phase0 through Gloas), each requiring separate protobuf messages, state types, block types, and upgrade functions. Adding a new fork requires touching dozens of files with near-identical boilerplate.

## Evidence

### 1. Fork Version Enumeration

`runtime/version/fork.go:9-18`:

```go
const (
    Phase0 = iota   // Genesis (2020)
    Altair           // Oct 2021
    Bellatrix        // Sept 2022
    Capella          // April 2023
    Deneb            // March 2024
    Electra          // 2025
    Fulu             // 2025
    Gloas            // Future (marked unsupported)
)
```

Only Gloas is in `unsupportedVersions`. All 7 others are fully active.

### 2. Separate Proto Messages Per Fork

`proto/prysm/v1alpha1/beacon_state.proto` defines 7 separate message types:

| Fork | Message | Approx Fields |
|------|---------|---------------|
| Phase0 | `BeaconState` | 21 |
| Altair | `BeaconStateAltair` | 23 |
| Bellatrix | `BeaconStateBellatrix` | 24 |
| Capella | `BeaconStateCapella` | 27 |
| Deneb | `BeaconStateDeneb` | 27 |
| Electra | `BeaconStateElectra` | 36 |
| Fulu | `BeaconStateFulu` | 37 |

`proto/prysm/v1alpha1/beacon_block.proto` defines 9+ `SignedBeaconBlock*` types plus blinded variants, totaling ~16 distinct block messages.

### 3. Fork-Specific State Field Lists

`beacon-chain/state/state-native/state_trie.go:28-140` maintains explicit field arrays per fork:

```go
var phase0Fields = []types.FieldIndex{...}    // 21 fields
var altairFields = []types.FieldIndex{...}    // 23 fields
var bellatrixFields = append(altairFields, types.LatestExecutionPayloadHeader)
var capellaFields = slices.Concat(altairFields, ..., withdrawalAndHistoricalSummaryFields)
var denebFields = slices.Concat(altairFields, ..., withdrawalAndHistoricalSummaryFields)
var electraFields = slices.Concat(denebFields, electraAdditionalFields)
var fuluFields = append(electraFields, types.ProposerLookahead)
var gloasFields = slices.Concat(altairFields, ..., gloasAdditionalFields)
```

Plus fork-specific shared reference counts (`state_trie.go:142-151`):
```go
const (
    phase0SharedFieldRefCount    = 5
    altairSharedFieldRefCount    = 5
    bellatrixSharedFieldRefCount = 6
    capellaSharedFieldRefCount   = 7
    // ...
    gloasSharedFieldRefCount     = 13
)
```

### 4. Phase0-Specific Attestation Fields (Altair Breaking Change)

Phase0 uses `PreviousEpochAttestations` / `CurrentEpochAttestations` (arrays of `PendingAttestation`), which were replaced in Altair with `PreviousEpochParticipationBits` / `CurrentEpochParticipationBits` (byte arrays).

This means Phase0 has fundamentally different attestation state from all later forks, requiring separate read/write/upgrade paths throughout the codebase.

### 5. 8x `InitializeFromProto*` Functions

`state_trie.go:153-160+` -- one pair per fork:

```go
func InitializeFromProtoPhase0(st *ethpb.BeaconState) (state.BeaconState, error) { ... }
func InitializeFromProtoAltair(st *ethpb.BeaconStateAltair) (state.BeaconState, error) { ... }
func InitializeFromProtoBellatrix(st *ethpb.BeaconStateBellatrix) (state.BeaconState, error) { ... }
// ... 5 more
```

### 6. Upgrade Functions Duplicated Per Fork

Each fork transition requires its own upgrade function with ~90% identical code:

- `beacon-chain/core/capella/upgrade.go` (97 lines)
- `beacon-chain/core/deneb/upgrade.go` (115 lines)
- `beacon-chain/core/electra/upgrade.go`
- `beacon-chain/core/fulu/upgrade.go`
- `beacon-chain/core/gloas/upgrade.go`

The Capella and Deneb upgrade functions share ~80 identical lines extracting sync committees, participation bits, inactivity scores, payload headers, etc.

### 7. 20+ `case version.Phase0` Switch Statements

Fork-specific switch statements are scattered throughout:

- `beacon-chain/state/state-native/getters_state.go:35-60+` (ToProtoUnsafe)
- `beacon-chain/blockchain/process_block.go` (execution payload check)
- `beacon-chain/state/state-native/proofs.go` (sync committee proof)
- `consensus-types/blocks/factory.go:33-89` (block construction)
- `encoding/ssz/detect/configfork.go` (SSZ detection)
- `beacon-chain/db/kv/state.go` (state serialization)

## Key Files

| File | Role |
|------|------|
| `runtime/version/fork.go` | Fork version enumeration |
| `proto/prysm/v1alpha1/beacon_state.proto` | 7 separate state message definitions |
| `proto/prysm/v1alpha1/beacon_block.proto` | 16+ block message definitions |
| `beacon-chain/state/state-native/state_trie.go` | Fork-specific field lists, init functions |
| `beacon-chain/core/*/upgrade.go` | Duplicated upgrade logic per fork |

## Solution and Action Plan

### Goal
Reduce the per-fork cost from "dozens of files" to a minimal, localized diff.

### Phase 1: Unified State Proto Message (High Effort, High Impact)
1. Define a single `BeaconState` proto with all fields across all forks, using field presence to indicate fork-specific fields.
2. A `version` field (or convention based on field presence) determines which fields are valid.
3. Eliminates 7 separate proto messages and all `InitializeFromProto*` variants.

### Phase 2: Generic Upgrade Function (Medium Effort)
1. Create a single `UpgradeState(state, targetFork)` function that reads field diffs from a fork-specific config rather than hardcoding field extraction.
2. Each fork registers its additions/removals via a declarative struct:
   ```go
   var DenebUpgrade = ForkUpgrade{
       AddedFields: []FieldSpec{
           {Name: "ExcessBlobGas", Default: 0},
           {Name: "BlobGasUsed", Default: 0},
       },
       PayloadHeaderType: &enginev1.ExecutionPayloadHeaderDeneb{},
   }
   ```
3. Eliminates 6 nearly-identical upgrade files.

### Phase 3: Phase0 Deprecation Path (Medium Effort, Long-Term)
1. Since mainnet has been on Altair+ since Oct 2021, evaluate whether Phase0 state handling can be moved to a `legacy/` package.
2. Phase0 states would only need to be readable (for historical data) but not writable.
3. Remove `PreviousEpochAttestations` / `CurrentEpochAttestations` from the active state interface.

### Phase 4: Reduce Switch Statement Sprawl (Medium Effort)
1. Use a registry pattern: each fork registers its handlers at init time.
2. Replace `switch b.version` with method dispatch on a `ForkHandler` interface.
3. New forks only need to register, not modify existing switch statements.

### Estimated Impact
- Adding a new fork goes from touching ~30 files to ~5 files
- Removes ~1,000 lines of duplicated upgrade code
- Centralizes fork-specific logic instead of scattering it across the codebase
