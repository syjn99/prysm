### Changed

- Multi-value slice `Value` and `Reset` copy the shared items in bulk and then apply the object's own values, instead of a map lookup at every index. Reading 2.34M validators or balances from a state with few changes is at least twice as fast.
