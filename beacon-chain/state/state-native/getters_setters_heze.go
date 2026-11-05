package state_native

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// emptyPQKeyMaterial is the all-zero entry of a builder without post-quantum keys.
var emptyPQKeyMaterial = &ethpb.PQKeyMaterial{
	XmssPublicKey: make([]byte, fieldparams.RootLength),
	VrfRoot:       make([]byte, fieldparams.RootLength),
}

// pqKeyLists selects the validator or builder registry lists of a registrant.
// A registrant with BuilderIndexFlag set names a builder.
func (b *BeaconState) pqKeyLists(registrant primitives.ValidatorIndex) (keys *[]*ethpb.PQKeyMaterial, epochs *[]primitives.Epoch, keyField, epochField types.FieldIndex, idx uint64) {
	if registrant.IsBuilderIndex() {
		return &b.builderPQKeyMaterial, &b.builderPQKeyChangeEpochs, types.BuilderPostQuantumKeyMaterial, types.BuilderPostQuantumKeyChangeEpochs, uint64(registrant.ToBuilderIndex())
	}
	return &b.pqKeyMaterial, &b.pqKeyChangeEpochs, types.PostQuantumKeyMaterial, types.PostQuantumKeyChangeEpochs, uint64(registrant)
}

// PQKeyEntry returns a copy of the post-quantum keys of a registrant and the epoch of their last change.
// The registrant is a validator index, or a builder index with BuilderIndexFlag set.
func (b *BeaconState) PQKeyEntry(registrant primitives.ValidatorIndex) (*ethpb.PQKeyMaterial, primitives.Epoch, error) {
	if b.version < version.Heze {
		return nil, 0, errNotSupported("PQKeyEntry", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	keys, epochs, _, _, idx := b.pqKeyLists(registrant)
	if idx >= uint64(len(*keys)) || idx >= uint64(len(*epochs)) {
		return nil, 0, fmt.Errorf("post-quantum key index %d out of range", registrant)
	}
	return (*keys)[idx].Copy(), (*epochs)[idx], nil
}

// PQKeyListsReadOnly returns the four post-quantum key lists without copying them.
// Callers must not mutate the returned slices or their entries.
func (b *BeaconState) PQKeyListsReadOnly() (keys []*ethpb.PQKeyMaterial, epochs []primitives.Epoch, builderKeys []*ethpb.PQKeyMaterial, builderEpochs []primitives.Epoch, err error) {
	if b.version < version.Heze {
		return nil, nil, nil, nil, errNotSupported("PQKeyListsReadOnly", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.pqKeyMaterial, b.pqKeyChangeEpochs, b.builderPQKeyMaterial, b.builderPQKeyChangeEpochs, nil
}

// SetPQKeyEntry sets the post-quantum keys of a registrant and the epoch of their last change.
// An index equal to the list length appends the entry, as set_or_append_list does.
func (b *BeaconState) SetPQKeyEntry(registrant primitives.ValidatorIndex, key *ethpb.PQKeyMaterial, changeEpoch primitives.Epoch) error {
	if b.version < version.Heze {
		return errNotSupported("SetPQKeyEntry", b.version)
	}
	if key == nil || len(key.XmssPublicKey) != fieldparams.RootLength || len(key.VrfRoot) != fieldparams.RootLength {
		return fmt.Errorf("invalid post-quantum key material")
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	return b.setPQKeyEntryLockFree(registrant, key.Copy(), changeEpoch)
}

func (b *BeaconState) setPQKeyEntryLockFree(registrant primitives.ValidatorIndex, key *ethpb.PQKeyMaterial, changeEpoch primitives.Epoch) error {
	keysPtr, epochsPtr, keyField, epochField, idx := b.pqKeyLists(registrant)
	if len(*keysPtr) != len(*epochsPtr) || idx > uint64(len(*keysPtr)) {
		return fmt.Errorf("post-quantum key index %d out of range for length %d", registrant, len(*keysPtr))
	}

	keys := *keysPtr
	if b.sharedFieldReferences[keyField].Refs() > 1 {
		keys = slices.Clone(keys)
		b.sharedFieldReferences[keyField].MinusRef()
		b.sharedFieldReferences[keyField] = stateutil.NewRef(1)
	}
	epochs := *epochsPtr
	if b.sharedFieldReferences[epochField].Refs() > 1 {
		epochs = slices.Clone(epochs)
		b.sharedFieldReferences[epochField].MinusRef()
		b.sharedFieldReferences[epochField] = stateutil.NewRef(1)
	}

	if idx == uint64(len(keys)) {
		keys = append(keys, key)
		epochs = append(epochs, changeEpoch)
	} else {
		keys[idx] = key
		epochs[idx] = changeEpoch
	}
	*keysPtr = keys
	*epochsPtr = epochs
	b.markFieldAsDirty(keyField)
	b.markFieldAsDirty(epochField)
	return nil
}

// IsPQPublicKeyInUse returns whether any validator or builder holds the leanXMSS public key.
//
//	def is_pq_public_key_in_use(state: BeaconState, xmss_public_key: XMSSPublicKey) -> bool:
//	    entries = list(state.post_quantum_key_material) + list(state.builder_post_quantum_key_material)
//	    return any(entry.xmss_public_key == xmss_public_key for entry in entries)
//
// ponytail: linear scan of both registries, about 1 ms per million entries; keep a key-to-index map, like the BLS pubkey map, if registrations show in block-processing profiles.
func (b *BeaconState) IsPQPublicKeyInUse(key [fieldparams.RootLength]byte) (bool, error) {
	if b.version < version.Heze {
		return false, errNotSupported("IsPQPublicKeyInUse", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	for _, entries := range [][]*ethpb.PQKeyMaterial{b.pqKeyMaterial, b.builderPQKeyMaterial} {
		for _, entry := range entries {
			if entry != nil && bytes.Equal(entry.XmssPublicKey, key[:]) {
				return true, nil
			}
		}
	}
	return false, nil
}
