### Changed

- A finalized state whose nearest stored state is more than two archive intervals below it now returns 404 instead of replaying from genesis or the origin checkpoint.
