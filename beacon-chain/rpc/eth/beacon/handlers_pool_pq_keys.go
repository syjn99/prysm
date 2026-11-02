package beacon

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// ListPQKeyRegistrations returns the EIP-8450 post-quantum key registrations in the node's pool.
func (s *Server) ListPQKeyRegistrations(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "beacon.ListPQKeyRegistrations")
	defer span.End()

	pending := s.PQKeyPool.PendingRegistrations()
	data := make([]*structs.SignedPQKeyRegistration, len(pending))
	for i, reg := range pending {
		data[i] = structs.SignedPQKeyRegistrationFromConsensus(reg)
	}
	httputil.WriteJson(w, &structs.ListPQKeyRegistrationsResponse{Data: data})
}

// SubmitPQKeyRegistrations validates EIP-8450 post-quantum key registrations against the head state,
// adds the valid ones to the pool and broadcasts them. Each registration is checked on its own against
// the head state, as peers check it on gossip, so the node never broadcasts a registration that is valid
// only after another registration of the same request. The pool resolves conflicts at block production.
func (s *Server) SubmitPQKeyRegistrations(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.SubmitPQKeyRegistrations")
	defer span.End()

	var req []*structs.SignedPQKeyRegistration
	err := json.NewDecoder(r.Body).Decode(&req)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req) == 0 {
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	}

	headState, err := s.ChainInfoFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		httputil.HandleError(w, "Could not get head state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if headState.Version() < version.Heze {
		httputil.HandleError(w, "PQ key registrations are not supported before Heze", http.StatusBadRequest)
		return
	}

	var failures []*server.IndexedError
	for i, item := range req {
		if item == nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: "Unable to decode SignedPQKeyRegistration: nil value"})
			continue
		}
		reg, err := item.ToConsensus()
		if err != nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: "Unable to decode SignedPQKeyRegistration: " + err.Error()})
			continue
		}
		if err := heze.ValidatePQKeyRegistration(headState, reg); err != nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: "Invalid PQ key registration: " + err.Error()})
			continue
		}
		s.PQKeyPool.InsertRegistration(reg)
		if err := s.Broadcaster.Broadcast(ctx, reg); err != nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: "Could not broadcast PQ key registration: " + err.Error()})
		}
	}
	if len(failures) > 0 {
		httputil.WriteError(w, &server.IndexedErrorContainer{
			Code:     http.StatusBadRequest,
			Message:  server.ErrIndexedValidationFail,
			Failures: failures,
		})
	}
}
