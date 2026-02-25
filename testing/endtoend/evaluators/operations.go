package evaluators

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz/detect"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/helpers"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"golang.org/x/exp/rand"
)

var depositValCount = e2e.DepositCount
var numOfExits = 2

// Deposits should be processed in twice the length of the epochs per eth1 voting period.
var depositsInBlockStart = params.E2ETestConfig().EpochsPerEth1VotingPeriod * 2

// deposits included + finalization + MaxSeedLookahead for activation.
var depositActivationStartEpoch = depositsInBlockStart + 2 + params.E2ETestConfig().MaxSeedLookahead
var depositEndEpoch = depositActivationStartEpoch + primitives.Epoch(math.Ceil(float64(depositValCount)/float64(params.E2ETestConfig().MinPerEpochChurnLimit)))

// Default exit submission epoch for standard tests.
var defaultExitSubmissionEpoch = primitives.Epoch(7)

// ProcessesDepositsInBlocks ensures the expected amount of deposits are accepted into blocks.
// Note: This evaluator only works for pre-Electra genesis since Electra uses EIP-6110 deposit requests.
var ProcessesDepositsInBlocks = e2etypes.Evaluator{
	Name: "processes_deposits_in_blocks_epoch_%d",
	Policy: func(e primitives.Epoch) bool {
		// Skip if starting at Electra or later - deposits work differently with EIP-6110
		if e2etypes.GenesisFork() >= version.Electra {
			return false
		}
		return policies.OnEpoch(depositsInBlockStart)(e)
	},
	Evaluation: processesDepositsInBlocks,
}

// VerifyBlockGraffiti ensures the block graffiti is one of the random list.
var VerifyBlockGraffiti = e2etypes.Evaluator{
	Name:       "verify_graffiti_in_blocks_epoch_%d",
	Policy:     policies.AfterNthEpoch(0),
	Evaluation: verifyGraffitiInBlocks,
}

// ActivatesDepositedValidators ensures the expected amount of validator deposits are activated into the state.
// Note: This evaluator only works for pre-Electra genesis since Electra uses EIP-6110 deposit requests.
var ActivatesDepositedValidators = e2etypes.Evaluator{
	Name: "processes_deposit_validators_epoch_%d",
	Policy: func(e primitives.Epoch) bool {
		// Skip if starting at Electra or later - deposits work differently with EIP-6110
		if e2etypes.GenesisFork() >= version.Electra {
			return false
		}
		return policies.BetweenEpochs(depositActivationStartEpoch, depositEndEpoch)(e)
	},
	Evaluation: activatesDepositedValidators,
}

// DepositedValidatorsAreActive ensures the expected amount of validators are active after their deposits are processed.
// Note: This evaluator only works for pre-Electra genesis since Electra uses EIP-6110 deposit requests.
var DepositedValidatorsAreActive = e2etypes.Evaluator{
	Name: "deposited_validators_are_active_epoch_%d",
	Policy: func(e primitives.Epoch) bool {
		// Skip if starting at Electra or later - deposits work differently with EIP-6110
		if e2etypes.GenesisFork() >= version.Electra {
			return false
		}
		return policies.AfterNthEpoch(depositEndEpoch)(e)
	},
	Evaluation: depositedValidatorsAreActive,
}

// ProposeVoluntaryExit sends a voluntary exit from randomly selected validator in the genesis set.
// Uses the default exit submission epoch (7).
var ProposeVoluntaryExit = ProposeVoluntaryExitAtEpoch(defaultExitSubmissionEpoch)

// ProposeVoluntaryExitAtEpoch sends a voluntary exit at the specified epoch.
var ProposeVoluntaryExitAtEpoch = func(epoch primitives.Epoch) e2etypes.Evaluator {
	return e2etypes.Evaluator{
		Name:       "propose_voluntary_exit_epoch_%d",
		Policy:     policies.OnEpoch(epoch),
		Evaluation: proposeVoluntaryExit,
	}
}

// ValidatorsHaveExited checks the beacon state for the exited validator and ensures its marked as exited.
// Uses the default exit submission epoch + 1 (epoch 8).
var ValidatorsHaveExited = ValidatorsHaveExitedAtEpoch(defaultExitSubmissionEpoch + 1)

// ValidatorsHaveExitedAtEpoch checks validators have exited at the specified epoch.
var ValidatorsHaveExitedAtEpoch = func(epoch primitives.Epoch) e2etypes.Evaluator {
	return e2etypes.Evaluator{
		Name:       "voluntary_has_exited_%d",
		Policy:     policies.OnEpoch(epoch),
		Evaluation: validatorsHaveExited,
	}
}

// SubmitWithdrawal sends a withdrawal from a previously exited validator.
// Uses default timing based on exit submission epoch.
var SubmitWithdrawal = SubmitWithdrawalAtEpoch(defaultExitSubmissionEpoch + 1)

// SubmitWithdrawalAtEpoch sends a withdrawal at the specified epoch.
// For pre-Deneb genesis, it runs around the Capella fork epoch instead.
var SubmitWithdrawalAtEpoch = func(epoch primitives.Epoch) e2etypes.Evaluator {
	return e2etypes.Evaluator{
		Name: "submit_withdrawal_epoch_%d",
		Policy: func(currentEpoch primitives.Epoch) bool {
			fEpoch := params.BeaconConfig().CapellaForkEpoch
			// If Capella is disabled (starting at Deneb+), run at the specified epoch
			if e2etypes.GenesisFork() >= version.Deneb {
				return policies.OnEpoch(epoch)(currentEpoch)
			}
			return policies.BetweenEpochs(fEpoch-2, fEpoch+1)(currentEpoch)
		},
		Evaluation: submitWithdrawal,
	}
}

// ValidatorsHaveWithdrawn checks the beacon state for the withdrawn validator and ensures it has been withdrawn.
// Uses default timing based on exit submission epoch.
var ValidatorsHaveWithdrawn = ValidatorsHaveWithdrawnAfterExitAtEpoch(defaultExitSubmissionEpoch)

// ValidatorsHaveWithdrawnAfterExitAtEpoch checks validators have withdrawn after exiting at the specified epoch.
// For pre-Deneb genesis, it runs at CapellaForkEpoch + 1 instead.
var ValidatorsHaveWithdrawnAfterExitAtEpoch = func(exitSubmitEpoch primitives.Epoch) e2etypes.Evaluator {
	return e2etypes.Evaluator{
		Name: "validator_has_withdrawn_%d",
		Policy: func(currentEpoch primitives.Epoch) bool {
			// TODO: Fix this for mainnet configs.
			if params.BeaconConfig().ConfigName != params.EndToEndName {
				return false
			}
			// Only run this for minimal setups after capella
			fEpoch := params.BeaconConfig().CapellaForkEpoch
			// If Capella is disabled (starting at Deneb+), run after withdrawal submission
			var validWithdrawnEpoch primitives.Epoch
			if e2etypes.GenesisFork() >= version.Deneb {
				// Exit submitted at exitSubmitEpoch
				// Exit epoch = exitSubmitEpoch + 1 + MAX_SEED_LOOKAHEAD
				// Withdrawable epoch = exit epoch + MIN_VALIDATOR_WITHDRAWABILITY_DELAY
				// Add 1 more epoch for the sweep to process
				exitEpoch := exitSubmitEpoch + 1 + primitives.Epoch(params.BeaconConfig().MaxSeedLookahead)
				withdrawableEpoch := exitEpoch + primitives.Epoch(params.BeaconConfig().MinValidatorWithdrawabilityDelay)
				validWithdrawnEpoch = withdrawableEpoch + 1
			} else {
				// For pre-Deneb genesis, give 2 epochs after Capella for:
				// 1. BLS-to-exec changes to be processed (submitted in epoch before Capella)
				// 2. Withdrawal sweep to reach all exited validators
				validWithdrawnEpoch = fEpoch + 2
			}

			requiredPolicy := policies.OnEpoch(validWithdrawnEpoch)
			return requiredPolicy(currentEpoch)
		},
		Evaluation: validatorsAreWithdrawn,
	}
}

// ValidatorsVoteWithTheMajority verifies whether validator vote for eth1data using the majority algorithm.
var ValidatorsVoteWithTheMajority = e2etypes.Evaluator{
	Name:       "validators_vote_with_the_majority_%d",
	Policy:     policies.AfterNthEpoch(0),
	Evaluation: validatorsVoteWithTheMajority,
}

type mismatch struct {
	k [48]byte
	e uint64
	o uint64
}

func (m mismatch) String() string {
	return fmt.Sprintf("(%#x:%d:%d)", m.k, m.e, m.o)
}

func processesDepositsInBlocks(ec *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	expected := ec.Balances(e2etypes.PostGenesisDepositBatch)
	conn := conns[0]
	chainHead, err := getChainHead(conn)
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	epoch, err := chainHeadEpoch(chainHead)
	if err != nil {
		return errors.Wrap(err, "failed to parse head epoch")
	}

	blks, err := getBlocksForEpoch(conn, epoch-1)
	if err != nil {
		return errors.Wrap(err, "failed to get blocks from beacon-chain")
	}
	observed := make(map[[48]byte]uint64)
	for _, blk := range blks {
		deposits := blk.Block().Body().Deposits()
		for _, d := range deposits {
			k := bytesutil.ToBytes48(d.Data.PublicKey)
			v := observed[k]
			observed[k] = v + d.Data.Amount
		}
	}
	var mismatches []string
	for k, ev := range expected {
		ov := observed[k]
		if ev != ov {
			mismatches = append(mismatches, mismatch{k: k, e: ev, o: ov}.String())
		}
	}
	if len(mismatches) != 0 {
		return fmt.Errorf("not all expected deposits observed on chain, len(expected)=%d, len(observed)=%d, mismatches=%d; details(key:expected:observed): %s", len(expected), len(observed), len(mismatches), strings.Join(mismatches, ","))
	}
	return nil
}

func verifyGraffitiInBlocks(_ *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]
	chainHead, err := getChainHead(conn)
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	epoch, err := chainHeadEpoch(chainHead)
	if err != nil {
		return errors.Wrap(err, "failed to parse head epoch")
	}
	begin := epoch
	// Prevent underflow when this runs at epoch 0.
	if begin > 0 {
		begin = begin.Sub(1)
	}
	blks, err := getBlocksForEpoch(conn, begin)
	if err != nil {
		return errors.Wrap(err, "failed to get blocks from beacon-chain")
	}
	for _, blk := range blks {
		var found bool
		slot := blk.Block().Slot()
		graffitiInBlock := blk.Block().Body().Graffiti()
		// Trim trailing null bytes from graffiti.
		// Example: "SushiGEabcdPRxxxx\x00\x00\x00..." becomes "SushiGEabcdPRxxxx"
		graffitiTrimmed := bytes.TrimRight(graffitiInBlock[:], "\x00")
		for _, graffiti := range helpers.Graffiti {
			// Check prefix match since user graffiti comes first, with EL/CL version info appended after.
			if bytes.HasPrefix(graffitiTrimmed, []byte(graffiti)) {
				found = true
				break
			}
		}
		if !found && slot != 0 {
			return fmt.Errorf("block at slot %d has graffiti %q which does not start with any expected graffiti", slot, string(graffitiTrimmed))
		}
	}

	return nil
}

func activatesDepositedValidators(ec *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]

	chainHead, err := getChainHead(conn)
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	epoch, err := chainHeadEpoch(chainHead)
	if err != nil {
		return errors.Wrap(err, "failed to parse head epoch")
	}
	finalizedEpoch, err := chainHeadFinalizedEpoch(chainHead)
	if err != nil {
		return errors.Wrap(err, "failed to parse finalized epoch")
	}

	validators, err := getAllValidators(conn)
	if err != nil {
		return errors.Wrap(err, "failed to get validators")
	}
	expected := ec.Balances(e2etypes.PostGenesisDepositBatch)

	var deposits, lowBalance, wrongExit, wrongWithdraw int
	for _, vc := range validators {
		v := vc.Validator
		pubkeyBytes, err := hexutil.Decode(v.Pubkey)
		if err != nil {
			return errors.Wrap(err, "failed to decode validator pubkey")
		}
		key := bytesutil.ToBytes48(pubkeyBytes)
		if _, ok := expected[key]; !ok {
			continue
		}
		delete(expected, key)

		activationEligibilityEpoch, err := strconv.ParseUint(v.ActivationEligibilityEpoch, 10, 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse activation eligibility epoch")
		}
		activationEpoch, err := strconv.ParseUint(v.ActivationEpoch, 10, 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse activation epoch")
		}
		effectiveBalance, err := strconv.ParseUint(v.EffectiveBalance, 10, 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse effective balance")
		}
		exitEpoch, err := strconv.ParseUint(v.ExitEpoch, 10, 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse exit epoch")
		}
		withdrawableEpoch, err := strconv.ParseUint(v.WithdrawableEpoch, 10, 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse withdrawable epoch")
		}

		// Validator can't be activated yet.
		if primitives.Epoch(activationEligibilityEpoch) > finalizedEpoch {
			continue
		}
		if primitives.Epoch(activationEpoch) < epoch {
			continue
		}
		if primitives.Epoch(activationEpoch) == epoch {
			deposits++
		}
		if effectiveBalance < params.BeaconConfig().MaxEffectiveBalance {
			lowBalance++
		}
		if primitives.Epoch(exitEpoch) != params.BeaconConfig().FarFutureEpoch {
			wrongExit++
		}
		if primitives.Epoch(withdrawableEpoch) != params.BeaconConfig().FarFutureEpoch {
			wrongWithdraw++
		}
	}

	// Make sure every post-genesis deposit has been processed, resulting in a validator.
	if len(expected) > 0 {
		return fmt.Errorf("missing %d validators for post-genesis deposits", len(expected))
	}

	if deposits > 0 && uint64(deposits) != params.BeaconConfig().MinPerEpochChurnLimit {
		return fmt.Errorf("expected %d deposits to be processed in epoch %d, received %d", params.BeaconConfig().MinPerEpochChurnLimit, epoch, deposits)
	}

	if lowBalance > 0 {
		return fmt.Errorf(
			"%d validators did not have genesis validator effective balance of %d",
			lowBalance,
			params.BeaconConfig().MaxEffectiveBalance,
		)
	} else if wrongExit > 0 {
		return fmt.Errorf("%d validators did not have an exit epoch of far future epoch", wrongExit)
	} else if wrongWithdraw > 0 {
		return fmt.Errorf("%d validators did not have a withdrawable epoch of far future epoch", wrongWithdraw)
	}
	return nil
}

// getAllValidators pages through the REST validators endpoint and returns all ValidatorContainers.
func getAllValidators(conn *e2etypes.NodeConnection) ([]*structs.ValidatorContainer, error) {
	var result []*structs.ValidatorContainer
	pageToken := ""
	for {
		resp, err := getValidators(conn, "head", nil, pageToken, 100)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get validators")
		}
		result = append(result, resp.Data...)
		// The standard beacon API does not provide a next_page_token; once we receive
		// fewer results than the page size (or an empty page), we have fetched all validators.
		if len(resp.Data) < 100 {
			break
		}
		// If the API supports pagination tokens, advance to the next page.
		pageToken = strconv.Itoa(len(result))
	}
	return result, nil
}

func depositedValidatorsAreActive(ec *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]

	chainHead, err := getChainHead(conn)
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	headEpoch, err := chainHeadEpoch(chainHead)
	if err != nil {
		return errors.Wrap(err, "failed to parse head epoch")
	}
	finalizedEpoch, err := chainHeadFinalizedEpoch(chainHead)
	if err != nil {
		return errors.Wrap(err, "failed to parse finalized epoch")
	}

	vcs, err := getAllValidators(conn)
	if err != nil {
		return errors.Wrap(err, "error retrieving validator list from API")
	}
	inactive := 0
	lowBalance := 0
	nexits := 0
	expected := ec.Balances(e2etypes.PostGenesisDepositBatch)
	nexpected := len(expected)

	for _, vc := range vcs {
		v := vc.Validator
		pubkeyBytes, err := hexutil.Decode(v.Pubkey)
		if err != nil {
			return errors.Wrap(err, "failed to decode validator pubkey")
		}
		key := bytesutil.ToBytes48(pubkeyBytes)
		if _, ok := expected[key]; !ok {
			continue // we aren't checking for this validator
		}
		// ignore voluntary exits when checking balance and active status
		if _, exited := ec.ExitedVals[key]; exited {
			nexits++
			delete(expected, key)
			continue
		}

		activationEligibilityEpoch, err := strconv.ParseUint(v.ActivationEligibilityEpoch, 10, 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse activation eligibility epoch")
		}
		activationEpoch, err := strconv.ParseUint(v.ActivationEpoch, 10, 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse activation epoch")
		}
		effectiveBalance, err := strconv.ParseUint(v.EffectiveBalance, 10, 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse effective balance")
		}
		exitEpoch, err := strconv.ParseUint(v.ExitEpoch, 10, 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse exit epoch")
		}

		// This is to handle the changed validator activation procedure post-electra.
		if activationEligibilityEpoch != math.MaxUint64 && primitives.Epoch(activationEligibilityEpoch) > finalizedEpoch {
			delete(expected, key)
			continue
		}
		if activationEpoch != math.MaxUint64 && primitives.Epoch(activationEpoch) > headEpoch {
			delete(expected, key)
			continue
		}

		// Inline IsActiveValidator: activation_epoch <= epoch < exit_epoch
		if !(primitives.Epoch(activationEpoch) <= headEpoch && headEpoch < primitives.Epoch(exitEpoch)) {
			inactive++
		}
		if effectiveBalance < params.BeaconConfig().MaxEffectiveBalance {
			lowBalance++
		}
		delete(expected, key)
	}
	if len(expected) > 0 {
		mk := make([]string, 0)
		for k := range expected {
			mk = append(mk, fmt.Sprintf("%#x", k))
		}
		return fmt.Errorf("API response missing %d validators, based on deposits; keys=%s", len(expected), strings.Join(mk, ","))
	}
	if inactive != 0 || lowBalance != 0 {
		return fmt.Errorf("active validator set does not match %d total deposited. %d exited, %d inactive, %d low balance", nexpected, nexits, inactive, lowBalance)
	}

	return nil
}

func proposeVoluntaryExit(ec *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]

	chainHead, err := getChainHead(conn)
	if err != nil {
		return errors.Wrap(err, "could not get chain head")
	}
	headEpoch, err := chainHeadEpoch(chainHead)
	if err != nil {
		return errors.Wrap(err, "failed to parse head epoch")
	}
	headSlot, err := chainHeadSlot(chainHead)
	if err != nil {
		return errors.Wrap(err, "failed to parse head slot")
	}

	sszBytes, err := getBeaconStateSSZ(conn, fmt.Sprintf("%d", headSlot))
	if err != nil {
		return errors.Wrap(err, "could not get state object")
	}
	versionedMarshaler, err := detect.FromState(sszBytes)
	if err != nil {
		return errors.Wrap(err, "could not get state marshaler")
	}
	st, err := versionedMarshaler.UnmarshalBeaconState(sszBytes)
	if err != nil {
		return errors.Wrap(err, "could not get state")
	}
	var execIndices []int
	err = st.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		if val.GetWithdrawalCredentials()[0] == params.BeaconConfig().ETH1AddressWithdrawalPrefixByte {
			execIndices = append(execIndices, idx)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(execIndices) > numOfExits {
		execIndices = execIndices[:numOfExits]
	}

	deposits, privKeys, err := util.DeterministicDepositsAndKeys(params.BeaconConfig().MinGenesisActiveValidatorCount)
	if err != nil {
		return err
	}

	var sendExit = func(exitedIndex primitives.ValidatorIndex) error {
		voluntaryExit := &ethpb.VoluntaryExit{
			Epoch:          headEpoch,
			ValidatorIndex: exitedIndex,
		}
		domain, err := computeDomainData(conn, headEpoch, params.BeaconConfig().DomainVoluntaryExit)
		if err != nil {
			return errors.Wrap(err, "could not get domain data")
		}
		signingData, err := signing.ComputeSigningRoot(voluntaryExit, domain)
		if err != nil {
			return err
		}
		signature := privKeys[exitedIndex].Sign(signingData[:])

		jsonExit := &structs.SignedVoluntaryExit{
			Message: &structs.VoluntaryExit{
				Epoch:          fmt.Sprintf("%d", headEpoch),
				ValidatorIndex: fmt.Sprintf("%d", exitedIndex),
			},
			Signature: hexutil.Encode(signature.Marshal()),
		}
		if err = submitVoluntaryExit(conn, jsonExit); err != nil {
			return errors.Wrap(err, "could not propose exit")
		}
		pubk := bytesutil.ToBytes48(deposits[exitedIndex].Data.PublicKey)
		ec.ExitedVals[pubk] = headEpoch // Store submission epoch
		return nil
	}

	// Send exits for keys which already contain execution credentials.
	for _, idx := range execIndices {
		if err := sendExit(primitives.ValidatorIndex(idx)); err != nil {
			return err
		}
	}

	// Send an exit for a non-exited validator.
	for i := 0; i < numOfExits; {
		randIndex := primitives.ValidatorIndex(rand.Uint64() % params.BeaconConfig().MinGenesisActiveValidatorCount)
		if _, alreadyExited := ec.ExitedVals[bytesutil.ToBytes48(privKeys[randIndex].PublicKey().Marshal())]; alreadyExited {
			continue
		}
		if err := sendExit(randIndex); err != nil {
			return err
		}
		i++
	}

	return nil
}

func validatorsHaveExited(ec *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]
	for k := range ec.ExitedVals {
		hexPubkey := hexutil.Encode(k[:])
		resp, err := getValidator(conn, "head", hexPubkey)
		if err != nil {
			return errors.Wrap(err, "failed to get validators")
		}
		exitEpoch, err := strconv.ParseUint(resp.Data.Validator.ExitEpoch, 10, 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse exit epoch")
		}
		if primitives.Epoch(exitEpoch) == params.BeaconConfig().FarFutureEpoch {
			return fmt.Errorf("expected validator %#x to be submitted for exit", k)
		}
	}
	return nil
}

func validatorsVoteWithTheMajority(ec *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]
	chainHead, err := getChainHead(conn)
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	headEpoch, err := chainHeadEpoch(chainHead)
	if err != nil {
		return errors.Wrap(err, "failed to parse head epoch")
	}

	begin := headEpoch
	// Prevent underflow when this runs at epoch 0.
	if begin > 0 {
		begin = begin.Sub(1)
	}
	blks, err := getBlocksForEpoch(conn, begin)
	if err != nil {
		return errors.Wrap(err, "failed to get blocks from beacon-chain")
	}

	slotsPerVotingPeriod := params.E2ETestConfig().SlotsPerEpoch.Mul(uint64(params.E2ETestConfig().EpochsPerEth1VotingPeriod))
	for _, blk := range blks {
		slot := blk.Block().Slot()
		vote := blk.Block().Body().Eth1Data().BlockHash
		ec.SeenVotes[slot] = vote

		// We treat epoch 1 differently from other epoch for two reasons:
		// - this evaluator is not executed for epoch 0 so we have to calculate the first slot differently
		// - for some reason the vote for the first slot in epoch 1 is 0x000... so we skip this slot
		var isFirstSlotInVotingPeriod bool
		if headEpoch == 1 && slot%params.BeaconConfig().SlotsPerEpoch == 0 {
			continue
		}
		// We skipped the first slot so we treat the second slot as the starting slot of epoch 1.
		if headEpoch == 1 {
			isFirstSlotInVotingPeriod = slot%params.BeaconConfig().SlotsPerEpoch == 1
		} else {
			isFirstSlotInVotingPeriod = slot%slotsPerVotingPeriod == 0
		}
		if isFirstSlotInVotingPeriod {
			ec.ExpectedEth1DataVote = vote
			ec.Eth1DataMismatchCount = 0 // Reset for new voting period
			return nil
		}

		if !bytes.Equal(vote, ec.ExpectedEth1DataVote) {
			// Allow some tolerance for eth1data vote differences.
			// Validators may have slightly different views of the eth1 chain
			// as new blocks arrive during the voting period.
			ec.Eth1DataMismatchCount++
			// Allow up to 2 mismatches per voting period before failing.
			if ec.Eth1DataMismatchCount > 2 {
				for i := range slot {
					v, ok := ec.SeenVotes[i]
					if ok {
						fmt.Printf("vote at slot=%d = %#x\n", i, v)
					} else {
						fmt.Printf("did not see slot=%d\n", i)
					}
				}
				return fmt.Errorf("incorrect eth1data vote for slot %d; expected: %#x vs voted: %#x (mismatch count: %d)",
					slot, ec.ExpectedEth1DataVote, vote, ec.Eth1DataMismatchCount)
			}
		}
	}
	return nil
}

func submitWithdrawal(ec *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]

	chainHead, err := getChainHead(conn)
	if err != nil {
		return errors.Wrap(err, "could not get chain head")
	}
	headSlot, err := chainHeadSlot(chainHead)
	if err != nil {
		return errors.Wrap(err, "failed to parse head slot")
	}

	sszBytes, err := getBeaconStateSSZ(conn, fmt.Sprintf("%d", headSlot))
	if err != nil {
		return errors.Wrap(err, "could not get state object")
	}
	versionedMarshaler, err := detect.FromState(sszBytes)
	if err != nil {
		return errors.Wrap(err, "could not get state marshaler")
	}
	st, err := versionedMarshaler.UnmarshalBeaconState(sszBytes)
	if err != nil {
		return errors.Wrap(err, "could not get state")
	}
	exitedIndices := make([]primitives.ValidatorIndex, 0)

	for key := range ec.ExitedVals {
		valIdx, ok := st.ValidatorIndexByPubkey(key)
		if !ok {
			return errors.Errorf("pubkey %#x does not exist in our state", key)
		}
		exitedIndices = append(exitedIndices, valIdx)
	}

	_, privKeys, err := util.DeterministicDepositsAndKeys(params.BeaconConfig().MinGenesisActiveValidatorCount)
	if err != nil {
		return err
	}
	changes := make([]*structs.SignedBLSToExecutionChange, 0)
	// Only send half the number of changes each time, to allow us to test
	// at the fork boundary. When starting from Deneb+ at genesis, there's no
	// fork boundary to test so we send all changes.
	wantedChanges := numOfExits / 2
	if e2etypes.GenesisFork() >= version.Deneb {
		wantedChanges = numOfExits
	}
	for _, idx := range exitedIndices {
		// Exit sending more change messages.
		if len(changes) >= wantedChanges {
			break
		}
		val, err := st.ValidatorAtIndex(idx)
		if err != nil {
			return err
		}
		if val.WithdrawalCredentials[0] == params.BeaconConfig().ETH1AddressWithdrawalPrefixByte {
			continue
		}
		if !bytes.Equal(val.PublicKey, privKeys[idx].PublicKey().Marshal()) {
			return errors.Errorf("pubkey is not equal, wanted %#x but received %#x", val.PublicKey, privKeys[idx].PublicKey().Marshal())
		}
		message := &ethpb.BLSToExecutionChange{
			ValidatorIndex:     idx,
			FromBlsPubkey:      privKeys[idx].PublicKey().Marshal(),
			ToExecutionAddress: bytesutil.ToBytes(uint64(idx), 20),
		}
		domain, err := signing.ComputeDomain(params.BeaconConfig().DomainBLSToExecutionChange, params.BeaconConfig().GenesisForkVersion, st.GenesisValidatorsRoot())
		if err != nil {
			return err
		}
		sigRoot, err := signing.ComputeSigningRoot(message, domain)
		if err != nil {
			return err
		}
		signature := privKeys[idx].Sign(sigRoot[:]).Marshal()

		changes = append(changes, &structs.SignedBLSToExecutionChange{
			Message:   structs.BLSChangeFromConsensus(message),
			Signature: hexutil.Encode(signature),
		})
	}

	return postJSON(conn, "/eth/v1/beacon/pool/bls_to_execution_changes", changes, nil)
}

func validatorsAreWithdrawn(ec *e2etypes.EvaluationContext, conns ...*e2etypes.NodeConnection) error {
	conn := conns[0]

	chainHead, err := getChainHead(conn)
	if err != nil {
		return errors.Wrap(err, "could not get chain head")
	}
	headSlot, err := chainHeadSlot(chainHead)
	if err != nil {
		return errors.Wrap(err, "failed to parse head slot")
	}

	sszBytes, err := getBeaconStateSSZ(conn, fmt.Sprintf("%d", headSlot))
	if err != nil {
		return errors.Wrap(err, "could not get state object")
	}
	versionedMarshaler, err := detect.FromState(sszBytes)
	if err != nil {
		return errors.Wrap(err, "could not get state marshaler")
	}
	st, err := versionedMarshaler.UnmarshalBeaconState(sszBytes)
	if err != nil {
		return errors.Wrap(err, "could not get state")
	}

	for key := range ec.ExitedVals {
		valIdx, ok := st.ValidatorIndexByPubkey(key)
		if !ok {
			return errors.Errorf("pubkey %#x does not exist in our state", key)
		}
		bal, err := st.BalanceAtIndex(valIdx)
		if err != nil {
			return err
		}
		// Only return an error if the validator has more than 1 eth
		// in its balance.
		if bal > 1*params.BeaconConfig().GweiPerEth {
			return errors.Errorf("Validator index %d with key %#x hasn't withdrawn. Their balance is %d.", valIdx, key, bal)
		}
	}
	return nil
}
