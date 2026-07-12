package local

import (
	"context"
	"os"

	"github.com/OffchainLabs/prysm/v7/async"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

// startAccountsChangeListener spawns listenForAccountChanges, guarding against
// more than one listener running at a time: NewKeymanager starts one when
// configured to listen for changes, and SaveStoreAndReInitialize starts one
// after writing the accounts file for the first time. The guard re-arms when
// the listener exits, e.g. when it started before the accounts file existed.
// A start racing a just-exiting listener can be dropped; that window only
// exists if the accounts file is created concurrently with keymanager setup.
func (km *Keymanager) startAccountsChangeListener(ctx context.Context) {
	if !km.listeningForChanges.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer km.listeningForChanges.Store(false)
		km.listenForAccountChanges(ctx)
	}()
}

// Listen for changes to the store's backing file or directory to load in new
// keys we observe into our keymanager. This uses the fsnotify library to
// listen for file-system changes and debounces these events to ensure we can
// handle thousands of events fired in a short time-span.
func (km *Keymanager) listenForAccountChanges(ctx context.Context) {
	debounceFileChangesInterval := features.Get().KeystoreImportDebounceInterval
	if km.store == nil {
		return
	}
	watchPath := km.store.WatchPath()
	if _, err := os.Stat(watchPath); err != nil {
		if !os.IsNotExist(err) {
			log.WithError(err).Errorf("Could not check if path exists: %s", watchPath)
			return
		}
		log.Warnf("Starting without accounts located at %s", watchPath)
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.WithError(err).Error("Could not initialize file watcher")
		return
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			log.WithError(err).Error("Could not close file watcher")
		}
	}()
	if err := watcher.Add(watchPath); err != nil {
		log.WithError(err).Errorf("Could not add path %s to file watcher", watchPath)
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	fileChangesChan := make(chan any, 100)
	defer close(fileChangesChan)

	// We debounce events sent over the file changes channel by an interval
	// to ensure we are not overwhelmed by a ton of events fired over the channel in
	// a short span of time.
	go async.Debounce(ctx, debounceFileChangesInterval, fileChangesChan, func(event any) {
		if _, ok := event.(fsnotify.Event); !ok {
			log.Errorf("Type %T is not a valid file system event", event)
			return
		}
		km.reloadFromStore(ctx)
	})
	for {
		select {
		case event := <-watcher.Events:
			// If a file was modified, we attempt to reload the accounts
			// from the store.
			fileChangesChan <- event
		case err := <-watcher.Errors:
			log.WithError(err).Errorf("Could not watch for file changes for: %s", watchPath)
		case <-ctx.Done():
			return
		}
	}
}

// reloadFromStore replaces the in-memory accounts with the store's on-disk
// contents, refreshing key caches and notifying accounts-changed subscribers.
func (km *Keymanager) reloadFromStore(ctx context.Context) {
	if km.store == nil {
		log.Error("Could not reload accounts because store was undefined")
		return
	}
	store, err := km.store.Load(ctx)
	if err != nil {
		log.WithError(err).Error("Could not reload accounts from store")
		return
	}
	if store == nil {
		log.Error("Could not reload accounts: store has no accounts")
		return
	}
	if err := km.replaceStore(store); err != nil {
		log.WithError(err).Error("Could not replace the accounts store")
	}
}

// Replaces the accounts store struct in the local keymanager with the
// provided one after validating its keys.
func (km *Keymanager) replaceStore(newAccountsStore *accountStore) error {
	if len(newAccountsStore.PublicKeys) != len(newAccountsStore.PrivateKeys) {
		return errors.New("number of public and private keys in keystore do not match")
	}

	pubKeys := make([][fieldparams.BLSPubkeyLength]byte, len(newAccountsStore.PublicKeys))
	for i := 0; i < len(newAccountsStore.PrivateKeys); i++ {
		privKey, err := bls.SecretKeyFromBytes(newAccountsStore.PrivateKeys[i])
		if err != nil {
			return errors.Wrap(err, "could not initialize private key")
		}
		pubKeyBytes := privKey.PublicKey().Marshal()
		pubKeys[i] = bytesutil.ToBytes48(pubKeyBytes)
	}
	km.accountsStore = newAccountsStore
	if err := km.initializeKeysCachesFromKeystore(); err != nil {
		return err
	}
	log.Info(keymanager.KeysReloaded)
	km.accountsChangedFeed.Send(pubKeys)
	return nil
}
