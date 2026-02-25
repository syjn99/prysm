# Problem: Fragile Manual Test Infrastructure

## Summary

Test infrastructure relies on manually-maintained mock generation scripts, fragmented mock locations, and complex state generation helpers. Mock regeneration is easy to forget after interface changes, leading to silently stale mocks.

## Evidence

### 1. Manual Mock Generation Script

`hack/update-mockgen.sh` (83 lines) must be run manually after interface changes:

```bash
# Script to update mock files after proto/prysm/v1alpha1/services.proto changes.
# Be sure to install mockgen before use: https://github.com/uber-go/mock
```

The script generates mocks for 4 separate module groups:

**Proto mocks (v1alpha1)** -- lines 12-16:
```bash
proto_mocks_v1alpha1=(
    "$mock_path/beacon_service_mock.go BeaconChainClient"
    "$mock_path/beacon_validator_server_mock.go BeaconNodeValidatorServer,..."
    "$mock_path/beacon_validator_client_mock.go BeaconNodeValidatorClient,..."
    "$mock_path/node_service_mock.go NodeClient"
)
```

**Interface mocks** -- lines 29-34:
```bash
iface_mocks=(
    "$iface_mock_path/chain_client_mock.go ChainClient"
    "$iface_mock_path/prysm_chain_client_mock.go PrysmChainClient"
    "$iface_mock_path/node_client_mock.go NodeClient"
    "$iface_mock_path/validator_client_mock.go ValidatorClient"
)
```

**Beacon API mocks** -- lines 49-55:
```bash
beacon_api_mocks=(
    "$beacon_api_mock_path/genesis_mock.go genesis.go"
    "$beacon_api_mock_path/duties_mock.go duties.go"
    "$beacon_api_mock_path/json_rest_handler_mock.go json_rest_handler.go"
    "$beacon_api_mock_path/state_validators_mock.go state_validators.go"
    "$beacon_api_mock_path/beacon_block_converter_mock.go beacon_block_converter.go"
)
```

**BLS crypto mocks** -- lines 70-72:
```bash
crypto_bls_common_mocks=(
    "$crypto_bls_common_mock_path/interface_mock.go interface.go"
)
```

### 2. Mocks Scattered Across 4+ Locations

| Location | Purpose |
|----------|---------|
| `testing/mock/` | Proto service mocks |
| `testing/validator-mock/` | Interface mocks |
| `validator/client/beacon-api/mock/` | Beacon API mocks |
| `crypto/bls/common/mock/` | BLS crypto mocks |

Each location has its own naming conventions, package names, and formatting steps. The script runs `goimports` and `gofmt` separately for each group.

### 3. No Automated Mock Regeneration

There is no `go generate` directive, CI check, or pre-commit hook that verifies mocks are up to date. If an interface changes and `hack/update-mockgen.sh` is not run:
- Old mocks still compile (they implement the old interface)
- Tests using stale mocks may pass incorrectly
- The mismatch is only caught when someone notices unexpected behavior

### 4. Proto Equality Issues in Test Assertions

`testing/assert/assertions.go:19-20`:

```go
// NOTE: this function does not work for checking arrays/slices or maps of protobuf messages.
// For arrays/slices, please use DeepSSZEqual.
```

Tests must use special assertion functions (`DeepSSZEqual`) for protobuf types, creating a non-obvious testing pitfall.

### 5. Complex State Generation Helpers

The `testing/` directory contains helper packages for generating test fixtures:
- `beacon-chain/state/testing/` -- state generators for all forks
- `beacon-chain/state/stategen/` -- 4+ test files with complex state generation

Test helpers must be kept in sync with state changes across all forks.

## Key Files

| File | Role |
|------|------|
| `hack/update-mockgen.sh` | Manual mock generation script |
| `testing/mock/` | Proto service mocks |
| `testing/validator-mock/` | Interface mocks |
| `validator/client/beacon-api/mock/` | Beacon API mocks |
| `testing/assert/assertions.go` | Assertion functions with proto caveats |

## Solution and Action Plan

### Goal
Automate mock generation so it's impossible for mocks to drift out of sync with interfaces.

### Phase 1: Add `go generate` Directives (Low Effort)
1. Add `//go:generate mockgen ...` comments to the source interfaces:
   ```go
   //go:generate mockgen -destination=../../testing/mock/beacon_service_mock.go -package=mock . BeaconChainClient
   type BeaconChainClient interface { ... }
   ```
2. Run `go generate ./...` to regenerate all mocks.
3. Remove `hack/update-mockgen.sh` in favor of the standard Go pattern.

### Phase 2: CI Verification (Low Effort)
1. Add a CI step that runs `go generate ./...` and checks for diffs.
2. If generated files differ from committed files, fail the build.
3. This ensures mocks are always up to date on every PR.

### Phase 3: Consolidate Mock Locations (Medium Effort)
1. Move all mocks to a single `testing/mocks/` directory with sub-packages:
   ```
   testing/mocks/
       proto/       # Proto service mocks
       validator/   # Validator client mocks
       beaconapi/   # Beacon API mocks
       crypto/      # BLS mocks
   ```
2. Consistent naming: `<interface>_mock.go`.

### Phase 4: Address Proto Assertion Pitfall (Low Effort)
1. Consider replacing `DeepEqual` + `DeepSSZEqual` with a single `ProtoEqual` function that handles proto messages transparently.
2. Or add a linter rule that flags `DeepEqual` usage on proto types.

### Estimated Impact
- Eliminates stale mock bugs entirely
- New contributors use standard `go generate` instead of discovering a hidden script
- CI catches mock drift before merge
- Consolidates mock locations for discoverability
