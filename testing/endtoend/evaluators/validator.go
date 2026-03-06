package evaluators

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	e2eparams "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

var expectedParticipation = 0.98

var expectedMulticlientParticipation = 0.95

var expectedSyncParticipation = 0.95

// ValidatorsAreActive ensures the expected amount of validators are active.
var ValidatorsAreActive = types.Evaluator{
	Name:       "validators_active_epoch_%d",
	Policy:     policies.AllEpochs,
	Evaluation: validatorsAreActive,
}

// ValidatorsParticipatingAtEpoch ensures the expected amount of validators are participating.
var ValidatorsParticipatingAtEpoch = func(epoch primitives.Epoch) types.Evaluator {
	return types.Evaluator{
		Name:       "validators_participating_epoch_%d",
		Policy:     policies.AfterNthEpoch(epoch),
		Evaluation: validatorsParticipating,
	}
}

// ValidatorSyncParticipation ensures the expected amount of sync committee participants
// are active.
var ValidatorSyncParticipation = types.Evaluator{
	Name: "validator_sync_participation_%d",
	Policy: func(e primitives.Epoch) bool {
		fEpoch := params.BeaconConfig().AltairForkEpoch
		return policies.OnwardsNthEpoch(fEpoch)(e)
	},
	Evaluation: validatorsSyncParticipation,
}

func validatorsAreActive(ec *types.EvaluationContext, conns ...*types.NodeConnection) error {
	conn := conns[0]

	// Balances actually fluctuate but we just want to check initial balance.
	validators, err := getValidators(conn, "head", []string{"active"})
	if err != nil {
		return errors.Wrap(err, "failed to get validators")
	}

	// Count should be MinGenesisActiveValidatorCount minus any validators that have exited.
	// We determine actual exited count from the difference, as exits may be submitted but
	// not yet processed, or affected by churn limits.
	receivedCount := uint64(len(validators.Data))
	maxExpected := params.BeaconConfig().MinGenesisActiveValidatorCount
	minExpected := maxExpected - uint64(len(ec.ExitedVals))

	if receivedCount > maxExpected {
		return fmt.Errorf("validator count %d exceeds genesis count %d", receivedCount, maxExpected)
	}
	if receivedCount < minExpected {
		return fmt.Errorf("validator count %d is less than expected minimum %d (genesis %d - %d submitted exits)",
			receivedCount, minExpected, maxExpected, len(ec.ExitedVals))
	}

	effBalanceLowCount := 0
	exitEpochWrongCount := 0
	withdrawEpochWrongCount := 0
	for _, item := range validators.Data {
		// Decode the hex pubkey and convert to [48]byte for the ExitedVals lookup.
		pubkeyHex := strings.TrimPrefix(item.Validator.Pubkey, "0x")
		pubkeyBytes, err := hex.DecodeString(pubkeyHex)
		if err != nil {
			return errors.Wrapf(err, "failed to decode pubkey %s", item.Validator.Pubkey)
		}
		if _, exited := ec.ExitedVals[bytesutil.ToBytes48(pubkeyBytes)]; exited {
			continue
		}

		effBalance, err := strconv.ParseUint(item.Validator.EffectiveBalance, 10, 64)
		if err != nil {
			return errors.Wrapf(err, "failed to parse effective balance %s", item.Validator.EffectiveBalance)
		}
		if effBalance < params.BeaconConfig().MaxEffectiveBalance {
			effBalanceLowCount++
		}

		exitEpoch, err := strconv.ParseUint(item.Validator.ExitEpoch, 10, 64)
		if err != nil {
			return errors.Wrapf(err, "failed to parse exit epoch %s", item.Validator.ExitEpoch)
		}
		if primitives.Epoch(exitEpoch) != params.BeaconConfig().FarFutureEpoch {
			exitEpochWrongCount++
		}

		withdrawableEpoch, err := strconv.ParseUint(item.Validator.WithdrawableEpoch, 10, 64)
		if err != nil {
			return errors.Wrapf(err, "failed to parse withdrawable epoch %s", item.Validator.WithdrawableEpoch)
		}
		if primitives.Epoch(withdrawableEpoch) != params.BeaconConfig().FarFutureEpoch {
			withdrawEpochWrongCount++
		}
	}

	if effBalanceLowCount > 0 {
		return fmt.Errorf(
			"%d validators did not have genesis validator effective balance of %d",
			effBalanceLowCount,
			params.BeaconConfig().MaxEffectiveBalance,
		)
	} else if exitEpochWrongCount > 0 {
		return fmt.Errorf("%d validators did not have genesis validator exit epoch of far future epoch", exitEpochWrongCount)
	} else if withdrawEpochWrongCount > 0 {
		return fmt.Errorf("%d validators did not have genesis validator withdrawable epoch of far future epoch", withdrawEpochWrongCount)
	}

	return nil
}

// validatorsParticipating ensures the validators have an acceptable participation rate.
func validatorsParticipating(_ *types.EvaluationContext, conns ...*types.NodeConnection) error {
	conn := conns[0]
	participation, err := getValidatorParticipation(conn)
	if err != nil {
		return errors.Wrap(err, "failed to get validator participation")
	}

	partRateRaw, err := strconv.ParseFloat(participation.Participation.GlobalParticipationRate, 32)
	if err != nil {
		return errors.Wrapf(err, "failed to parse global participation rate %s", participation.Participation.GlobalParticipationRate)
	}
	partRate := float32(partRateRaw)

	participationEpoch, err := strconv.ParseUint(participation.Epoch, 10, 64)
	if err != nil {
		return errors.Wrapf(err, "failed to parse participation epoch %s", participation.Epoch)
	}
	epoch := primitives.Epoch(participationEpoch)

	expected := float32(expectedParticipation)
	if e2eparams.TestParams.LighthouseBeaconNodeCount != 0 {
		expected = float32(expectedMulticlientParticipation)
	}
	if epoch == params.BeaconConfig().ElectraForkEpoch {
		// The first slot of Electra will be missed due to the switching of attestation types
		// 5/6 slots =~0.83
		// validator REST always is slightly reduced at ~0.82
		expected = 0.82
	}
	if epoch > 0 && epoch.Sub(1) == params.BeaconConfig().BellatrixForkEpoch {
		// Reduce Participation requirement to 95% to account for longer EE calls for
		// the merge block. Target and head will likely be missed for a few validators at
		// slot 0.
		expected = 0.95
	}
	if partRate < expected {
		resp, err := getBeaconState(conn, "head")
		if err != nil {
			return err
		}

		var respPrevEpochParticipation []string
		switch resp.Version {
		case version.String(version.Phase0):
		// Do Nothing
		case version.String(version.Altair):
			st := &structs.BeaconStateAltair{}
			if err = json.Unmarshal(resp.Data, st); err != nil {
				return err
			}
			respPrevEpochParticipation = st.PreviousEpochParticipation
		case version.String(version.Bellatrix):
			st := &structs.BeaconStateBellatrix{}
			if err = json.Unmarshal(resp.Data, st); err != nil {
				return err
			}
			respPrevEpochParticipation = st.PreviousEpochParticipation
		case version.String(version.Capella):
			st := &structs.BeaconStateCapella{}
			if err = json.Unmarshal(resp.Data, st); err != nil {
				return err
			}
			respPrevEpochParticipation = st.PreviousEpochParticipation
		case version.String(version.Deneb):
			st := &structs.BeaconStateDeneb{}
			if err = json.Unmarshal(resp.Data, st); err != nil {
				return err
			}
			respPrevEpochParticipation = st.PreviousEpochParticipation
		case version.String(version.Electra):
			st := &structs.BeaconStateElectra{}
			if err = json.Unmarshal(resp.Data, st); err != nil {
				return err
			}
			respPrevEpochParticipation = st.PreviousEpochParticipation
		case version.String(version.Fulu):
			st := &structs.BeaconStateFulu{}
			if err = json.Unmarshal(resp.Data, st); err != nil {
				return err
			}
			respPrevEpochParticipation = st.PreviousEpochParticipation
		default:
			return fmt.Errorf("unrecognized version %s", resp.Version)
		}

		prevEpochParticipation := make([]byte, len(respPrevEpochParticipation))
		for i, p := range respPrevEpochParticipation {
			n, err := strconv.ParseUint(p, 10, 64)
			if err != nil {
				return err
			}
			prevEpochParticipation[i] = byte(n)
		}
		missSrcVals, missTgtVals, missHeadVals, err := findMissingValidators(prevEpochParticipation)
		if err != nil {
			return errors.Wrap(err, "failed to get missing validators")
		}

		return fmt.Errorf(
			"validator participation was below for epoch %d, expected %f, received: %f."+
				" Missing Source,Target and Head validators are %v, %v, %v",
			epoch,
			expected,
			partRate,
			missSrcVals,
			missTgtVals,
			missHeadVals,
		)
	}
	return nil
}

// validatorsSyncParticipation ensures the validators have an acceptable participation rate for
// sync committee assignments.
func validatorsSyncParticipation(_ *types.EvaluationContext, conns ...*types.NodeConnection) error {
	conn := conns[0]

	genesisTime, err := getGenesisTime(conn)
	if err != nil {
		return errors.Wrap(err, "failed to get genesis data")
	}
	currSlot := slots.CurrentSlot(genesisTime)
	currEpoch := slots.ToEpoch(currSlot)
	lowestBound := primitives.Epoch(0)
	if currEpoch >= 1 {
		lowestBound = currEpoch - 1
	}

	if lowestBound < params.BeaconConfig().AltairForkEpoch {
		lowestBound = params.BeaconConfig().AltairForkEpoch
	}

	blks, err := getBlocksForEpoch(conn, lowestBound)
	if err != nil {
		return errors.Wrap(err, "failed to get validator participation")
	}
	for _, b := range blks {
		if err := checkSyncParticipationForBlock(b, lowestBound, true); err != nil {
			return err
		}
	}

	if lowestBound == currEpoch {
		return nil
	}

	blks, err = getBlocksForEpoch(conn, currEpoch)
	if err != nil {
		return errors.Wrap(err, "failed to get validator participation")
	}
	for _, b := range blks {
		if err := checkSyncParticipationForBlock(b, lowestBound, false); err != nil {
			return err
		}
	}
	return nil
}

// checkSyncParticipationForBlock validates the sync committee participation for a single block.
// When isLowestBound is true, it applies the simplified fork-slot check used for the lower epoch.
// When false, it applies the full multi-fork skip logic used for the current epoch.
func checkSyncParticipationForBlock(b interfaces.ReadOnlySignedBeaconBlock, lowestBound primitives.Epoch, isLowestBound bool) error {
	if b == nil || b.IsNil() {
		return errors.New("nil block provided")
	}

	if isLowestBound {
		forkStartSlot, err := slots.EpochStart(params.BeaconConfig().AltairForkEpoch)
		if err != nil {
			return err
		}
		if forkStartSlot == b.Block().Slot() {
			// Skip fork slot.
			return nil
		}
		// Skip slots 1-2 at genesis - validators need time to ramp up after chain start
		// due to doppelganger protection. This is a startup timing issue, not a fork transition issue.
		if b.Block().Slot() < 3 {
			return nil
		}
		expectedParticipation := expectedSyncParticipation
		switch slots.ToEpoch(b.Block().Slot()) {
		case params.BeaconConfig().AltairForkEpoch:
			// Drop expected sync participation figure.
			expectedParticipation = 0.90
		default:
			// no-op
		}
		syncAgg, err := b.Block().Body().SyncAggregate()
		if err != nil {
			return err
		}
		threshold := uint64(float64(syncAgg.SyncCommitteeBits.Len()) * expectedParticipation)
		if syncAgg.SyncCommitteeBits.Count() < threshold {
			return errors.Errorf("In block of slot %d ,the aggregate bitvector with length of %d only got a count of %d", b.Block().Slot(), threshold, syncAgg.SyncCommitteeBits.Count())
		}
		return nil
	}

	// Current epoch: skip the first two slots of each fork epoch.
	forkEpochs := []primitives.Epoch{
		params.BeaconConfig().AltairForkEpoch,
		params.BeaconConfig().BellatrixForkEpoch,
		params.BeaconConfig().CapellaForkEpoch,
		params.BeaconConfig().DenebForkEpoch,
		params.BeaconConfig().ElectraForkEpoch,
		params.BeaconConfig().FuluForkEpoch,
	}
	for _, forkEpoch := range forkEpochs {
		// Skip fork epochs set to far future (not scheduled).
		if forkEpoch == params.BeaconConfig().FarFutureEpoch {
			continue
		}
		forkSlot, err := slots.EpochStart(forkEpoch)
		if err != nil {
			return err
		}
		// Skip the first two slots of each fork epoch.
		if b.Block().Slot() == forkSlot || b.Block().Slot() == forkSlot+1 {
			return nil
		}
	}
	syncAgg, err := b.Block().Body().SyncAggregate()
	if err != nil {
		return err
	}
	threshold := uint64(float64(syncAgg.SyncCommitteeBits.Len()) * expectedSyncParticipation)
	if syncAgg.SyncCommitteeBits.Count() < threshold {
		return errors.Errorf("In block of slot %d ,the aggregate bitvector with length of %d only got a count of %d", b.Block().Slot(), threshold, syncAgg.SyncCommitteeBits.Count())
	}
	return nil
}

func findMissingValidators(participation []byte) ([]uint64, []uint64, []uint64, error) {
	cfg := params.BeaconConfig()
	sourceFlagIndex := cfg.TimelySourceFlagIndex
	targetFlagIndex := cfg.TimelyTargetFlagIndex
	headFlagIndex := cfg.TimelyHeadFlagIndex
	var missingSourceValidators []uint64
	var missingHeadValidators []uint64
	var missingTargetValidators []uint64
	for i, b := range participation {
		hasSource, err := altair.HasValidatorFlag(b, sourceFlagIndex)
		if err != nil {
			return nil, nil, nil, err
		}
		if !hasSource {
			missingSourceValidators = append(missingSourceValidators, uint64(i))
		}
		hasTarget, err := altair.HasValidatorFlag(b, targetFlagIndex)
		if err != nil {
			return nil, nil, nil, err
		}
		if !hasTarget {
			missingTargetValidators = append(missingTargetValidators, uint64(i))
		}
		hasHead, err := altair.HasValidatorFlag(b, headFlagIndex)
		if err != nil {
			return nil, nil, nil, err
		}
		if !hasHead {
			missingHeadValidators = append(missingHeadValidators, uint64(i))
		}
	}
	return missingSourceValidators, missingTargetValidators, missingHeadValidators, nil
}
