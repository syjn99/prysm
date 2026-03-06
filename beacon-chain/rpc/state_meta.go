package rpc

import (
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api/server/middleware"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
)

// stateMetaConfig contains the dependencies needed by the stateMetaHandler.
type stateMetaConfig struct {
	Stater                lookup.Stater
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
	ChainInfoFetcher      blockchain.ChainInfoFetcher
	BeaconDB              db.ReadOnlyDatabase
	FinalizationFetcher   blockchain.FinalizationFetcher
}

// stateMetaHandler is middleware that precomputes IsOptimistic and IsFinalized
// for state-based endpoints. It also stores the loaded BeaconState in the context
// so handlers don't need to reload it.
func stateMetaHandler(cfg *stateMetaConfig) middleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			stateId := r.PathValue("state_id")
			if stateId == "" {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			st, err := cfg.Stater.State(ctx, []byte(stateId))
			if err != nil {
				shared.WriteStateFetchError(w, err)
				return
			}

			isOptimistic, err := helpers.IsOptimistic(
				ctx, []byte(stateId),
				cfg.OptimisticModeFetcher,
				cfg.Stater,
				cfg.ChainInfoFetcher,
				cfg.BeaconDB,
			)
			if err != nil {
				helpers.HandleIsOptimisticError(w, err)
				return
			}

			blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
			if err != nil {
				httputil.HandleError(w, "Could not calculate root of latest block header: "+err.Error(), http.StatusInternalServerError)
				return
			}
			isFinalized := cfg.FinalizationFetcher.IsFinalized(ctx, blockRoot)

			meta := &middleware.StateMeta{
				IsOptimistic: isOptimistic,
				IsFinalized:  isFinalized,
				State:        st,
			}
			next.ServeHTTP(w, r.WithContext(middleware.WithStateMeta(ctx, meta)))
		})
	}
}
