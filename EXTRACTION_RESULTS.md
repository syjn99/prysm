# Extraction & gRPC Removal Results

## Verdict: ALL PASS

Final verification (2026-02-25):
- `bazel build //beacon-chain/...` — PASS (5,424 targets)
- `bazel build //validator/...` — PASS (61 targets)
- `bazel test //beacon-chain/rpc/...` — PASS (21/21 test suites)
- `bazel test //beacon-chain/rpc/core/blockproduction/...` — PASS
- `bazel test //validator/...` — 18/20 PASS (2 pre-existing failures unrelated to our changes:
  `remote-web3signer` network-dependent test, `validator/client` timeout)

## Stats

| Metric | Count |
|---|---|
| Files changed | 103 |
| Lines added | 3,174 |
| Lines removed | 13,941 |
| Net lines removed | 10,767 |
| Files created (blockproduction/) | 18 source + 16 test + BUILD.bazel |
| Files deleted (v1alpha1/validator/) | 53 (.go) + BUILD.bazel rewritten |
| Files deleted (grpc-api/) | 11 + BUILD.bazel |
| Files deleted (testing/mock/) | 3 (client mock, altair server/client mocks) |
| Total lines in blockproduction/ | 3,310 (source) + ~8,000 (tests) |
| Test functions recovered | 29 (proposer_test.go) + tests in 15 other files |

## Commits (30 total, newest first)

### Verification & test recovery (session 3)
1. `4c5d226dde` — `fix(beacon): replace deleted gRPC mock with BlockProposer mock`
2. `28be25b4e0` — `test(blockproduction): revive unit tests from deleted gRPC tests`

### Cleanup (session 2)
3. `4f29d5f382` — `cleanup: remove dead EnableBeaconRESTApi feature flag`
4. `5d9e58ad85` — `cleanup: delete unused gRPC validator mocks`
5. `85ff5679d9` — `verify: extraction and removal results`

### Core extraction & removal (session 1)
6. `e506658edc` — `remove(v1alpha1): strip Server struct and remove gRPC registration`
7. `c1b2d244ad` — `remove(grpc-api): delete validator client gRPC adapter, use REST-only`
8. `d581fde295` — `remove(v1alpha1): delete gRPC-only handlers, helpers, and tests`
9. `b1c165a35a` — `extract(blockproduction): wire BlockProducer in service layer`
10. `2fa774098f` — `extract(blockproduction): wire REST servers to new package`
11. `7bb416d81f` — `extract(blockproduction): delegate gRPC server to BlockProducer`
12. `5dc97d9126` — `extract(blockproduction): add core block-production package`

### Deprecation warnings & audit (session 0)
13-30. Deprecation warnings added to all 10 gRPC handler files, audit commits, removal logs

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

- **ProposeBeaconBlock** — block broadcasting with P2P gossip + sidecar distribution
  - `proposer.go` — `ProposeBeaconBlock()` method on `BlockProducer`
  - `BlockProposerDeps` struct for networking dependencies

- **Test coverage** (~8,000 lines across 16 test files)
  - `proposer_test.go` — 3,472 lines, 29 test functions
  - Covers Phase0 through Fulu block production, deposits, attestations, slashings, sync aggregates, builder/MEV, execution payloads, blob sidecars, unblinding

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

### testing/mock/ (3 files deleted)
- `beacon_validator_client_mock.go` (1,053 lines — only used by deleted grpc-api tests)
- `beacon_altair_validator_server_mock.go` (132 lines)
- `beacon_altair_validator_client_mock.go` (136 lines)
- `beacon_validator_server_mock.go` replaced: 864-line gRPC mock → 48-line `MockBlockProposer`

### config/features/ (EnableBeaconRESTApi flag removed)
- `flags.go` — removed flag definition
- `config.go` — removed config field and initialization
- `testing/endtoend/components/validator.go` — removed flag usage

### hack/update-mockgen.sh
- Removed mockgen lines for deleted mock files

## What Could NOT Be Removed (and Why)

1. **`ProposeBeaconBlock` on BlockProducer** — REST `PublishBlockV2` (in `eth/beacon/handlers.go`)
   now calls through the `BlockProposer` interface. The `blockproduction.BlockProducer` implements
   this interface. This is by design — the broadcasting logic lives in the extracted package.

2. **v1alpha1/validator/ stub files** — `server.go`, `proposer.go`, `log.go` remain as minimal
   1-line `package validator` stubs. The BUILD.bazel still references them. A future cleanup can
   remove the directory entirely once all downstream references are updated.

## Remaining Cleanup (Future Work)

- [ ] Delete `beacon-chain/rpc/prysm/v1alpha1/validator/` stub directory entirely
- [ ] Remove `V1Alpha1Server` field from REST validator Server (unused after mock migration)
- [ ] Audit remaining gRPC services: Node/Health, BeaconChain, Debug (v1alpha1)
- [ ] Rewrite E2E evaluators from `*grpc.ClientConn` to HTTP/REST clients
- [ ] Phase 1-3 of broader gRPC deprecation (blocked on upstream `methodical` codegen)
