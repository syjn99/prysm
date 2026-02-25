# Problem: Deprecated gRPC API Still Actively Maintained

## Summary

The v1alpha1 gRPC validator API is officially deprecated in favor of a REST API, yet it remains fully maintained with ~19,500 lines of code. This creates a dual API maintenance burden where every feature and bug fix must be implemented twice.

## Evidence

### 1. Explicit Deprecation Notice

`beacon-chain/rpc/prysm/v1alpha1/validator/proposer.go:50-51`:

```go
// Deprecated: The gRPC API will remain the default and fully supported through v8
// (expected in 2026) but will be eventually removed in favor of REST API.
//
// GetBeaconBlock is called by a proposer during its assigned slot...
```

The target date (2026) has arrived, but the code remains fully active.

### 2. Massive Code Volume

The `beacon-chain/rpc/prysm/v1alpha1/validator/` directory contains:

| File | Lines |
|------|-------|
| `proposer.go` | 717 |
| `proposer_test.go` | 3,742 |
| `status.go` | 446 |
| `status_test.go` | 1,298 |
| `server.go` | ~200 |
| Other files | ~13,000+ |
| **Total** | **~19,500** |

### 3. Feature Flags Reflect Deprecation

`config/features/flags.go:48-51`:

```go
disableGRPCConnectionLogging = &cli.BoolFlag{
    Name: "disable-grpc-connection-logging",
    Usage: `WARNING: The gRPC API will remain the default and fully supported through v8
    (expected in 2026) but will be eventually removed in favor of REST API...`,
}
```

### 4. Deprecated Proto Service Definitions

The exploration found deprecated RPC service definitions in:
- `proto/prysm/v1alpha1/debug.proto` (10+ deprecated messages)
- `proto/prysm/v1alpha1/node.proto` (9+ deprecated RPC methods)

These generate Go code that must be compiled and maintained.

### 5. Dual Implementation Requirement

Every validator-facing feature must be implemented in both:
1. **gRPC path**: `beacon-chain/rpc/prysm/v1alpha1/validator/` (proto-based)
2. **REST path**: `beacon-chain/rpc/eth/` (JSON-based, Beacon API standard)

This doubles the implementation, testing, and review effort for every validator-related change.

## Key Files

| File | Role |
|------|------|
| `beacon-chain/rpc/prysm/v1alpha1/validator/proposer.go` | Deprecated block proposal gRPC handler |
| `beacon-chain/rpc/prysm/v1alpha1/validator/` | Full deprecated gRPC validator API |
| `config/features/flags.go` | Deprecation warnings in CLI flags |
| `proto/prysm/v1alpha1/debug.proto` | Deprecated debug service definitions |
| `proto/prysm/v1alpha1/node.proto` | Deprecated node service definitions |

## Solution and Action Plan

### Goal
Complete the migration from gRPC to REST API and remove the deprecated gRPC validator API.

### Phase 1: Audit REST API Completeness (Low Effort)
1. Inventory all gRPC endpoints in `v1alpha1/validator/`.
2. For each endpoint, verify it has a REST equivalent in `beacon-chain/rpc/eth/`.
3. Identify any gaps where gRPC has functionality not yet in REST.
4. Produce a migration gap report.

### Phase 2: Fill REST API Gaps (Medium Effort)
1. Implement any missing REST endpoints identified in Phase 1.
2. Ensure feature parity between gRPC and REST.
3. Add integration tests for the REST equivalents.

### Phase 3: Deprecation Warnings in Client (Low Effort)
1. Add runtime log warnings when gRPC endpoints are called: "This gRPC endpoint is deprecated. Please migrate to the REST API."
2. Add a CLI flag `--disable-grpc-validator-api` to allow operators to verify they don't depend on it.

### Phase 4: Remove gRPC Validator API (High Effort)
1. Remove `beacon-chain/rpc/prysm/v1alpha1/validator/` entirely.
2. Remove deprecated proto service definitions.
3. Remove gRPC-specific mock generation from `hack/update-mockgen.sh`.
4. Clean up `config/features/flags.go` deprecation flags.

### Phase 5: Clean Up Proto Definitions (Medium Effort)
1. Remove `GenericSignedBeaconBlock`, `GenericBeaconBlock` and other deprecated message wrappers.
2. Remove deprecated service definitions from `debug.proto` and `node.proto`.

### Estimated Impact
- Removes ~19,500 lines of code and tests
- Halves the effort for every future validator API change
- Simplifies the proto build pipeline
- Reduces binary size and compilation time
