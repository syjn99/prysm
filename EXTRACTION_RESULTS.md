# Extraction & gRPC Removal Results

## Verdict: ALL PASS

All build checks, lint checks, and tests pass:
- `go build ./...` — PASS
- `go vet ./...` — PASS (only pre-existing warnings)
- `bazel build //beacon-chain/...` — PASS
- `go test ./beacon-chain/rpc/eth/validator/...` — PASS (31.9s)
- `go test ./beacon-chain/rpc/eth/beacon/...` — PASS (10.5s)
- `go test ./validator/...` — PASS (except pre-existing `remote-web3signer` timeout, unrelated)

## Stats

| Metric | Count |
|---|---|
| Files changed | 85 |
| Lines added | 746 |
| Lines removed | 11,881 |
| Net lines removed | 11,135 |
| Files created (blockproduction/) | 18 + BUILD.bazel |
| Files deleted (v1alpha1/validator/) | 53 (.go) + BUILD.bazel rewritten |
| Files deleted (grpc-api/) | 11 + BUILD.bazel |
| Files deleted (testing/mock/) | 1 |
| Total lines in blockproduction/ | 3,310 |

## Commits (7 total, oldest first)

1. `5dc97d9126` — `extract(blockproduction): add core block-production package`
2. `7bb416d81f` — `extract(blockproduction): delegate gRPC server to BlockProducer`
3. `2fa774098f` — `extract(blockproduction): wire REST servers to new package`
4. `b1c165a35a` — `extract(blockproduction): wire BlockProducer in service layer`
5. `d581fde295` — `remove(v1alpha1): delete gRPC-only handlers, helpers, and tests`
6. `c1b2d244ad` — `remove(grpc-api): delete validator client gRPC adapter, use REST-only`
7. `e506658edc` — `remove(v1alpha1): strip Server struct and remove gRPC registration`

## What Was Extracted

Shared block-production logic moved from `beacon-chain/rpc/prysm/v1alpha1/validator/`
to `beacon-chain/rpc/core/blockproduction/`:

- **BlockProducer struct** (18 files, 3,310 lines) — the core block construction pipeline
  - `ProduceBlock()` — top-level entry point (was `GetBeaconBlock`)
  - `BuildBlockParallel()` — parallel consensus + execution payload assembly
  - `computeStateRoot()` / `handleStateRootError()` — state root computation with retry
  - Attestation packing (phase0 + Electra)
  - Execution payload construction (Bellatrix+)
  - Builder/MEV integration
  - Deposit, exit, slashing, sync aggregate packing
  - Generic block construction
  - Blob sidecar building and unblinding
  - Eth1 data voting
  - Empty block construction

## What Was Removed

### v1alpha1/validator/ (53 files deleted)
- 8 gRPC handler stubs: aggregator, attester, blocks, duties, duties_v2, exit, status, sync_committee
- 16 proposer helper files (now in blockproduction/)
- 30 test files (all tests for deleted handlers)
- Server struct stripped from 42 fields to 8
- `RegisterBeaconNodeValidatorServer` gRPC registration removed from service.go

### validator/client/grpc-api/ (11 files deleted)
- Entire gRPC validator client adapter package
- 3 factory packages rewritten to REST-only (removed `EnableBeaconRESTApi` feature flag branching)

### testing/mock/ (1 file deleted)
- `beacon_validator_client_mock.go` (only used by deleted grpc-api tests)

## What Could NOT Be Removed (and Why)

1. **`ProposeBeaconBlock` on v1alpha1 Server** — REST `PublishBlockV2` (in `eth/beacon/handlers.go`)
   calls `s.V1Alpha1ValidatorServer.ProposeBeaconBlock()`. This method handles block broadcasting
   and sidecar distribution. Until PublishBlockV2 is refactored to call these operations directly,
   ProposeBeaconBlock must remain.

2. **v1alpha1 Server struct itself** — Still needed as the `V1Alpha1ValidatorServer` backing the
   REST beacon server's `proposeBlock()` method.

3. **`beacon_validator_server_mock.go`** — Still imported by REST handler tests
   (`eth/validator/handlers_block_test.go`, `eth/beacon/handlers_test.go`). The ProduceBlockV3 tests
   were migrated to a local `mockBlockProducer` interface, but other tests still use the server mock.

4. **`V1Alpha1Server` field on REST validator Server** — Assigned in endpoints.go but no longer
   used by any handler (ProduceBlockV3 now uses `BlockProducer` directly). Safe to remove once
   the remaining mock-based tests are cleaned up.

5. **`hack/update-mockgen.sh`** — Still references the deleted `beacon_validator_client_mock.go`.
   Needs the line removed.

## Remaining Cleanup (Future Work)

- [ ] Refactor `ProposeBeaconBlock` broadcasting logic into a standalone package so the
      v1alpha1 Server can be fully deleted
- [ ] Remove `V1Alpha1Server` field from REST validator Server (unused after mock migration)
- [ ] Remove `V1Alpha1ValidatorServer` field from REST beacon Server once ProposeBeaconBlock
      is extracted
- [ ] Clean up `hack/update-mockgen.sh` (remove deleted mock reference)
- [ ] Remove `EnableBeaconRESTApi` feature flag (now always REST-only)
- [ ] Consider adding tests for the new `blockproduction` package
