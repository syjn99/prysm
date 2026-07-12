package local

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/pkg/errors"
	keystorev4 "github.com/wealdtech/go-eth2-wallet-encryptor-keystorev4"
)

// WalletIO is the narrow slice of wallet functionality the legacy blob store
// depends on; any Prysm wallet satisfies it.
type WalletIO interface {
	// AccountsDir is the directory containing the accounts keystore file.
	AccountsDir() string
	// Password encrypts and decrypts the accounts keystore file.
	Password() string
	// ReadFileAtPath reads fileName under filePath, relative to the accounts
	// directory. The returned error contains "no files found" when the file
	// is absent.
	ReadFileAtPath(ctx context.Context, filePath string, fileName string) ([]byte, error)
	// WriteFileAtPath writes fileName under filePath, relative to the accounts
	// directory, reporting whether the file already existed.
	WriteFileAtPath(ctx context.Context, filePath string, fileName string, data []byte) (bool, error)
}

// walletStore persists all accounts as a single password-encrypted
// all-accounts.keystore.json blob inside a wallet — the legacy Prysm format.
type walletStore struct {
	wallet WalletIO
}

// NewWalletStore wraps a wallet in the AccountStore interface, preserving the
// legacy all-accounts.keystore.json on-disk format.
func NewWalletStore(w WalletIO) AccountStore {
	return &walletStore{wallet: w}
}

// Load implements AccountStore.
func (s *walletStore) Load(ctx context.Context) (*accountStore, error) {
	encoded, err := s.wallet.ReadFileAtPath(ctx, AccountsPath, AccountsKeystoreFileName)
	if err != nil && strings.Contains(err.Error(), "no files found") {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "could not read keystore file for accounts %s", AccountsKeystoreFileName)
	}
	keystoreFile := &AccountsKeystoreRepresentation{}
	if err := json.Unmarshal(encoded, keystoreFile); err != nil {
		return nil, errors.Wrapf(err, "could not decode keystore file for accounts %s", AccountsKeystoreFileName)
	}
	// We extract the validator signing private keys from the keystore
	// by utilizing the wallet password.
	decryptor := keystorev4.New()
	enc, err := decryptor.Decrypt(keystoreFile.Crypto, s.wallet.Password())
	if err != nil && strings.Contains(err.Error(), keymanager.IncorrectPasswordErrMsg) {
		return nil, errors.Wrap(err, "wrong password for wallet entered")
	} else if err != nil {
		return nil, errors.Wrap(err, "could not decrypt keystore")
	}
	store := &accountStore{}
	if err := json.Unmarshal(enc, store); err != nil {
		return nil, err
	}
	if len(store.PublicKeys) != len(store.PrivateKeys) {
		return nil, errors.New("unequal number of public keys and private keys")
	}
	return store, nil
}

// Save implements AccountStore. The blob is re-encrypted with the wallet
// password, so the already-encrypted import keystores are not needed.
func (s *walletStore) Save(ctx context.Context, store *accountStore, _ []*keymanager.Keystore, _ []string) (bool, error) {
	accountsKeystore, err := CreateAccountsKeystoreRepresentation(ctx, store, s.wallet.Password())
	if err != nil {
		return false, err
	}
	encodedAccounts, err := json.MarshalIndent(accountsKeystore, "", "\t")
	if err != nil {
		return false, err
	}
	return s.wallet.WriteFileAtPath(ctx, AccountsPath, AccountsKeystoreFileName, encodedAccounts)
}

// WatchPath implements AccountStore.
func (s *walletStore) WatchPath() string {
	return filepath.Join(s.wallet.AccountsDir(), AccountsPath, AccountsKeystoreFileName)
}
