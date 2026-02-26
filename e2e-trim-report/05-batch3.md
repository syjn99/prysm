# Phase 5 — Batch 3: Remaining Evaluators & beaconapi Subpackage

**Commit**: (see below)
**Branch**: `deprecate-grpc-api`

## Summary

All 5 evaluator files and all 4 beaconapi files were already fully REST-based.
The only change required was removing a stale `@org_golang_google_grpc` Bazel
dependency from `testing/endtoend/evaluators/beaconapi/BUILD.bazel`.

## Files Audited

### Evaluator Files (all already REST)

| File | Lines | gRPC refs | Status |
|------|-------|-----------|--------|
| `evaluators/execution_engine.go` | 145 | 0 | Already REST — uses `http.Get` + `structs.GetBlockV2Response` |
| `evaluators/builder.go` | 140 | 0 | Already REST — uses `getGenesisTime`, `getBlocksForEpoch` helpers |
| `evaluators/finality.go` | 73 | 0 | Already REST — uses `getChainHead` helper |
| `evaluators/peers.go` | 45 | 0 | Already REST — uses `getNodePeers` helper |
| `evaluators/data.go` | 40 | 0 | Already REST — uses `getAttesterDuties` helper |

### beaconapi Subpackage (already REST)

| File | Lines | gRPC refs | Status |
|------|-------|-----------|--------|
| `beaconapi/verify.go` | 345 | 0 | Pure REST — HTTP GET/POST, SSZ over HTTP. `ethpb` import is for SSZ unmarshal only. |
| `beaconapi/util.go` | 216 | 0 | Pure REST — `http.Get`, `http.Post`, `http.DefaultClient.Do` |
| `beaconapi/requests.go` | 325 | 0 | Pure REST — endpoint definitions with REST path templates |
| `beaconapi/types.go` | 187 | 0 | Pure type definitions — no network calls |

### Was beaconapi/ already REST?

**Yes.** The beaconapi subpackage was designed from the start as a REST-based
Beacon API verification framework. It makes HTTP GET/POST requests directly
(both JSON and SSZ), comparing Prysm responses against Lighthouse. It never
used gRPC RPCs. The only gRPC artifact was a stale Bazel dep that snuck in.

## Changes Made

| File | Change |
|------|--------|
| `evaluators/beaconapi/BUILD.bazel:26` | Removed `@org_golang_google_grpc//:go_default_library` |

## Proto Imports Retained (not gRPC)

- `beaconapi/verify.go` imports `ethpb` for SSZ unmarshaling block types
  (`SignedBeaconBlock`, `SignedBeaconBlockAltair`, etc.) in `postEvaluation()`.
  This is protocol buffer usage for serialization, not gRPC service calls.

## Build Status

```
$ go build ./testing/endtoend/evaluators/...     ✅ PASS
$ go build ./testing/endtoend/evaluators/beaconapi/...  ✅ PASS
```

## What the Next Phase Needs to Know

1. **All evaluator files are now 100% gRPC-free.** Every evaluator function
   uses REST helpers from `rest.go` or direct `http.Get`/`http.Post` calls.

2. **The Evaluator type signature** has already been migrated:
   `Evaluation func(ec *EvaluationContext, conns ...*NodeConnection) error`
   where `NodeConnection` wraps `BaseURL string` + `*http.Client`.

3. **Remaining gRPC usage** is only in the test runner infrastructure
   (e.g., `endtoend/components/`, connection setup code). The evaluator
   layer is fully decoupled from gRPC.

4. **Proto imports are still present** in some files but only for:
   - SSZ serialization/deserialization (beacon block types)
   - Signing helpers (domain types, attestation data)
   - These are legitimate protocol buffer usages, not gRPC service calls.
