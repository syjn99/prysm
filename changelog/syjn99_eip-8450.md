### Added

- EIP-8450: post-quantum key registry lists in the Heze state and `pq_key_registrations` in the Heze block body, with fork upgrade, hdiff storage, and the four lists in the Heze state JSON.
- EIP-8450: `process_pq_key_registration` in Heze block processing.
- `crypto/xmss`: a Go port of leanXMSS (`Verify`, `KeyGen`, `Sign`) from ethereum/EthLib 6390c73e, checked against EthLib and compiled-Lean vectors.
- EIP-8450: `pq_key_registration` gossip topic with the EIP's validations, and a capped operation pool for registrations that also recovers them from orphaned blocks.
- EIP-8450: proposers pack up to `MAX_PQ_KEY_REGISTRATIONS` pooled registrations into Heze blocks.
- EIP-8450: `GET`/`POST /eth/v1/beacon/pool/pq_key_registrations`, `pq_key_registrations` in Heze block JSON, and the EIP-8450 domains and preset values in `/eth/v1/config/spec`.
