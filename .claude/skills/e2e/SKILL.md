---
name: e2e
description: Run Prysm end-to-end tests. Use after significant changes to verify system integration.
invocation: user
---

# E2E Test Runner

## Usage
- `/e2e` — run basic MinimalConfig (default)
- `/e2e current-fork` — run CurrentFork only (fastest)
- `/e2e builder` — run with builder (MEV changes)
- `/e2e api` — run with REST API (API changes)
- `/e2e slasher` — run with slasher

## Test Map

| Argument | Test Filter | When to use |
|---|---|---|
| (default) | `TestEndToEnd_MinimalConfig` | General/core logic changes |
| `current-fork` | `TestEndToEnd_MinimalConfig_CurrentFork` | Quick sanity check |
| `builder` | `TestEndToEnd_MinimalConfig_WithBuilder` | Builder/MEV changes |
| `api` | `TestEndToEnd_MinimalConfig_ValidatorRESTApi` | API changes |
| `slasher` | `TestEndToEnd_Slasher_MinimalConfig` | Slasher changes |

## Command Template
```bash
cd ~/prysm
bazel test //testing/endtoend:go_default_test \
  --//proto:network=minimal \
  --test_filter=<TEST_FILTER> \
  --test_env=E2E_EPOCHS=10 \
  --test_timeout=10000 \
  --test_output=streamed
```

## Notes
- E2E tests take several minutes to run
- They spin up actual beacon nodes, validators, and eth1 nodes
- Stream output so progress is visible
- If E2E fails, check component logs in the test output for root cause
