# Problem: Code Duplication Across Fork Versions

## Summary

Fork-specific logic is copy-pasted across multiple packages with minimal differences. Upgrade functions, state getters/setters, and core processing logic are duplicated per fork, creating a maintenance burden where bug fixes must be replicated across 6-8 locations.

## Evidence

### 1. Upgrade Functions: ~80% Identical

Comparing `beacon-chain/core/capella/upgrade.go` (97 lines) and `beacon-chain/core/deneb/upgrade.go` (115 lines):

**Identical code across both files:**
```go
// Lines 16-35 in both files - extracting state fields
currentSyncCommittee, err := state.CurrentSyncCommittee()
nextSyncCommittee, err := state.NextSyncCommittee()
prevEpochParticipation, err := state.PreviousEpochParticipation()
currentEpochParticipation, err := state.CurrentEpochParticipation()
inactivityScores, err := state.InactivityScores()
payloadHeader, err := state.LatestExecutionPayloadHeader()
txRoot, err := payloadHeader.TransactionsRoot()
```

**Identical code across both files:**
```go
// Lines 45-86 (Capella) / 61-111 (Deneb) - building the proto state
GenesisTime:           uint64(state.GenesisTime().Unix()),
GenesisValidatorsRoot: state.GenesisValidatorsRoot(),
Slot:                  state.Slot(),
Fork: &ethpb.Fork{...},
LatestBlockHeader:           state.LatestBlockHeader(),
BlockRoots:                  state.BlockRoots(),
StateRoots:                  state.StateRoots(),
// ... 15+ more identical field assignments
```

The only differences:
- Proto struct type (`BeaconStateCapella` vs `BeaconStateDeneb`)
- Fork version constant
- Payload header type (`ExecutionPayloadHeaderCapella` vs `ExecutionPayloadHeaderDeneb`)
- Deneb adds `ExcessBlobGas` and `BlobGasUsed` fields

This pattern repeats for Electra, Fulu, and Gloas upgrades.

### 2. `ToProtoUnsafe`: 8-Way Duplication

`beacon-chain/state/state-native/getters_state.go:35-300+` contains 8 version cases, each constructing a proto struct with 20-35 fields. Approximately 80% of fields are identical across cases:

```go
case version.Phase0:
    return &ethpb.BeaconState{
        GenesisTime: b.genesisTime,
        GenesisValidatorsRoot: gvrCopy[:],
        Slot: b.slot, Fork: b.fork,
        // ... 16 identical fields
        PreviousEpochAttestations: b.previousEpochAttestations,  // Phase0 only
        CurrentEpochAttestations:  b.currentEpochAttestations,   // Phase0 only
    }
case version.Altair:
    return &ethpb.BeaconStateAltair{
        GenesisTime: b.genesisTime,    // IDENTICAL
        GenesisValidatorsRoot: gvrCopy[:], // IDENTICAL
        Slot: b.slot, Fork: b.fork,     // IDENTICAL
        // ... 16 identical fields       // IDENTICAL
        PreviousEpochParticipation: b.previousEpochParticipation, // Altair+
        CurrentEpochParticipation:  b.currentEpochParticipation,  // Altair+
        InactivityScores: inactivityScores,                       // Altair+
    }
// ... 6 more nearly identical cases
```

### 3. Fork-Specific Getter/Setter Files for Gloas

The newest fork (Gloas) alone adds:

| File | Lines |
|------|-------|
| `beacon-chain/state/state-native/getters_gloas.go` | ~320 |
| `beacon-chain/state/state-native/setters_gloas.go` | ~589 |
| `beacon-chain/state/interfaces_gloas.go` | 56 |
| **Total** | **~965 lines** |

Each future fork will require similar files.

### 4. Core Processing Logic Per Fork

The `beacon-chain/core/` directory has fork-specific packages:

```
beacon-chain/core/
    altair/       # attestation, block, deposit, epoch processing
    capella/      # upgrade
    deneb/        # upgrade
    electra/      # attestation, block, deposit, upgrade
    fulu/         # upgrade
    gloas/        # attestation, block, deposit, upgrade, epoch processing
```

Many files across these packages contain near-identical logic with fork-specific adjustments (e.g., attestation processing differs only in how participation bits are handled).

### 5. Block Factory Init Functions

`consensus-types/blocks/factory.go` references 14+ `initSignedBlockFromProto*` functions:

- `initSignedBlockFromProtoPhase0`
- `initSignedBlockFromProtoAltair`
- `initSignedBlockFromProtoBellatrix`
- `initBlindedSignedBlockFromProtoBellatrix`
- `initSignedBlockFromProtoCapella`
- `initBlindedSignedBlockFromProtoCapella`
- `initSignedBlockFromProtoDeneb`
- `initBlindedSignedBlockFromProtoDeneb`
- `initSignedBlockFromProtoElectra`
- `initBlindedSignedBlockFromProtoElectra`
- `initSignedBlockFromProtoFulu`
- `initBlindedSignedBlockFromProtoFulu`
- `initSignedBlockFromProtoGloas`

Each follows the same pattern of extracting fields from a proto message into a native struct.

## Key Files

| File | Lines of Duplication |
|------|---------------------|
| `beacon-chain/core/capella/upgrade.go` | 97 lines (~80 shared) |
| `beacon-chain/core/deneb/upgrade.go` | 115 lines (~80 shared) |
| `beacon-chain/state/state-native/getters_state.go` | 300+ lines (8 cases) |
| `consensus-types/blocks/factory.go` | 14 init functions |
| `beacon-chain/state/state-native/getters_gloas.go` | 320 lines |
| `beacon-chain/state/state-native/setters_gloas.go` | 589 lines |

## Solution and Action Plan

### Goal
Extract shared fork logic into reusable components. New forks should only define their delta.

### Phase 1: Shared State Builder (Medium Effort)
1. Create a `buildCommonStateFields(state)` helper that returns the ~16 fields shared across all post-Altair forks.
2. Fork-specific upgrade functions only specify their unique additions.
3. Example:
   ```go
   func UpgradeToDeneb(state state.BeaconState) (state.BeaconState, error) {
       common := extractCommonFields(state)
       return state_native.InitializeFromFields(version.Deneb, common, DenebSpecificFields{
           ExcessBlobGas: 0,
           BlobGasUsed:   0,
       })
   }
   ```

### Phase 2: Declarative Fork Diffs (Medium Effort)
1. Define each fork as a diff from its predecessor:
   ```go
   var DenebDiff = ForkDiff{
       AddedFields:   []Field{ExcessBlobGas, BlobGasUsed},
       RemovedFields: nil,
       PayloadType:   reflect.TypeOf(enginev1.ExecutionPayloadHeaderDeneb{}),
   }
   ```
2. A single generic upgrade function applies diffs.
3. New forks only register a diff struct.

### Phase 3: Code Generation for Getters/Setters (Medium-High Effort)
1. Generate fork-specific getter/setter files from a field schema.
2. The schema defines which fields exist per fork.
3. Eliminates hand-written files like `getters_gloas.go` / `setters_gloas.go`.

### Phase 4: Consolidate Block Init Functions (Medium Effort)
1. Replace 14 `initSignedBlockFromProto*` functions with a single generic function:
   ```go
   func initSignedBlockFromProto(version int, pb proto.Message) (interfaces.SignedBeaconBlock, error)
   ```
2. Use reflection or a field mapping table to extract fields.

### Estimated Impact
- Reduces per-fork boilerplate from ~1,000 lines to ~50-100 lines
- Bug fixes in shared logic apply to all forks automatically
- New fork additions become a small, reviewable diff
