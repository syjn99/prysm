# Prysm Legacy Code & Maintainability Report

This report documents technical debt and maintainability problems in the Prysm codebase, with concrete evidence from source code and actionable remediation plans.

## Problems

| # | Report | Category | Severity | Summary |
|---|--------|----------|----------|---------|
| 1 | [Protobuf as Domain Models](01-protobuf-as-domain-models.md) | Architecture | High | Proto types used directly in core logic; 50+ type assertion cases in factories; `any` return types; `proto.Clone` overhead |
| 2 | [Fork Version Proliferation](02-fork-version-proliferation.md) | Architecture | High | 8 fork versions each requiring separate proto messages, field lists, init functions, and upgrade code |
| 3 | [Interface Bloat](03-interface-bloat.md) | Design | High | `BeaconState` interface composes 30+ sub-interfaces totaling ~190 methods; grows with every fork |
| 4 | [Code Duplication Across Forks](04-code-duplication-across-forks.md) | Maintenance | High | Upgrade functions, getters/setters, and core logic copy-pasted per fork with ~80% overlap |
| 5 | [Deprecated gRPC API](05-deprecated-grpc-api.md) | Legacy | High | ~19,500 lines of deprecated gRPC validator API still actively maintained alongside REST API |
| 6 | [State-Native God Package](06-state-native-god-package.md) | Organization | Medium | 84 Go files in one package mixing trie, proto conversion, locking, and fork-specific logic |
| 7 | [Test Infrastructure](07-test-infrastructure.md) | Tooling | Medium | Manual mock generation via shell script; mocks in 4 scattered locations; no CI verification |
| 8 | [Deprecated Database Schemas](08-deprecated-database-schemas.md) | Legacy | Low | Deprecated attestation protection code maintained for migration; no lifecycle policy |

## How to Use This Report

Each problem file contains:
- **Summary** -- one-paragraph description
- **Evidence** -- specific file paths, line numbers, and code excerpts
- **Key Files** -- table of critical files to examine
- **Solution and Action Plan** -- phased remediation strategy with estimated effort

## Priority Recommendations

**Immediate wins (low effort, high value):**
1. Add `go:generate` directives and CI mock verification (#7)
2. Establish database migration lifecycle policy (#8)

**Medium-term (medium effort, high impact):**
3. Extract shared state builder to reduce fork duplication (#4)
4. Move proto conversion out of state-native package (#1, #6)
5. Accept narrow interfaces at call sites (#3)

**Long-term (high effort, transformative):**
6. Replace proto domain models with native Go types (#1)
7. Unified state proto with optional fork fields (#2)
8. Complete gRPC-to-REST migration and remove deprecated API (#5)
