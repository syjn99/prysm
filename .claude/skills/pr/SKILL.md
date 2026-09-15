---
name: pr
description: Full PR workflow for Prysm — precheck, test, commit, and push to fork
invocation: user
---

# PR Workflow

Complete workflow to prepare and push a Prysm PR.

## Steps

### 1. Pre-checks
Run all pre-commit checks (same as `/precheck`):
- gofmt
- goimports
- gazelle sync
- bazel build

### 2. Unit Tests
Run tests on affected packages (same as `/test`):
- Determine affected packages from diff
- Run bazel test
- Compare against baseline
- Fail if NEW test failures exist

### 3. E2E (if applicable)
Determine if E2E is needed based on changed files:
- `beacon-chain/` changes → run `TestEndToEnd_MinimalConfig`
- `api/` or `rpc/` changes → run `TestEndToEnd_MinimalConfig_ValidatorRESTApi`
- `validator/` changes → run `TestEndToEnd_MinimalConfig`
- Config-only or docs changes → skip E2E

### 4. Commit
```bash
cd ~/prysm
git add -A
git commit -m "<type>: <description>

<body explaining what and why>

Fixes #<issue-number>"
```

Commit types: `fix`, `feat`, `refactor`, `test`, `chore`, `docs`

### 5. Push to Fork
```bash
git push fork <branch-name>
```

⚠️ **NEVER push to origin.** Always push to `fork`.

### 6. Report
```
✅ PR ready!
Branch: <branch-name>
Fork: https://github.com/syjn99/prysm/tree/<branch-name>
Changes: <summary>

Jun, ready for you to open the upstream PR when you're happy with it.
```

## Rules
- Do NOT run `gh pr create` against OffchainLabs/prysm
- Do NOT merge anything
- Jun opens upstream PRs manually
