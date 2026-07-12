### Added

- New `--validator-keys` and `--keystore-passwords` flags: the validator client can now load a plain directory of standalone EIP-2335 keystore files directly, without a wallet. Passwords come from a single shared password file or a directory of per-keystore `<keystore-name>.txt` files. Keymanager API imports write standalone keystore files (no re-encryption) and persist their passwords so restarts unlock without re-import. `--validator-keys` and `--wallet-dir` are mutually exclusive.
