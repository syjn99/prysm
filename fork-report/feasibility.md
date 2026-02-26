# Feasibility Analysis

> Can we nuke all code below N-1 and keep only the two most recent forks?

## Verdict: Feasible, with caveats

The proposal is **architecturally feasible** thanks to Prysm's existing design choices (unified state struct, checkpoint sync support, interface-based block handling). However, it requires a carefully staged approach and a commitment to the "legacy binary" for historical sync.

---

## What Works in Our Favor

### 1. Unified BeaconState struct

Unlike some CL clients that use per-fork state types, Prysm already has a **single `BeaconState` struct** (`beacon-chain/state/state-native/beacon_state.go`) with a `version int` field. Fork-specific fields are simply zero-valued when inactive.

This means removing old fork support is primarily about:
- Deleting unused `InitializeFromProto*` functions
- Removing old-fork branches from version switches
- Cleaning up field indices that only old forks use

It does **not** require a fundamental restructuring of the state model.

### 2. Checkpoint sync already exists

`beacon-chain/sync/checkpoint/api.go` downloads a finalized state and block, then starts from there. A node never needs to process Phase0 through Deneb states if it checkpoint-syncs from an Electra+ epoch.

This is the key enabler: the main binary doesn't need old-fork STF if it never processes old-fork slots.

### 3. Go module versioning aligns

The module is already at `v7` targeting Fulu. Bumping to `v8` for Gloas is natural. The previous `v7` module remains importable as a dependency for the legacy binary.

### 4. Upgrade functions are local

Each `UpgradeTo<Fork>()` function only transforms from fork N-1 to N. They don't depend on the full old-fork STF. The only upgrade function the main binary needs is the one from N-1 to N (e.g., `UpgradeToGloas` from Fulu state).

---

## What Makes It Hard

### 1. Proto type proliferation

Each fork defines distinct protobuf messages (`BeaconStatePhase0`, `BeaconBlockAltair`, etc.). These are used in:
- gRPC service definitions
- Database serialization
- P2P message encoding
- The `GenericSignedBeaconBlock` oneOf container

Removing old proto messages requires:
- Updating all proto files and regenerating code
- Ensuring the database can still read existing data (or deciding not to)
- Updating P2P message handlers

### 2. Sequential upgrade chain

The `UpgradeState()` function in `transition.go:342-413` calls upgrades sequentially:
```
Phase0 -> Altair -> Bellatrix -> Capella -> Deneb -> Electra -> Fulu -> Gloas
```

If we remove the middle links, a node that receives a Phase0 genesis state can't upgrade it to Electra. This is fine **if checkpoint sync is mandatory**, but breaks genesis sync.

### 3. Altair epoch processing is shared

`altair.ProcessEpoch()` is used for **Altair through Deneb** (4 forks). `electra.ProcessEpoch()` handles Electra, and `fulu.ProcessEpoch()` handles Fulu+. If we remove Altair's epoch processing, we need to verify that Electra's epoch processing doesn't internally depend on any Altair-specific helpers.

Inspection shows that `beacon-chain/core/altair/` contains:
- `epoch_precompute.go` - shared precompute logic reused by Electra
- `reward.go` - reward calculation helpers
- `sync_committee.go` - sync committee utilities

Verified: Electra directly re-exports **11 functions** from `altair/`:

From `electra/transition.go` (aliased into Electra's epoch processing):
- `altair.InitializePrecomputeValidators`
- `altair.ProcessEpochParticipation`
- `altair.ProcessInactivityScores`
- `altair.ProcessRewardsAndPenaltiesPrecompute`
- `altair.ProcessParticipationFlagUpdates`
- `altair.ProcessSyncCommitteeUpdates`
- `altair.AttestationsDelta`

From `electra/attestation.go`:
- `altair.ProcessAttestationsNoVerifySignature`
- `altair.AttestationParticipationFlagIndices`
- `altair.BaseRewardWithTotalBalance`
- `altair.HasValidatorFlag`

**These 11 functions cannot be deleted without porting them to a shared package first.**

### 4. Database backward compatibility

Existing node databases contain SSZ-encoded states and blocks from old forks. Options:
- **Drop and re-sync**: Require checkpoint sync on upgrade. Simple but forces downtime.
- **Migration tool**: Convert old data. Expensive and error-prone.
- **Ignore old data**: Leave it in DB but don't read it. Pragmatic but wastes disk space.

### 5. Spec test coverage

The Ethereum consensus spec tests include test vectors for all forks. Removing old-fork code means:
- Dropping old-fork spec tests from the test suite
- Ensuring remaining tests still pass (no hidden dependencies on old fixtures)

---

## What "N-1 Only" Actually Means

If current fork is Gloas (N=7), keeping N-1 means keeping **Fulu (N-1=6)**.

But Fulu's epoch processing delegates to `fulu.ProcessEpoch`, which internally calls Electra helpers, which call Altair helpers. The dependency chain for epoch processing is:

```
Gloas epoch -> fulu.ProcessEpoch -> calls electra helpers -> calls altair helpers
```

So "keeping only N-1" doesn't mean keeping only the `fulu/` package. It means keeping:
- `fulu/` package (direct N-1)
- `electra/` package (helpers used by Fulu)
- Parts of `altair/` (shared utilities like precompute, rewards, sync committee)

**In practice, the safe deletion set is**: Phase0 epoch processing, Phase0 attestation types, Bellatrix merge transition logic, and the upgrade functions for Altair/Bellatrix/Capella/Deneb. The shared helper code in `altair/` must be carefully audited before removal.

---

## Prerequisites

1. **Checkpoint sync must be mandatory** for the main binary (no genesis sync)
2. **Legacy binary** must be built and tested before deleting old code
3. **Shared helper audit**: identify which functions in `altair/` are imported by `electra/` and `fulu/`
4. **Database strategy**: decide on drop-and-resync vs. ignore-old-data
5. **P2P compatibility**: verify the node can still participate in the network without old-fork message types
6. **Spec test triage**: identify which spec tests cover old-fork-only behavior

---

## Conclusion

| Aspect | Feasibility | Notes |
|--------|------------|-------|
| State struct cleanup | High | Unified struct makes this straightforward |
| Proto/SSZ removal | High | Generated code, mechanical deletion |
| STF simplification | Medium | Shared helpers create hidden dependencies |
| Block factory cleanup | High | Type-switch deletion |
| Checkpoint sync requirement | High | Already implemented |
| Legacy binary | Medium | Needs design and testing |
| Database migration | Low-Medium | Simplest to just require re-sync |
| Altair helper porting | Medium | Requires careful audit |
| **Overall** | **Medium-High** | Feasible with staged approach |
