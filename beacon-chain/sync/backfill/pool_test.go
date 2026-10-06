package backfill

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
)

type mockAssigner struct {
	err    error
	assign []peer.ID
}

// Assign satisfies the PeerAssigner interface so that mockAssigner can be used in tests
// in place of the concrete p2p implementation of PeerAssigner.
func (m mockAssigner) Assign(filter peers.AssignmentFilter) ([]peer.ID, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.assign, nil
}

var _ PeerAssigner = &mockAssigner{}

func mockNewBlobVerifier(_ blocks.ROBlob, _ []verification.Requirement) verification.BlobVerifier {
	return &verification.MockBlobVerifier{}
}

// TestPoolReturnsExpiredBatch checks that a batch the router finds expired comes back through complete()
// as batchEndSequence, so the sequencer can mark its slot ended. If the router dropped it, the service
// would wait in complete() forever when that batch was the last one in flight.
func TestPoolReturnsExpiredBatch(t *testing.T) {
	nw := 2
	p2p := p2ptest.NewTestP2P(t)
	needs := func() das.CurrentNeeds { return das.CurrentNeeds{Block: das.NeedSpan{Begin: 100, End: 200}} }
	pool := newP2PBatchWorkerPool(p2p, nw, needs)
	ma := &mockAssigner{assign: []peer.ID{"peer"}}
	pool.spawn(t.Context(), nw, ma, &workerCfg{})
	expired := batch{begin: 10, end: 20, state: batchSequenced}
	pool.todo(expired)
	b, err := pool.complete()
	require.NoError(t, err)
	require.Equal(t, batchEndSequence, b.state)
	require.Equal(t, expired.begin, b.begin)
	require.Equal(t, expired.end, b.end)
}

type mockPool struct {
	spawnCalled  []int
	finishedChan chan batch
	finishedErr  chan error
	todoChan     chan batch
}

func (m *mockPool) spawn(_ context.Context, _ int, _ PeerAssigner, _ *workerCfg) {
}

func (m *mockPool) todo(b batch) {
	m.todoChan <- b
}

func (m *mockPool) complete() (batch, error) {
	select {
	case b := <-m.finishedChan:
		return b, nil
	case err := <-m.finishedErr:
		return batch{}, err
	}
}

var _ batchWorkerPool = &mockPool{}

// TestTodoInterceptsBatchEndSequence tests that todo() drops batchEndSequence batches instead of routing them to workers
func TestTodoInterceptsBatchEndSequence(t *testing.T) {
	testCases := []struct {
		name             string
		batches          []batch
		expectedToRouter int
	}{
		{
			name: "AllRegularBatches",
			batches: []batch{
				{state: batchInit},
				{state: batchInit},
				{state: batchErrRetryable},
			},
			expectedToRouter: 3,
		},
		{
			name: "MixedBatches",
			batches: []batch{
				{state: batchInit},
				{state: batchEndSequence},
				{state: batchInit},
				{state: batchEndSequence},
			},
			expectedToRouter: 2,
		},
		{
			name: "AllEndSequence",
			batches: []batch{
				{state: batchEndSequence},
				{state: batchEndSequence},
				{state: batchEndSequence},
			},
			expectedToRouter: 0,
		},
		{
			name:             "EmptyBatches",
			batches:          []batch{},
			expectedToRouter: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool := &p2pBatchWorkerPool{toRouter: make(chan batch, len(tc.batches))}
			for _, b := range tc.batches {
				pool.todo(b)
			}
			require.Equal(t, tc.expectedToRouter, len(pool.toRouter))
		})
	}
}

// TestExpirationFlowEndToEnd tests that sequence() stops returning batches below a raised minimum
func TestExpirationFlowEndToEnd(t *testing.T) {
	testCases := []struct {
		name        string
		seqLen      int
		min         primitives.Slot
		max         primitives.Slot
		size        primitives.Slot
		moveMinTo   primitives.Slot
		expired     int
		description string
	}{
		{
			name:        "SingleBatchExpires",
			seqLen:      2,
			min:         100,
			max:         300,
			size:        50,
			moveMinTo:   150,
			expired:     1,
			description: "Initial [150-200] and [100-150]; moveMinimum(150) expires [100-150]",
		},
		/*
			{
				name:        "ProgressiveExpiration",
				seqLen:      4,
				min:         100,
				max:         500,
				size:        50,
				moveMinTo:   250,
				description: "4 batches; moveMinimum(250) expires 2 of them",
			},
		*/
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the flow: batcher creates batches → sequence() → pool.todo() → pool.processTodo()

			// Step 1: Create sequencer (simulating batcher)
			seq := newBatchSequencer(tc.seqLen, tc.max, tc.size, mockCurrentNeedsFunc(tc.min, tc.max+1))
			initializeBatchWithSlots(seq.seq, tc.min, tc.size)
			for i := range seq.seq {
				seq.seq[i].state = batchInit
			}

			// Step 3: Initial sequence() call - all batches should be returned (none expired yet)
			batches1, err := seq.sequence()
			if err != nil {
				t.Fatalf("initial sequence() failed: %v", err)
			}
			if len(batches1) != tc.seqLen {
				t.Fatalf("expected %d batches from initial sequence(), got %d", tc.seqLen, len(batches1))
			}

			// Step 4: Move minimum (simulating epoch advancement)
			seq.currentNeeds = mockCurrentNeedsFunc(tc.moveMinTo, tc.max+1)
			seq.batcher.currentNeeds = seq.currentNeeds

			for i := range batches1 {
				seq.update(batches1[i])
			}

			// Step 5: Process batches through pool (second sequence call would happen here in real code)
			batches2, err := seq.sequence()
			if err != nil && err != errMaxBatches {
				t.Fatalf("second sequence() failed: %v", err)
			}
			require.Equal(t, tc.seqLen-tc.expired, len(batches2))

			// Verify: no returned work batch is below the new minimum.
			for _, b := range batches2 {
				if b.state != batchEndSequence && b.end <= tc.moveMinTo {
					t.Fatalf("batch [%d-%d] should not be returned when min=%d", b.begin, b.end, tc.moveMinTo)
				}
			}
		})
	}
}
