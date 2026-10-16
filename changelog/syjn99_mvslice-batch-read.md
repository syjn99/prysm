### Changed

- `ValidatorsReadOnlySeq` and `PublicKeys` now read the validator registry in chunks under one multi-value slice lock per chunk, instead of taking the lock and looking up the override map for every validator.
