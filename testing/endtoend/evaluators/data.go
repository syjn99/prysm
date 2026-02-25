package evaluators

import (
	"errors"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

const epochToCheck = 50 // must be more than 46 (32 hot states + 16 chkpt interval)

// ColdStateCheckpoint checks data from the database using cold state storage.
var ColdStateCheckpoint = e2etypes.Evaluator{
	Name: "cold_state_assignments_from_epoch_%d",
	Policy: func(currentEpoch primitives.Epoch) bool {
		return currentEpoch == epochToCheck
	},
	Evaluation: checkColdStateCheckpoint,
}

// Checks the first node for an old checkpoint using cold state storage.
// The attester duties endpoint (POST /eth/v1/validator/duties/attester/{epoch}) exercises
// historical state retrieval, confirming cold-state storage is functional for past epochs.
func checkColdStateCheckpoint(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]

	for i := range primitives.Epoch(epochToCheck) {
		res, err := getAttesterDuties(conn, i, []string{"0"})
		if err != nil {
			return err
		}
		// A simple check to ensure we received some data.
		if res == nil || len(res.Data) == 0 {
			return errors.New("failed to return a validator assignments response for an old epoch " +
				"using cold state storage from the database")
		}
	}

	return nil
}
