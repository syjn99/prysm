# CLAUDE.md - Prysm Development Guide

> ⚠️ **NEVER commit this file or the `.claude/` directory. Both are excluded via `.git/info/exclude`.**

## Project
- **Repo:** OffchainLabs/prysm (Ethereum consensus client, Go)
- **Build:** Bazel (primary) — `bazel build //...`
- **Module:** `github.com/OffchainLabs/prysm/v7`

## Git Rules
- `origin` = OffchainLabs/prysm — **NEVER push here. NEVER open PRs here.**
- `fork` = syjn99/prysm — push all branches here
- Default branch: `develop`
- Jun opens upstream PRs manually after review

## Skills (use these instead of raw commands)
- `/precheck` — gofmt, goimports, gazelle, build
- `/test` — unit tests with baseline comparison
- `/e2e` — end-to-end tests
- `/pr` — full PR workflow (precheck → test → e2e → commit → push)

## Code Style
- Follow existing Go conventions
- Table-driven tests preferred
- `gofmt` / `goimports` required
- Update BUILD.bazel when adding new Go files (gazelle)

## Key Directories
- `beacon-chain/` — Core beacon node
- `validator/` — Validator client
- `proto/` — Protobuf definitions
- `api/` — REST API server
- `cmd/` — CLI entrypoints
- `config/` — Network configs
- `consensus-types/` — Consensus types
- `testing/endtoend/` — E2E tests
- `hack/` — Dev scripts (update-go-pbs.sh, update-go-ssz.sh, check_gazelle.sh)

## Task Tracking
See `~/.openclaw/workspace-coding/PRYSM_TODOS.md` for issue triage and work log.

## Test Baseline
See `~/.openclaw/workspace-coding/prysm-baseline-tests.json` for known test failures on develop.
Compare your test results against this to distinguish pre-existing vs new failures.
