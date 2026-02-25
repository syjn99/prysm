# Problem: Interface Bloat in State Package

## Summary

The `BeaconState` interface is composed of 30+ micro-interfaces totaling 100+ methods. This makes it extremely difficult to implement, mock, or understand, and violates the Interface Segregation Principle in practice because most consumers need only a small subset.

## Evidence

### 1. ReadOnlyBeaconState: 16 Embedded Interfaces

`beacon-chain/state/interfaces.go:52-86`:

```go
type ReadOnlyBeaconState interface {
    ReadOnlyBlockRoots
    ReadOnlyStateRoots
    ReadOnlyRandaoMixes
    ReadOnlyEth1Data
    ReadOnlyExits
    ReadOnlyValidators
    ReadOnlyBalances
    ReadOnlyCheckpoint
    ReadOnlyAttestations
    ReadOnlyWithdrawals
    ReadOnlyParticipation
    ReadOnlyInactivity
    ReadOnlySyncCommittee
    ReadOnlyDeposits
    ReadOnlyConsolidations
    ReadOnlyProposerLookahead
    readOnlyGloasFields          // 17 more methods from Gloas
    ToProtoUnsafe() any
    ToProto() any
    GenesisTime() time.Time
    // ... 14 more direct methods
}
```

### 2. WriteOnlyBeaconState: 15 Embedded Interfaces

`beacon-chain/state/interfaces.go:89-117`:

```go
type WriteOnlyBeaconState interface {
    WriteOnlyBlockRoots
    WriteOnlyStateRoots
    WriteOnlyRandaoMixes
    WriteOnlyEth1Data
    WriteOnlyValidators
    WriteOnlyBalances
    WriteOnlyCheckpoint
    WriteOnlyAttestations
    WriteOnlyParticipation
    WriteOnlyInactivity
    WriteOnlySyncCommittee
    WriteOnlyConsolidations
    WriteOnlyWithdrawals
    WriteOnlyDeposits
    WriteOnlyProposerLookahead
    writeOnlyGloasFields         // 12+ methods from Gloas
    SetGenesisTime(val time.Time) error
    // ... 10 more direct methods
}
```

### 3. BeaconState Combines Everything

`beacon-chain/state/interfaces.go:23-33`:

```go
type BeaconState interface {
    SpecParametersProvider     // 2 methods
    ReadOnlyBeaconState        // 100+ methods
    WriteOnlyBeaconState       // 80+ methods
    Copy() BeaconState
    CopyAllTries()
    Defragment()
    HashTreeRoot(ctx context.Context) ([32]byte, error)
    Prover                     // 4 methods
    json.Marshaler             // 1 method
}
```

Total: approximately **190+ methods** on the full `BeaconState` interface.

### 4. Fork-Specific Interface Extensions

`beacon-chain/state/interfaces_gloas.go` adds fork-specific interfaces:

- `readOnlyGloasFields` (17 methods) -- builders, bids, payments, withdrawals, block hash
- `writeOnlyGloasFields` (12 methods) -- setters for all Gloas-specific state

Each new fork that adds state fields requires new interface additions.

### 5. Practical Impact: Method Counts by Sub-Interface

| Interface | Methods |
|-----------|---------|
| `ReadOnlyValidators` | 10 |
| `ReadOnlyCheckpoint` | 7 |
| `ReadOnlyWithdrawals` | 7 |
| `ReadOnlyBalances` | 4 |
| `ReadOnlyEth1Data` | 3 |
| `ReadOnlyBlockRoots` | 2 |
| `ReadOnlyStateRoots` | 2 |
| `ReadOnlyRandaoMixes` | 3 |
| `ReadOnlyAttestations` | 2 |
| `ReadOnlyParticipation` | 4 |
| `ReadOnlyInactivity` | 1 |
| `ReadOnlySyncCommittee` | 2 |
| `ReadOnlyDeposits` | 3 |
| `ReadOnlyConsolidations` | 4 |
| `ReadOnlyProposerLookahead` | 1 |
| `readOnlyGloasFields` | 17 |
| Direct methods on `ReadOnlyBeaconState` | 14 |
| **Total ReadOnly** | **~86** |

The WriteOnly side adds another ~80 methods.

## Key Files

| File | Role |
|------|------|
| `beacon-chain/state/interfaces.go` | 367 lines defining 30+ interfaces |
| `beacon-chain/state/interfaces_gloas.go` | 56 lines of Gloas-specific additions |
| `beacon-chain/state/state-native/*.go` | 84 files implementing these interfaces |

## Solution and Action Plan

### Goal
Reduce interface surface area so consumers only depend on what they actually use, and new forks don't bloat the shared interface.

### Phase 1: Audit Interface Usage (Low Effort)
1. For each function that accepts `BeaconState` or `ReadOnlyBeaconState`, determine the actual methods called.
2. Many functions likely only need 2-5 methods (e.g., slot processing only needs `Slot()`, `SetSlot()`, `Fork()`, `Version()`).
3. Create a map of "function -> required interface subset".

### Phase 2: Accept Narrow Interfaces at Call Sites (Medium Effort)
1. Define small, purpose-specific interfaces at the call site (Go idiom):
   ```go
   type slotAccessor interface {
       Slot() primitives.Slot
       Version() int
   }
   ```
2. Functions accept these narrow interfaces instead of the full `BeaconState`.
3. The concrete `BeaconState` struct already satisfies these narrow interfaces.

### Phase 3: Fork-Specific Methods Behind Version Guards (Medium Effort)
1. Move fork-specific methods (Gloas builders, Electra consolidations) out of the base interface.
2. Callers that need fork-specific data do a version check + type assertion to a fork-specific interface:
   ```go
   if st.Version() >= version.Gloas {
       gloasSt := st.(state.GloasState)
       builders := gloasSt.Builders()
   }
   ```
3. This stops the base interface from growing with every fork.

### Phase 4: Consider Code Generation (Low-Medium Effort)
1. Generate getter/setter interfaces from the field list definitions already in `state_trie.go`.
2. This ensures interfaces stay in sync with implementation automatically.

### Estimated Impact
- Most functions would depend on interfaces with 2-10 methods instead of 190+
- New forks wouldn't modify the base `BeaconState` interface
- Mocking for tests becomes trivial
