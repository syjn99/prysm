# Problem: Protobuf Types Used as Domain Models

## Summary

Protobuf types (`ethpb.*`) are used directly as domain models throughout Prysm's core logic rather than being isolated to a serialization/persistence layer. This creates tight coupling, runtime type safety issues, and significant performance overhead.

## Evidence

### 1. Proto Types Embedded in Core Structs

The `BeaconBlockBody` struct in `consensus-types/blocks/types.go:42-62` directly embeds proto types:

```go
type BeaconBlockBody struct {
    version                   int
    randaoReveal              [field_params.BLSSignatureLength]byte
    eth1Data                  *eth.Eth1Data              // proto type
    proposerSlashings         []*eth.ProposerSlashing    // proto type
    attesterSlashings         []*eth.AttesterSlashing    // proto type
    attestations              []*eth.Attestation         // proto type
    deposits                  []*eth.Deposit             // proto type
    voluntaryExits            []*eth.SignedVoluntaryExit // proto type
    syncAggregate             *eth.SyncAggregate         // proto type
    // ...
}
```

Similarly, `beacon-chain/state/state-native/state_trie.go` stores proto messages directly as state fields (e.g., `b.fork`, `b.latestBlockHeader`, `b.eth1Data`, `b.validators`).

### 2. Factory Pattern With 50+ Type Assertions

`consensus-types/blocks/factory.go:33-89` - `NewSignedBeaconBlock` accepts `any` and uses a 50+ case type switch:

```go
func NewSignedBeaconBlock(i any) (interfaces.SignedBeaconBlock, error) {
    switch b := i.(type) {
    case *eth.GenericSignedBeaconBlock_Phase0:
        return initSignedBlockFromProtoPhase0(b.Phase0)
    case *eth.SignedBeaconBlock:
        return initSignedBlockFromProtoPhase0(b)
    // ... 48 more cases for every fork + blinded variant
    }
}
```

No compile-time safety. If a new type is added and a case is missed, it fails at runtime.

### 3. `ToProto()` Returns `any`

`beacon-chain/state/interfaces.go:70-71`:

```go
ToProtoUnsafe() any
ToProto() any
```

All callers must type-assert the result. Example from `beacon-chain/state/state-native/getters_state.go:35-59`:

```go
switch b.version {
case version.Phase0:
    return &ethpb.BeaconState{...}       // 22 fields
case version.Altair:
    return &ethpb.BeaconStateAltair{...} // 23 fields
case version.Bellatrix:
    return &ethpb.BeaconStateBellatrix{...} // 24 fields
// ... 5 more cases, each duplicating 20+ lines
}
```

### 4. `proto.Clone()` Performance Overhead

`state_trie.go:154-155` uses `proto.Clone` for safe initialization:

```go
func InitializeFromProtoPhase0(st *ethpb.BeaconState) (state.BeaconState, error) {
    return InitializeFromProtoUnsafePhase0(proto.Clone(st).(*ethpb.BeaconState))
}
```

This deep-copies the entire state (validators, balances, etc.) with a type assertion. There are 8 such pairs (one per fork), and `proto.Clone` is called in 50+ locations across the codebase.

### 5. Deprecated `GenericSignedBeaconBlock` Still in Use

`proto/prysm/v1alpha1/beacon_block.proto` marks `GenericSignedBeaconBlock` as `option deprecated = true`, yet it's still used in the factory (`factory.go:37-84`) and throughout the RPC layer.

### 6. Test Framework Limitations

`testing/assert/assertions.go:19-20`:

```go
// NOTE: this function does not work for checking arrays/slices or maps of protobuf messages.
// For arrays/slices, please use DeepSSZEqual.
```

Proto messages break standard Go equality checks, requiring a separate SSZ-based comparison path.

## Key Files

| File | Role |
|------|------|
| `consensus-types/blocks/types.go` | Core block types with embedded proto fields |
| `consensus-types/blocks/factory.go` | 50+ case factory for proto block conversion |
| `beacon-chain/state/state-native/state_trie.go` | 8 `InitializeFromProto*` functions |
| `beacon-chain/state/state-native/getters_state.go` | `ToProtoUnsafe` with 8-way version switch |
| `beacon-chain/state/interfaces.go` | `ToProto() any` interface definition |
| `proto/prysm/v1alpha1/beacon_block.proto` | Deprecated Generic block wrappers |
| `testing/assert/assertions.go` | Proto equality limitation documented |

## Solution and Action Plan

### Goal
Decouple domain models from protobuf types so proto is only used at serialization boundaries (disk, network, RPC).

### Phase 1: Define Native Domain Types (Medium Effort)
1. Create pure Go structs for core types (`Validator`, `Eth1Data`, `Fork`, `Checkpoint`, `SyncCommittee`, etc.) under `consensus-types/`.
2. These structs should be simple, with no proto dependency.
3. The `BeaconBlockBody` already partially does this (it wraps proto types) -- convert the remaining proto fields to native Go types.

### Phase 2: Conversion Layer at Boundaries (Medium Effort)
1. Implement `ToProto()` / `FromProto()` converters as standalone functions in a `convert` package.
2. These converters should only be called at RPC handlers, DB read/write, and SSZ serialization boundaries.
3. Remove `ToProto() any` and `ToProtoUnsafe() any` from the `BeaconState` interface; replace with typed methods.

### Phase 3: Eliminate Factory Type Switches (High Effort)
1. Replace `NewSignedBeaconBlock(any)` with versioned constructors: `NewSignedBeaconBlockPhase0(...)`, etc.
2. Alternatively, use a builder pattern with a version discriminator.
3. Remove the `GenericSignedBeaconBlock` oneof wrapper entirely (it's already marked deprecated).

### Phase 4: Remove `proto.Clone` Overhead (Medium Effort)
1. With native Go types, use standard struct copying or implement a custom `Copy()` method.
2. Remove the `InitializeFromProto*` / `InitializeFromProtoUnsafe*` dual pattern.

### Estimated Impact
- Eliminates 50+ `proto.Clone` calls
- Removes 50+ case type switch in factory
- Enables compile-time type safety for all state/block operations
- Simplifies test assertions (standard Go equality works)
