package beacon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	blockchainmock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/pqkeys"
	p2pMock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestPQKeyRegistrationsPool(t *testing.T) {
	gloasSt, keys := util.DeterministicGenesisStateGloas(t, 64)
	st, err := heze.UpgradeToHeze(gloasSt.Copy())
	require.NoError(t, err)
	far := params.BeaconConfig().FarFutureEpoch

	pk, proof := util.PQPossessionProof(t, st, keys[0].PublicKey().Marshal(), [32]byte{1}, 0)
	valid := util.SignPQKeyRegistration(t, st, keys[0], &ethpb.PQKeyRegistration{
		Index:               0,
		KeyMaterial:         &ethpb.PQKeyMaterial{XmssPublicKey: pk[:], VrfRoot: make([]byte, 32)},
		PreviousChangeEpoch: far,
	}, proof)
	// Validator 1 tries the key of validator 0 with validator 0's proof of possession.
	sameKey := util.SignPQKeyRegistration(t, st, keys[1], &ethpb.PQKeyRegistration{
		Index:               1,
		KeyMaterial:         valid.Message.KeyMaterial,
		PreviousChangeEpoch: far,
	}, proof)

	post := func(t *testing.T, s *Server, body []byte) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "http://example.com", bytes.NewReader(body))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.SubmitPQKeyRegistrations(writer, request)
		return writer
	}
	newServer := func(head *blockchainmock.ChainService) (*Server, *p2pMock.MockBroadcaster) {
		b := &p2pMock.MockBroadcaster{}
		return &Server{ChainInfoFetcher: head, PQKeyPool: pqkeys.NewPool(), Broadcaster: b}, b
	}

	t.Run("submits, pools and broadcasts a valid registration, then lists it", func(t *testing.T) {
		s, b := newServer(&blockchainmock.ChainService{State: st})
		body, err := json.Marshal([]*structs.SignedPQKeyRegistration{structs.SignedPQKeyRegistrationFromConsensus(valid)})
		require.NoError(t, err)
		w := post(t, s, body)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, true, b.BroadcastCalled.Load())
		pending := s.PQKeyPool.PendingRegistrations()
		require.Equal(t, 1, len(pending))
		require.DeepSSZEqual(t, valid, pending[0])

		request := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		s.ListPQKeyRegistrations(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &structs.ListPQKeyRegistrationsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.Equal(t, 1, len(resp.Data))
		got, err := resp.Data[0].ToConsensus()
		require.NoError(t, err)
		require.DeepSSZEqual(t, valid, got)
		// The head state is not changed.
		_, epoch, err := st.PQKeyEntry(0)
		require.NoError(t, err)
		require.Equal(t, far, epoch)
	})

	t.Run("reports invalid entries by index and keeps the valid ones", func(t *testing.T) {
		s, _ := newServer(&blockchainmock.ChainService{State: st})
		body, err := json.Marshal([]*structs.SignedPQKeyRegistration{
			structs.SignedPQKeyRegistrationFromConsensus(valid),
			structs.SignedPQKeyRegistrationFromConsensus(sameKey),
			{Message: &structs.PQKeyRegistration{Index: "x"}},
		})
		require.NoError(t, err)
		w := post(t, s, body)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.StringContains(t, `"index":1`, w.Body.String())
		require.StringContains(t, "invalid proof of possession", w.Body.String())
		require.StringContains(t, `"index":2`, w.Body.String())
		pending := s.PQKeyPool.PendingRegistrations()
		require.Equal(t, 1, len(pending), w.Body.String())
		require.DeepSSZEqual(t, valid, pending[0])
	})

	t.Run("does not broadcast a key that another entry of the request releases", func(t *testing.T) {
		// Builder 0 shares validator 0's BLS key, so validator 0's proof of possession for K also
		// proves K for builder 0. Validator 0 holds K since epoch 10 and may change it now.
		pre := gloasSt.Copy()
		require.NoError(t, pre.SetBuilders([]*ethpb.Builder{{
			Pubkey: keys[0].PublicKey().Marshal(), Version: []byte{0}, ExecutionAddress: make([]byte, 20),
			Balance: 1, DepositEpoch: 1, WithdrawableEpoch: far,
		}}))
		require.NoError(t, pre.SetSlot(params.BeaconConfig().SlotsPerEpoch*10))
		require.NoError(t, pre.SetFinalizedCheckpoint(&ethpb.Checkpoint{Epoch: 5, Root: make([]byte, 32)}))
		head, err := heze.UpgradeToHeze(pre)
		require.NoError(t, err)
		require.NoError(t, heze.ProcessPQKeyRegistration(head, util.SignPQKeyRegistration(t, head, keys[0], &ethpb.PQKeyRegistration{
			Index: 0, KeyMaterial: valid.Message.KeyMaterial, PreviousChangeEpoch: far,
		}, proof)))
		changeEpoch := 10 + params.BeaconConfig().MinPQKeyChangeDelay
		require.NoError(t, head.SetSlot(params.BeaconConfig().SlotsPerEpoch.Mul(uint64(changeEpoch))))

		pkL, proofL := util.PQPossessionProof(t, head, keys[0].PublicKey().Marshal(), [32]byte{2}, 0)
		release := util.SignPQKeyRegistration(t, head, keys[0], &ethpb.PQKeyRegistration{
			Index: 0, KeyMaterial: &ethpb.PQKeyMaterial{XmssPublicKey: pkL[:], VrfRoot: make([]byte, 32)}, PreviousChangeEpoch: 10,
		}, proofL)
		claim := util.SignPQKeyRegistration(t, head, keys[0], &ethpb.PQKeyRegistration{
			Index: primitives.BuilderIndex(0).ToValidatorIndex(), KeyMaterial: valid.Message.KeyMaterial, PreviousChangeEpoch: 1,
		}, proof)

		s, _ := newServer(&blockchainmock.ChainService{State: head})
		body, err := json.Marshal([]*structs.SignedPQKeyRegistration{
			structs.SignedPQKeyRegistrationFromConsensus(release),
			structs.SignedPQKeyRegistrationFromConsensus(claim),
		})
		require.NoError(t, err)
		w := post(t, s, body)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.StringContains(t, `"index":1`, w.Body.String())
		require.StringContains(t, "already registered", w.Body.String())
		pending := s.PQKeyPool.PendingRegistrations()
		require.Equal(t, 1, len(pending))
		require.DeepSSZEqual(t, release, pending[0])
	})

	t.Run("rejects an empty request", func(t *testing.T) {
		s, _ := newServer(&blockchainmock.ChainService{State: st})
		require.Equal(t, http.StatusBadRequest, post(t, s, []byte("[]")).Code)
	})

	t.Run("rejects a head state before Heze", func(t *testing.T) {
		s, b := newServer(&blockchainmock.ChainService{State: gloasSt})
		body, err := json.Marshal([]*structs.SignedPQKeyRegistration{structs.SignedPQKeyRegistrationFromConsensus(valid)})
		require.NoError(t, err)
		w := post(t, s, body)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, false, b.BroadcastCalled.Load())
	})
}
