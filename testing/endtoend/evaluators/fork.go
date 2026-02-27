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
	Evaluation: forkOccurs("altair", params.BeaconConfig().AltairForkEpoch, version.Altair),
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
	Evaluation: forkOccurs("bellatrix", params.BeaconConfig().BellatrixForkEpoch, version.Bellatrix),
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
	Evaluation: forkOccurs("capella", params.BeaconConfig().CapellaForkEpoch, version.Capella),
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
	Evaluation: forkOccurs("deneb", params.BeaconConfig().DenebForkEpoch, version.Deneb),
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
	Evaluation: forkOccurs("electra", params.BeaconConfig().ElectraForkEpoch, version.Electra),
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
	Evaluation: forkOccurs("fulu", params.BeaconConfig().FuluForkEpoch, version.Fulu),
}

func forkOccurs(name string, forkEpoch primitives.Epoch, v int) func(*e2etypes.EvaluationContext, ...*e2etypes.NodeConnection) error {
	return func(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
		conn := conns[0]
		ctx, cancel := context.WithTimeout(context.Background(), streamDeadline)
		defer cancel()

		fSlot, err := slots.EpochStart(forkEpoch)
		if err != nil {
			return err
		}

		blk, err := pollForBlock(ctx, conn, fSlot)
		if err != nil {
			return errors.Wrapf(err, "failed waiting for %s block", name)
		}
		if blk.Version() < v {
			return errors.Errorf("expected %s or later block, got version %s", name, version.String(blk.Version()))
		}
		if blk.Block().Slot() < fSlot {
			return errors.Errorf("wanted a block at slot >= %d but received %d", fSlot, blk.Block().Slot())
		}
		return nil
	}
}
