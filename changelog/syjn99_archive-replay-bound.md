### Fixed

- Archive mode: return 404 for a state request between the archive regeneration frontier and the sync origin while regeneration runs. Before, one such request replayed every block from the frontier and could make the node run out of memory.
