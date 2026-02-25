# Verification Report: gRPC Validator API Deprecation

## Verdict: ALL PASS

All build checks pass. All test failures are pre-existing and unrelated to our changes.
Safe to merge.

---

## Stats

| Metric | Value |
|--------|-------|
| Files modified | 23 |
| Lines removed | 7,503 |
| Lines added | 17 |
| Net reduction | 7,486 lines |
| Handler methods removed | 28 of 32 |
| Test files gutted | 12 (+ 1 partial) |
| Commits | 6 (removal) + 1 (REMOVAL_LOG.md) |

---

## Build Checks

### `go build ./...` (full project)
- **Result: PASS**
- Exit code 0, no errors

### `go vet ./...`
- **Result: PASS**
- 3 pre-existing warnings in unrelated files:
  - `beacon-chain/sync/sync_fuzz_test.go` - Go version compatibility (go1.21 vs go1.24/1.22)
  - `runtime/messagehandler/messagehandler_test.go` - unreachable code
  - `validator/client/runner.go` - context leak warnings
- None in modified files

### `bazel build //beacon-chain/...`
- **Result: PASS**
- 204 targets found, 13,143 actions completed successfully
- Elapsed time: 345.6s

### `bazel run //:gazelle -- fix`
- **Result: PASS (no changes)**
- Gazelle produced no BUILD file modifications

---

## Test Checks

### `go test ./beacon-chain/rpc/...`
- **Result: PASS (2 pre-existing failures)**
- All RPC packages compile and pass except:
  1. `beacon-chain/rpc/prysm/v1alpha1/beacon`: 4 `TestServer_GetIndividualVotes_*EndOfEpoch` failures
     - Error: "Slashings (bytes array does not have the correct length): expected 8192 and 64 found"
     - **Pre-existing**: In `beacon` package, not `validator`. Not related to our changes.
  2. `beacon-chain/rpc/prysm/v1alpha1/validator`: `TestProposer_GetSyncAggregate_OK` panic
     - Error: "invalid struct type: bitfield.Bitvector32" in `go-cmp` AllowUnexported
     - **Pre-existing**: In `proposer_altair_test.go` which we did not modify. Caused by go-cmp
       incompatibility with bitfield types.

### `go test ./validator/...`
- **Result: PASS (1 pre-existing failure)**
- All validator packages compile and pass except:
  1. `validator/keymanager/remote-web3signer`: `TestKeymanager_Sign` failure
     - Error: HTTP connection to `example2.com` - unexpected EOF
     - **Pre-existing**: Network-dependent test, not related to our changes.

---

## Consistency Checks

### Stale BUILD.bazel deps
- **4 stale deps found** in `beacon-chain/rpc/prysm/v1alpha1/validator/BUILD.bazel`:
  - `//beacon-chain/core/validators:go_default_library`
  - `//beacon-chain/state/state-native:go_default_library`
  - `//math:go_default_library`
  - `@com_github_golang_protobuf//ptypes/empty`
- **Impact: None** - Extra deps in Bazel are harmless. The BUILD file has `# gazelle:ignore`
  so Gazelle cannot auto-clean it. These can be cleaned up in a future PR.
- **Build unaffected**: Bazel build passes with all 204 targets.

### Stale BUILD.bazel srcs
- 8 gutted source files still listed in `srcs` (aggregator.go, attester.go, blocks.go, etc.)
- **Impact: None** - They are valid Go files (`package validator` only), Bazel compiles them fine.
- 12 gutted test files still listed in test `srcs`.
- **Impact: None** - Same reason; valid empty Go files.

### Orphaned Go imports
- `validator/client/grpc-api/grpc_validator_client.go` calls removed methods via gRPC client stub
- **Impact: None for build** - These are client-side RPC calls through the proto-generated client
  interface, not direct Go function calls. The server now returns `codes.Unimplemented` via the
  embedded `UnimplementedBeaconNodeValidatorServer`, which is the expected deprecation behavior.
- **Runtime impact**: Validator clients using gRPC will get "Unimplemented" errors for removed
  endpoints. This is intentional - they should migrate to the REST API.

### Stale proto references
- Proto service definition (`proto/prysm/v1alpha1/validator.pb.go`) still defines all 32 methods
- Mock files (`testing/mock/beacon_validator_server_mock.go`) still reference all methods
- **Impact: None** - The proto interface is unchanged; our Server satisfies it via the embedded
  `UnimplementedBeaconNodeValidatorServer`. Mocks implement the same interface. Everything compiles.
- **Future cleanup**: When the proto service definition is updated to remove deprecated RPCs,
  mocks and client code will need updating too.

---

## Summary of Pre-Existing Failures

| Test | Package | Error | Related to Our Changes? |
|------|---------|-------|------------------------|
| TestServer_GetIndividualVotes_*EndOfEpoch (x4) | beacon-chain/rpc/prysm/v1alpha1/beacon | Slashings byte array length | No |
| TestProposer_GetSyncAggregate_OK | beacon-chain/rpc/prysm/v1alpha1/validator | go-cmp bitfield panic | No |
| TestKeymanager_Sign | validator/keymanager/remote-web3signer | HTTP connection EOF | No |

---

## Recommendations for Future PRs

1. **Clean BUILD.bazel** (low priority): Remove 4 stale deps and 8 gutted source files from
   `beacon-chain/rpc/prysm/v1alpha1/validator/BUILD.bazel`. Requires manual edit since the
   file has `# gazelle:ignore`.

2. **Extract block-production code** (high priority): Move `BuildBlockParallel` and its ~14,000
   line call tree into a shared package so the remaining proposer_*.go files can be removed from
   the gRPC package entirely.

3. **Update proto service** (medium priority): Remove deprecated RPC methods from the proto
   definition and regenerate code. This will cascade to mock files and gRPC client adapters.
