### Changed

- With tracing enabled, `StartSpan` reuses one tracer instead of looking it up from the provider, under its mutex, for every span.
