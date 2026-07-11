package local

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/async/event"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	mock "github.com/OffchainLabs/prysm/v7/validator/accounts/testing"
	"github.com/google/uuid"
	keystorev4 "github.com/wealdtech/go-eth2-wallet-encryptor-keystorev4"
)

func TestLocalKeymanager_reloadAccountsFromKeystore_MismatchedNumKeys(t *testing.T) {
	password := "Passw03rdz293**%#2"
	wallet := &mock.Wallet{
		Files:            make(map[string]map[string][]byte),
		AccountPasswords: make(map[string]string),
		WalletPassword:   password,
	}
	dr := &Keymanager{
		wallet: wallet,
	}
	accountsStore := &accountStore{
		PrivateKeys: [][]byte{[]byte("hello")},
		PublicKeys:  [][]byte{[]byte("hi"), []byte("world")},
	}
	encodedStore, err := json.MarshalIndent(accountsStore, "", "\t")
	require.NoError(t, err)
	encryptor := keystorev4.New()
	cryptoFields, err := encryptor.Encrypt(encodedStore, dr.wallet.Password())
	require.NoError(t, err)
	id, err := uuid.NewRandom()
	require.NoError(t, err)
	keystore := &AccountsKeystoreRepresentation{
		Crypto:  cryptoFields,
		ID:      id.String(),
		Version: encryptor.Version(),
		Name:    encryptor.Name(),
	}
	err = dr.reloadAccountsFromKeystore(keystore)
	assert.ErrorContains(t, "do not match", err)
}

func TestLocalKeymanager_reloadAccountsFromKeystore(t *testing.T) {
	password := "Passw03rdz293**%#2"
	wallet := &mock.Wallet{
		Files:            make(map[string]map[string][]byte),
		AccountPasswords: make(map[string]string),
		WalletPassword:   password,
	}
	dr := &Keymanager{
		wallet:              wallet,
		accountsChangedFeed: new(event.Feed),
	}

	numAccounts := 20
	privKeys := make([][]byte, numAccounts)
	pubKeys := make([][]byte, numAccounts)
	for i := range numAccounts {
		privKey, err := bls.RandKey()
		require.NoError(t, err)
		privKeys[i] = privKey.Marshal()
		pubKeys[i] = privKey.PublicKey().Marshal()
	}

	accountsStore, err := dr.CreateAccountsKeystore(t.Context(), privKeys, pubKeys)
	require.NoError(t, err)
	require.NoError(t, dr.reloadAccountsFromKeystore(accountsStore))

	// Check that the public keys were added to the public keys cache.
	for i, keyBytes := range pubKeys {
		require.Equal(t, bytesutil.ToBytes48(keyBytes), orderedPublicKeys[i])
	}

	// Check that the secret keys were added to the secret keys cache.
	lock.RLock()
	defer lock.RUnlock()
	for i, keyBytes := range privKeys {
		privKey, ok := secretKeysCache[bytesutil.ToBytes48(pubKeys[i])]
		require.Equal(t, true, ok)
		require.Equal(t, bytesutil.ToBytes48(keyBytes), bytesutil.ToBytes48(privKey.Marshal()))
	}

	// Check the key was added to the global accounts store.
	require.Equal(t, numAccounts, len(dr.accountsStore.PublicKeys))
	require.Equal(t, numAccounts, len(dr.accountsStore.PrivateKeys))
	assert.DeepEqual(t, dr.accountsStore.PublicKeys[0], pubKeys[0])
}

func encodeAccountsKeystoreFile(t *testing.T, privKeys []bls.SecretKey, password string) []byte {
	store := &accountStore{}
	for _, privKey := range privKeys {
		store.PrivateKeys = append(store.PrivateKeys, privKey.Marshal())
		store.PublicKeys = append(store.PublicKeys, privKey.PublicKey().Marshal())
	}
	rep, err := CreateAccountsKeystoreRepresentation(t.Context(), store, password)
	require.NoError(t, err)
	encoded, err := json.MarshalIndent(rep, "", "\t")
	require.NoError(t, err)
	return encoded
}

// TestListenForAccountChanges_ReloadsOnFileChange pins the fsnotify reload behavior:
// a change to the all-accounts.keystore.json file on disk is picked up by the
// running watcher, replaces the in-memory accounts store, and notifies subscribers.
func TestListenForAccountChanges_ReloadsOnFileChange(t *testing.T) {
	resetFeatures := features.InitWithReset(&features.Flags{
		KeystoreImportDebounceInterval: 10 * time.Millisecond,
	})
	defer resetFeatures()

	password := "Passw03rdz293**%#2"
	accountsDir := t.TempDir()
	privKey1, err := bls.RandKey()
	require.NoError(t, err)
	encodedOneKey := encodeAccountsKeystoreFile(t, []bls.SecretKey{privKey1}, password)

	// The accounts file must exist on disk before NewKeymanager runs, otherwise
	// the watcher goroutine exits early.
	require.NoError(t, os.MkdirAll(filepath.Join(accountsDir, AccountsPath), 0700))
	accountsFilePath := filepath.Join(accountsDir, AccountsPath, AccountsKeystoreFileName)
	require.NoError(t, os.WriteFile(accountsFilePath, encodedOneKey, 0600))

	wallet := &mock.Wallet{
		InnerAccountsDir: accountsDir,
		Files: map[string]map[string][]byte{
			AccountsPath: {AccountsKeystoreFileName: encodedOneKey},
		},
		WalletPassword: password,
	}
	km, err := NewKeymanager(t.Context(), &SetupConfig{Wallet: wallet, ListenForChanges: true})
	require.NoError(t, err)
	pubKeys, err := km.FetchValidatingPublicKeys(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, len(pubKeys))

	accountsChanged := make(chan [][fieldparams.BLSPubkeyLength]byte, 1)
	sub := km.SubscribeAccountChanges(accountsChanged)
	defer sub.Unsubscribe()

	privKey2, err := bls.RandKey()
	require.NoError(t, err)
	encodedTwoKeys := encodeAccountsKeystoreFile(t, []bls.SecretKey{privKey1, privKey2}, password)

	// The watcher registers asynchronously, so keep rewriting the file until the
	// reload event arrives.
	deadline := time.After(2 * time.Minute)
	for {
		require.NoError(t, os.WriteFile(accountsFilePath, encodedTwoKeys, 0600))
		select {
		case updated := <-accountsChanged:
			require.Equal(t, 2, len(updated))
			pubKeys, err = km.FetchValidatingPublicKeys(t.Context())
			require.NoError(t, err)
			require.Equal(t, 2, len(pubKeys))
			return
		case <-time.After(250 * time.Millisecond):
		case <-deadline:
			t.Fatal("timed out waiting for accounts reload after keystore file change")
		}
	}
}
