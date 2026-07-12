package local

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
)

func TestDirStore_LoadImportDeleteRoundTrip(t *testing.T) {
	keysDir := t.TempDir()
	pwFile := filepath.Join(t.TempDir(), "password.txt")
	require.NoError(t, os.WriteFile(pwFile, []byte(password+"\n"), 0600))

	// Seed the directory with one keystore and one non-keystore JSON file.
	ks1 := createRandomKeystore(t, password)
	encoded, err := json.Marshal(ks1)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(keysDir, "validator1.json"), encoded, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(keysDir, "deposit_data.json"), []byte(`[{"amount":32}]`), 0600))

	store, err := NewDirStore(keysDir, pwFile)
	require.NoError(t, err)
	km, err := NewKeymanager(t.Context(), &SetupConfig{Store: store})
	require.NoError(t, err)
	keys, err := km.FetchValidatingPublicKeys(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))

	// Import a key with a different password through the keymanager API path.
	otherPassword := "Other#Passw0rd!"
	ks2 := createRandomKeystore(t, otherPassword)
	statuses, err := km.ImportKeystores(t.Context(), []*keymanager.Keystore{ks2}, []string{otherPassword})
	require.NoError(t, err)
	require.Equal(t, keymanager.StatusImported, statuses[0].Status)

	// The keystore and its password were persisted as standalone files.
	ksName := fmt.Sprintf("keystore-%s.json", ks2.Pubkey)
	_, err = os.Stat(filepath.Join(keysDir, ksName))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(keysDir, fmt.Sprintf("keystore-%s.txt", ks2.Pubkey)))
	require.NoError(t, err)

	// A fresh store over the same directory sees both keys (restart).
	store2, err := NewDirStore(keysDir, pwFile)
	require.NoError(t, err)
	km2, err := NewKeymanager(t.Context(), &SetupConfig{Store: store2})
	require.NoError(t, err)
	keys, err = km2.FetchValidatingPublicKeys(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, len(keys))

	// Deleting removes the standalone files again.
	pubKey2, err := hex.DecodeString(ks2.Pubkey)
	require.NoError(t, err)
	delStatuses, err := km2.DeleteKeystores(t.Context(), [][]byte{pubKey2})
	require.NoError(t, err)
	require.Equal(t, keymanager.StatusDeleted, delStatuses[0].Status)
	_, err = os.Stat(filepath.Join(keysDir, ksName))
	require.Equal(t, true, os.IsNotExist(err))
	keys, err = km2.FetchValidatingPublicKeys(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, len(keys))
}

func TestDirStore_PerKeyPasswordDir(t *testing.T) {
	keysDir := t.TempDir()
	pwDir := t.TempDir()
	ks := createRandomKeystore(t, password)
	encoded, err := json.Marshal(ks)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(keysDir, "mykey.json"), encoded, 0600))

	// Missing password file fails with an actionable error.
	store, err := NewDirStore(keysDir, pwDir)
	require.NoError(t, err)
	_, err = store.Load(t.Context())
	require.ErrorContains(t, "no password found for keystore mykey.json", err)

	require.NoError(t, os.WriteFile(filepath.Join(pwDir, "mykey.txt"), []byte(password), 0600))
	loaded, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, len(loaded.PublicKeys))

	// A second file holding the same key is rejected.
	require.NoError(t, os.WriteFile(filepath.Join(keysDir, "copy.json"), encoded, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(pwDir, "copy.txt"), []byte(password), 0600))
	_, err = store.Load(t.Context())
	require.ErrorContains(t, "duplicate key", err)
}
