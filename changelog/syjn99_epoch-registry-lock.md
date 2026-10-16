### Fixed

- Registry updates read the finalized epoch once before iterating validators, instead of taking the state's read lock again for every validator while the iteration already holds it.
