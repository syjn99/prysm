# Add Heze fork boilerplate

Adds the skeleton for Heze, the consensus-layer fork after Gloas. **No EIP-7805
(FOCIL) logic**: post-Heze behaves exactly like Gloas except for the state and
bid type changes the spec forces.

Mirrors the shape of #14771 ("Add Fulu fork boilerplate"), using the Gloas
piecewise PRs (#15601, #15611, #15618, #16378, #16291, #17358) for anything the
Fulu PR predates (progressive-container state schema, bid types, PTC).

Spec source: consensus-specs `v1.7.0-beta.0` (the Bazel-pinned version).

## What Heze changes

Per `specs/heze/beacon-chain.md`, the only forced type changes are:

- `ExecutionPayloadBid` gains `inclusion_list_bits: InclusionListBits`
  (`Bitvector[INCLUSION_LIST_COMMITTEE_SIZE]`), taking the progressive container
  from 12 to 13 active fields. That cascades into `SignedExecutionPayloadBid`,
  `BeaconBlockBody` and `BeaconState`.
- New `InclusionList` / `SignedInclusionList` containers.

`upgrade_to_heze` copies every Gloas field through and sets the new bid field to
an empty bitvector.

## Commits

1. **Add Heze fork config params and version enum** — `HEZE_FORK_VERSION` /
   `HEZE_FORK_EPOCH`, the EIP-7805 config values from `configs/mainnet.yaml`,
   the Heze preset SSZ bounds as `config/fieldparams` constants (like
   `PTC_SIZE`), `version.Heze` (gated in `unsupportedVersions`),
   `ToForkVersion` and `CanUpgradeToHeze`. The `HEZE_*` and inclusion-list
   entries are removed from the `loader_test.go` placeholder list so the
   spec-config test enforces them.
2. **Add Heze fork protobuf definitions** — `proto/prysm/v1alpha1/heze.proto`
   plus the `heze.yaml` methodical-ssz config, the generic block oneof entries
   and the `SignRequest` case.
3. **Add Heze state upgrade, state-native and block wiring** —
   `beacon-chain/core/heze/upgrade.go`, the `latestExecutionPayloadBidHeze`
   state field, `InitializeFromProtoUnsafeHeze`, the Heze progressive schema and
   the `consensus-types/blocks` plumbing.
4. **Wire Heze into p2p gossip mappings and the beacon DB** — block and bid
   gossip types, fork-version object maps, `heze` DB key prefix.
5. **Wire Heze into the beacon API, validator client and spectest runner** —
   JSON structs and conversions, publish decoders, `getStateV2`, empty/generic
   block construction, the fork-aware V4 block production path, and the
   fork-choice spectest runner.

## Notes for review

- **Dual bid storage in the block body.** `BeaconBlockBody` keeps both
  `signedExecutionPayloadBidHeze` (used by `Proto()`/SSZ) and a Gloas-shaped
  projection in `signedExecutionPayloadBid` (`gloasBidView` in
  `consensus-types/blocks/proto.go`). That keeps the ~35 existing
  `SignedExecutionPayloadBid()` callers working unchanged. Both are set once at
  init and never mutated, so they cannot drift.
- **`InclusionListBits()` on the RO interface.** Added so
  `SetExecutionPayloadBid` does not silently drop the bits on the Heze branch.
  The Gloas wrapper returns `nil` and the setter zero-fills.
- **`hdiff`** gets a Gloas→Heze case in `updateToVersion`. The diff still
  serializes the Gloas bid shape; that is lossless while the bits are always
  empty, and EIP-7805 will need to extend it.

## Deliberately not done

- No EIP-7805 logic: no inclusion list committee, gossip topic, RPC method or
  fork-choice rule.
- No Heze spec-test suites. The pinned tarball does ship `tests/*/heze`, but the
  FOCIL branch covers those; the Fulu boilerplate PR did not add spec tests
  either.
- `runtime/interop/premine-state.go` and
  `consensus-types/blocks/testing/factory.go` are untouched — neither supports
  Gloas today, so there is nothing for Heze to mirror.
- `beacon-chain/p2p/fork_watcher.go`, `pubsub_filter.go`,
  `beacon-chain/sync/rpc.go` and `rpc_chunked_response.go` needed no change:
  they are driven by the network schedule or by `>= version.Gloas` comparisons,
  both of which pick up Heze automatically.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
