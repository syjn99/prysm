package accounts

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/cmd/validator/flags"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/urfave/cli/v2"
)

func keySourceCtx(t *testing.T, set map[string]string) *cli.Context {
	fs := flag.NewFlagSet("test", 0)
	fs.String(flags.ValidatorKeysDirFlag.Name, "", "")
	fs.String(flags.KeystorePasswordsFlag.Name, "", "")
	fs.String(flags.WalletDirFlag.Name, "", "")
	for k, v := range set {
		require.NoError(t, fs.Set(k, v))
	}
	return cli.NewContext(&cli.App{}, fs, nil)
}

func TestKeySourceForAccounts_ValidatorKeysDir(t *testing.T) {
	keysDir := t.TempDir()
	pwFile := filepath.Join(t.TempDir(), "password.txt")
	require.NoError(t, os.WriteFile(pwFile, []byte("secretPassw0rd$1999"), 0600))

	t.Run("mutually exclusive with wallet-dir", func(t *testing.T) {
		_, _, err := keySourceForAccounts(keySourceCtx(t, map[string]string{
			flags.ValidatorKeysDirFlag.Name: keysDir,
			flags.WalletDirFlag.Name:        t.TempDir(),
		}))
		require.ErrorContains(t, "cannot be used together", err)
	})
	t.Run("requires keystore-passwords", func(t *testing.T) {
		_, _, err := keySourceForAccounts(keySourceCtx(t, map[string]string{
			flags.ValidatorKeysDirFlag.Name: keysDir,
		}))
		require.ErrorContains(t, "requires --keystore-passwords", err)
	})
	t.Run("returns a keymanager and no wallet", func(t *testing.T) {
		w, km, err := keySourceForAccounts(keySourceCtx(t, map[string]string{
			flags.ValidatorKeysDirFlag.Name:  keysDir,
			flags.KeystorePasswordsFlag.Name: pwFile,
		}))
		require.NoError(t, err)
		require.IsNil(t, w)
		require.NotNil(t, km)
	})
}
