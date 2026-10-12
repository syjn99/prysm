### Fixed

- Archive mode: do not build the state at the finalized epoch start from an orphaned block. The finalized index holds every block of the finalized epoch, so the canonical block lookup now accepts only the checkpoint root in that epoch.
