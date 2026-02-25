# Problem: State-Native God Package

## Summary

The `beacon-chain/state/state-native/` package contains 84 Go files with extensive getter/setter sprawl. This "god package" is difficult to navigate, hard to understand, and risky to modify because changes to any part of the state can have subtle side effects on other parts.

## Evidence

### 1. Package Size

```
$ ls beacon-chain/state/state-native/*.go | wc -l
84
```

The package contains 84 Go source files, making it one of the largest packages in the codebase.

### 2. Getter/Setter File Explosion

The package splits state access into dozens of files by category:

**Getter files:**
- `getters_state.go` (737 lines)
- `getters_validator.go`
- `getters_withdrawal.go`
- `getters_participation.go`
- `getters_gloas.go` (320 lines)
- `getters_deposit_requests.go`
- `getters_consolidation.go`
- `getters_exit.go`
- `getters_attestation.go`
- `getters_checkpoint.go`
- `getters_block.go`
- `getters_eth1.go`
- `getters_misc.go`
- `getters_randao.go`
- `getters_sync_committee.go`

**Setter files** (mirroring getters):
- `setters_state.go`
- `setters_validator.go`
- `setters_gloas.go` (589 lines)
- ... and more

**Plus supporting files:**
- `state_trie.go` (1,572 lines -- the core)
- `beacon_state_mainnet.go` / `beacon_state_minimal.go` (build-tagged)
- `proofs.go`
- `types.go`
- `hasher.go`
- `spec_parameters.go`

### 3. Largest Single File

`state_trie.go` at 1,572 lines contains:
- Fork-specific field lists (lines 28-140)
- 8 shared field reference count constants (lines 142-151)
- 16 `InitializeFromProto*` / `InitializeFromProtoUnsafe*` functions (lines 153+)
- State initialization logic
- Field reference tracking
- Copy/finalize logic

### 4. Cross-Cutting Concerns Mixed Together

The package intermixes:
- Merkle trie computation (`hasher.go`, `proofs.go`)
- Proto conversion (`getters_state.go`)
- Thread-safe locking (`lock` field used in all getters/setters)
- Multi-value slice optimization (`container/multi-value-slice`)
- Fork-specific field indexing

A change to any one concern (e.g., adding a new fork's fields) requires understanding all the others.

### 5. No Sub-Package Organization

Everything is at the top level. There are no sub-packages for:
- Proto conversion
- Trie operations
- Thread-safety wrappers
- Fork-specific logic

## Key Files

| File | Lines | Role |
|------|-------|------|
| `state_trie.go` | 1,572 | Core state management, init functions |
| `getters_state.go` | 737 | Proto conversion, version-switched getters |
| `setters_gloas.go` | 589 | Gloas-specific setters |
| `getters_gloas.go` | 320 | Gloas-specific getters |
| `types.go` | ~150 | BeaconState struct definition |

## Solution and Action Plan

### Goal
Break the god package into focused sub-packages with clear responsibilities.

### Phase 1: Extract Proto Conversion (Medium Effort)
1. Move `ToProto()`, `ToProtoUnsafe()`, and all `InitializeFromProto*` functions into a `state/convert/` package.
2. These functions are a self-contained concern that only depends on field accessors.
3. Reduces `getters_state.go` from 737 lines to near-zero.

### Phase 2: Extract Trie/Proof Logic (Medium Effort)
1. Move `hasher.go`, `proofs.go` into a `state/trie/` package.
2. The trie layer should depend on a minimal field accessor interface, not the full state struct.
3. Decouples Merkle proof generation from state mutation.

### Phase 3: Consolidate Getters/Setters by Fork (Medium Effort)
1. Instead of splitting by data type (validator, withdrawal, deposit), group by fork version.
2. Shared getters/setters stay in a `common.go` file.
3. Fork-specific additions go in `electra.go`, `gloas.go`, etc.
4. This aligns file organization with how forks are added (new files per fork, not new methods scattered across existing files).

### Phase 4: Consider Embedding for Thread Safety (Low-Medium Effort)
1. Extract the locking concern into a wrapper:
   ```go
   type ThreadSafeState struct {
       mu    sync.RWMutex
       inner *UnsafeState
   }
   ```
2. The inner state has no locking -- the wrapper provides it.
3. Callers that don't need thread safety (e.g., single-goroutine state transitions) use `UnsafeState` directly.

### Estimated Impact
- Package goes from 84 files to ~20-30 files per sub-package
- New fork additions are localized to 1-2 files per sub-package
- Clear dependency graph between sub-packages
- Easier for new contributors to understand where to make changes
