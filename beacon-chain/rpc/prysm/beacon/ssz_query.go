package beacon

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func (s *Server) QueryBeaconState(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.QueryBeaconState")
	defer span.End()

	stateID := r.PathValue("state_id")
	if stateID == "" {
		httputil.HandleError(w, "state_id is required in URL params", http.StatusBadRequest)
		return
	}

	// Fetch state root for the given state ID.
	stateRoot, err := s.Stater.StateRoot(ctx, []byte(stateID))
	if err != nil {
		var rootNotFoundErr *lookup.StateRootNotFoundError
		if errors.As(err, &rootNotFoundErr) {
			httputil.HandleError(w, "State root not found: "+rootNotFoundErr.Error(), http.StatusNotFound)
			return
		}
		httputil.HandleError(w, "Could not get state root: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch state from given state ID.
	st, err := s.Stater.State(ctx, []byte(stateID))
	if err != nil {
		shared.WriteStateFetchError(w, err)
		return
	}

	// Gather `execution_optimistic` and `finalized` flags.
	isOptimistic, err := helpers.IsOptimistic(ctx, []byte(stateID), s.OptimisticModeFetcher, s.Stater, s.ChainInfoFetcher, s.BeaconDB)
	if err != nil {
		httputil.HandleError(w, "Could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
		return
	}
	isFinalized := s.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	// Parse request body.
	var req structs.QuerySSZRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Query) == 0 {
		httputil.HandleError(w, "No query submitted", http.StatusBadRequest)
		return
	}

	// Analyze the state object to get sszInfo.
	protoState := st.ToProto()
	info, err := query.AnalyzeObject(&protoState)
	if err != nil {
		httputil.HandleError(w, "Could not analyze state object: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// marshalledData is needed to slice out the requested paths.
	marshalledData, err := st.MarshalSSZ()
	if err != nil {
		httputil.HandleError(w, "Could not marshal state to SSZ: "+err.Error(), http.StatusInternalServerError)
		return
	}

	paths := make([]string, 0, len(req.Query))
	results := make([]json.RawMessage, 0, len(req.Query))
	for _, eachQuery := range req.Query {
		pathStr := eachQuery.Path

		path, err := query.ParsePath(pathStr)
		if err != nil {
			httputil.HandleError(w, "Could not parse path '"+pathStr+"': "+err.Error(), http.StatusBadRequest)
			return
		}

		paths = append(paths, pathStr)

		_, offset, length, err := query.CalculateOffsetAndLength(info, path)
		if err != nil {
			httputil.HandleError(w, "Could not calculate offset and length for path '"+pathStr+"': "+err.Error(), http.StatusBadRequest)
			return
		}

		_ = marshalledData[offset : offset+length]
	}

	querySSZResponse := &structs.QuerySSZResponse{
		Version:             version.String(st.Version()),
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
		Data: &structs.QuerySSZData{
			Root: hexutil.Encode(stateRoot),
			Values: &structs.QuerySSZValue{
				Paths:   paths,
				Results: results,
			},
			// For now, as we can't build proofs,
			// just assume `req.IncludeProof` and `req.Multiproof` are both false.
			Proofs: nil,
		},
	}

	httputil.WriteJson(w, querySSZResponse)
}

func (s *Server) QueryBeaconBlock(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "beacon.QueryBeaconBlock")
	defer span.End()

	httputil.HandleError(w, "not implemented", http.StatusNotImplemented)
}
