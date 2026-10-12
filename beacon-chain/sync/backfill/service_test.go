package backfill

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

type mockMinimumSlotter struct {
	min primitives.Slot
}

func (m mockMinimumSlotter) minimumSlot(_ primitives.Slot) primitives.Slot {
	return m.min
}

type mockInitalizerWaiter struct {
}

func (*mockInitalizerWaiter) WaitForInitializer(_ context.Context) (*verification.Initializer, error) {
	return &verification.Initializer{}, nil
}

func TestServiceInit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*300)
	defer cancel()
	db := &mockBackfillDB{}
	su, err := NewUpdater(ctx, db)
	require.NoError(t, err)
	nWorkers := 5
	var batchSize uint64 = 4
	nBatches := nWorkers * 2
	var high uint64 = 1 + batchSize*uint64(nBatches) // extra 1 because upper bound is exclusive
	originRoot := [32]byte{}
	origin, err := util.NewBeaconState()
	require.NoError(t, err)
	db.states = map[[32]byte]state.BeaconState{originRoot: origin}
	su.bs = &dbval.BackfillStatus{
		LowSlot:    high,
		OriginRoot: originRoot[:],
	}
	remaining := nBatches
	cw := startup.NewClockSynchronizer()

	clock := startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(primitives.Slot(high)+1))
	require.NoError(t, cw.SetClock(clock))
	pool := &mockPool{todoChan: make(chan batch, nWorkers), finishedChan: make(chan batch, nWorkers)}
	p2pt := p2ptest.NewTestP2P(t)
	bfs := filesystem.NewEphemeralBlobStorage(t)
	dcs := filesystem.NewEphemeralDataColumnStorage(t)
	snw := func() (das.SyncNeeds, error) {
		return das.NewSyncNeeds(
			clock.CurrentSlot,
			nil,
			nil,
			primitives.Epoch(0),
		)
	}
	srv, err := NewService(ctx, su, bfs, dcs, cw, p2pt, &mockAssigner{},
		WithBatchSize(batchSize), WithWorkerCount(nWorkers), WithEnableBackfill(true), WithVerifierWaiter(&mockInitalizerWaiter{}),
		WithSyncNeedsWaiter(snw))
	require.NoError(t, err)
	srv.pool = pool
	srv.batchImporter = func(context.Context, primitives.Slot, batch, *Store) (*dbval.BackfillStatus, error) {
		return &dbval.BackfillStatus{}, nil
	}
	go srv.Start()
	todo := make([]batch, 0)
	todo = testReadN(ctx, t, pool.todoChan, nWorkers, todo)
	require.Equal(t, nWorkers, len(todo))
	for i := range remaining {
		b := todo[i]
		if b.state == batchSequenced {
			b.state = batchImportable
		}
		for i := b.begin; i < b.end; i++ {
			blk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, primitives.Slot(i), 0)
			b.blocks = append(b.blocks, blk)
		}
		require.Equal(t, int(batchSize), len(b.blocks))
		pool.finishedChan <- b
		todo = testReadN(ctx, t, pool.todoChan, 1, todo)
	}
	require.Equal(t, remaining+nWorkers, len(todo))
	for i := remaining; i < remaining+nWorkers; i++ {
		require.Equal(t, batchEndSequence, todo[i].state)
	}
}

func TestServiceCompletesOnRestartAtPinnedFloor(t *testing.T) {
	floor := primitives.Slot(1024)
	for _, tc := range []struct {
		name                  string
		oldest, archiveOrigin *primitives.Slot
		// headerSlot is the slot of the archive origin state's latest block header. It is below the floor
		// when the origin slot has no block.
		headerSlot primitives.Slot
		// lowSlot is the slot of the lowest imported block. parentIsHeader makes its parent the origin header.
		lowSlot        primitives.Slot
		parentIsHeader bool
		complete       bool
	}{
		{name: "archive origin", archiveOrigin: &floor, headerSlot: floor, lowSlot: floor, complete: true},
		{name: "backfill oldest slot", oldest: &floor, lowSlot: floor, complete: true},
		{name: "archive origin at a skipped slot", archiveOrigin: &floor, headerSlot: floor - 1, lowSlot: floor + 1,
			parentIsHeader: true, complete: true},
		{name: "archive origin at a skipped slot, parent is another block", archiveOrigin: &floor, headerSlot: floor - 1,
			lowSlot: floor + 1},
		{name: "archive origin block not imported", archiveOrigin: &floor, headerSlot: floor, lowSlot: floor + 1,
			parentIsHeader: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetCfg := features.InitWithReset(&features.Flags{EnableArchive: tc.archiveOrigin != nil})
			defer resetCfg()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			bdb := &mockBackfillDB{status: &dbval.BackfillStatus{LowSlot: uint64(tc.lowSlot), LowParentRoot: make([]byte, 32)}}
			if tc.archiveOrigin != nil {
				originState, err := util.NewBeaconState()
				require.NoError(t, err)
				require.NoError(t, originState.SetSlot(*tc.archiveOrigin))
				header := &ethpb.BeaconBlockHeader{
					Slot:       tc.headerSlot,
					ParentRoot: make([]byte, 32),
					StateRoot:  bytes.Repeat([]byte{0x01}, 32),
					BodyRoot:   make([]byte, 32),
				}
				require.NoError(t, originState.SetLatestBlockHeader(header))
				bdb.diffTreeStates = map[primitives.Slot]state.BeaconState{*tc.archiveOrigin: originState}
				if tc.parentIsHeader {
					root, err := header.HashTreeRoot()
					require.NoError(t, err)
					bdb.status.LowParentRoot = root[:]
				}
			}
			su, err := NewUpdater(ctx, bdb)
			require.NoError(t, err)
			cw := startup.NewClockSynchronizer()
			clock := startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(floor+2_000_000))
			require.NoError(t, cw.SetClock(clock))
			snw := func() (das.SyncNeeds, error) {
				return das.NewSyncNeeds(clock.CurrentSlot, tc.oldest, tc.archiveOrigin, 0)
			}
			srv, err := NewService(ctx, su, nil, nil, cw, nil, &mockAssigner{},
				WithBatchSize(32), WithWorkerCount(2), WithEnableBackfill(true), WithSyncNeedsWaiter(snw))
			require.NoError(t, err)
			pool := &mockPool{todoChan: make(chan batch, 8), finishedErr: make(chan error, 1)}
			srv.pool = pool
			srv.workerCfg = &workerCfg{}
			stopped := make(chan struct{})
			go func() {
				srv.Start()
				close(stopped)
			}()
			// The run loop blocks in pool.complete(), which does not watch the context. errEndSequence ends it.
			defer func() {
				pool.finishedErr <- errEndSequence
				<-stopped
			}()

			if tc.complete {
				require.NoError(t, srv.WaitForCompletion())
				return
			}
			// A block can still exist between the floor and the lowest block, so backfill must request it.
			todo := testReadN(ctx, t, pool.todoChan, 1, nil)
			require.Equal(t, floor, todo[0].begin)
		})
	}
}

func testReadN(ctx context.Context, t *testing.T, c chan batch, n int, into []batch) []batch {
	for range n {
		select {
		case b := <-c:
			into = append(into, b)
		case <-ctx.Done():
			// this means we hit the timeout, so something went wrong.
			require.Equal(t, true, false)
		}
	}
	return into
}

func TestDefaultBatchImporter(t *testing.T) {
	_, fuluSlot := testDenebAndFuluSlots(t)
	current := fuluSlot + 100
	parentHash, lastHash := [32]byte{'p'}, [32]byte{'l'}

	// setup returns a batch whose last block has blobs and an undecided payload, and a store
	// whose lowest backfilled block is the child of that last block.
	setup := func(t *testing.T, childParentHash [32]byte) (*Service, batch, *Store) {
		last := testGloasBlock(t, fuluSlot+1, [32]byte{}, parentHash, lastHash, 1)
		child := testGloasBlock(t, fuluSlot+2, last.Root(), childParentHash, [32]byte{'c'}, 0)
		childRoot, lastRoot := child.Root(), last.Root()
		su := &Store{
			store: &mockBackfillDB{blocks: map[[32]byte]blocks.ROBlock{childRoot: child}},
			bs: &dbval.BackfillStatus{
				LowSlot:       uint64(child.Block().Slot()),
				LowRoot:       childRoot[:],
				LowParentRoot: lastRoot[:],
			},
		}

		sn, err := das.NewSyncNeeds(func() primitives.Slot { return current }, nil, nil, 0)
		require.NoError(t, err)
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)
		blks := []blocks.ROBlock{last}
		cb, err := buildColumnBatch(t.Context(), last.Block().Slot(), child.Block().Slot(), blks, p, store, sn.Currently())
		require.NoError(t, err)
		require.Equal(t, true, cb.toDownload[lastRoot].undecidedPayload)

		retain := func(primitives.Slot) bool { return true }
		colStore := das.NewLazilyPersistentStoreColumn(store, nil, p.NodeID(), params.BeaconConfig().CustodyRequirement, nil, retain)
		b := batch{blocks: blks, columns: &columnSync{columnBatch: cb, store: colStore}}
		return &Service{syncNeeds: sn}, b, su
	}

	t.Run("empty payload of the last block is imported without columns", func(t *testing.T) {
		s, b, su := setup(t, parentHash)
		status, err := s.defaultBatchImporter(t.Context(), current, b, su)
		require.NoError(t, err)
		lastRoot := b.blocks[0].Root()
		require.DeepEqual(t, lastRoot[:], status.LowRoot)
	})

	t.Run("full payload of the last block requires columns", func(t *testing.T) {
		s, b, su := setup(t, lastHash)
		_, err := s.defaultBatchImporter(t.Context(), current, b, su)
		require.ErrorContains(t, "no sidecar in cache for block commitment", err)
		require.Equal(t, false, b.columns.toDownload[b.blocks[0].Root()].undecidedPayload)
	})
}
