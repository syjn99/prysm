package middleware

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
)

type stateMetaKeyType struct{}

var stateMetaKey = stateMetaKeyType{}

// StateMeta contains precomputed metadata about a beacon state.
type StateMeta struct {
	IsOptimistic bool
	IsFinalized  bool
	State        state.BeaconState
}

// StateMetaFromContext retrieves the StateMeta from the context, if present.
func StateMetaFromContext(ctx context.Context) (*StateMeta, bool) {
	meta, ok := ctx.Value(stateMetaKey).(*StateMeta)
	return meta, ok
}

// WithStateMeta returns a new context with the StateMeta attached.
func WithStateMeta(ctx context.Context, meta *StateMeta) context.Context {
	return context.WithValue(ctx, stateMetaKey, meta)
}
