# Execution Roadmap

> Step-by-step plan to debloat old fork code from Prysm, keeping only N-1 (Fulu) and N (Gloas).

## Phased Approach

The work is divided into 5 phases. Each phase is independently shippable and testable.

---

## Phase 0: Preparation (pre-requisite)

### 0.1 Audit shared helpers in `altair/`

**Goal**: Identify which functions in `beacon-chain/core/altair/` are imported by `electra/`, `fulu/`, or `gloas/`.

**Action**:
```
grep -r "altair\." beacon-chain/core/electra/ beacon-chain/core/fulu/ beacon-chain/core/gloas/
```

Known shared dependencies:
- `altair.ProcessEpoch` -> called for Altair-Deneb states
- Precompute helpers in `altair/epoch_precompute.go`
- Sync committee utilities in `altair/sync_committee.go`
- Reward calculation in `altair/reward.go`

**Decision**: For any function still used by Electra+, either:
- (a) Move it to a shared `helpers/` package, or
- (b) Keep it in `altair/` and rename the package to something fork-neutral (e.g., `shared/`)

### 0.2 Decide database strategy

**Options**:
| Strategy | Effort | Downtime |
|----------|--------|----------|
| Require checkpoint re-sync on upgrade | Low | ~30 min |
| Ignore old data in DB (leave unreadable) | Low | None |
| Migration tool to convert old states | High | Variable |

**Recommendation**: Require checkpoint re-sync. It's the simplest and aligns with the proposal's philosophy.

### 0.3 Design legacy binary

**Goal**: A separate binary that can process Phase0 through Fulu states for historical sync.

**Architecture**:
```
cmd/legacy-beacon/
  main.go
  go.mod  -> imports github.com/OffchainLabs/prysm/v7 (current)
```

This binary would:
1. Accept a genesis state
2. Run the full Phase0-through-Fulu STF
3. Output a Fulu-era finalized state + block
4. This output becomes the checkpoint for the main binary

---

## Phase 1: Remove Phase0-Specific Code

**Scope**: Phase0 is the most isolated fork with unique types not shared by any later fork.

### 1.1 Remove Phase0 attestation types

- Delete `PreviousEpochAttestations` and `CurrentEpochAttestations` field indices from `types/types.go`
- Remove Phase0 attestation getters/setters from `state-native/`
- Remove `phase0Fields` from `state_trie.go`
- Delete `InitializeFromProtoUnsafePhase0` and `InitializeFromProtoPhase0`

### 1.2 Remove Phase0 epoch processing

- Delete `ProcessEpochPrecompute` and related functions in `beacon-chain/core/epoch/`
- Remove the Phase0 branch from `ProcessEpoch()` in `transition.go`

### 1.3 Remove Phase0 proto messages

- Remove `BeaconState` (Phase0) from `beacon_state.proto`
- Remove `BeaconBlock` / `SignedBeaconBlock` (Phase0) from `beacon_block.proto`
- Remove Phase0 cases from `GenericSignedBeaconBlock` oneOf
- Regenerate SSZ and protobuf code

### 1.4 Remove Phase0 block factory cases

- Remove Phase0 cases from `NewSignedBeaconBlock` and `NewBeaconBlock` in `factory.go`

**Estimated deletion**: ~5,000 lines
**Test**: Run all Electra+ spec tests. Verify checkpoint sync still works.

---

## Phase 2: Remove Bellatrix/Capella/Deneb-Specific Code

**Scope**: These forks added execution payloads and withdrawals. Their unique code is mostly upgrade functions and distinct execution payload header types.

### 2.1 Remove Bellatrix merge transition

- Delete `beacon-chain/core/execution/upgrade.go` (172 lines)
- Delete `beacon-chain/core/transition/state-bellatrix.go`
- Remove Bellatrix upgrade call from `UpgradeState()`

### 2.2 Remove Capella and Deneb upgrade functions

- Delete `beacon-chain/core/capella/upgrade.go` (202 lines)
- Delete `beacon-chain/core/deneb/upgrade.go` (215 lines)
- Remove upgrade calls from `UpgradeState()`

### 2.3 Consolidate execution payload header fields

- Remove `LatestExecutionPayloadHeader` (Bellatrix) field index
- Remove `LatestExecutionPayloadHeaderCapella` field index
- Keep `LatestExecutionPayloadHeaderDeneb` (used by Electra/Fulu) and `LatestExecutionPayloadBid` (Gloas)

### 2.4 Remove proto messages

- Remove `BeaconStateBellatrix`, `BeaconStateCapella`, `BeaconStateDeneb` from `beacon_state.proto`
- Remove corresponding block types from `beacon_block.proto`
- Remove blinded block types for Bellatrix and Capella
- Regenerate code

### 2.5 Remove block factory cases

- Remove Bellatrix, Capella, Deneb cases from factory functions
- Remove `initBlockFromProtoBellatrix`, `...Capella`, `...Deneb` init functions

**Estimated deletion**: ~12,000 lines (including generated code)
**Test**: Run Electra+ spec tests. Verify Electra state initialization still works.

---

## Phase 3: Audit and Refactor Altair Shared Code

**Scope**: Altair is the trickiest because its code is shared. This phase is surgical.

### 3.1 Identify truly shared functions

Run the import audit from Phase 0.1. Expected result:

| Function | Used by Electra+? | Action |
|----------|-------------------|--------|
| `altair.InitializePrecomputeValidators` | Yes (electra/transition.go) | Move to shared |
| `altair.ProcessEpochParticipation` | Yes (electra/transition.go) | Move to shared |
| `altair.ProcessInactivityScores` | Yes (electra/transition.go) | Move to shared |
| `altair.ProcessRewardsAndPenaltiesPrecompute` | Yes (electra/transition.go) | Move to shared |
| `altair.ProcessParticipationFlagUpdates` | Yes (electra/transition.go) | Move to shared |
| `altair.ProcessSyncCommitteeUpdates` | Yes (electra/transition.go) | Move to shared |
| `altair.AttestationsDelta` | Yes (electra/transition.go) | Move to shared |
| `altair.ProcessAttestationsNoVerifySignature` | Yes (electra/attestation.go) | Move to shared |
| `altair.AttestationParticipationFlagIndices` | Yes (electra/attestation.go) | Move to shared |
| `altair.BaseRewardWithTotalBalance` | Yes (electra/attestation.go) | Move to shared |
| `altair.HasValidatorFlag` | Yes (electra/attestation.go) | Move to shared |
| `altair.ProcessEpoch` | No (Electra has its own) | Delete |
| `altair.TranslateParticipation` | No (Phase0->Altair only) | Delete |
| `altair.UpgradeToAltair` | No | Delete |

### 3.2 Move shared utilities

Move the 11 verified shared functions to a fork-neutral package (e.g., `beacon-chain/core/consensus/` or `beacon-chain/core/helpers/`):
- Precompute/validator initialization
- Epoch participation & inactivity processing
- Reward/penalty calculation
- Sync committee updates
- Attestation flag helpers

### 3.3 Delete remaining Altair-only code

- Delete `altair.ProcessEpoch` (replaced by `electra.ProcessEpoch` for Electra+ and `fulu.ProcessEpoch` for Fulu+)
- Delete `altair.UpgradeToAltair`
- Remove `BeaconStateAltair` proto message
- Remove Altair block types from proto

### 3.4 Remove Altair block/state factory cases

- Remove Altair cases from block factory
- Remove `InitializeFromProtoUnsafeAltair`

**Estimated deletion**: ~3,000 lines (after porting shared code)
**Test**: Full spec test suite for Electra, Fulu, Gloas.

---

## Phase 4: Build Legacy Binary

### 4.1 Create legacy binary module

```
cmd/legacy-beacon/
  main.go       -- imports prysm/v7 for full Phase0-Fulu STF
  go.mod         -- depends on github.com/OffchainLabs/prysm/v7
```

### 4.2 Implement sync-to-checkpoint

The legacy binary:
1. Accepts genesis state + config
2. Downloads blocks from P2P or a block archive
3. Runs full STF from Phase0 through Fulu
4. Exports finalized state at a recent Fulu epoch
5. Output is used as checkpoint for the main binary

### 4.3 Test end-to-end

- Start legacy binary from mainnet genesis
- Sync to a recent Fulu finalized epoch
- Export checkpoint
- Start main binary from that checkpoint
- Verify it syncs to head

---

## Phase 5: Module Version Bump

### 5.1 Bump to v8

- Update `go.mod`: `module github.com/OffchainLabs/prysm/v8`
- Update all internal import paths
- Update `runtime/version/fork.go` to start from Electra (or Fulu)
- Tag the v7 release before bumping (legacy binary depends on it)

### 5.2 Clean up version enum

```go
const (
    Electra = iota
    Fulu
    Gloas
)
```

Or keep the original numbering for compatibility:
```go
const (
    Electra = 5
    Fulu    = 6
    Gloas   = 7
)
```

**Recommendation**: Keep original numbering. The version values are used in P2P and storage, changing them would break compatibility.

### 5.3 Update CI/CD

- Build both main binary and legacy binary in CI
- Publish legacy binary as a separate artifact
- Update documentation

---

## Timeline Estimate

| Phase | Scope | Dependencies |
|-------|-------|-------------|
| Phase 0 | Preparation & audit | None |
| Phase 1 | Remove Phase0 | Phase 0 |
| Phase 2 | Remove Bellatrix/Capella/Deneb | Phase 1 |
| Phase 3 | Refactor Altair shared code | Phase 2 |
| Phase 4 | Legacy binary | Phase 0 (can parallel with 1-3) |
| Phase 5 | Module version bump | Phases 1-4 |

Phases 1-3 can be done incrementally with separate PRs. Phase 4 can be developed in parallel.

---

## Risk Mitigations

| Risk | Mitigation |
|------|-----------|
| Breaking consensus for Electra+ states | Run full Electra/Fulu/Gloas spec test suites after each phase |
| Shared helper deletion breaks Electra | Import audit in Phase 0.1; move before delete |
| Database incompatibility | Require checkpoint re-sync; document in release notes |
| Legacy binary bitrot | Include in CI; run genesis-to-checkpoint test monthly |
| P2P incompatibility with old-fork messages | Keep old fork version constants for protocol negotiation |
| Validator client breakage | Validator client only uses current fork; minimal impact |

---

## Success Criteria

1. Main binary compiles and passes all Electra+ spec tests
2. Main binary can checkpoint-sync to head on mainnet
3. Legacy binary can sync from genesis to a Fulu checkpoint
4. Module is at v8 with clean import paths
5. No old-fork code (Phase0-Deneb) remains in main binary
6. CI builds both binaries and runs full test suites
