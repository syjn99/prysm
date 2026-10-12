### Added

- `--prune-historical-states` keeps only the two newest archived states (plus genesis, origin and hot states) instead of one every `--slots-per-archive-point` forever. Without it, the node warns at startup that archived states accumulate without bound.
