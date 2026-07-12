### Changed

- `validator accounts import/list/delete/backup/voluntary-exit` now work against a plain directory of EIP-2335 keystores via `--validator-keys` (with `--keystore-passwords`), in addition to the legacy `--wallet-dir`. To migrate: run `validator accounts backup` to export your keys as keystores, drop them into a directory, and start the validator with `--validator-keys <dir> --keystore-passwords <file-or-dir>`.

### Deprecated

- `validator wallet create`, `validator wallet recover`, and `--wallet-dir` are deprecated in favor of direct keystore loading with `--validator-keys`. They keep working and now print a deprecation notice pointing at the new flags.
