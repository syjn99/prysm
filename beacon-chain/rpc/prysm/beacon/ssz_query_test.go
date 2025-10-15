package beacon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	chainMock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	dbTest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/testutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestServer_QueryBeaconState(t *testing.T) {
	ctx := t.Context()
	fakeState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, fakeState.SetSlot(100))

	// Set a finalized checkpoint
	fcRoot, err := hexutil.Decode("0x4a2c7e9d1f0b85a632e4c9b0f8d716a54b0e8f2d9c5a7136b8d0f4a9e27c1b63")
	require.NoError(t, err)
	fakeState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: 2,
		Root:  fcRoot,
	})

	fakeBlockHeader := &ethpb.BeaconBlockHeader{
		Slot:          112,
		ProposerIndex: 1,
		ParentRoot:    fcRoot,
		StateRoot:     fcRoot,
		BodyRoot:      fcRoot,
	}
	fakeState.SetLatestBlockHeader(fakeBlockHeader)
	// Ensure the latest block header is set correctly.

	stateRoot, err := fakeState.HashTreeRoot(ctx)
	require.NoError(t, err)
	db := dbTest.SetupDB(t)
	parentRoot := [32]byte{'a'}
	blk := util.NewBeaconBlock()
	blk.Block.ParentRoot = parentRoot[:]
	root, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, db, blk)
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

	chainService := &chainMock.ChainService{}
	s := &Server{
		Stater: &testutil.MockStater{
			BeaconStateRoot: stateRoot[:],
			BeaconState:     fakeState,
		},
		HeadFetcher:           chainService,
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
		BeaconDB:              db,
		ChainInfoFetcher:      chainService,
	}

	// t.Run("success - query single field", func(t *testing.T) {
	// 	requestBody := &structs.QuerySSZRequest{
	// 		Query: []*structs.QueryObject{
	// 			{Path: ".slot"},
	// 		},
	// 	}
	// 	var buf bytes.Buffer
	// 	require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

	// 	request := httptest.NewRequest(http.MethodPost, "http://example.com/prysm/v1/beacon/states/{state_id}/query", &buf)
	// 	request.SetPathValue("state_id", "head")
	// 	writer := httptest.NewRecorder()
	// 	writer.Body = &bytes.Buffer{}

	// 	s.QueryBeaconState(writer, request)
	// 	fmt.Println(string(writer.Body.Bytes()))
	// 	require.Equal(t, http.StatusOK, writer.Code)

	// 	resp := &structs.QuerySSZResponse{}
	// 	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	// 	assert.Equal(t, hexutil.Encode(stateRoot[:]), resp.Data.Root)
	// 	assert.Equal(t, 1, len(resp.Data.Values.Paths))
	// 	assert.Equal(t, ".slot", resp.Data.Values.Paths[0])
	// 	assert.Equal(t, 1, len(resp.Data.Values.Results))
	// 	assert.Equal(t, `"100"`, string(resp.Data.Values.Results[0]))
	// })

	t.Run("success - query single field bh", func(t *testing.T) {
		requestBody := &structs.QuerySSZRequest{
			Query: []*structs.QueryObject{
				{Path: ".latest_block_header.slot"},
				{Path: ".latest_block_header.proposer_index"},
				{Path: ".latest_block_header"},
			},
		}
		var buf bytes.Buffer
		require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

		request := httptest.NewRequest(http.MethodPost, "http://example.com/prysm/v1/beacon/states/{state_id}/query", &buf)
		request.SetPathValue("state_id", "head")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.QueryBeaconState(writer, request)
		fmt.Println(string(writer.Body.Bytes()))
		require.Equal(t, http.StatusOK, writer.Code)

		jsonBeaconBlockHeader, err := json.Marshal(structs.BeaconBlockHeaderFromConsensus(fakeBlockHeader))
		require.NoError(t, err)

		resp := &structs.QuerySSZResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, hexutil.Encode(stateRoot[:]), resp.Data.Root)
		assert.Equal(t, 3, len(resp.Data.Values.Paths))
		assert.Equal(t, ".latest_block_header.slot", resp.Data.Values.Paths[0])
		assert.Equal(t, 3, len(resp.Data.Values.Results))
		assert.Equal(t, `"112"`, string(resp.Data.Values.Results[0]))
		assert.Equal(t, `"1"`, string(resp.Data.Values.Results[1]))
		assert.DeepEqual(t, string(jsonBeaconBlockHeader), string(resp.Data.Values.Results[2]), "latest_block_header does not match")
	})

	// t.Run("success - query finalized checkpoint", func(t *testing.T) {
	// 	requestBody := &structs.QuerySSZRequest{
	// 		Query: []*structs.QueryObject{
	// 			{Path: ".finalized_checkpoint"},
	// 		},
	// 	}
	// 	var buf bytes.Buffer
	// 	require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

	// 	request := httptest.NewRequest(http.MethodPost, "http://example.com/prysm/v1/beacon/states/{state_id}/query", &buf)
	// 	request.SetPathValue("state_id", "head")
	// 	writer := httptest.NewRecorder()
	// 	writer.Body = &bytes.Buffer{}

	// 	s.QueryBeaconState(writer, request)
	// 	fmt.Println(string(writer.Body.Bytes()))
	// 	require.Equal(t, http.StatusOK, writer.Code)

	// 	resp := &structs.QuerySSZResponse{}
	// 	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	// 	assert.Equal(t, hexutil.Encode(stateRoot[:]), resp.Data.Root)
	// 	assert.Equal(t, 1, len(resp.Data.Values.Paths))
	// 	assert.Equal(t, ".finalized_checkpoint", resp.Data.Values.Paths[0])
	// 	assert.Equal(t, 1, len(resp.Data.Values.Results))
	// 	assert.Equal(t, `{"epoch":"2","root":"0x4a2c7e9d1f0b85a632e4c9b0f8d716a54b0e8f2d9c5a7136b8d0f4a9e27c1b63"}`, string(resp.Data.Values.Results[0]))
	// })

	// t.Run("success - query multiple fields", func(t *testing.T) {
	// 	requestBody := &structs.QuerySSZRequest{
	// 		Query: []*structs.QueryObject{
	// 			{Path: ".slot"},
	// 			{Path: ".finalized_checkpoint"},
	// 		},
	// 	}
	// 	var buf bytes.Buffer
	// 	require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

	// 	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/beacon/states/{state_id}/ssz_query", &buf)
	// 	request.SetPathValue("state_id", "head")
	// 	writer := httptest.NewRecorder()
	// 	writer.Body = &bytes.Buffer{}

	// 	s.QueryBeaconState(writer, request)
	// 	require.Equal(t, http.StatusOK, writer.Code)

	// 	resp := &structs.QuerySSZResponse{}
	// 	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	// 	assert.Equal(t, 2, len(resp.Data.Values.Paths))
	// 	assert.Equal(t, ".slot", resp.Data.Values.Paths[0])
	// 	assert.Equal(t, ".finalized_checkpoint", resp.Data.Values.Paths[1])
	// 	assert.Equal(t, 2, len(resp.Data.Values.Results))
	// 	assert.Equal(t, `"100"`, string(resp.Data.Values.Results[0]))
	// 	assert.Equal(t, `{"epoch":"2","root":"0x4a2c7e9d1f0b85a632e4c9b0f8d716a54b0e8f2d9c5a7136b8d0f4a9e27c1b63"}`, string(resp.Data.Values.Results[1]))
	// })

	// t.Run("error - no state_id", func(t *testing.T) {
	// 	requestBody := &structs.QuerySSZRequest{
	// 		Query: []*structs.QuerySSZQuery{
	// 			{Path: ".slot"},
	// 		},
	// 	}
	// 	var buf bytes.Buffer
	// 	require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

	// 	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/beacon/states/{state_id}/ssz_query", &buf)
	// 	// Don't set state_id
	// 	writer := httptest.NewRecorder()
	// 	writer.Body = &bytes.Buffer{}

	// 	s.QueryBeaconState(writer, request)
	// 	require.Equal(t, http.StatusBadRequest, writer.Code)
	// 	e := &httputil.DefaultJsonError{}
	// 	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
	// 	assert.StringContains(t, "state_id is required", e.Message)
	// })

	// t.Run("error - empty request body", func(t *testing.T) {
	// 	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/beacon/states/{state_id}/ssz_query", &bytes.Buffer{})
	// 	request.SetPathValue("state_id", "head")
	// 	writer := httptest.NewRecorder()
	// 	writer.Body = &bytes.Buffer{}

	// 	s.QueryBeaconState(writer, request)
	// 	require.Equal(t, http.StatusBadRequest, writer.Code)
	// 	e := &httputil.DefaultJsonError{}
	// 	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
	// 	assert.StringContains(t, "No data submitted", e.Message)
	// })

	// t.Run("error - no query", func(t *testing.T) {
	// 	requestBody := &structs.QuerySSZRequest{
	// 		Query: []*structs.QuerySSZQuery{},
	// 	}
	// 	var buf bytes.Buffer
	// 	require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

	// 	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/beacon/states/{state_id}/ssz_query", &buf)
	// 	request.SetPathValue("state_id", "head")
	// 	writer := httptest.NewRecorder()
	// 	writer.Body = &bytes.Buffer{}

	// 	s.QueryBeaconState(writer, request)
	// 	require.Equal(t, http.StatusBadRequest, writer.Code)
	// 	e := &httputil.DefaultJsonError{}
	// 	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
	// 	assert.StringContains(t, "No query submitted", e.Message)
	// })

	// t.Run("error - invalid path", func(t *testing.T) {
	// 	requestBody := &structs.QuerySSZRequest{
	// 		Query: []*structs.QuerySSZQuery{
	// 			{Path: "invalid_path"},
	// 		},
	// 	}
	// 	var buf bytes.Buffer
	// 	require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

	// 	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/beacon/states/{state_id}/ssz_query", &buf)
	// 	request.SetPathValue("state_id", "head")
	// 	writer := httptest.NewRecorder()
	// 	writer.Body = &bytes.Buffer{}

	// 	s.QueryBeaconState(writer, request)
	// 	require.Equal(t, http.StatusBadRequest, writer.Code)
	// 	e := &httputil.DefaultJsonError{}
	// 	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
	// 	assert.StringContains(t, "Could not parse path", e.Message)
	// })

	// t.Run("error - field not found", func(t *testing.T) {
	// 	requestBody := &structs.QuerySSZRequest{
	// 		Query: []*structs.QuerySSZQuery{
	// 			{Path: ".nonexistent_field"},
	// 		},
	// 	}
	// 	var buf bytes.Buffer
	// 	require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

	// 	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/beacon/states/{state_id}/ssz_query", &buf)
	// 	request.SetPathValue("state_id", "head")
	// 	writer := httptest.NewRecorder()
	// 	writer.Body = &bytes.Buffer{}

	// 	s.QueryBeaconState(writer, request)
	// 	require.Equal(t, http.StatusBadRequest, writer.Code)
	// 	e := &httputil.DefaultJsonError{}
	// 	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
	// 	assert.StringContains(t, "Could not calculate offset and length", e.Message)
	// })

	// t.Run("execution optimistic", func(t *testing.T) {
	// 	chainService := &chainMock.ChainService{Optimistic: true}
	// 	s := &Server{
	// 		Stater: &testutil.MockStater{
	// 			BeaconStateRoot: stateRoot[:],
	// 			BeaconState:     fakeState,
	// 		},
	// 		HeadFetcher:           chainService,
	// 		OptimisticModeFetcher: chainService,
	// 		FinalizationFetcher:   chainService,
	// 		BeaconDB:              db,
	// 		ChainInfoFetcher:      chainService,
	// 	}

	// 	requestBody := &structs.QuerySSZRequest{
	// 		Query: []*structs.QuerySSZQuery{
	// 			{Path: ".slot"},
	// 		},
	// 	}
	// 	var buf bytes.Buffer
	// 	require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

	// 	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/beacon/states/{state_id}/ssz_query", &buf)
	// 	request.SetPathValue("state_id", "head")
	// 	writer := httptest.NewRecorder()
	// 	writer.Body = &bytes.Buffer{}

	// 	s.QueryBeaconState(writer, request)
	// 	require.Equal(t, http.StatusOK, writer.Code)
	// 	resp := &structs.QuerySSZResponse{}
	// 	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	// 	assert.Equal(t, true, resp.ExecutionOptimistic)
	// })

	// t.Run("finalized", func(t *testing.T) {
	// 	headerRoot, err := fakeState.LatestBlockHeader().HashTreeRoot()
	// 	require.NoError(t, err)
	// 	chainService := &chainMock.ChainService{
	// 		FinalizedRoots: map[[32]byte]bool{
	// 			headerRoot: true,
	// 		},
	// 	}
	// 	s := &Server{
	// 		Stater: &testutil.MockStater{
	// 			BeaconStateRoot: stateRoot[:],
	// 			BeaconState:     fakeState,
	// 		},
	// 		HeadFetcher:           chainService,
	// 		OptimisticModeFetcher: chainService,
	// 		FinalizationFetcher:   chainService,
	// 		BeaconDB:              db,
	// 		ChainInfoFetcher:      chainService,
	// 	}

	// 	requestBody := &structs.QuerySSZRequest{
	// 		Query: []*structs.QuerySSZQuery{
	// 			{Path: ".slot"},
	// 		},
	// 	}
	// 	var buf bytes.Buffer
	// 	require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

	// 	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/beacon/states/{state_id}/ssz_query", &buf)
	// 	request.SetPathValue("state_id", "head")
	// 	writer := httptest.NewRecorder()
	// 	writer.Body = &bytes.Buffer{}

	// 	s.QueryBeaconState(writer, request)
	// 	require.Equal(t, http.StatusOK, writer.Code)
	// 	resp := &structs.QuerySSZResponse{}
	// 	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
	// 	assert.Equal(t, true, resp.Finalized)
	// })
}

// func TestServer_QueryBeaconBlock(t *testing.T) {
// 	request := httptest.NewRequest(http.MethodPost, "http://example.com/eth/v1/beacon/blocks/{block_id}/ssz_query", nil)
// 	request.SetPathValue("block_id", "head")
// 	writer := httptest.NewRecorder()
// 	writer.Body = &bytes.Buffer{}

// 	s := &Server{}
// 	s.QueryBeaconBlock(writer, request)
// 	require.Equal(t, http.StatusNotImplemented, writer.Code)
// }

func TestServer_QueryBeaconStateSSZ(t *testing.T) {
	ctx := t.Context()
	fakeState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, fakeState.SetSlot(100))

	// Set a finalized checkpoint
	fcRoot, err := hexutil.Decode("0x4a2c7e9d1f0b85a632e4c9b0f8d716a54b0e8f2d9c5a7136b8d0f4a9e27c1b63")
	require.NoError(t, err)
	fakeState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: 2,
		Root:  fcRoot,
	})

	fakeBlockHeader := &ethpb.BeaconBlockHeader{
		Slot:          112,
		ProposerIndex: 1,
		ParentRoot:    fcRoot,
		StateRoot:     fcRoot,
		BodyRoot:      fcRoot,
	}
	fakeState.SetLatestBlockHeader(fakeBlockHeader)
	// Ensure the latest block header is set correctly.

	stateRoot, err := fakeState.HashTreeRoot(ctx)
	require.NoError(t, err)
	db := dbTest.SetupDB(t)
	parentRoot := [32]byte{'a'}
	blk := util.NewBeaconBlock()
	blk.Block.ParentRoot = parentRoot[:]
	root, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, db, blk)
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

	chainService := &chainMock.ChainService{}
	s := &Server{
		Stater: &testutil.MockStater{
			BeaconStateRoot: stateRoot[:],
			BeaconState:     fakeState,
		},
		HeadFetcher:           chainService,
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
		BeaconDB:              db,
		ChainInfoFetcher:      chainService,
	}

	t.Run("success - query single field", func(t *testing.T) {
		requestBody := &structs.QuerySSZRequest{
			Query: []*structs.QueryObject{
				{Path: ".slot"},
			},
		}
		var buf bytes.Buffer
		require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

		request := httptest.NewRequest(http.MethodPost, "http://example.com/prysm/v1/beacon/states/{state_id}/query", &buf)
		request.SetPathValue("state_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.QueryBeaconStateSSZ(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)

		slot := fakeState.Slot()
		expected, err := slot.MarshalSSZ()
		require.NoError(t, err)

		assert.DeepEqual(t, expected, writer.Body.Bytes())
	})

	t.Run("success - query finalized_checkpoint", func(t *testing.T) {
		requestBody := &structs.QuerySSZRequest{
			Query: []*structs.QueryObject{
				{Path: ".finalized_checkpoint"},
			},
		}
		var buf bytes.Buffer
		require.NoError(t, json.NewEncoder(&buf).Encode(requestBody))

		request := httptest.NewRequest(http.MethodPost, "http://example.com/prysm/v1/beacon/states/{state_id}/query", &buf)
		request.SetPathValue("state_id", "head")
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.QueryBeaconStateSSZ(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)

		finalizedCheckpoint := fakeState.FinalizedCheckpoint()
		expected, err := finalizedCheckpoint.MarshalSSZ()
		require.NoError(t, err)

		assert.DeepEqual(t, expected, writer.Body.Bytes())
	})
}
