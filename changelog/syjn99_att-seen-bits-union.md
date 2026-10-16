### Changed

- The attestation pool answers "already seen" for single-bit attestations from one union bitlist per attestation data, instead of scanning every seen bitlist.

### Fixed

- Concurrent inserts into the attestation pool's seen bits no longer drop each other's bits.
