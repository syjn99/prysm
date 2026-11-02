### Added

- EIP-8450: post-quantum key registry lists in the Heze state and `pq_key_registrations` in the Heze block body.
- EIP-8450: `process_pq_key_registration` in Heze block processing, with a Go port of the leanXMSS verifier in `crypto/xmss`.
- EIP-8450: `pq_key_registration` gossip topic with the EIP's validations, and an operation pool for registrations.
