### Fixed

- Archive mode: the handoff from state regeneration now advances the cold migration cursor to the walk end, so cold migration does not write the regenerated boundaries again. A late migration for an older checkpoint no longer moves the cursor back.
