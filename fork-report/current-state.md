# Fork Code Inventory

> Quantitative snapshot of fork-specific code in the Prysm codebase (module `github.com/OffchainLabs/prysm/v7`).

## Fork Timeline

| # | Fork | Version Const | Status |
|---|------|---------------|--------|
| 0 | Phase0 | `version.Phase0` | Mainnet (genesis) |
| 1 | Altair | `version.Altair` | Mainnet |
| 2 | Bellatrix | `version.Bellatrix` | Mainnet |
| 3 | Capella | `version.Capella` | Mainnet |
| 4 | Deneb | `version.Deneb` | Mainnet |
| 5 | Electra | `version.Electra` | Mainnet |
| 6 | Fulu | `version.Fulu` | Target (v7) |
| 7 | Gloas | `version.Gloas` | Unsupported (gated) |

Source: `runtime/version/fork.go:9-17`

## 1. Protobuf / Generated Code

### SSZ Generated Code (`proto/prysm/v1alpha1/*.ssz.go`)

| File | Lines |
|------|------:|
| `electra.ssz.go` | 5,294 |
| `deneb.ssz.go` | 4,574 |
| `phase0.ssz.go` | 4,130 |
| `gloas.ssz.go` | 3,992 |
| `capella.ssz.go` | 3,883 |
| `altair.ssz.go` | 2,787 |
| `fulu.ssz.go` | 2,496 |
| `bellatrix.ssz.go` | 2,329 |
| `non-core.ssz.go` | 1,106 |
| **Total** | **30,591** |

### Protobuf Generated Code (`proto/prysm/v1alpha1/*.pb.go`)

| File | Lines |
|------|------:|
| `beacon_block.pb.go` | 6,131 |
| `validator.pb.go` | 5,764 |
| `beacon_chain.pb.go` | 5,547 |
| `beacon_state.pb.go` | 3,711 |
| `gloas.pb.go` | 2,351 |
| Others | 10,368 |
| **Total** | **33,872** |

### Proto Message Types

**BeaconState messages** (`proto/prysm/v1alpha1/beacon_state.proto`):
- `BeaconState` (Phase0)
- `BeaconStateAltair`
- `BeaconStateBellatrix`
- `BeaconStateCapella`
- `BeaconStateDeneb`
- `BeaconStateElectra`
- `BeaconStateFulu`
- `BeaconStateGloas` (in `gloas.proto`)

**BeaconBlock messages** (`proto/prysm/v1alpha1/beacon_block.proto`):
- Phase0: `BeaconBlock`, `SignedBeaconBlock`
- Altair: `BeaconBlockAltair`, `SignedBeaconBlockAltair`
- Bellatrix+: normal + blinded variants for each fork
- All wrapped by `GenericSignedBeaconBlock` (oneOf with 14+ fields)

## 2. Core Fork Packages

Located in `beacon-chain/core/`:

| Package | Lines (incl. tests) | Key Responsibility |
|---------|--------------------:|---------------------|
| `altair/` | 5,171 | Sync committees, participation, rewards, epoch processing |
| `electra/` | 3,640 | Deposits, consolidations, effective balance, validators |
| `gloas/` | 3,596 | Payload bids, payload attestations, builders |
| `fulu/` | 448 | Proposer lookahead, epoch (delegates to electra) |
| `deneb/` | 215 | Upgrade function only |
| `capella/` | 202 | Upgrade function only |
| `execution/` | 172 | Bellatrix upgrade function |
| **Total** | **13,444** | |

Notable: Phase0 epoch processing lives in `beacon-chain/core/epoch/` (542 lines) and `beacon-chain/core/epoch/precompute/` rather than a dedicated package.

## 3. State Implementation

### Unified BeaconState Struct

**File**: `beacon-chain/state/state-native/beacon_state.go`

A single Go struct holds fields for **all 8 forks**. The `version int` field determines which fields are active:

- Phase0-only: `previousEpochAttestations`, `currentEpochAttestations`
- Altair+: `previousEpochParticipation`, `currentEpochParticipation`, `currentSyncCommittee`, `nextSyncCommittee`, `inactivityScores`
- Bellatrix+: `latestExecutionPayloadHeader`
- Capella+: `nextWithdrawalIndex`, `nextWithdrawalValidatorIndex`, `historicalSummaries`
- Electra+: 9 additional fields (deposits, consolidations, exit balances)
- Fulu+: `proposerLookahead`
- Gloas: 8 additional fields (builders, payload bids, etc.)

### FieldIndex Enum

**File**: `beacon-chain/state/state-native/types/types.go:254-305`

45 field indices total. Several occupy the same `RealPosition()` across forks:
- Position 15: `PreviousEpochAttestations` (Phase0) vs `PreviousEpochParticipationBits` (Altair+)
- Position 16: `CurrentEpochAttestations` (Phase0) vs `CurrentEpochParticipationBits` (Altair+)
- Position 24: `LatestExecutionPayloadHeader` / `...Capella` / `...Deneb` / `...Bid` (one per fork range)

### Per-Fork Field Lists

**File**: `beacon-chain/state/state-native/state_trie.go:29-139`

Each fork defines its field set:
- `phase0Fields`: 21 fields
- `altairFields`: 23 fields (replaces attestations with participation)
- `bellatrixFields`: altair + execution payload header
- `capellaFields`: altair + capella execution header + withdrawals + historical summaries
- `denebFields`: altair + deneb execution header + withdrawals + historical summaries
- `electraFields`: deneb + 9 Electra fields
- `fuluFields`: electra + proposer lookahead
- `gloasFields`: altair + payload bid + withdrawals + electra extras + proposer lookahead + 7 Gloas fields

### InitializeFromProto Functions

8 safe + 8 unsafe initializers (lines 153-191 in `state_trie.go`):
- `InitializeFromProtoPhase0` / `InitializeFromProtoUnsafePhase0`
- `InitializeFromProtoAltair` / `InitializeFromProtoUnsafeAltair`
- ... through `InitializeFromProtoGloas` / `InitializeFromProtoUnsafeGloas`

Each is 40-80 lines mapping proto fields to the unified struct.

## 4. Block Factory

**File**: `consensus-types/blocks/factory.go`

`NewSignedBeaconBlock()` has a 30+ case type switch covering:
- Phase0, Altair: 2 cases each (generic + direct)
- Bellatrix, Capella: 4 cases each (generic + direct + blinded variants)
- Deneb, Electra, Fulu: 4 cases each
- Gloas: 1 case (so far)

`NewBeaconBlock()` has a similar switch.

## 5. Version Reference Distribution

Files containing `version.<Fork>` references across the codebase:

| Fork | References | Files |
|------|----------:|------:|
| Phase0 | 341 | - |
| Altair | 243 | - |
| Bellatrix | 274 | - |
| Capella | 271 | - |
| Deneb | 289 | - |
| Electra | 434 | - |
| Fulu | 224 | - |
| Gloas | 167 | - |
| **Total** | **2,243** | **352** |

The "old" forks (Phase0 through Deneb) account for **1,418 references** (63% of total).

## 6. Upgrade Chain

**File**: `beacon-chain/core/transition/transition.go:342-413`

Sequential upgrade checks:
```
CanUpgradeToAltair    -> altair.UpgradeToAltair
CanUpgradeToBellatrix -> execution.UpgradeToBellatrix
CanUpgradeToCapella   -> capella.UpgradeToCapella
CanUpgradeToDeneb     -> deneb.UpgradeToDeneb
CanUpgradeToElectra   -> electra.UpgradeToElectra
CanUpgradeToFulu      -> fulu.UpgradeToFulu
CanUpgradeToGloas     -> gloas.UpgradeToGloas
```

Each upgrade function transforms state from fork N-1 to N. They are called once at the fork boundary epoch.

## 7. Epoch Processing Dispatch

**File**: `beacon-chain/core/transition/transition.go:317-340`

```
version >= Fulu    -> fulu.ProcessEpoch
version >= Electra -> electra.ProcessEpoch
version >= Altair  -> altair.ProcessEpoch
else (Phase0)      -> ProcessEpochPrecompute
```

Note: Fulu's `ProcessEpoch` is used for both Fulu and Gloas (Gloas inherits Fulu's epoch processing).
