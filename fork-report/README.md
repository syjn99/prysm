# Fork Debloat: Feasibility Report

> Analysis of removing old hard-fork code from Prysm, keeping only the latest two forks (N-1, N).

## Executive Summary

Prysm carries code for **8 Ethereum consensus forks** (Phase0 through Gloas). The proposal in `PROBLEM_FORK.md` suggests removing all code below N-1, bumping the Go module version per hardfork, and providing a legacy binary for historical sync.

**Verdict: Feasible with a staged approach.**

Key findings:
- **~31,000+ lines** of code can be removed (generated + hand-written)
- Prysm's **unified BeaconState struct** makes this easier than it would be in clients with per-fork state types
- **Checkpoint sync** is the enabler: the main binary never needs to process old-fork slots
- The main risk is **shared helper code** in `altair/` that Electra+ epoch processing depends on
- Runtime performance gains are **modest** (~0%); the win is in **developer experience** (-36% generated code, -20% build time, -61% version switch cases)

## Reports

| Report | Description |
|--------|-------------|
| [Current State](current-state.md) | Quantitative inventory of all fork-specific code: 8 forks, 64K+ generated lines, 13K+ core fork logic, 352 files with version references |
| [Dependency Map](dependency-map.md) | Cross-cutting analysis of 7 subsystems affected: state, blocks, proto, STF, sync, fork choice, RPC. Blast radius estimates per subsystem |
| [Feasibility Analysis](feasibility.md) | Can we do it? What works in our favor (unified state, checkpoint sync, Go module versioning) and what makes it hard (proto proliferation, sequential upgrade chain, shared helpers) |
| [Performance](performance.md) | Binary size (-2-5 MB), build time (-20%), CI time (-60%), runtime (~0%). This is a maintainability play, not a performance play |
| [Action Plan](action-plan.md) | 5-phase roadmap: audit shared helpers, remove Phase0, remove Bellatrix/Capella/Deneb, refactor Altair, build legacy binary, bump module to v8 |

## Quick Numbers

| Metric | Current | After Debloat | Change |
|--------|---------|---------------|--------|
| Supported forks | 8 | 2-3 | -63% |
| SSZ generated lines | 30,591 | ~13,000 | -57% |
| Protobuf generated lines | 33,872 | ~25,000 | -26% |
| Core fork packages | 13,444 lines | ~7,500 lines | -44% |
| Version switch cases | ~510 | ~200 | -61% |
| Files with fork refs | 352 | ~150 | -57% |

## Key Decision Points

1. **Database strategy**: Require checkpoint re-sync on upgrade? (Recommended: yes)
2. **Shared helper handling**: Move to `helpers/` or rename `altair/` to something fork-neutral?
3. **Version enum values**: Keep original numbering (Electra=5) or renumber from 0?
4. **Legacy binary scope**: Full P2P sync capability or just STF-from-archive?
