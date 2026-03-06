package evaluators

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/pkg/errors"
)

// FinalizationOccurs is an evaluator to make sure finalization is performing as it should.
// Requires to be run after at least 4 epochs have passed.
var FinalizationOccurs = func(epoch primitives.Epoch) types.Evaluator {
	return types.Evaluator{
		Name:       "finalizes_at_epoch_%d",
		Policy:     policies.AfterNthEpoch(epoch),
		Evaluation: finalizationOccurs,
	}
}

func finalizationOccurs(_ *types.EvaluationContext, conns ...*types.NodeConnection) error {
	chainHead, err := getChainHead(conns[0])
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}

	currentEpoch, err := chainHeadEpoch(chainHead)
	if err != nil {
		return err
	}
	finalizedEpoch, err := chainHeadFinalizedEpoch(chainHead)
	if err != nil {
		return err
	}

	expectedFinalizedEpoch := currentEpoch - 2
	if expectedFinalizedEpoch != finalizedEpoch {
		return fmt.Errorf(
			"expected finalized epoch to be %d, received: %d",
			expectedFinalizedEpoch,
			finalizedEpoch,
		)
	}

	previousJustifiedRaw, err := strconv.ParseUint(chainHead.PreviousJustifiedEpoch, 10, 64)
	if err != nil {
		return errors.Wrap(err, "failed to parse previous justified epoch")
	}
	currentJustifiedRaw, err := strconv.ParseUint(chainHead.JustifiedEpoch, 10, 64)
	if err != nil {
		return errors.Wrap(err, "failed to parse justified epoch")
	}
	previousJustifiedEpoch := primitives.Epoch(previousJustifiedRaw)
	currentJustifiedEpoch := primitives.Epoch(currentJustifiedRaw)

	if previousJustifiedEpoch+1 != currentJustifiedEpoch {
		return fmt.Errorf(
			"there should be no gaps between current and previous justified epochs, received current %d and previous %d",
			currentJustifiedEpoch,
			previousJustifiedEpoch,
		)
	}
	if currentJustifiedEpoch+1 != currentEpoch {
		return fmt.Errorf(
			"there should be no gaps between current epoch and current justified epoch, received current %d and justified %d",
			currentEpoch,
			currentJustifiedEpoch,
		)
	}
	return nil
}
