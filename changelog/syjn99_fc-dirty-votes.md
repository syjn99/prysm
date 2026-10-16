### Changed

- Forkchoice `updateBalances` now visits only votes that changed since the last head computation, instead of every validator, unless the justified balances changed. Slashed indices are kept in a bitset.
