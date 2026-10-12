package node

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/genesis"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestValidateArchiveOrigin(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())
	require.NoError(t, genesis.Initialize(t.Context(), t.TempDir()))
	gvr := genesis.ValidatorsRoot()

	t.Run("epoch boundary is accepted", func(t *testing.T) {
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetGenesisValidatorsRoot(gvr[:]))
		require.NoError(t, st.SetSlot(8192))
		require.NoError(t, validateArchiveOrigin(st))
	})

	t.Run("mid-epoch slot is rejected", func(t *testing.T) {
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetGenesisValidatorsRoot(gvr[:]))
		require.NoError(t, st.SetSlot(8193))
		require.ErrorContains(t, "must be an epoch boundary", validateArchiveOrigin(st))
	})

	t.Run("wrong network is rejected", func(t *testing.T) {
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetGenesisValidatorsRoot(make([]byte, 32)))
		require.NoError(t, st.SetSlot(8192))
		require.ErrorContains(t, "different network", validateArchiveOrigin(st))
	})

	t.Run("nil is rejected", func(t *testing.T) {
		require.ErrorContains(t, "nil", validateArchiveOrigin(nil))
	})
}

// Anchoring is irreversible, so an origin the node cannot backfill down to must be rejected before any write.
func TestFinalizeArchiveOrigin_RejectedOriginLeavesDBReAnchorable(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	flags.Init(&flags.GlobalFlags{StateDiffExponents: []int{21, 18, 16, 13, 11, 9, 5}})

	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	store := beaconDB.(*kv.Store)

	// A checkpoint-synced node whose sync origin is one epoch in.
	syncOrigin := uint64(params.BeaconConfig().SlotsPerEpoch)
	require.NoError(t, store.SaveBackfillStatus(ctx, &dbval.BackfillStatus{
		OriginSlot: syncOrigin,
		LowSlot:    syncOrigin,
	}))

	tooHigh, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, tooHigh.SetSlot(primitives.Slot(syncOrigin)*4))
	tooHighSlot := tooHigh.Slot()

	b := &BeaconNode{ctx: ctx, db: beaconDB, ArchiveOriginSlot: &tooHighSlot}
	require.ErrorContains(t, "is above the sync origin slot", b.finalizeArchiveOrigin(ctx, tooHigh))

	// Nothing was written, so the corrected origin anchors cleanly rather than hitting
	// "archive origin state changed" / "already anchored".
	_, err = store.ArchiveStatus(ctx)
	require.ErrorIs(t, err, kv.ErrNotFound)

	good, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, good.SetSlot(primitives.Slot(syncOrigin)))
	goodSlot := good.Slot()

	b = &BeaconNode{ctx: ctx, db: beaconDB, ArchiveOriginSlot: &goodSlot}
	require.NoError(t, b.finalizeArchiveOrigin(ctx, good))

	as, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, goodSlot, as.OriginSlot)
	require.Equal(t, true, b.archiveRegenPending)
}

type originBytesInitializer struct {
	state, block []byte
}

func (i *originBytesInitializer) Initialize(ctx context.Context, d db.Database) error {
	return d.SaveOrigin(ctx, i.state, i.block)
}

// Genesis and checkpoint data are written before the archive origin anchors the state-diff tree. A rejected
// archive origin must not leave a database that later starts treat as one without state-diff support.
func TestStartDB_RejectedArchiveOriginLeavesDataDirUsable(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()
	flags.Init(&flags.GlobalFlags{StateDiffExponents: []int{21, 18, 16, 13, 11, 9, 5}})

	ctx := t.Context()
	syncOrigin := params.BeaconConfig().SlotsPerEpoch
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(syncOrigin))
	blk := util.NewBeaconBlock()
	blk.Block.Slot = syncOrigin
	blk.Block.StateRoot = bytesutil.PadTo([]byte("origin-state-root"), 32)
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	bodyRoot, err := wsb.Block().Body().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, st.SetLatestBlockHeader(&ethpb.BeaconBlockHeader{
		Slot:       blk.Block.Slot,
		ParentRoot: blk.Block.ParentRoot,
		StateRoot:  blk.Block.StateRoot,
		BodyRoot:   bodyRoot[:],
	}))
	stBytes, err := st.MarshalSSZ()
	require.NoError(t, err)
	blkBytes, err := wsb.MarshalSSZ()
	require.NoError(t, err)
	initializer := &originBytesInitializer{state: stBytes, block: blkBytes}

	// start runs the database steps of startBaseServices.
	start := func(d db.Database, archiveSlot primitives.Slot) error {
		archiveState, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, archiveState.SetSlot(archiveSlot))
		b := &BeaconNode{ctx: ctx, db: d, ArchiveOriginSlot: &archiveSlot, CheckpointInitializer: initializer}
		if err := b.startDB(nil, params.BeaconConfig().DepositContractAddress); err != nil {
			return err
		}
		return b.finalizeArchiveOrigin(ctx, archiveState)
	}

	dir := t.TempDir()
	store, err := kv.NewKVStore(context.Background(), dir)
	require.NoError(t, err)
	require.ErrorContains(t, "is above the sync origin slot", start(store, syncOrigin*4))
	require.NoError(t, store.Close())

	store, err = kv.NewKVStore(context.Background(), dir)
	// NewKVStore returns an open store with ErrStateDiffIncompatible, so close it before the check.
	t.Cleanup(func() {
		if store != nil {
			require.NoError(t, store.Close())
		}
	})
	require.NoError(t, err)
	require.NoError(t, start(store, syncOrigin))
	as, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, syncOrigin, as.OriginSlot)
}

// The frontier guard blocks cold-state migration until the archive walk completes, and only archive mode runs
// the walk.
func TestInitArchiveOrigin_PendingArchiveRequiresArchiveMode(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	flags.Init(&flags.GlobalFlags{StateDiffExponents: []int{21, 18, 16, 13, 11, 9, 5}})

	setup := func(t *testing.T, complete bool) *BeaconNode {
		resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
		t.Cleanup(resetCfg)
		ctx := t.Context()
		beaconDB := testDB.SetupDB(t)
		store := beaconDB.(*kv.Store)
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, store.InitializeArchiveOrigin(ctx, st))
		if complete {
			as, err := store.ArchiveStatus(ctx)
			require.NoError(t, err)
			as.Complete = true
			require.NoError(t, store.SaveArchiveStatus(ctx, as))
		}
		features.Init(&features.Flags{EnableStateDiff: true})
		return &BeaconNode{ctx: ctx, db: beaconDB}
	}

	t.Run("pending archive is rejected", func(t *testing.T) {
		b := setup(t, false)
		_, err := b.initArchiveOrigin(nil)
		require.ErrorContains(t, "--enable-archive", err)
	})

	t.Run("complete archive is accepted", func(t *testing.T) {
		b := setup(t, true)
		_, err := b.initArchiveOrigin(nil)
		require.NoError(t, err)
	})

	t.Run("non-archive database is accepted", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
		defer resetCfg()
		b := &BeaconNode{ctx: t.Context(), db: testDB.SetupDB(t)}
		_, err := b.initArchiveOrigin(nil)
		require.NoError(t, err)
	})
}
