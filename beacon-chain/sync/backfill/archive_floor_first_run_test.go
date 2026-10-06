package backfill

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// Reproduces the first-run deadlock at the archive floor observed on Hoodi on 2026-10-06
// (origin 4037888, --backfill-worker-count default 2).
//
// The pool returned the two final downloads out of order: B (the floor batch) before A. When A
// arrived, importable() returned [A, B] and a single importBatches pass imported both. update(B)
// converted the pending end-of-sequence batch in place and created a new one, so the one
// sequence() call that followed had two end-of-sequence slots but, because of the len(s)==0
// rule, handed the pool only one of them. p2pBatchWorkerPool.complete() waited for maxBatches of
// them and blocked forever. The service now ends backfill from the sequencer's own state instead.
// The control case shows the ordering under which the old pool count happened to work.
func TestSequenceEndsWhenFinalBatchesDrainInOnePass(t *testing.T) {
	const (
		floor  = 4037888 // archive origin == needs.Block.Begin
		size   = 32
		seqLen = 2 // --backfill-worker-count default, also pool.maxBatches
		end    = 4088480
	)

	start := func(t *testing.T) (*batchSequencer, batch, batch) {
		// Two batches of work remain above the floor: A = [floor+32, floor+64), B = [floor, floor+32).
		seq := newBatchSequencer(seqLen, floor+2*size, size, mockCurrentNeedsFunc(floor, end))
		got, err := seq.sequence()
		require.NoError(t, err)
		require.Equal(t, seqLen, len(got))
		a, b := got[0], got[1]
		require.Equal(t, primitives.Slot(floor+size), a.begin)
		require.Equal(t, primitives.Slot(floor), b.begin)
		return seq, a, b
	}

	// endSequences mirrors scheduleTodos (one sequence() call) and then reports how many sequencer
	// slots are end-of-sequence. The service ends backfill once every slot is, which holds regardless
	// of how many of those batches sequence() handed to the pool.
	endSequences := func(t *testing.T, seq *batchSequencer) int {
		_, err := seq.sequence()
		require.NoError(t, err)
		return seq.countWithState(batchEndSequence)
	}

	t.Run("both final downloads land before the upper one imports", func(t *testing.T) {
		seq, a, b := start(t)
		// Pool hands back B first, then A. Nothing is importable until A is back.
		seq.update(b.withState(batchImportable))
		require.Equal(t, 0, len(seq.importable()))
		seq.update(a.withState(batchImportable))

		// One importBatches pass now imports both.
		imp := seq.importable()
		require.Equal(t, 2, len(imp))
		for _, ib := range imp {
			seq.update(ib.withState(batchImportComplete))
		}

		// One scheduleTodos follows the pass. Both slots are end-of-sequence, so the service must
		// treat backfill as complete here; the pool saw only one of them.
		require.Equal(t, seqLen, endSequences(t, seq))
	})

	t.Run("control: final downloads land in order and drain in separate passes", func(t *testing.T) {
		seq, a, b := start(t)
		seq.update(a.withState(batchImportable))
		imp := seq.importable()
		require.Equal(t, 1, len(imp))
		seq.update(imp[0].withState(batchImportComplete))
		require.Equal(t, 1, endSequences(t, seq)) // one slot done, B still in flight

		seq.update(b.withState(batchImportable))
		imp = seq.importable()
		require.Equal(t, 1, len(imp))
		seq.update(imp[0].withState(batchImportComplete))
		require.Equal(t, seqLen, endSequences(t, seq))
	})
}
