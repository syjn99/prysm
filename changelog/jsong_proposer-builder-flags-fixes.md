### Fixed

- `--builder-urls` no longer rewrites per-key builder settings in the validator DB. When the default `builders` list is non-empty (from the flag or a settings source), a per-key block that carries only legacy builder fields (`enabled`, `gas_limit`) and no `enabled: true` inherits the default's pre-Gloas registration at runtime, and returns to its own setting when the default list is gone. A per-key `builders: []` still opts out.
- Per-key gas limits, including a legacy per-key `builder.gas_limit`, now always take precedence over the default gas limit from `--suggested-gas-limit` or `default_config`.
- `--suggested-gas-limit` is no longer silently discarded when a version 2 settings file or URL provides a `default_config` without its own `gas_limit`. A source `gas_limit` still wins, the startup warning names the overridden flag, and the Gloas schedule warning only fires when the flag value is in effect.
- `--suggested-gas-limit`, `--suggested-fee-recipient` or a single builder flag no longer drops the legacy `enabled` toggle and builder gas limit stored in a version 2 validator DB's `default_config`; such a start keeps what a flagless start keeps.
- A default whose only content is the per-run gas limit is no longer written to the validator DB, so the "Dropped the default gas limit" warning fires once rather than on every restart.
- The "Dropped the default gas limit" warning is no longer emitted when a legacy builder-level gas limit still applies to pre-Gloas registrations.
- `--builder-urls` help text and the flags changelog entry describe the real default auth data: the builder's lowercased hostname.
