### Changed

- Extracted an `AccountStore` interface in the local keymanager covering the slice of wallet functionality it uses (read/write of the accounts keystore file, accounts dir, password), decoupling the keymanager from the wallet package. No behavior change.

### Fixed

- Guarded the accounts-file fsnotify listener so only one listener goroutine runs per local keymanager; `SaveStoreAndReInitialize` could previously start a duplicate watcher.
