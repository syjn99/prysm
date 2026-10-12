### Fixed

- Archive mode: while regeneration runs, return 404 for a state or block rewards request between the archive regeneration frontier and the sync origin. Before, one such request replayed every block from the frontier and could make the node run out of memory.
