---
name: precheck
description: Run pre-commit checks (gofmt, goimports, gazelle) before committing Prysm changes
invocation: user
---

# Pre-commit Checks

Run these checks in order on the Prysm repo. Stop and report if any fail.

## Steps

1. **gofmt check** — Ensure all changed .go files are formatted:
   ```bash
   cd ~/prysm
   gofmt -l $(git diff --name-only --diff-filter=ACM HEAD | grep '\.go$')
   ```
   If output is non-empty, run `gofmt -w` on those files.

2. **goimports check** — Ensure imports are organized:
   ```bash
   goimports -l $(git diff --name-only --diff-filter=ACM HEAD | grep '\.go$')
   ```
   If output is non-empty, run `goimports -w` on those files.

3. **Gazelle deps.bzl sync**:
   ```bash
   bazel run //:gazelle -- update-repos -from_file=go.mod -to_macro=deps.bzl%prysm_deps -prune=true
   git diff --exit-code deps.bzl
   ```

4. **Gazelle BUILD.bazel sync**:
   ```bash
   bazel run //:gazelle -- fix --mode=diff
   ```
   If diff output is non-empty, run `bazel run //:gazelle -- fix` to auto-fix.

5. **Bazel build**:
   ```bash
   bazel build //...
   ```

## Known Warnings (ignore these)
- `go-bip39 file path replacement` — known gazelle limitation
- `date: illegal option` on macOS — workspace_status.sh uses Linux date syntax
- `config/fieldparams/BUILD.bazel: could not merge expression` — known gazelle limitation

## On Success
Report: "✅ All pre-checks passed. Ready to commit."

## On Failure
Report which check failed and suggest the fix command.
