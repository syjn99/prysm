# beacon-chain/core/helpers

Useful helper functions.

- `attestation.go`: validate conditions (is attestation not nil, etc.)
  - `ComputeSubnetForAttestation`: While spec's input includes `committee_index`, prysm's implementation passes `activeValCount`. `activeValCount` is used to calculate committee count(`SlotCommitteeCount`).
- `beacon_committee.go`
  - `SlotCommitteeCount`: calculate committees count per slot.
  - `BeaconCommitteeFromState`: return beacon committee from state(expensive, so utilize `committeeCache`)
  - `BeaconCommittee`: calculate from given arguments.
    - How does a committee be computed?
    - See `ComputeCommittee`.
    - First calculate start and end index.
    - Then, copy the slice into new slice(`shuffledIndices`).
    - Call `UnshuffleList` and return slice. -> Why?
  - `ProposerAssignments`: iterate through an epoch, return validator index to proposing duties. It is possible that a validator can propose twice or more in an epoch, so value is a slice of slot.
  - `CommitteeAssignments`: struct `CommitteeAssignment` looks like below:
    ```go
    type CommitteeAssignment struct {
    	Committee      []primitives.ValidatorIndex
    	AttesterSlot   primitives.Slot
    	CommitteeIndex primitives.CommitteeIndex
    }
    ```
    - For each slot, iterate through `committeesPerSlot` (nested loop).
    - It gets committee by `BeaconCommitteeFromState`. A committee is a list of validator indices.
    - Set assignment for each validators.
  - `UpdateCommitteeCache`: called at **every epoch-end slot**. (`handleEpochBoundary`) Also called when committeeCache is missed by `ActiveIndices` or `ActiveIndicesCount`.
    - The actual cache update is `AddCommitteeShuffledList`
  - `UpdateProposerIndicesInCache`: called at **every epoch-end slot**. (`handleEpochBoundary`) Also called when committeeCache is missed by `cachedProposerIndexAtSlot`.
    - The actual cache update is `proposerIndicesCache.Set`
    - The actual computation is `PrecomputeProposerIndices`.
      - It iterates through an epoch, call `ComputeProposerIndex`.
- `block.go`: Just helper functions to access block root and state root.
- `randao.go`: Compute seed, which is hash of 1) domain 2) epoch 3) randaoMix. This seed is also used in various cache implementation.
  - Keep aware that seed is not strongly tied to state itself.
- `rewards_penalties.go`
- `shuffle.go` => What is difference between unshuffle and shuffle? How shuffle is implemented?
- `sync_committee.go`
- `validator_churn.go`: For electra, EIP-7251.
- `validators.go`: Useful functions about validator status, active indices(which will be heavily utilized in computing committees), and churn limit. It also describes how the proposer index is computed.
  - `ComputeProposerIndex`: It iterate forever until candidate index be elected.
- `weak_subjectivity.go`


### Cache utilization

- `CommitteeCache`: Since getting committee every time in an epoch is very expensive computation, prysm stores every committee with its seed.
  - When updated: By calling `UpdateCommitteeCache`
- `ProposerIndicesCache`