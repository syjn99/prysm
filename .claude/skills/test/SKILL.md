---
name: test
description: Run Prysm unit tests with baseline comparison. Use when testing changes locally.
invocation: user
---

# Unit Test Runner with Baseline Comparison

## Usage
- `/test` — test affected packages only (based on git diff)
- `/test //beacon-chain/sync/...` — test specific package
- `/test //...` — test everything (slow)

## Steps

1. **Determine test targets**:
   - If user specified a target, use that
   - Otherwise, find affected packages from git diff:
     ```bash
     cd ~/prysm
     git diff --name-only HEAD | grep '\.go$' | xargs -I{} dirname {} | sort -u | sed 's|^|//|;s|$|/...|'
     ```

2. **Run tests**:
   ```bash
   cd ~/prysm
   bazel test <targets> \
     --keep_going \
     --test_output=errors \
     --flaky_test_attempts=3 \
     --jobs=4 \
     --nostamp \
     --build_tests_only
   ```

3. **Compare against baseline**:
   - Read `~/.openclaw/workspace-coding/prysm-baseline-tests.json`
   - If a test failure exists in baseline → report as "known failure (pre-existing), ignore"
   - If a test failure is NOT in baseline → report as "NEW failure, needs fixing"

4. **Report results**:
   ```
   ✅ Passed: X tests
   ⚠️ Known failures (pre-existing): Y tests
   ❌ New failures: Z tests
     - //package:test_name — error summary
   ```

## Notes
- If baseline file doesn't exist yet, report all failures as-is and suggest running `/test //...` on clean develop to create baseline
- Flaky tests that pass on retry are fine
