package structs

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestSignedBeaconBlockHezeRoundTrip(t *testing.T) {
	blk := util.NewBeaconBlockHeze()
	blk.Block.Slot = 7
	blk.Block.Body.SignedExecutionPayloadBid.Message.InclusionListBits = []byte{0x01, 0x80}
	blk.Block.Body.PqKeyRegistrations = []*eth.SignedPQKeyRegistration{{
		Message: &eth.PQKeyRegistration{
			Index:               primitives.ValidatorIndex(primitives.BuilderIndexFlag | 3),
			KeyMaterial:         &eth.PQKeyMaterial{XmssPublicKey: bytes.Repeat([]byte{1}, 32), VrfRoot: make([]byte, 32)},
			PreviousChangeEpoch: 9,
		},
		Possession: bytes.Repeat([]byte{2}, 2404),
		Signature:  bytes.Repeat([]byte{3}, 96),
	}}

	jsonBlock, err := SignedBeaconBlockHezeFromConsensus(blk)
	require.NoError(t, err)
	encoded, err := json.Marshal(jsonBlock)
	require.NoError(t, err)
	require.Equal(t, true, strings.Contains(string(encoded), `"inclusion_list_bits":"0x0180"`))
	require.Equal(t, true, strings.Contains(string(encoded), `"pq_key_registrations":[{"message":{"index":"1099511627779"`))

	decoded := &SignedBeaconBlockHeze{}
	require.NoError(t, json.Unmarshal(encoded, decoded))
	got, err := decoded.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, blk, got)

	generic, err := decoded.ToGeneric()
	require.NoError(t, err)
	require.DeepEqual(t, blk, generic.GetHeze())
}

func TestBeaconBlockBodyHeze_ToConsensus_NilFields(t *testing.T) {
	tests := []struct {
		name  string
		field string
		clear func(b *BeaconBlockBodyHeze)
	}{
		{name: "nil Eth1Data", field: "Eth1Data", clear: func(b *BeaconBlockBodyHeze) { b.Eth1Data = nil }},
		{name: "nil SyncAggregate", field: "SyncAggregate", clear: func(b *BeaconBlockBodyHeze) { b.SyncAggregate = nil }},
		{name: "nil SignedExecutionPayloadBid", field: "SignedExecutionPayloadBid", clear: func(b *BeaconBlockBodyHeze) { b.SignedExecutionPayloadBid = nil }},
		{name: "nil Deposits element", field: "Deposits.[0]", clear: func(b *BeaconBlockBodyHeze) { b.Deposits = []*Deposit{nil} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blk, err := SignedBeaconBlockHezeFromConsensus(util.NewBeaconBlockHeze())
			require.NoError(t, err)
			body := blk.Message.Body
			tt.clear(body)
			_, err = body.ToConsensus()
			require.ErrorContains(t, "could not decode "+tt.field+": "+errNilValue.Error(), err)
		})
	}
}

func TestExecutionPayloadBidHezeToConsensus_BadInclusionListBits(t *testing.T) {
	bid := ExecutionPayloadBidHezeFromConsensus(util.NewBeaconBlockHeze().Block.Body.SignedExecutionPayloadBid.Message)
	for _, bits := range []string{"0x01", "0x010203", "zz"} {
		t.Run(bits, func(t *testing.T) {
			bid.InclusionListBits = bits
			_, err := bid.ToConsensus()
			require.ErrorContains(t, "InclusionListBits", err)
		})
	}
}
