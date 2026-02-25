package evaluators

import (
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/pkg/errors"
)

// PeersCheck performs a check on peer data to ensure that any connected peers
// are not publishing invalid data.
//
// NOTE: The standard REST endpoint GET /eth/v1/node/peers does not expose peer
// scoring data (GossipScore, BehaviourPenalty, BlockProviderScore, OverallScore,
// ValidationError, or FaultCount). A Prysm-specific debug REST endpoint would be
// required to restore those checks. For now this evaluator verifies that the node
// has at least one peer in the "connected" state.
//
// TODO: Restore full peer scoring checks once a Prysm debug REST endpoint
// (e.g. /prysm/v1/node/peers) exposing scoring data is available.
var PeersCheck = types.Evaluator{
	Name:       "peers_check_epoch_%d",
	Policy:     policies.AfterNthEpoch(0),
	Evaluation: peersTest,
}

func peersTest(_ *types.EvaluationContext, conns ...*types.NodeConnection) error {
	peerResponse, err := getNodePeers(conns[0])
	if err != nil {
		return err
	}

	connectedCount := 0
	for _, p := range peerResponse.Data {
		if p.State == "connected" {
			connectedCount++
		}
	}

	if connectedCount == 0 {
		return errors.New("node has no connected peers")
	}

	return nil
}

