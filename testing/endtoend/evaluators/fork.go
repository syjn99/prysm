package evaluators

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

var streamDeadline = 1 * time.Minute

// AltairForkTransition ensures that the Altair hard fork has occurred successfully.
var AltairForkTransition = e2etypes.Evaluator{
	Name: "altair_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Altair
		if e2etypes.GenesisFork() >= version.Altair {
			return false
		}
		altair := policies.OnEpoch(params.BeaconConfig().AltairForkEpoch)
		return altair(e)
	},
	Evaluation: altairForkOccurs,
}

// BellatrixForkTransition ensures that the Bellatrix hard fork has occurred successfully.
var BellatrixForkTransition = e2etypes.Evaluator{
	Name: "bellatrix_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Bellatrix
		if e2etypes.GenesisFork() >= version.Bellatrix {
			return false
		}
		fEpoch := params.BeaconConfig().BellatrixForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: bellatrixForkOccurs,
}

// CapellaForkTransition ensures that the Capella hard fork has occurred successfully.
var CapellaForkTransition = e2etypes.Evaluator{
	Name: "capella_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Capella
		if e2etypes.GenesisFork() >= version.Capella {
			return false
		}
		fEpoch := params.BeaconConfig().CapellaForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: capellaForkOccurs,
}

// DenebForkTransition ensures that the Deneb hard fork has occurred successfully
var DenebForkTransition = e2etypes.Evaluator{
	Name: "deneb_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Deneb
		if e2etypes.GenesisFork() >= version.Deneb {
			return false
		}
		fEpoch := params.BeaconConfig().DenebForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: denebForkOccurs,
}

// ElectraForkTransition ensures that the electra hard fork has occurred successfully
var ElectraForkTransition = e2etypes.Evaluator{
	Name: "electra_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Electra
		if e2etypes.GenesisFork() >= version.Electra {
			return false
		}
		fEpoch := params.BeaconConfig().ElectraForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: electraForkOccurs,
}

// FuluForkTransition ensures that the fulu hard fork has occurred successfully
var FuluForkTransition = e2etypes.Evaluator{
	Name: "fulu_fork_transition_%d",
	Policy: func(e primitives.Epoch) bool {
		// Only run if we started before Fulu
		if e2etypes.GenesisFork() >= version.Fulu {
			return false
		}
		fEpoch := params.BeaconConfig().FuluForkEpoch
		return policies.OnEpoch(fEpoch)(e)
	},
	Evaluation: fuluForkOccurs,
}

func altairForkOccurs(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]
	ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()

	fSlot, err := slots.EpochStart(params.BeaconConfig().AltairForkEpoch)
	if err != nil {
		return err
	}

	blk, err := pollForBlock(ctx, conn, fSlot)
	if err != nil {
		return errors.Wrap(err, "failed waiting for altair block")
	}
	if blk.Version() < version.Altair {
		return errors.Errorf("expected altair or later block, got version %s", version.String(blk.Version()))
	}
	if blk.Block().Slot() < fSlot {
		return errors.Errorf("wanted a block >= %d but received %d", fSlot, blk.Block().Slot())
	}
	return nil
}

func bellatrixForkOccurs(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]
	ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()

	fSlot, err := slots.EpochStart(params.BeaconConfig().BellatrixForkEpoch)
	if err != nil {
		return err
	}

	blk, err := pollForBlock(ctx, conn, fSlot)
	if err != nil {
		return errors.Wrap(err, "failed waiting for bellatrix block")
	}
	if blk.Version() < version.Bellatrix {
		return errors.Errorf("expected bellatrix or later block, got version %s", version.String(blk.Version()))
	}
	if blk.Block().Slot() < fSlot {
		return errors.Errorf("wanted a block >= %d but received %d", fSlot, blk.Block().Slot())
	}
	return nil
}

func capellaForkOccurs(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]
	ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()

	fSlot, err := slots.EpochStart(params.BeaconConfig().CapellaForkEpoch)
	if err != nil {
		return err
	}

	blk, err := pollForBlock(ctx, conn, fSlot)
	if err != nil {
		return errors.Wrap(err, "failed waiting for capella block")
	}
	if blk.Version() < version.Capella {
		return errors.Errorf("expected capella or later block, got version %s", version.String(blk.Version()))
	}
	if blk.Block().Slot() < fSlot {
		return errors.Errorf("wanted a block at slot >= %d but received %d", fSlot, blk.Block().Slot())
	}
	return nil
}

func denebForkOccurs(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]
	ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()

	fSlot, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
	if err != nil {
		return err
	}

	blk, err := pollForBlock(ctx, conn, fSlot)
	if err != nil {
		return errors.Wrap(err, "failed waiting for deneb block")
	}
	if blk.Version() < version.Deneb {
		return errors.Errorf("expected deneb or later block, got version %s", version.String(blk.Version()))
	}
	if blk.Block().Slot() < fSlot {
		return errors.Errorf("wanted a block at slot >= %d but received %d", fSlot, blk.Block().Slot())
	}
	return nil
}

func electraForkOccurs(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]
	ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()

	fSlot, err := slots.EpochStart(params.BeaconConfig().ElectraForkEpoch)
	if err != nil {
		return err
	}

	blk, err := pollForBlock(ctx, conn, fSlot)
	if err != nil {
		return errors.Wrap(err, "failed waiting for electra block")
	}
	if blk.Version() < version.Electra {
		return errors.Errorf("expected electra or later block, got version %s", version.String(blk.Version()))
	}
	if blk.Block().Slot() < fSlot {
		return errors.Errorf("wanted a block at slot >= %d but received %d", fSlot, blk.Block().Slot())
	}
	return nil
}

func fuluForkOccurs(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]
	ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()

	fSlot, err := slots.EpochStart(params.BeaconConfig().FuluForkEpoch)
	if err != nil {
		return err
	}

	blk, err := pollForBlock(ctx, conn, fSlot)
	if err != nil {
		return errors.Wrap(err, "failed waiting for fulu block")
	}
	if blk.Version() < version.Fulu {
		return errors.Errorf("expected fulu or later block, got version %s", version.String(blk.Version()))
	}
	if blk.Block().Slot() < fSlot {
		return errors.Errorf("wanted a block at slot >= %d but received %d", fSlot, blk.Block().Slot())
	}
	return nil
}
