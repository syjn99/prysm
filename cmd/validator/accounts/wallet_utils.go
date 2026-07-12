package accounts

import (
	"fmt"
	"strings"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	"github.com/OffchainLabs/prysm/v7/validator/accounts"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/iface"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/wallet"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/local"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

// keySourceForAccounts resolves the accounts CLI key source: a --validator-keys
// directory of EIP-2335 keystores when set, otherwise the legacy wallet. The
// returned wallet is nil on the directory path.
func keySourceForAccounts(c *cli.Context) (*wallet.Wallet, keymanager.IKeymanager, error) {
	if c.IsSet(flags.ValidatorKeysDirFlag.Name) {
		if c.IsSet(flags.WalletDirFlag.Name) {
			return nil, nil, fmt.Errorf("--%s and --%s cannot be used together; provide one key source", flags.ValidatorKeysDirFlag.Name, flags.WalletDirFlag.Name)
		}
		if !c.IsSet(flags.KeystorePasswordsFlag.Name) {
			return nil, nil, fmt.Errorf("--%s requires --%s", flags.ValidatorKeysDirFlag.Name, flags.KeystorePasswordsFlag.Name)
		}
		store, err := local.NewDirStore(c.String(flags.ValidatorKeysDirFlag.Name), c.String(flags.KeystorePasswordsFlag.Name))
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not open validator keys directory")
		}
		km, err := local.NewKeymanager(c.Context, &local.SetupConfig{Store: store})
		if err != nil {
			return nil, nil, errors.Wrap(err, accounts.ErrCouldNotInitializeKeymanager)
		}
		return nil, km, nil
	}
	return walletWithKeymanager(c)
}

func walletWithKeymanager(c *cli.Context) (*wallet.Wallet, keymanager.IKeymanager, error) {
	w, err := wallet.OpenWalletOrElseCli(c, func(cliCtx *cli.Context) (*wallet.Wallet, error) {
		return nil, wallet.ErrNoWalletFound
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not open wallet")
	}
	log.Warnf("--%s wallets are deprecated. Migrate with `validator accounts backup`, then load the exported keystores with --%s and --%s.",
		flags.WalletDirFlag.Name, flags.ValidatorKeysDirFlag.Name, flags.KeystorePasswordsFlag.Name)
	km, err := w.InitializeKeymanager(c.Context, iface.InitKeymanagerConfig{ListenForChanges: false})
	if err != nil && strings.Contains(err.Error(), keymanager.IncorrectPasswordErrMsg) {
		return nil, nil, errors.New("wrong wallet password entered")
	}
	if err != nil {
		return nil, nil, errors.Wrap(err, accounts.ErrCouldNotInitializeKeymanager)
	}
	return w, km, nil
}
