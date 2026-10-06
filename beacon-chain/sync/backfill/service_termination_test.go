package backfill

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// routerlessPool keeps the real pool's todo() and complete() but never spawns the router or the workers. The test plays the worker role on
// toRouter / fromRouter instead, so download order is deterministic.
type routerlessPool struct{ *p2pBatchWorkerPool }

func (r routerlessPool) spawn(ctx context.Context, _ int, _ PeerAssigner, _ *workerCfg) {
	r.ctx, r.cancel = context.WithCancel(ctx)
}

// The two final batches come back from the workers in reverse order (floor batch first), and one
// importBatches pass imports both. The single sequence() call after that pass hands the pool one
// batchEndSequence batch. Backfill must still complete, because no work remains.
func TestServiceCompletesWhenFinalBatchesDrainInOnePass(t *testing.T) {
	const (
		floor = primitives.Slot(4037888) // retention floor == needs.Block.Begin
		size  = 32
		nw    = 2 // --backfill-worker-count default
	)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	// Two batches of work remain above the floor: A = [floor+32, floor+64), B = [floor, floor+32).
	su, err := NewUpdater(ctx, &mockBackfillDB{status: &dbval.BackfillStatus{LowSlot: uint64(floor + 2*size)}})
	require.NoError(t, err)
	cw := startup.NewClockSynchronizer()
	// Plain mode, no flags: pick the wall clock so the MIN_EPOCHS_FOR_BLOCK_REQUESTS retention floor lands exactly on `floor`.
	retention := primitives.Slot(params.BeaconConfig().MinEpochsForBlockRequests) * params.BeaconConfig().SlotsPerEpoch
	clock := startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(floor+retention))
	require.NoError(t, cw.SetClock(clock))
	snw := func() (das.SyncNeeds, error) {
		return das.NewSyncNeeds(clock.CurrentSlot, nil, 0)
	}
	needs := func() das.CurrentNeeds {
		sn, err := snw()
		require.NoError(t, err)
		return sn.Currently()
	}

	srv, err := NewService(ctx, su, nil, nil, cw, nil, &mockAssigner{},
		WithBatchSize(size), WithWorkerCount(nw), WithEnableBackfill(true), WithSyncNeedsWaiter(snw))
	require.NoError(t, err)
	pool := routerlessPool{newP2PBatchWorkerPool(nil, nw, needs)}
	srv.pool = pool
	srv.workerCfg = &workerCfg{}
	srv.batchImporter = func(context.Context, primitives.Slot, batch, *Store) (*dbval.BackfillStatus, error) {
		return &dbval.BackfillStatus{}, nil
	}
	go srv.Start()

	// initBatches hands the router A then B.
	handed := testReadN(ctx, t, pool.toRouter, nw, nil)
	a, b := handed[0], handed[1]
	require.Equal(t, floor+size, a.begin)
	require.Equal(t, floor, b.begin)

	downloaded := func(b batch) batch {
		b.state = batchImportable
		for s := b.begin; s < b.end; s++ {
			blk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, s, 0)
			b.blocks = append(b.blocks, blk)
		}
		return b
	}
	// Workers return B first, then A. B cannot import until A has.
	pool.fromRouter <- downloaded(b)
	pool.fromRouter <- downloaded(a)

	// Nothing is left below the floor, so backfill must complete on its own.
	done := make(chan error, 1)
	go func() { done <- srv.WaitForCompletion() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		states := make([]string, 0, len(srv.batchSeq.seq))
		for _, sb := range srv.batchSeq.seq {
			states = append(states, fmt.Sprintf("%d:%d=%s", sb.begin, sb.end, sb.state))
		}
		t.Fatalf("backfill never completed with both workers idle; sequencer slots %v", states)
	}
}
