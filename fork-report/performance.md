# Performance Considerations

> How fork debloating affects binary size, build time, runtime performance, and memory usage.

## TL;DR

The fork debloat is primarily a **developer experience and maintainability** improvement. Runtime performance gains are modest because Prysm already uses a unified state struct and version-gated branching. The biggest measurable wins are in **binary size** and **build time**.

---

## 1. Binary Size

### Generated code reduction

Removing Phase0 through Deneb SSZ and protobuf code eliminates:

| Category | Lines Removed | Estimated Size Reduction |
|----------|-------------:|------------------------:|
| SSZ (phase0 + altair + bellatrix + capella + deneb) | ~17,714 | ~500 KB source |
| Protobuf (old fork messages in beacon_block.pb.go, beacon_state.pb.go) | ~5,000+ | ~150 KB source |
| Core fork packages (altair partial, capella, deneb, execution) | ~5,700+ | ~170 KB source |
| **Total source** | **~28,000+** | **~820 KB** |

After compilation, the binary size reduction would be roughly **2-5 MB** (Go binaries include significant metadata).

### Impact

- Faster container image pulls in CI/CD
- Slightly faster cold start (less code to page in)
- Reduced attack surface (less code = fewer potential bugs)

---

## 2. Build Time

### Current state

The proto/SSZ code generation and compilation of 64,000+ lines of generated code in `proto/prysm/v1alpha1/` is a significant contributor to build time.

### Expected improvement

- **Proto generation**: ~30% faster (fewer messages to process)
- **Go compilation**: Moderate improvement from fewer files and types
- **Test compilation**: Significant improvement if old-fork test fixtures are removed

### Spec test reduction

Old-fork spec tests in `testing/spectest/` can be dropped:
- Phase0, Altair, Bellatrix, Capella, Deneb test suites (both mainnet and minimal configs)
- This reduces CI time substantially as spec tests are the longest-running test suite

---

## 3. Runtime: Epoch Processing

### Current dispatch

```go
// transition.go:317-340
if state.Version() >= version.Fulu {
    fulu.ProcessEpoch(ctx, state)
} else if state.Version() >= version.Electra {
    electra.ProcessEpoch(ctx, state)
} else if state.Version() >= version.Altair {
    altair.ProcessEpoch(ctx, state)
} else {
    ProcessEpochPrecompute(ctx, state) // Phase0
}
```

After debloat, this becomes:

```go
if state.Version() >= version.Fulu {
    fulu.ProcessEpoch(ctx, state)
} else {
    electra.ProcessEpoch(ctx, state)
}
```

### Impact

- **Branch prediction**: Negligible. Modern CPUs predict well for these patterns.
- **Dead code elimination**: Go compiler already optimizes unreachable branches somewhat.
- **Net runtime impact**: **< 0.1%**. Epoch processing cost is dominated by validator set iteration (~1M validators), not version dispatch.

---

## 4. Runtime: State Hashing

### Current state

`beacon-chain/state/state-native/hasher.go` uses version switches to determine which fields to hash. The field trie in `fieldtrie/` builds Merkle trees over state fields.

### After debloat

- Fewer field definitions means slightly simpler trie construction
- The `RealPosition()` function in `types.go` has overlapping positions (e.g., position 15 maps to two different field types) - removing old fields cleans this up
- Field trie initialization is faster with fewer field variants

### Impact

- **Marginal**: State hashing is dominated by tree computation, not field dispatch
- **One-time benefit at startup**: Initializing field tries for a new state is slightly faster

---

## 5. Memory Usage

### Current unified struct

Every `BeaconState` instance allocates space for **all** fork fields regardless of version:

```go
type BeaconState struct {
    // Phase0 fields
    previousEpochAttestations []*ethpb.PendingAttestation  // nil for Altair+
    currentEpochAttestations  []*ethpb.PendingAttestation  // nil for Altair+

    // Altair+ fields
    previousEpochParticipation []byte                      // nil for Phase0
    currentEpochParticipation  []byte                      // nil for Phase0
    currentSyncCommittee      *ethpb.SyncCommittee         // nil for Phase0
    nextSyncCommittee         *ethpb.SyncCommittee         // nil for Phase0

    // Bellatrix+ fields
    latestExecutionPayloadHeader *enginev1.ExecutionPayloadHeader  // nil for pre-Bellatrix

    // ... etc
}
```

### After debloat

Removing Phase0-only fields (`previousEpochAttestations`, `currentEpochAttestations`) and consolidating execution payload header fields saves:
- ~48 bytes per BeaconState instance (2 slice headers + 1-2 pointer fields)
- On a node with ~100 cached states, this saves ~5 KB total

### Impact

- **Negligible at runtime**. The 48 bytes saved per state is insignificant compared to the ~100 MB+ that validator sets and balances consume.
- The real memory savings come from not having to load old-fork proto types into memory, but Go's linker doesn't aggressively dead-strip unused types.

---

## 6. Potential Regressions

### What could get worse

| Concern | Likelihood | Mitigation |
|---------|-----------|------------|
| Legacy binary adds deployment complexity | Medium | Document clearly, automate builds |
| Checkpoint sync becomes a hard requirement | Low (it's already standard) | Provide migration docs |
| Shared helper refactoring introduces bugs | Medium | Comprehensive spec test coverage |
| Database incompatibility on upgrade | Low | Require clean re-sync |

### What won't change

- P2P networking performance (version negotiation is trivial)
- Attestation/block processing speed for current fork (unchanged code paths)
- Validator client performance (operates only on current fork)

---

## 7. Quantitative Summary

| Metric | Before | After | Delta |
|--------|--------|-------|-------|
| Generated code (SSZ + pb) | 64,463 lines | ~41,000 lines | -36% |
| Core fork packages | 13,444 lines | ~7,500 lines | -44% |
| Version switch cases (estimated) | ~510 | ~200 | -61% |
| Binary size (estimated) | ~80 MB | ~75 MB | -6% |
| Epoch processing latency | N ms | N ms | ~0% |
| Memory per BeaconState | M bytes | M - 48 bytes | ~0% |
| Build time (estimated) | T seconds | ~0.8T seconds | -20% |
| CI spec test time | S seconds | ~0.4S seconds | -60% |

The largest gains are in **developer-facing metrics** (build time, code volume, cognitive load) rather than **runtime performance**.
