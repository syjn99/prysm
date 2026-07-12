### Changed

- Web3signer keymanager is now constructed directly from its config instead of through a temporary in-memory wallet; `wallet.NewWalletForWeb3Signer` and the `Web3SignerConfig` field of `InitKeymanagerConfig` are removed. No CLI or API behavior change: the keymanager API endpoints behave the same for web3signer setups.
