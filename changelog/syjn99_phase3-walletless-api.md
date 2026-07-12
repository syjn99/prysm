### Added

- The validator client with `--rpc` and no key source now starts with an empty local keymanager over `<datadir>/keystores`; keys are added via `POST /eth/v1/keystores`. Running headless with no key source now fails fast with an actionable error instead of blocking.

### Changed

- The keymanager API (`/eth/v1/keystores`, `/eth/v1/remotekeys`, etc.) no longer requires a Prysm wallet to be initialized first; it works whenever a keymanager exists (wallet, web3signer, direct keystore dir, or the empty default store).

### Removed

- The frozen Prysm validator web UI, its embedded assets, and the Prysm-specific wallet HTTP endpoints (`/v2/validator/wallet/*`, `/v2/validator/initialize`). The `--web` and `--write-wallet-password-on-web-onboarding` flags are now deprecated no-ops. The keymanager API behind `--rpc` (used by third-party UIs and DVT tooling) is unaffected.
