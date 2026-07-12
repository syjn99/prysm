package local

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/pkg/errors"
	keystorev4 "github.com/wealdtech/go-eth2-wallet-encryptor-keystorev4"
)

const (
	keystoreFileSuffix = ".json"
	passwordFileSuffix = ".txt"
)

// dirStore persists accounts as a plain directory of standalone EIP-2335
// keystore files — the format shared with other consensus clients. Passwords
// come from a single shared password file or per-keystore <keystore-name>.txt
// files in a passwords directory (Teku-style).
type dirStore struct {
	keysDir        string
	passwordsDir   string // "" when a shared password file is used
	sharedPassword string
	mu             sync.Mutex
	files          map[string]string // pubkey hex -> keystore file name
}

// NewDirStore creates an AccountStore over a directory of EIP-2335 keystore
// files. passwordsPath is either a directory holding per-keystore
// <keystore-name>.txt password files or a file holding one shared password.
func NewDirStore(keysDir, passwordsPath string) (AccountStore, error) {
	if err := file.MkdirAll(keysDir); err != nil {
		return nil, errors.Wrapf(err, "could not create keystore directory %s", keysDir)
	}
	s := &dirStore{keysDir: keysDir, files: make(map[string]string)}
	info, err := os.Stat(passwordsPath)
	if err != nil {
		return nil, errors.Wrapf(err, "keystore passwords path %s must be an existing password file or directory of <keystore-name>%s files", passwordsPath, passwordFileSuffix)
	}
	if info.IsDir() {
		s.passwordsDir = passwordsPath
		return s, nil
	}
	data, err := os.ReadFile(filepath.Clean(passwordsPath))
	if err != nil {
		return nil, errors.Wrapf(err, "could not read keystore password file %s", passwordsPath)
	}
	s.sharedPassword = strings.TrimRight(string(data), "\r\n")
	if s.sharedPassword == "" {
		return nil, fmt.Errorf("keystore password file %s is empty", passwordsPath)
	}
	return s, nil
}

// Load implements AccountStore, decrypting every keystore file in the
// directory. Non-keystore JSON files (e.g. deposit data) are skipped.
func (s *dirStore) Load(_ context.Context) (*accountStore, error) {
	entries, err := os.ReadDir(s.keysDir)
	if err != nil {
		return nil, errors.Wrapf(err, "could not read keystore directory %s", s.keysDir)
	}
	store := &accountStore{}
	files := make(map[string]string)
	decryptor := keystorev4.New()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), keystoreFileSuffix) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.keysDir, entry.Name())) // #nosec G304
		if err != nil {
			return nil, errors.Wrapf(err, "could not read keystore file %s", entry.Name())
		}
		keystore := &keymanager.Keystore{}
		if err := json.Unmarshal(data, keystore); err != nil || keystore.Crypto == nil {
			log.WithField("file", entry.Name()).Debug("Skipping JSON file that is not an EIP-2335 keystore")
			continue
		}
		password, err := s.password(entry.Name())
		if err != nil {
			return nil, err
		}
		privKey, err := decryptor.Decrypt(keystore.Crypto, password)
		if err != nil && strings.Contains(err.Error(), keymanager.IncorrectPasswordErrMsg) {
			return nil, fmt.Errorf("wrong password for keystore %s", entry.Name())
		} else if err != nil {
			return nil, errors.Wrapf(err, "could not decrypt keystore %s", entry.Name())
		}
		pubKey, err := publicKeyFromKeystore(keystore, privKey)
		if err != nil {
			return nil, errors.Wrapf(err, "could not get public key for keystore %s", entry.Name())
		}
		pubHex := hex.EncodeToString(pubKey)
		if other, ok := files[pubHex]; ok {
			return nil, fmt.Errorf("duplicate key %#x in keystores %s and %s", pubKey, other, entry.Name())
		}
		files[pubHex] = entry.Name()
		store.PrivateKeys = append(store.PrivateKeys, privKey)
		store.PublicKeys = append(store.PublicKeys, pubKey)
	}
	s.mu.Lock()
	s.files = files
	s.mu.Unlock()
	return store, nil
}

// Save implements AccountStore. Imported keystores are written verbatim as
// standalone files (no re-encryption) with their passwords persisted so a
// restart can unlock them; keys absent from store have their files removed.
func (s *dirStore) Save(_ context.Context, store *accountStore, keystores []*keymanager.Keystore, passwords []string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, ks := range keystores {
		pubHex := strings.TrimPrefix(ks.Pubkey, "0x")
		if pubHex == "" {
			return false, errors.New("imported keystore is missing its public key")
		}
		if _, ok := s.files[pubHex]; ok {
			continue
		}
		name := fmt.Sprintf("keystore-%s%s", pubHex, keystoreFileSuffix)
		encoded, err := json.MarshalIndent(ks, "", "\t")
		if err != nil {
			return false, err
		}
		if err := file.WriteFile(filepath.Join(s.keysDir, name), encoded); err != nil {
			return false, errors.Wrapf(err, "could not write keystore file %s", name)
		}
		if err := s.savePassword(name, passwords[i]); err != nil {
			return false, err
		}
		s.files[pubHex] = name
	}
	keep := make(map[string]bool, len(store.PublicKeys))
	for _, pubKey := range store.PublicKeys {
		pubHex := hex.EncodeToString(pubKey)
		if _, ok := s.files[pubHex]; !ok {
			// Raw keypairs (e.g. derived-wallet recovery) have no keystore to persist.
			return false, errors.New("cannot persist keys without their EIP-2335 keystores to a keystore directory")
		}
		keep[pubHex] = true
	}
	for pubHex, name := range s.files {
		if keep[pubHex] {
			continue
		}
		if err := os.Remove(filepath.Join(s.keysDir, name)); err != nil && !os.IsNotExist(err) {
			return false, errors.Wrapf(err, "could not remove keystore file %s", name)
		}
		pwFile := filepath.Join(s.passwordFileDir(), strings.TrimSuffix(name, keystoreFileSuffix)+passwordFileSuffix)
		if err := os.Remove(pwFile); err != nil && !os.IsNotExist(err) {
			return false, errors.Wrapf(err, "could not remove password file %s", pwFile)
		}
		delete(s.files, pubHex)
	}
	// The directory always exists, so the change listener is already armed.
	return true, nil
}

// WatchPath implements AccountStore.
func (s *dirStore) WatchPath() string {
	return s.keysDir
}

// passwordFileDir is where per-keystore password files live: the passwords
// directory when one is configured, otherwise next to the keystores.
func (s *dirStore) passwordFileDir() string {
	if s.passwordsDir != "" {
		return s.passwordsDir
	}
	return s.keysDir
}

func (s *dirStore) password(keystoreName string) (string, error) {
	pwName := strings.TrimSuffix(keystoreName, keystoreFileSuffix) + passwordFileSuffix
	data, err := os.ReadFile(filepath.Join(s.passwordFileDir(), pwName)) // #nosec G304
	if err == nil {
		return strings.TrimRight(string(data), "\r\n"), nil
	}
	if !os.IsNotExist(err) {
		return "", errors.Wrapf(err, "could not read password file %s", pwName)
	}
	if s.sharedPassword != "" {
		return s.sharedPassword, nil
	}
	return "", fmt.Errorf("no password found for keystore %s: add %s to the passwords directory", keystoreName, pwName)
}

func (s *dirStore) savePassword(keystoreName, password string) error {
	if s.sharedPassword != "" && password == s.sharedPassword {
		return nil
	}
	pwName := strings.TrimSuffix(keystoreName, keystoreFileSuffix) + passwordFileSuffix
	if err := file.WriteFile(filepath.Join(s.passwordFileDir(), pwName), []byte(password)); err != nil {
		return errors.Wrapf(err, "could not write password file %s", pwName)
	}
	return nil
}

func publicKeyFromKeystore(keystore *keymanager.Keystore, privKey []byte) ([]byte, error) {
	if keystore.Pubkey != "" {
		return hex.DecodeString(strings.TrimPrefix(keystore.Pubkey, "0x"))
	}
	secretKey, err := bls.SecretKeyFromBytes(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not initialize private key from bytes")
	}
	return secretKey.PublicKey().Marshal(), nil
}
