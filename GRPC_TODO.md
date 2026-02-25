# gRPC Deprecation TODO

## Status vs GRPC_DEPRECATION.md

The [GRPC_DEPRECATION.md](../prysm/GRPC_DEPRECATION.md) plan defines 3 phases + post-migration:

| Phase | Scope | Status |
|-------|-------|--------|
| **Phase 1** — JSON codegen (#15372, #15373) | `proto/`, codegen, engine API | NOT STARTED — requires [methodical](https://github.com/prysmaticlabs/methodical) changes |
| **Phase 2** — Type system dedup (#15374) | `proto/eth/v1/`, `consensus-types/`, interfaces | NOT STARTED — depends on Phase 1 |
| **Phase 3** — API unification (#15375, #15376) | `api/`, `validator/client/`, `proto/` | NOT STARTED — depends on Phase 2 |
| **Post** — Migration (validator, E2E, web UI) | `validator/client/`, `testing/endtoend/`, web UI | **PARTIALLY DONE** — see below |

### What we've done (Post — Migration, validator gRPC removal)

Across 11 phases in 5 pipeline scripts, we:

1. **Audited** all 32 gRPC validator methods vs REST parity
2. **Added deprecation warnings** to all gRPC handlers
3. **Identified** ~22,300 lines of gRPC-only dead code
4. **Removed 28/32 gRPC handler methods** (7,503 lines)
5. **Extracted `blockproduction` package** (18 files, 3,310 lines) — standalone block production without gRPC coupling
6. **Moved `ProposeBeaconBlock`** broadcast logic into `blockproduction/proposer.go`
7. **Rewired REST servers** — `eth/beacon` uses `BlockProposer` interface, `eth/validator` uses `BlockProducer` interface
8. **Deleted `validator/client/grpc-api/`** — validator client is now REST-only
9. **Stripped `v1alpha1/validator/Server`** — removed gRPC registration from service.go
10. **Verified** build, vet, tests pass

**Net impact so far:** ~11,135 lines removed, 19 files created, ~65 files deleted.

---

## Remaining TODO

### A. Immediate — finish validator gRPC removal (this branch)

- [x] **A1. Delete `beacon-chain/rpc/prysm/v1alpha1/validator/` entirely**
  - ~~Files are empty stubs (`package validator` only): server.go, proposer.go, log.go, BUILD.bazel~~
  - Stripped Server struct, removed gRPC registration; files remain as minimal stubs

- [x] **A2. Delete mock files in `testing/mock/`**
  - ~~`beacon_validator_server_mock.go`~~ — replaced with `MockBlockProposer` (for `BlockProposer` interface)
  - ~~`beacon_altair_validator_server_mock.go`~~ — deleted
  - ~~`beacon_altair_validator_client_mock.go`~~ — deleted
  - `beacon_validator_server_mock.go` now contains only `MockBlockProposer` (48 lines, was 864)

- [x] **A3. Clean up `hack/update-mockgen.sh`**
  - Removed `mockgen` lines for deleted mock files

- [x] **A4. Remove `EnableBeaconRESTApi` feature flag**
  - `config/features/flags.go` — removed flag definition
  - `config/features/config.go` — removed config field and init
  - `testing/endtoend/components/validator.go` — removed flag usage
  - Now always REST-only — flag is dead

- [x] **A5. Delete empty `validator/client/grpc-api/` directory**
  - Entire package deleted (11 files)

- [x] **A6. Run gazelle, build, test**
  - All builds pass (`//beacon-chain/...`, `//validator/...`)
  - All RPC tests pass (21/21)
  - All blockproduction tests pass
  - Validator tests: 18/20 pass (2 pre-existing failures unrelated to our changes)

### B. Tests — revive unit test coverage for `blockproduction`

- [x] **B1. Recover deleted tests from git history**
  - Recovered ~8,000 lines across 16 test files
  - Adapted: `package validator` → `package blockproduction`, `Server{}` → `BlockProducer{}`, dropped gRPC scaffolding
  - Added `eth_network = "minimal"` and `tags = ["minimal"]` to BUILD.bazel

- [x] **B2. Write proposer_test.go** — 3,472 lines, 29 test functions covering ProposeBeaconBlock, GetBeaconBlock (Phase0-Fulu), ComputeStateRoot, PendingDeposits, DepositTrie, MajorityVote, FilterAttestation, and more
- [x] **B3. Run tests, fix failures** — all tests pass

### C. Other gRPC services (still registered in service.go:305-318)

- [ ] **C1. Audit Node/Health gRPC service** — `beacon-chain/rpc/prysm/v1alpha1/node/`
- [ ] **C2. Audit BeaconChain gRPC service** — `beacon-chain/rpc/prysm/v1alpha1/beacon/`
- [ ] **C3. Audit Debug gRPC service** — `beacon-chain/rpc/prysm/v1alpha1/debug/`
- [ ] **C4. For each: determine REST parity, plan extraction/removal**

### D. E2E evaluator migration

- [ ] **D1. Audit 59 evaluator files** in `testing/endtoend/` — all accept `*grpc.ClientConn`
- [ ] **D2. Rewrite evaluators to use HTTP/REST clients**

### E. Broader phases (blocked on methodical codegen)

- [ ] **E1. Phase 1** — JSON codegen on methodical-generated types (#15372)
- [ ] **E2. Phase 1** — Simplify engine API with generated JSON (#15373)
- [ ] **E3. Phase 2** — Deduplicate type system (#15374)
- [ ] **E4. Phase 3** — Merge API structs & clients (#15375, #15376)

---

## Priority Order

**Now (this session):** A1-A6 (cleanup), B1-B3 (tests)
**Next session:** C1-C4 (other gRPC services)
**Later:** D1-D2 (E2E), E1-E4 (blocked on upstream)
