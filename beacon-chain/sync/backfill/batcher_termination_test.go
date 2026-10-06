package backfill

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// The service ends backfill when every sequencer slot is batchEndSequence. This must hold after
// the final batches import, whatever order the workers return them in.
//
// When the floor batch B downloads before the batch A above it, nothing imports until A returns.
// Then importable() returns [A, B] and one importBatches pass imports both. sequence() returns at
// most one batchEndSequence batch per call, so a completion check that counts the batches the
// pool receives sees only one of the two and waits forever. The control case shows why the order
// matters: when the final batches import in separate passes, sequence() runs between them.
func TestSequenceEndsWhenFinalBatchesDrainInOnePass(t *testing.T) {
	const (
		floor  = 4037888 // needs.Block.Begin
		size   = 32
		seqLen = 2 // --backfill-worker-count default
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

	// endSequences mirrors scheduleTodos and counts the batchEndSequence batches that sequence() hands the pool.
	endSequences := func(t *testing.T, seq *batchSequencer) int {
		got, err := seq.sequence()
		require.NoError(t, err)
		n := 0
		for _, g := range got {
			require.Equal(t, batchEndSequence, g.state)
			n++
		}
		return n
	}

	t.Run("both final downloads land before the upper one imports", func(t *testing.T) {
		seq, a, b := start(t)
		// The pool returns B first, then A. Nothing is importable until A is back.
		seq.update(b.withState(batchImportable))
		require.Equal(t, 0, len(seq.importable()))
		seq.update(a.withState(batchImportable))

		// One importBatches pass now imports both.
		imp := seq.importable()
		require.Equal(t, 2, len(imp))
		for _, ib := range imp {
			seq.update(ib.withState(batchImportComplete))
		}

		// One scheduleTodos follows the pass. It hands the pool only one batchEndSequence batch,
		// so the pool cannot tell that work is done. The sequencer can.
		require.Equal(t, 1, endSequences(t, seq))
		require.Equal(t, true, seq.allEnded())
	})

	t.Run("control: final downloads land in order and drain in separate passes", func(t *testing.T) {
		seq, a, b := start(t)
		seq.update(a.withState(batchImportable))
		imp := seq.importable()
		require.Equal(t, 1, len(imp))
		seq.update(imp[0].withState(batchImportComplete))
		n := endSequences(t, seq) // first end-of-sequence slot reaches the pool here
		require.Equal(t, false, seq.allEnded())

		seq.update(b.withState(batchImportable))
		imp = seq.importable()
		require.Equal(t, 1, len(imp))
		seq.update(imp[0].withState(batchImportComplete))
		n += endSequences(t, seq) // second one here

		require.Equal(t, seqLen, n)
		require.Equal(t, true, seq.allEnded())
	})
}
