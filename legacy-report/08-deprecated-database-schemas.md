# Problem: Deprecated Database Schemas and Migration Burden

## Summary

The validator database maintains deprecated attestation protection schemas alongside their replacements. Database migration code must be maintained indefinitely to support upgrade paths, and the deprecated code adds cognitive overhead.

## Evidence

### 1. Deprecated Attester Protection Code

`validator/db/kv/deprecated_attester_protection.go` (126 lines) contains a full implementation of the old attestation history format:

```go
// deprecatedHistoryData stores the needed data to confirm if an attestation is slashable
type deprecatedHistoryData struct {
    Source      primitives.Epoch
    SigningRoot []byte
}

type deprecatedEncodedAttestingHistory []byte
```

This includes:
- `assertSize()` -- validation
- `getLatestEpochWritten()` / `setLatestEpochWritten()` -- epoch tracking
- `getTargetData()` / `setTargetData()` -- attestation data access
- `newDeprecatedAttestingHistory()` -- constructor

All of these exist solely for migration support.

### 2. Deprecated Bucket Still in Schema

`validator/db/kv/schema.go:9`:

```go
deprecatedAttestationHistoryBucket = []byte("attestation-history-bucket-interchange")
```

This bucket is referenced alongside the new optimized buckets (lines 26-29):

```go
// Optimized slashing protection buckets and keys.
pubKeysBucket                 = []byte("pubkeys-bucket")
attestationSigningRootsBucket = []byte("att-signing-roots-bucket")
attestationSourceEpochsBucket = []byte("att-source-epochs-bucket")
attestationTargetEpochsBucket = []byte("att-target-epochs-bucket")
```

The deprecated bucket must remain defined because the migration code references it.

### 3. Migration Files

The validator DB contains a migration from the deprecated format:

- `validator/db/kv/migration_optimal_attester_protection.go` -- migrates from `deprecatedAttestationHistoryBucket` to the new optimized buckets
- `validator/db/kv/migration_optimal_attester_protection_test.go` -- tests for the migration

The beacon-chain DB has its own set of migrations in `beacon-chain/db/kv/`.

### 4. Deprecated Feature Flags Infrastructure

`config/features/deprecated_flags.go` defines a deprecation framework:

```go
const deprecatedUsage = "DEPRECATED. DO NOT USE."

var deprecatedFlags = []cli.Flag{}  // Empty -- nothing fully deprecated yet

var upcomingDeprecation = []cli.Flag{
    enableHistoricalSpaceRepresentation,
}

// deprecatedBeaconFlags contains flags that are still used by other components
// and therefore cannot be added to deprecatedFlags
var deprecatedBeaconFlags = []cli.Flag{
    deprecatedDisableLastEpochTargets,
}
```

The `deprecatedBeaconFlags` category exists for flags that are deprecated but can't be removed because other components depend on them -- a maintenance limbo.

## Key Files

| File | Role |
|------|------|
| `validator/db/kv/deprecated_attester_protection.go` | 126 lines of deprecated attestation format |
| `validator/db/kv/schema.go` | Deprecated + current bucket definitions |
| `validator/db/kv/migration_optimal_attester_protection.go` | Migration from deprecated to new format |
| `config/features/deprecated_flags.go` | Deprecated feature flag infrastructure |

## Solution and Action Plan

### Goal
Remove deprecated code where safe, and establish a migration lifecycle so deprecated schemas don't persist indefinitely.

### Phase 1: Assess Migration Completion (Low Effort)
1. Determine what percentage of validator databases in production have already been migrated.
2. If migration was introduced multiple versions ago (it was), most databases should already be upgraded.
3. Add telemetry to track how often the migration path is triggered.

### Phase 2: Version-Gate Deprecated Code (Medium Effort)
1. Define a minimum supported database version (e.g., "databases created before v6 are not supported -- run the migration tool first").
2. Add a startup check: if the deprecated bucket exists and hasn't been migrated, print instructions and exit.
3. This allows removing the inline migration logic from the normal startup path.

### Phase 3: Remove Deprecated Code (Low Effort, After Phase 2)
1. Delete `deprecated_attester_protection.go` and its test file.
2. Remove `deprecatedAttestationHistoryBucket` from `schema.go`.
3. Move the migration to a standalone CLI tool for users who still need it.

### Phase 4: Establish Migration Lifecycle Policy (Low Effort)
1. Define a policy: "Migrations are supported for N major versions. After that, users must run an offline migration tool."
2. Tag each migration with the version it was introduced.
3. Periodically prune old migrations.

### Phase 5: Clean Up Feature Flags (Low Effort)
1. Remove flags in `deprecatedBeaconFlags` that are no longer needed by any component.
2. Move `upcomingDeprecation` flags to `deprecatedFlags` if they're past their deprecation window.
3. Remove the `exampleDeprecatedFeatureFlag` template -- it adds noise.

### Estimated Impact
- Removes ~250 lines of deprecated code and tests
- Simplifies the validator DB startup path
- Establishes a clear lifecycle for future deprecations
- Reduces confusion for new contributors reading the schema
