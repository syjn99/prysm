package util

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/xmss"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// PQPossessionProof derives a leanXMSS key from seed and signs, at leaf, the EIP-8450 proof of possession
// of that key for blsPubkey. It returns the leanXMSS public key and the proof.
func PQPossessionProof(t testing.TB, st state.ReadOnlyBeaconState, blsPubkey []byte, seed [32]byte, leaf uint32) ([32]byte, []byte) {
	sk, pk := xmss.KeyGen(seed)
	root, err := heze.PQKeyPossessionSigningRoot(st.GenesisValidatorsRoot(), blsPubkey, pk[:])
	require.NoError(t, err)
	proof, err := xmss.Sign(sk, leaf, root)
	require.NoError(t, err)
	return pk, proof
}

// SignPQKeyRegistration signs an EIP-8450 registration message with the registrant's BLS key.
func SignPQKeyRegistration(t testing.TB, st state.ReadOnlyBeaconState, sk bls.SecretKey, msg *ethpb.PQKeyRegistration, possession []byte) *ethpb.SignedPQKeyRegistration {
	c := params.BeaconConfig()
	domain, err := signing.ComputeDomain(c.DomainPQKeyRegistration, c.GenesisForkVersion, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(msg, domain)
	require.NoError(t, err)
	return &ethpb.SignedPQKeyRegistration{Message: msg, Possession: possession, Signature: sk.Sign(root[:]).Marshal()}
}
