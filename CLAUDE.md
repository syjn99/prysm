# Prysm – Ethereum Consensus Layer Client

Module: `github.com/OffchainLabs/prysm/v7` | Go 1.25.1 | Branch: `develop`

> ⚠️ **NEVER commit this file or the `.claude/` directory. Both are excluded via `.git/info/exclude`.**

## Architecture

Executables (`cmd/`): `beacon-chain`, `validator`, `prysmctl`, `client-stats`

- `beacon-chain/core/` – State transitions per fork (Phase0→Altair→Bellatrix→Capella→Deneb→Electra→Fulu→Gloas)
- `beacon-chain/blockchain/` – Block processing, fork choice, execution engine
- `beacon-chain/state/` – BeaconState: copy-on-write, ReadOnly/WriteOnly interfaces
- `beacon-chain/db/` – BoltDB + filesystem (blobs, data columns)
- `beacon-chain/p2p/`, `sync/` – libp2p networking, gossipsub
- `validator/` – Key mgmt, slashing protection, duties
- `api/` – REST + gRPC | `proto/` – Protobuf defs | `consensus-types/` – Wrapped read-only interfaces
- `config/params/` – Chain params | `config/features/` – Feature flags

## Skills

- `/precheck` — gofmt, goimports, gazelle, hack scripts, build
- `/test` — unit tests with baseline comparison
- `/e2e` — end-to-end tests
- `/pr` — full PR workflow (precheck → test → e2e → commit → push)

## Patterns

- **Service Registry**: `runtime.ServiceRegistry` – lifecycle Start/Stop/Status
- **Functional Options**: `WithXxx` for DI (e.g. `blockchain.WithDatabase(db)`)
- **Interface Segregation**: ReadOnly/WriteOnly sub-interfaces; use narrowest type
- **Fork detection**: `block.Version()`, `state.Version()`; per-fork sub-packages in `core/`
- **State immutability**: Copy-on-write; call `state.Copy()` before mutating shared state
- Use `interfaces.ReadOnlySignedBeaconBlock` etc. over concrete proto types

## Build Tags

`develop` (required for `go test`), `minimal`/`mainnet` (config size), `fuzz`, `debug` (E2E)

## Testing

- `testing/assert/` (non-fatal), `testing/require/` (fatal) – custom helpers, not testify
- `DeepSSZEqual` for proto/SSZ comparison
- Spec tests: `testing/spectest/{mainnet,minimal}/` – tag-gated
- E2E: `testing/endtoend/` – multi-node in-process

## Nogo Analyzers

20+ custom analyzers in `tools/analyzers/` enforced by Bazel. Key rules: `cryptorand` (no math/rand), `errcheck`, `logcapitalization` (lowercase logs), `nopanic` (no panics), `featureconfig`, `recursivelock`. Build fails on violations.
