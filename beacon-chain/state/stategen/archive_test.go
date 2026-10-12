package stategen

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

// While the archive walk owns the tree, the live chain must not migrate cold states into it: the anchors the
// live region needs lie in the not yet regenerated past.
func TestMigrateToCold_SuppressedWhileArchivePending(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	service := New(beaconDB, doublylinkedtree.New())
	service.SetArchivePending(true)
	require.Equal(t, true, service.ArchivePending())

	// A finalized root that does not exist in the db: MigrateToCold would fail looking it up, so returning
	// nil proves it never got that far.
	require.NoError(t, service.MigrateToCold(ctx, [32]byte{1}))

	service.SetArchivePending(false)
	require.NotNil(t, service.MigrateToCold(ctx, [32]byte{1}))
}

// ForceCheckpoint runs on every graceful shutdown. Against a re-based offset the tree write would either fail
// or clear the walk's anchors, so it must persist by root instead.
func TestForceCheckpoint_WritesSnapshotWhileArchivePending(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	service := New(beaconDB, doublylinkedtree.New())
	service.SetArchivePending(true)

	st, _ := util.DeterministicGenesisState(t, 32)
	require.NoError(t, st.SetSlot(64))
	root := [32]byte{2}
	service.hotStateCache.put(root, st)

	require.NoError(t, service.ForceCheckpoint(ctx, root[:]))
	require.Equal(t, true, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, root))
}

// Clearing the bucket would drop the checkpoint origin state and the resume snapshots that backfill and the
// next boot depend on.
func TestDisableSaveHotStateToDB_KeepsSnapshotsWhileArchivePending(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	service := New(beaconDB, doublylinkedtree.New())
	service.SetArchivePending(true)
	service.EnableSaveHotStateToDB(ctx)

	st, _ := util.DeterministicGenesisState(t, 32)
	root := [32]byte{3}
	require.NoError(t, beaconDB.(*kv.Store).SaveHotStateSnapshot(ctx, st, root))

	require.NoError(t, service.DisableSaveHotStateToDB(ctx))
	require.Equal(t, true, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, root))

	// Once regeneration is done the normal clearing behavior returns.
	service.SetArchivePending(false)
	service.EnableSaveHotStateToDB(ctx)
	require.NoError(t, service.DisableSaveHotStateToDB(ctx))
	require.Equal(t, false, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, root))
}

// Resume snapshots are written independently of saveHotStateDB, since checkSaveHotStateDB turns that mode off
// on every block while finality is healthy. Only the newest is kept.
func TestSaveArchiveResumeSnapshot_KeepsOnlyNewest(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	service := New(beaconDB, doublylinkedtree.New())
	service.SetArchivePending(true)

	st, _ := util.DeterministicGenesisState(t, 32)
	first := [32]byte{4}
	require.NoError(t, st.SetSlot(archiveResumeSnapshotInterval))
	require.NoError(t, service.saveArchiveResumeSnapshot(ctx, first, st))
	require.Equal(t, true, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, first))

	// A slot that is not on the interval is skipped.
	skipped := [32]byte{5}
	require.NoError(t, st.SetSlot(archiveResumeSnapshotInterval+1))
	require.NoError(t, service.saveArchiveResumeSnapshot(ctx, skipped, st))
	require.Equal(t, false, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, skipped))

	second := [32]byte{6}
	require.NoError(t, st.SetSlot(2*archiveResumeSnapshotInterval))
	require.NoError(t, service.saveArchiveResumeSnapshot(ctx, second, st))
	require.Equal(t, true, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, second))
	require.Equal(t, false, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, first))
}

// The handoff must not happen until every boundary at or below the finalized slot has been written, otherwise
// a live write can resolve to an anchor the walk never produced. After it, cold migration resumes at the first
// boundary the walk did not write.
func TestCompleteArchiveRegeneration(t *testing.T) {
	t.Run("waits for the walk to pass the finalized slot", func(t *testing.T) {
		ctx := t.Context()
		setStateDiffExponents()
		beaconDB := testDB.SetupDB(t)
		require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
		resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
		defer resetCfg()

		service := New(beaconDB, doublylinkedtree.New())
		service.SetArchivePending(true)

		slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
		fEpoch := primitives.Epoch(4)
		fSlot := primitives.Slot(fEpoch) * slotsPerEpoch

		st, _ := util.DeterministicGenesisState(t, 32)
		genesisStateRoot, err := st.HashTreeRoot(ctx)
		require.NoError(t, err)
		genesisBlk := blocks.NewGenesisBlock(genesisStateRoot[:])
		util.SaveBlock(t, ctx, beaconDB, genesisBlk)
		gRoot, err := genesisBlk.Block.HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

		require.NoError(t, st.SetSlot(fSlot))
		stRoot, err := st.HashTreeRoot(ctx)
		require.NoError(t, err)
		blk := util.NewBeaconBlock()
		blk.Block.Slot = fSlot
		blk.Block.ParentRoot = gRoot[:]
		blk.Block.StateRoot = stRoot[:]
		util.SaveBlock(t, ctx, beaconDB, blk)
		fRoot, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: fSlot, Root: fRoot[:]}))
		require.NoError(t, beaconDB.SaveState(ctx, st, fRoot))
		require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: fEpoch, Root: fRoot[:]}))

		marked := 0
		markComplete := func(context.Context) error {
			marked++
			return nil
		}

		// Not yet: the next unwritten boundary is still below the finalized slot. Nothing is recorded either,
		// since there was no handoff to record.
		done, err := service.CompleteArchiveRegeneration(ctx, fSlot-1, markComplete)
		require.NoError(t, err)
		require.Equal(t, false, done)
		require.Equal(t, true, service.ArchivePending())
		require.Equal(t, 0, marked)

		done, err = service.CompleteArchiveRegeneration(ctx, fSlot+1, markComplete)
		require.NoError(t, err)
		require.Equal(t, true, done)
		require.Equal(t, false, service.ArchivePending())
		require.Equal(t, 1, marked)
		require.Equal(t, fSlot, service.finalizedInfo.state.Slot())
		require.Equal(t, fRoot, service.finalizedInfo.root)
	})

	// Otherwise migration writes the walked boundaries again, which takes hours after a long walk.
	t.Run("advances the migration cursor", func(t *testing.T) {
		hook := logTest.NewGlobal()
		ctx := t.Context()
		setStateDiffExponents()
		beaconDB := testDB.SetupDB(t)
		require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
		resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
		defer resetCfg()

		service := New(beaconDB, doublylinkedtree.New())
		service.SetArchivePending(true)

		slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
		fEpoch := primitives.Epoch(4)
		fSlot := primitives.Slot(fEpoch) * slotsPerEpoch
		// Resume sets the cursor to the finalized slot at startup, far below the walk end.
		service.migratedSlot = slotsPerEpoch

		st, _ := util.DeterministicGenesisState(t, 32)
		genesisStateRoot, err := st.HashTreeRoot(ctx)
		require.NoError(t, err)
		genesisBlk := blocks.NewGenesisBlock(genesisStateRoot[:])
		util.SaveBlock(t, ctx, beaconDB, genesisBlk)
		gRoot, err := genesisBlk.Block.HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

		require.NoError(t, st.SetSlot(fSlot))
		stRoot, err := st.HashTreeRoot(ctx)
		require.NoError(t, err)
		blk := util.NewBeaconBlock()
		blk.Block.Slot = fSlot
		blk.Block.ParentRoot = gRoot[:]
		blk.Block.StateRoot = stRoot[:]
		util.SaveBlock(t, ctx, beaconDB, blk)
		fRoot, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: fSlot, Root: fRoot[:]}))
		require.NoError(t, beaconDB.SaveState(ctx, st, fRoot))
		require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: fEpoch, Root: fRoot[:]}))
		require.NoError(t, service.epochBoundaryStateCache.put(fRoot, st))

		next := fSlot + slotsPerEpoch
		done, err := service.CompleteArchiveRegeneration(ctx, next, func(context.Context) error { return nil })
		require.NoError(t, err)
		require.Equal(t, true, done)
		require.Equal(t, next, service.migratedSlot)

		hook.Reset()
		// A migration for the handoff checkpoint can wait on the migration lock during the handoff.
		require.NoError(t, service.MigrateToCold(ctx, fRoot))
		require.Equal(t, next, service.migratedSlot)

		later := util.NewBeaconBlock()
		later.Block.Slot = next - 1
		later.Block.ParentRoot = fRoot[:]
		util.SaveBlock(t, ctx, beaconDB, later)
		laterRoot, err := later.Block.HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, service.MigrateToCold(ctx, laterRoot))
		require.LogsDoNotContain(t, hook, "Saved state in DB")
		require.Equal(t, next, service.migratedSlot)
	})

	// The chain has blocks at 32, 64, 96, 128, 159 and 192. The walk wrote boundaries through slot 128 and stored
	// its progress. The next unwritten boundary is 160. Slot 160 is empty. Each boot follows the node startup:
	// pending comes from the stored status, and the finalized state comes from StateByRoot.
	t.Run("migrates across an empty first boundary after a restart during the walk", func(t *testing.T) {
		hook := logTest.NewGlobal()
		ctx := t.Context()
		c := newHandoffChain(t)
		c.finalize(t, 1, 32)
		service := c.boot(t)
		require.Equal(t, true, service.ArchivePending())

		c.finalize(t, 4, 128)
		done, err := service.CompleteArchiveRegeneration(ctx, 160, c.markComplete)
		require.NoError(t, err)
		require.Equal(t, true, done)

		hook.Reset()
		c.finalize(t, 6, 192)
		require.NoError(t, service.MigrateToCold(ctx, c.roots[192]))
		require.Equal(t, primitives.Slot(192), service.migratedSlot)
		c.requireBoundary(t, 160)
		require.LogsDoNotContain(t, hook, "slot=128")
	})

	t.Run("migrates across an empty first boundary after a restart after the handoff", func(t *testing.T) {
		ctx := t.Context()
		c := newHandoffChain(t)
		c.finalize(t, 4, 128)
		done, err := c.boot(t).CompleteArchiveRegeneration(ctx, 160, c.markComplete)
		require.NoError(t, err)
		require.Equal(t, true, done)

		service := c.boot(t)
		require.Equal(t, false, service.ArchivePending())
		c.finalize(t, 6, 192)
		require.NoError(t, service.MigrateToCold(ctx, c.roots[192]))
		c.requireBoundary(t, 160)
	})

	// The stored status stays incomplete, so the next boot keeps migration suppressed until the walk hands off again.
	t.Run("hands off again after a restart when the completion was not stored", func(t *testing.T) {
		ctx := t.Context()
		c := newHandoffChain(t)
		c.finalize(t, 4, 128)
		writeErr := errors.New("status write failed")
		_, err := c.boot(t).CompleteArchiveRegeneration(ctx, 160, func(context.Context) error { return writeErr })
		require.ErrorIs(t, err, writeErr)

		service := c.boot(t)
		require.Equal(t, true, service.ArchivePending())
		done, err := service.CompleteArchiveRegeneration(ctx, 160, c.markComplete)
		require.NoError(t, err)
		require.Equal(t, true, done)

		c.finalize(t, 6, 192)
		require.NoError(t, service.MigrateToCold(ctx, c.roots[192]))
		c.requireBoundary(t, 160)
		require.Equal(t, false, c.boot(t).ArchivePending())
	})

	// Resume sets the cursor to the finalized slot, so a boundary below it whose migration did not run before
	// the stop is not written. This window is the same for every state-diff node.
	t.Run("does not write a boundary whose migration did not run before a restart", func(t *testing.T) {
		ctx := t.Context()
		c := newHandoffChain(t)
		c.finalize(t, 4, 128)
		done, err := c.boot(t).CompleteArchiveRegeneration(ctx, 160, c.markComplete)
		require.NoError(t, err)
		require.Equal(t, true, done)

		c.finalize(t, 6, 192)
		service := c.boot(t)
		require.Equal(t, false, service.ArchivePending())
		require.Equal(t, primitives.Slot(192), service.migratedSlot)

		_, err = c.db.StateBySlotFromDiffTree(ctx, 160)
		require.ErrorContains(t, "state diff not found at level 1 slot 160", err)
	})
}

// If the handoff cannot be recorded, migration must stay suppressed rather than run against a closed gate.
func TestCompleteArchiveRegeneration_MarkCompleteFailureKeepsSuppression(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	service := New(beaconDB, doublylinkedtree.New())
	service.SetArchivePending(true)

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	fEpoch := primitives.Epoch(4)
	fSlot := primitives.Slot(fEpoch) * slotsPerEpoch

	st, _ := util.DeterministicGenesisState(t, 32)
	genesisStateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesisBlk := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesisBlk)
	gRoot, err := genesisBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	require.NoError(t, st.SetSlot(fSlot))
	stRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	blk := util.NewBeaconBlock()
	blk.Block.Slot = fSlot
	blk.Block.ParentRoot = gRoot[:]
	blk.Block.StateRoot = stRoot[:]
	util.SaveBlock(t, ctx, beaconDB, blk)
	fRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: fSlot, Root: fRoot[:]}))
	require.NoError(t, beaconDB.SaveState(ctx, st, fRoot))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: fEpoch, Root: fRoot[:]}))

	// A resume snapshot that must survive, since the next boot still needs it as a replay base.
	resumeRoot := [32]byte{'r'}
	require.NoError(t, beaconDB.(*kv.Store).SaveArchiveResumeSnapshot(ctx, st, resumeRoot))

	writeErr := errors.New("status write failed")
	done, err := service.CompleteArchiveRegeneration(ctx, fSlot+1, func(context.Context) error {
		return writeErr
	})
	require.ErrorIs(t, err, writeErr)
	require.Equal(t, false, done)
	require.Equal(t, true, service.ArchivePending())
	require.Equal(t, true, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, resumeRoot))

	// A later retry with a working write completes normally.
	done, err = service.CompleteArchiveRegeneration(ctx, fSlot+1, func(context.Context) error { return nil })
	require.NoError(t, err)
	require.Equal(t, true, done)
	require.Equal(t, false, service.ArchivePending())
	require.Equal(t, false, beaconDB.(*kv.Store).HasHotStateSnapshot(ctx, resumeRoot))
}

func TestSaveArchiveResumeSnapshot_DeletesSnapshotsFromPreviousRun(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	store := beaconDB.(*kv.Store)

	st, _ := util.DeterministicGenesisState(t, 32)
	first := [32]byte{4}
	require.NoError(t, st.SetSlot(archiveResumeSnapshotInterval))
	previous := New(beaconDB, doublylinkedtree.New())
	previous.SetArchivePending(true)
	require.NoError(t, previous.saveArchiveResumeSnapshot(ctx, first, st))

	restarted := New(beaconDB, doublylinkedtree.New())
	restarted.SetArchivePending(true)
	second := [32]byte{6}
	require.NoError(t, st.SetSlot(2*archiveResumeSnapshotInterval))
	require.NoError(t, restarted.saveArchiveResumeSnapshot(ctx, second, st))

	require.Equal(t, false, store.HasHotStateSnapshot(ctx, first))
	require.Equal(t, true, store.HasHotStateSnapshot(ctx, second))
	roots, err := store.ArchiveResumeSnapshotRoots(ctx)
	require.NoError(t, err)
	require.DeepEqual(t, [][32]byte{second}, roots)
}

func TestForceCheckpoint_ArchivePendingSnapshotIsTracked(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	store := beaconDB.(*kv.Store)

	service := New(beaconDB, doublylinkedtree.New())
	service.SetArchivePending(true)

	st, _ := util.DeterministicGenesisState(t, 32)
	root := [32]byte{'f'}
	service.hotStateCache.put(root, st)
	require.NoError(t, service.ForceCheckpoint(ctx, root[:]))

	roots, err := store.ArchiveResumeSnapshotRoots(ctx)
	require.NoError(t, err)
	require.DeepEqual(t, [][32]byte{root}, roots)
	require.Equal(t, true, store.HasHotStateSnapshot(ctx, root))
}

func TestSaveArchiveResumeSnapshot_LeavesUntrackedSnapshots(t *testing.T) {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	store := beaconDB.(*kv.Store)

	st, _ := util.DeterministicGenesisState(t, 32)
	origin := [32]byte{'o'}
	require.NoError(t, store.SaveHotStateSnapshot(ctx, st, origin))
	require.NoError(t, store.SaveArchiveResumeSnapshot(ctx, st, origin))

	roots, err := store.ArchiveResumeSnapshotRoots(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, len(roots))
	require.NoError(t, store.DeleteArchiveResumeSnapshots(ctx, [][32]byte{origin}))
	require.Equal(t, true, store.HasHotStateSnapshot(ctx, origin))
}

type handoffChain struct {
	db     *kv.Store
	roots  map[primitives.Slot][32]byte
	states map[primitives.Slot]state.BeaconState
}

// newHandoffChain builds a chain with real blocks and writes the tree boundaries the archive walk would write.
func newHandoffChain(t *testing.T) *handoffChain {
	ctx := t.Context()
	setStateDiffExponents()
	beaconDB := testDB.SetupDB(t)
	require.NoError(t, beaconDB.(*kv.Store).InitStateDiffCacheForTesting(t, 0))
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	t.Cleanup(resetCfg)

	c := &handoffChain{
		db:     beaconDB.(*kv.Store),
		roots:  make(map[primitives.Slot][32]byte),
		states: make(map[primitives.Slot]state.BeaconState),
	}
	st, pks := util.DeterministicGenesisState(t, 32)
	stRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(stRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: 0, Root: gRoot[:]}))
	require.NoError(t, beaconDB.SaveState(ctx, st, gRoot))
	as := &kv.ArchiveStatus{OriginSlot: 0, RegeneratedThroughSlot: 0, OriginStateRoot: stRoot}
	require.NoError(t, c.db.SaveArchiveStatus(ctx, as))

	for _, slot := range []primitives.Slot{32, 64, 96, 128, 159, 192} {
		b, err := util.GenerateFullBlock(st, pks, util.DefaultBlockGenConfig(), slot)
		require.NoError(t, err)
		wsb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		st, err = executeStateTransitionStateGen(ctx, st, wsb)
		require.NoError(t, err)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, beaconDB, b)
		require.NoError(t, beaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Slot: slot, Root: root[:]}))
		c.roots[slot] = root
		c.states[slot] = st.Copy()
		if slot <= 128 {
			require.NoError(t, beaconDB.SaveState(ctx, st, root))
			as.RegeneratedThroughSlot = slot
			require.NoError(t, c.db.SaveArchiveStatus(ctx, as))
		}
	}
	return c
}

func (c *handoffChain) finalize(t *testing.T, epoch primitives.Epoch, slot primitives.Slot) {
	root := c.roots[slot]
	require.NoError(t, c.db.SaveFinalizedCheckpoint(t.Context(), &ethpb.Checkpoint{Epoch: epoch, Root: root[:]}))
}

// requireBoundary checks that the tree holds the state at slot, built on the canonical block at or below it.
func (c *handoffChain) requireBoundary(t *testing.T, slot primitives.Slot) {
	ctx := t.Context()
	want, err := transition.ProcessSlots(ctx, c.states[slot-1].Copy(), slot)
	require.NoError(t, err)
	got, err := c.db.StateBySlotFromDiffTree(ctx, slot)
	require.NoError(t, err)
	wantRoot, err := want.HashTreeRoot(ctx)
	require.NoError(t, err)
	gotRoot, err := got.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, wantRoot, gotRoot)
}

// boot starts a State the way the node does: pending from the stored archive status, the finalized state from
// StateByRoot, and then Resume.
func (c *handoffChain) boot(t *testing.T) *State {
	ctx := t.Context()
	as, err := c.db.ArchiveStatus(ctx)
	require.NoError(t, err)
	service := New(c.db, doublylinkedtree.New())
	service.SetArchivePending(!as.Complete)
	cp, err := c.db.FinalizedCheckpoint(ctx)
	require.NoError(t, err)
	fState, err := service.StateByRoot(ctx, [32]byte(cp.Root))
	require.NoError(t, err)
	_, err = service.Resume(ctx, fState)
	require.NoError(t, err)
	return service
}

// markComplete stores the completed status, as the archive service does at the handoff.
func (c *handoffChain) markComplete(ctx context.Context) error {
	as, err := c.db.ArchiveStatus(ctx)
	if err != nil {
		return err
	}
	as.Complete = true
	return c.db.SaveArchiveStatus(ctx, as)
}
