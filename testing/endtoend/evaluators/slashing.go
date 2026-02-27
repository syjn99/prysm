package evaluators

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/slice"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	e2eTypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
)

// InjectDoubleVoteOnEpoch broadcasts a double vote into the beacon node pool for the slasher to detect.
var InjectDoubleVoteOnEpoch = func(n primitives.Epoch) e2eTypes.Evaluator {
	return e2eTypes.Evaluator{
		Name:       "inject_double_vote_%d",
		Policy:     policies.OnEpoch(n),
		Evaluation: insertDoubleAttestationIntoPool,
	}
}

// InjectDoubleBlockOnEpoch proposes a double block to the beacon node for the slasher to detect.
var InjectDoubleBlockOnEpoch = func(n primitives.Epoch) e2eTypes.Evaluator {
	return e2eTypes.Evaluator{
		Name:       "inject_double_block_%d",
		Policy:     policies.OnEpoch(n),
		Evaluation: proposeDoubleBlock,
	}
}

// ValidatorsSlashedAfterEpoch ensures the expected amount of validators are slashed.
var ValidatorsSlashedAfterEpoch = func(n primitives.Epoch) e2eTypes.Evaluator {
	return e2eTypes.Evaluator{
		Name:       "validators_slashed_epoch_%d",
		Policy:     policies.AfterNthEpoch(n),
		Evaluation: validatorsSlashed,
	}
}

// SlashedValidatorsLoseBalanceAfterEpoch checks if the validators slashed lose the right balance.
var SlashedValidatorsLoseBalanceAfterEpoch = func(n primitives.Epoch) e2eTypes.Evaluator {
	return e2eTypes.Evaluator{
		Name:       "slashed_validators_lose_balance_epoch_%d",
		Policy:     policies.AfterNthEpoch(n),
		Evaluation: validatorsLoseBalance,
	}
}

var slashedIndices []uint64

func validatorsSlashed(_ *e2eTypes.EvaluationContext, conns ...*e2eTypes.NodeConnection) error {
	conn := conns[0]

	actualSlashedIndices := 0
	for _, slashedIndex := range slashedIndices {
		valResp, err := getValidator(conn, "head", fmt.Sprintf("%d", slashedIndex))
		if err != nil {
			return err
		}
		if valResp.Data == nil || valResp.Data.Validator == nil {
			return fmt.Errorf("nil validator data for index %d", slashedIndex)
		}
		if valResp.Data.Validator.Slashed {
			actualSlashedIndices++
		}
	}

	if actualSlashedIndices != len(slashedIndices) {
		return fmt.Errorf("expected %d indices to be slashed, received %d", len(slashedIndices), actualSlashedIndices)
	}
	return nil
}

func validatorsLoseBalance(_ *e2eTypes.EvaluationContext, conns ...*e2eTypes.NodeConnection) error {
	conn := conns[0]

	for i, slashedIndex := range slashedIndices {
		valResp, err := getValidator(conn, "head", fmt.Sprintf("%d", slashedIndex))
		if err != nil {
			return err
		}
		if valResp.Data == nil || valResp.Data.Validator == nil {
			return fmt.Errorf("nil validator data for index %d", slashedIndex)
		}

		effectiveBalance, err := strconv.ParseUint(valResp.Data.Validator.EffectiveBalance, 10, 64)
		if err != nil {
			return fmt.Errorf("could not parse effective balance for validator %d: %w", slashedIndex, err)
		}

		slashedPenalty := params.BeaconConfig().MaxEffectiveBalance / params.BeaconConfig().MinSlashingPenaltyQuotient
		slashedBal := params.BeaconConfig().MaxEffectiveBalance - slashedPenalty + params.BeaconConfig().EffectiveBalanceIncrement/10
		if effectiveBalance >= slashedBal {
			return fmt.Errorf(
				"expected slashed validator %d balance to be less than %d, received %d",
				i,
				slashedBal,
				effectiveBalance,
			)
		}
	}
	return nil
}

func insertDoubleAttestationIntoPool(_ *e2eTypes.EvaluationContext, conns ...*e2eTypes.NodeConnection) error {
	h := doubleAttestationHelper{
		conn: conns[0],
	}
	if err := h.setup(); err != nil {
		return errors.Wrap(err, "could not setup doubleAttestationHelper")
	}

	valsToSlash := uint64(2)
	for i := uint64(0); i < valsToSlash; i++ {
		valIdx := h.validatorIndexAtCommitteeIndex(i)

		if len(slice.IntersectionUint64(slashedIndices, []uint64{uint64(valIdx)})) > 0 {
			valsToSlash++
			continue
		}

		// Need to send proposal to both beacon nodes to avoid flakiness.
		// See: https://github.com/prysmaticlabs/prysm/issues/12415#issuecomment-1874643269
		att, err := h.getSlashableAttestation(i)
		if err != nil {
			return err
		}
		if err := submitAttestation(conns[0], att); err != nil {
			return errors.Wrap(err, "could not propose attestation")
		}

		att1, err := h.getSlashableAttestation(i)
		if err != nil {
			return err
		}
		if err := submitAttestation(conns[1], att1); err != nil {
			return errors.Wrap(err, "could not propose attestation")
		}

		slashedIndices = append(slashedIndices, uint64(valIdx))
	}
	return nil
}

func proposeDoubleBlock(_ *e2eTypes.EvaluationContext, conns ...*e2eTypes.NodeConnection) error {
	conn := conns[0]

	chainHead, err := getChainHead(conn)
	if err != nil {
		return errors.Wrap(err, "could not get chain head")
	}

	headEpoch, err := chainHeadEpoch(chainHead)
	if err != nil {
		return errors.Wrap(err, "could not parse chain head epoch")
	}

	headSlot, err := chainHeadSlot(chainHead)
	if err != nil {
		return errors.Wrap(err, "could not parse chain head slot")
	}

	_, privKeys, err := util.DeterministicDepositsAndKeys(params.BeaconConfig().MinGenesisActiveValidatorCount)
	if err != nil {
		return err
	}

	duties, err := getProposerDuties(conn, headEpoch)
	if err != nil {
		return errors.Wrap(err, "could not get proposer duties")
	}

	// Find the proposer for headSlot-1 (the slot we want to submit slashable blocks at).
	targetSlot := fmt.Sprintf("%d", headSlot-1)
	var proposerIndex primitives.ValidatorIndex
	found := false
	for _, duty := range duties.Data {
		if duty.Slot == targetSlot {
			idx, parseErr := strconv.ParseUint(duty.ValidatorIndex, 10, 64)
			if parseErr != nil {
				return fmt.Errorf("could not parse proposer validator index: %w", parseErr)
			}
			proposerIndex = primitives.ValidatorIndex(idx)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("could not find proposer duty for slot %s", targetSlot)
	}

	validatorNum := int(params.BeaconConfig().MinGenesisActiveValidatorCount)
	beaconNodeNum := e2e.TestParams.BeaconNodeCount
	if validatorNum%beaconNodeNum != 0 {
		return errors.New("validator count is not easily divisible by beacon node count")
	}
	validatorsPerNode := validatorNum / beaconNodeNum

	// If the proposer index falls in the second validator client's range, connect to
	// the corresponding beacon node instead.
	publishConn := conns[0]
	if proposerIndex >= primitives.ValidatorIndex(uint64(validatorsPerNode)) {
		publishConn = conns[1]
	}

	wb, err := generateSignedBeaconBlock(publishConn, chainHead, proposerIndex, privKeys, "bad state root")
	if err != nil {
		return err
	}
	// publishBlock returns nil even on non-200 responses; both blocks are intentionally invalid
	// (bad state root) so they will be rejected by state transition but recorded for slashing.
	_ = publishBlock(publishConn, wb)

	wb2, err := generateSignedBeaconBlock(publishConn, chainHead, proposerIndex, privKeys, "bad state root 2")
	if err != nil {
		return err
	}
	_ = publishBlock(publishConn, wb2)

	slashedIndices = append(slashedIndices, uint64(proposerIndex))
	return nil
}

// generateSignedBeaconBlock constructs a Phase0 beacon block with an intentionally invalid
// stateRoot, signs it with the proposer's private key using the signing domain fetched from
// the beacon node, and returns an interfaces.ReadOnlySignedBeaconBlock ready for publishBlock.
// Two calls with different stateRoot values produce a slashable (double-proposal) pair.
func generateSignedBeaconBlock(
	conn *e2eTypes.NodeConnection,
	chainHead *structs.ChainHead,
	proposerIndex primitives.ValidatorIndex,
	privKeys []bls.SecretKey,
	stateRoot string,
) (interfaces.ReadOnlySignedBeaconBlock, error) {
	headEpoch, err := chainHeadEpoch(chainHead)
	if err != nil {
		return nil, errors.Wrap(err, "could not parse chain head epoch")
	}
	headSlot, err := chainHeadSlot(chainHead)
	if err != nil {
		return nil, errors.Wrap(err, "could not parse chain head slot")
	}

	headBlockRoot, err := bytesutil.DecodeHexWithLength(chainHead.HeadBlockRoot, 32)
	if err != nil {
		return nil, errors.Wrap(err, "could not decode head block root")
	}

	hashLen := 32
	blk := &eth.BeaconBlock{
		Slot:          headSlot - 1,
		ParentRoot:    headBlockRoot,
		StateRoot:     bytesutil.PadTo([]byte(stateRoot), hashLen),
		ProposerIndex: proposerIndex,
		Body: &eth.BeaconBlockBody{
			Eth1Data: &eth.Eth1Data{
				BlockHash:    bytesutil.PadTo([]byte("bad block hash"), hashLen),
				DepositRoot:  bytesutil.PadTo([]byte("bad deposit root"), hashLen),
				DepositCount: 1,
			},
			RandaoReveal:      bytesutil.PadTo([]byte("bad randao"), fieldparams.BLSSignatureLength),
			Graffiti:          bytesutil.PadTo([]byte("teehee"), hashLen),
			ProposerSlashings: []*eth.ProposerSlashing{},
			AttesterSlashings: []*eth.AttesterSlashing{},
			Attestations:      []*eth.Attestation{},
			Deposits:          []*eth.Deposit{},
			VoluntaryExits:    []*eth.SignedVoluntaryExit{},
		},
	}

	domainBytes, err := computeDomainData(conn, headEpoch, params.BeaconConfig().DomainBeaconProposer)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute domain data")
	}

	signingRoot, err := signing.ComputeSigningRoot(blk, domainBytes)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute signing root")
	}
	sig := privKeys[proposerIndex].Sign(signingRoot[:]).Marshal()
	signedBlk := &eth.SignedBeaconBlock{
		Block:     blk,
		Signature: sig,
	}

	// We only broadcast to a single node here since we can trust that at least 1 node will be
	// online. Only broadcasting the block to one node also helps test slashing propagation.
	wb, err := blocks.NewSignedBeaconBlock(signedBlk)
	if err != nil {
		return nil, err
	}
	return wb, nil
}
