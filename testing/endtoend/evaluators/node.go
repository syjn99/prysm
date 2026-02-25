// Package evaluators defines functions which can peer into end to end
// tests to determine if a chain is running as required.
package evaluators

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

// Allow a very short delay after disconnecting to prevent connection refused issues.
var connTimeDelay = 50 * time.Millisecond

// PeersConnect checks all beacon nodes and returns whether they are connected to each other as peers.
var PeersConnect = e2etypes.Evaluator{
	Name:       "peers_connect_epoch_%d",
	Policy:     policies.OnEpoch(0),
	Evaluation: peersConnect,
}

// HealthzCheck pings healthz and errors if it doesn't have the expected OK status.
var HealthzCheck = e2etypes.Evaluator{
	Name:       "healthz_check_epoch_%d",
	Policy:     policies.AfterNthEpoch(0),
	Evaluation: healthzCheck,
}

// FinishedSyncing returns whether the beacon node with the given rpc port has finished syncing.
var FinishedSyncing = e2etypes.Evaluator{
	Name:       "finished_syncing_%d",
	Policy:     policies.AllEpochs,
	Evaluation: finishedSyncing,
}

// AllNodesHaveSameHead ensures all nodes have the same head epoch. Checks finality and justification as well.
// Not checking head block root as it may change irregularly for the validator connected nodes.
var AllNodesHaveSameHead = e2etypes.Evaluator{
	Name:       "all_nodes_have_same_head_%d",
	Policy:     policies.AllEpochs,
	Evaluation: allNodesHaveSameHead,
}

func healthzCheck(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	count := len(conns)
	for i := range count {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/healthz", e2e.TestParams.Ports.PrysmBeaconNodeMetricsPort+i))
		if err != nil {
			// Continue if the connection fails, regular flake.
			continue
		}
		if resp.StatusCode != http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return fmt.Errorf("expected status code OK for beacon node %d, received %v with body %s", i, resp.StatusCode, body)
		}
		if err = resp.Body.Close(); err != nil {
			return err
		}
		time.Sleep(connTimeDelay)
	}

	for i := range count {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/healthz", e2e.TestParams.Ports.ValidatorMetricsPort+i))
		if err != nil {
			// Continue if the connection fails, regular flake.
			continue
		}
		if resp.StatusCode != http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return fmt.Errorf("expected status code OK for validator client %d, received %v with body %s", i, resp.StatusCode, body)
		}
		if err = resp.Body.Close(); err != nil {
			return err
		}
		time.Sleep(connTimeDelay)
	}
	return nil
}

func peersConnect(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	if len(conns) == 1 {
		return nil
	}
	for _, conn := range conns {
		peersResp, err := getNodePeers(conn)
		if err != nil {
			return err
		}
		expectedPeers := len(conns) - 1 + e2e.TestParams.LighthouseBeaconNodeCount
		if expectedPeers != len(peersResp.Data) {
			return fmt.Errorf("unexpected amount of peers, expected %d, received %d", expectedPeers, len(peersResp.Data))
		}
		time.Sleep(connTimeDelay)
	}
	return nil
}

func finishedSyncing(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]
	syncStatus, err := getSyncStatus(conn)
	if err != nil {
		return err
	}
	if syncStatus.IsSyncing {
		return errors.New("expected node to have completed sync")
	}
	return nil
}

// waitForMidEpoch waits until we're at least halfway into the current epoch
// and 3/4 into the current slot. This prevents race conditions at epoch
// boundaries and slot boundaries where different nodes may report different heads.
func waitForMidEpoch(ctx context.Context, conn *e2etypes.NodeConnection) error {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	secondsPerSlot := params.BeaconConfig().SecondsPerSlot
	midEpochSlot := slotsPerEpoch / 2

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		chainHead, err := getChainHead(conn)
		if err != nil {
			return err
		}
		headSlot, err := chainHeadSlot(chainHead)
		if err != nil {
			return err
		}
		slotInEpoch := headSlot % primitives.Slot(slotsPerEpoch)
		// If we're at least halfway into the epoch, we're safe
		if slotInEpoch >= primitives.Slot(midEpochSlot) {
			// Wait 3/4 into the slot to ensure block propagation
			if err := sleepWithContext(ctx, time.Duration(secondsPerSlot)*time.Second*3/4); err != nil {
				return err
			}
			return nil
		}
		// Wait for the remaining slots until mid-epoch
		slotsToWait := primitives.Slot(midEpochSlot) - slotInEpoch
		if err := sleepWithContext(ctx, time.Duration(slotsToWait)*time.Duration(secondsPerSlot)*time.Second); err != nil {
			return err
		}
	}
}

func allNodesHaveSameHead(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	ctx, cancel := context.WithTimeout(context.Background(), params.EpochsDuration(2, params.BeaconConfig()))
	defer cancel()
	// Wait until we're at least halfway into the epoch to avoid race conditions
	// at epoch boundaries where nodes may report different epochs.
	if err := waitForAllMidEpoch(ctx, conns...); err != nil {
		return errors.Wrap(err, "failed waiting for mid-epoch")
	}

	headEpochs := make([]primitives.Epoch, len(conns))
	headBlockRoots := make([]string, len(conns))
	justifiedRoots := make([]string, len(conns))
	prevJustifiedRoots := make([]string, len(conns))
	finalizedRoots := make([]string, len(conns))
	chainHeadStrs := make([]string, len(conns))
	g, _ := errgroup.WithContext(context.Background())

	for i, conn := range conns {
		conIdx := i
		currConn := conn
		g.Go(func() error {
			chainHead, err := getChainHead(currConn)
			if err != nil {
				return errors.Wrapf(err, "connection number=%d", conIdx)
			}
			epoch, err := chainHeadEpoch(chainHead)
			if err != nil {
				return errors.Wrapf(err, "connection number=%d: parse head epoch", conIdx)
			}
			headEpochs[conIdx] = epoch
			headBlockRoots[conIdx] = chainHead.HeadBlockRoot
			justifiedRoots[conIdx] = chainHead.JustifiedBlockRoot
			prevJustifiedRoots[conIdx] = chainHead.PreviousJustifiedBlockRoot
			finalizedRoots[conIdx] = chainHead.FinalizedBlockRoot
			chainHeadStrs[conIdx] = fmt.Sprintf(
				"head_slot=%s head_epoch=%s head_root=%s finalized_epoch=%s finalized_root=%s justified_epoch=%s justified_root=%s",
				chainHead.HeadSlot,
				chainHead.HeadEpoch,
				chainHead.HeadBlockRoot,
				chainHead.FinalizedEpoch,
				chainHead.FinalizedBlockRoot,
				chainHead.JustifiedEpoch,
				chainHead.JustifiedBlockRoot,
			)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	for i := range conns {
		if headEpochs[0] != headEpochs[i] {
			return fmt.Errorf(
				"received conflicting head epochs on node %d, expected %d, received %d",
				i,
				headEpochs[0],
				headEpochs[i],
			)
		}
		if headBlockRoots[0] != headBlockRoots[i] {
			return fmt.Errorf(
				"received conflicting head block roots on node %d, expected %s, received %s",
				i,
				headBlockRoots[0],
				headBlockRoots[i],
			)
		}
		if justifiedRoots[0] != justifiedRoots[i] {
			return fmt.Errorf(
				"received conflicting justified block roots on node %d, expected %s, received %s: %s and %s",
				i,
				justifiedRoots[0],
				justifiedRoots[i],
				chainHeadStrs[0],
				chainHeadStrs[i],
			)
		}
		if prevJustifiedRoots[0] != prevJustifiedRoots[i] {
			return fmt.Errorf(
				"received conflicting previous justified block roots on node %d, expected %s, received %s",
				i,
				prevJustifiedRoots[0],
				prevJustifiedRoots[i],
			)
		}
		if finalizedRoots[0] != finalizedRoots[i] {
			return fmt.Errorf(
				"received conflicting finalized epoch roots on node %d, expected %s, received %s",
				i,
				finalizedRoots[0],
				finalizedRoots[i],
			)
		}
	}

	return nil
}

func waitForAllMidEpoch(ctx context.Context, conns ...*e2etypes.NodeConnection) error {
	g, gctx := errgroup.WithContext(ctx)
	for _, conn := range conns {
		currConn := conn
		g.Go(func() error {
			return waitForMidEpoch(gctx, currConn)
		})
	}
	return g.Wait()
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
