package heze

import (
	"encoding/binary"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/crypto/xmss"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

// PQPossessionXMSSIndex is the leaf index at which the proof of possession is signed (EIP-8450).
const PQPossessionXMSSIndex uint32 = 0

// ProcessPQKeyRegistrations applies the EIP-8450 post-quantum key registrations of a block, in order.
// They run after every other operation of the block.
func ProcessPQKeyRegistrations(st state.BeaconState, body interfaces.ReadOnlyBeaconBlockBody) error {
	if body.Version() < version.Heze {
		return nil
	}
	registrations, err := body.PQKeyRegistrations()
	if err != nil {
		return err
	}
	for i, r := range registrations {
		if err := ProcessPQKeyRegistration(st, r); err != nil {
			return errors.Wrapf(err, "pq key registration %d", i)
		}
	}
	return nil
}

// ProcessPQKeyRegistration validates and applies one post-quantum key registration.
//
//	def process_pq_key_registration(
//	    state: BeaconState, signed_registration: SignedPQKeyRegistration
//	) -> None:
//	    registration = signed_registration.message
//	    key_material = registration.key_material
//
//	    if is_builder_index(registration.index):
//	        index = convert_validator_index_to_builder_index(registration.index)
//	        assert index < len(state.builders)
//	        assert is_active_builder(state, index)
//	        assert key_material.vrf_root == VRFRoot()
//	        pubkey = state.builders[index].pubkey
//	        entries = state.builder_post_quantum_key_material
//	        change_epochs = state.builder_post_quantum_key_change_epochs
//	    else:
//	        index = registration.index
//	        assert index < len(state.validators)
//	        assert state.validators[index].exit_epoch == FAR_FUTURE_EPOCH
//	        pubkey = state.validators[index].pubkey
//	        entries = state.post_quantum_key_material
//	        change_epochs = state.post_quantum_key_change_epochs
//
//	    assert registration.previous_change_epoch == change_epochs[index]
//	    if entries[index].xmss_public_key != XMSSPublicKey():
//	        assert get_current_epoch(state) >= change_epochs[index] + MIN_PQ_KEY_CHANGE_DELAY
//
//	    domain = compute_domain(
//	        DOMAIN_PQ_KEY_REGISTRATION, GENESIS_FORK_VERSION, state.genesis_validators_root
//	    )
//	    assert bls.Verify(pubkey, compute_signing_root(registration, domain), signed_registration.signature)
//
//	    assert key_material.xmss_public_key != XMSSPublicKey()
//	    assert not is_pq_public_key_in_use(state, key_material.xmss_public_key)
//	    assert is_valid_pq_possession_proof(
//	        state, pubkey, key_material.xmss_public_key, signed_registration.possession
//	    )
//
//	    entries[index] = key_material
//	    change_epochs[index] = get_current_epoch(state)
func ProcessPQKeyRegistration(st state.BeaconState, signed *ethpb.SignedPQKeyRegistration) error {
	if err := ValidatePQKeyRegistration(st, signed); err != nil {
		return err
	}
	return st.SetPQKeyEntry(signed.Message.Index, signed.Message.KeyMaterial, time.CurrentEpoch(st))
}

// ValidatePQKeyRegistration runs every assert of process_pq_key_registration against st without changing it.
func ValidatePQKeyRegistration(st state.ReadOnlyBeaconState, signed *ethpb.SignedPQKeyRegistration) error {
	if signed == nil || signed.Message == nil || signed.Message.KeyMaterial == nil {
		return errors.New("nil pq key registration")
	}
	registration := signed.Message
	keyMaterial := registration.KeyMaterial

	pubkey, err := RegistrantPubkey(st, registration)
	if err != nil {
		return err
	}

	// Replay protection and rate limit.
	stored, changeEpoch, err := st.PQKeyEntry(registration.Index)
	if err != nil {
		return err
	}
	if registration.PreviousChangeEpoch != changeEpoch {
		return fmt.Errorf("previous change epoch %d does not match stored change epoch %d", registration.PreviousChangeEpoch, changeEpoch)
	}
	currentEpoch := time.CurrentEpoch(st)
	if !isZero(stored.XmssPublicKey) {
		earliest, err := changeEpoch.SafeAddEpoch(params.BeaconConfig().MinPQKeyChangeDelay)
		if err != nil {
			return errors.Wrap(err, "change epoch overflow")
		}
		if currentEpoch < earliest {
			return fmt.Errorf("keys changed at epoch %d cannot change again before epoch %d", changeEpoch, earliest)
		}
	}

	// Authorization by the registrant's BLS signing key.
	if err := VerifyPQKeyRegistrationSignature(st, pubkey, signed); err != nil {
		return err
	}

	// A new leanXMSS key, whose possession is proven.
	if isZero(keyMaterial.XmssPublicKey) {
		return errors.New("zero leanXMSS public key")
	}
	inUse, err := st.IsPQPublicKeyInUse(bytesutil.ToBytes32(keyMaterial.XmssPublicKey))
	if err != nil {
		return err
	}
	if inUse {
		return errors.New("leanXMSS public key already registered")
	}
	if !IsValidPQPossessionProof(st, pubkey, keyMaterial.XmssPublicKey, signed.Possession) {
		return errors.New("invalid proof of possession")
	}
	return nil
}

// RegistrantPubkey checks that the registrant of a registration may register keys,
// and returns its BLS public key. These are the index checks of process_pq_key_registration.
func RegistrantPubkey(st state.ReadOnlyBeaconState, registration *ethpb.PQKeyRegistration) ([]byte, error) {
	if registration.Index.IsBuilderIndex() {
		index := registration.Index.ToBuilderIndex()
		builder, err := st.Builder(index)
		if err != nil {
			return nil, err
		}
		if builder == nil {
			return nil, fmt.Errorf("builder %d is not active", index)
		}
		active, err := st.IsActiveBuilder(index)
		if err != nil {
			return nil, err
		}
		if !active {
			return nil, fmt.Errorf("builder %d is not active", index)
		}
		if !isZero(registration.KeyMaterial.VrfRoot) {
			return nil, errors.New("builder registration has a nonzero VRF root")
		}
		return builder.Pubkey, nil
	}

	if uint64(registration.Index) >= uint64(st.NumValidators()) {
		return nil, fmt.Errorf("validator index %d out of range", registration.Index)
	}
	val, err := st.ValidatorAtIndexReadOnly(registration.Index)
	if err != nil {
		return nil, err
	}
	if val.ExitEpoch() != params.BeaconConfig().FarFutureEpoch {
		return nil, fmt.Errorf("validator %d has initiated an exit", registration.Index)
	}
	pubkey := val.PublicKey()
	return pubkey[:], nil
}

// VerifyPQKeyRegistrationSignature verifies the BLS authorization of a registration by the registrant's key.
// The domain uses the genesis fork version, so a registration stays valid across forks.
func VerifyPQKeyRegistrationSignature(st state.ReadOnlyBeaconState, pubkey []byte, signed *ethpb.SignedPQKeyRegistration) error {
	c := params.BeaconConfig()
	domain, err := signing.ComputeDomain(c.DomainPQKeyRegistration, c.GenesisForkVersion, st.GenesisValidatorsRoot())
	if err != nil {
		return errors.Wrap(err, "could not compute signing domain")
	}
	return signing.VerifySigningRoot(signed.Message, pubkey, signed.Signature, domain)
}

// IsValidPQPossessionProof checks that the holder of a leanXMSS key signed it for a BLS public key, at leaf index 0.
//
//	def is_valid_pq_possession_proof(
//	    state: BeaconState, pubkey: BLSPubkey, xmss_public_key: XMSSPublicKey, proof: XMSSSignature
//	) -> bool:
//	    if xmss_signature_index(proof) != PQ_POSSESSION_XMSS_INDEX:
//	        return False
//	    possession = PQKeyPossession(pubkey=pubkey, xmss_public_key=xmss_public_key)
//	    domain = compute_domain(
//	        DOMAIN_PQ_KEY_POSSESSION, GENESIS_FORK_VERSION, state.genesis_validators_root
//	    )
//	    return xmss.Verify(xmss_public_key, compute_signing_root(possession, domain), proof)
func IsValidPQPossessionProof(st state.ReadOnlyBeaconState, pubkey, xmssPublicKey, proof []byte) bool {
	if len(proof) < 4 || binary.LittleEndian.Uint32(proof[:4]) != PQPossessionXMSSIndex {
		return false
	}
	root, err := PQKeyPossessionSigningRoot(st.GenesisValidatorsRoot(), pubkey, xmssPublicKey)
	if err != nil {
		return false
	}
	return xmss.Verify(bytesutil.ToBytes32(xmssPublicKey), root, proof)
}

// PQKeyPossessionSigningRoot returns the message that a proof of possession signs:
// the signing root of PQKeyPossession(pubkey, xmss_public_key) under DOMAIN_PQ_KEY_POSSESSION.
func PQKeyPossessionSigningRoot(genesisValidatorsRoot, pubkey, xmssPublicKey []byte) ([32]byte, error) {
	c := params.BeaconConfig()
	domain, err := signing.ComputeDomain(c.DomainPQKeyPossession, c.GenesisForkVersion, genesisValidatorsRoot)
	if err != nil {
		return [32]byte{}, err
	}
	possession := &ethpb.PQKeyPossession{Pubkey: pubkey, XmssPublicKey: xmssPublicKey}
	return signing.ComputeSigningRoot(possession, domain)
}

func isZero(b []byte) bool {
	return bytesutil.ToBytes32(b) == [fieldparams.RootLength]byte{}
}
