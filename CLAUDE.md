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
- **NEVER force push** — always use new commits

## Skills (use these instead of raw commands)
- `/precheck` — gofmt, goimports, gazelle, hack scripts, build
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
- `hack/` — Dev scripts (update-go-pbs.sh, update-go-ssz.sh, update-mockgen.sh, check_gazelle.sh)

## External References
> ⚠️ The following paths are machine-specific (Jun's local setup).
> If using on another machine, update these paths accordingly.

- **Task tracking:** `~/.openclaw/workspace-coding/PRYSM_TODOS.md`
- **Test baseline:** `~/.openclaw/workspace-coding/prysm-baseline-tests.json`

These files live in the OpenClaw workspace directory and are NOT part of the Prysm repo.
On a different machine, create equivalent files or adjust the paths in the skill definitions.
