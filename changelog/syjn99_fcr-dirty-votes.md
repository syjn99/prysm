### Changed

- Fast confirmation now updates its vote support incrementally: forkchoice reports only the latest votes that changed since the previous run, and node support between slots is read from a root to voter index instead of a scan over every vote.
