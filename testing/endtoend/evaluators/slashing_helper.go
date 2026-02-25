package evaluators

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
)

// doubleAttestationHelper holds the state required to generate slashable attestation pairs.
// All communication with the beacon node is done via REST using conn.
type doubleAttestationHelper struct {
	conn       *e2etypes.NodeConnection
	privKeys   []bls.SecretKey
	pubKeys    [][]byte
	domainData []byte // signing domain bytes for DomainBeaconAttester
	attData    *eth.AttestationData

	committee []primitives.ValidatorIndex
}

// setup initialises the helper by fetching chain head state, attester duties, the full committee
// membership for the head slot, attestation data, and the signing domain.
func (h *doubleAttestationHelper) setup() error {
	chainHead, err := getChainHead(h.conn)
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
		return errors.Wrap(err, "could not get deposits and keys")
	}

	pubKeys := make([][]byte, len(privKeys))
	for i, priv := range privKeys {
		pubKeys[i] = priv.PublicKey().Marshal()
	}

	// Build the list of validator index strings to query attester duties for.
	indices := make([]string, len(privKeys))
	for i := range privKeys {
		indices[i] = fmt.Sprintf("%d", i)
	}

	duties, err := getAttesterDuties(h.conn, headEpoch, indices)
	if err != nil {
		return errors.Wrap(err, "could not get attester duties")
	}

	// Find the duty (if any) whose AttesterSlot matches the head slot, and record the committee
	// index so we can fetch the full committee membership from the committees endpoint.
	headSlotStr := fmt.Sprintf("%d", headSlot)
	var committeeIndex primitives.CommitteeIndex
	foundDuty := false
	for _, duty := range duties.Data {
		if duty.Slot == headSlotStr {
			ciRaw, parseErr := strconv.ParseUint(duty.CommitteeIndex, 10, 64)
			if parseErr != nil {
				return fmt.Errorf("could not parse committee index: %w", parseErr)
			}
			committeeIndex = primitives.CommitteeIndex(ciRaw)
			foundDuty = true
			break
		}
	}
	if !foundDuty {
		return fmt.Errorf("could not find attester duty for head slot %d", headSlot)
	}

	// Fetch the full committee membership (list of validator indices) from the committees
	// endpoint, since the attester duties response only provides committee metadata.
	committee, err := getBeaconCommittee(h.conn, headSlot, committeeIndex)
	if err != nil {
		return errors.Wrap(err, "could not get beacon committee")
	}

	// Fetch attestation data for the chosen committee.
	attDataREST, err := getAttestationData(h.conn, headSlot, committeeIndex)
	if err != nil {
		return errors.Wrap(err, "could not get attestation data")
	}

	// Convert the REST structs.AttestationData to the proto type used for signing.
	attData, err := attDataREST.ToConsensus()
	if err != nil {
		return errors.Wrap(err, "could not convert attestation data to consensus")
	}

	// Compute the signing domain for beacon attestation.
	domainBytes, err := computeDomainData(h.conn, headEpoch, params.BeaconConfig().DomainBeaconAttester)
	if err != nil {
		return errors.Wrap(err, "could not compute domain data")
	}

	h.privKeys = privKeys
	h.pubKeys = pubKeys
	h.domainData = domainBytes
	h.committee = committee
	h.attData = attData

	return nil
}

// validatorIndexAtCommitteeIndex returns the validator global index of the committee member at
// position idx.
func (h *doubleAttestationHelper) validatorIndexAtCommitteeIndex(idx uint64) primitives.ValidatorIndex {
	return h.committee[idx]
}

// getSlashableAttestation returns an attestation previously submitted (at headSlot), modified so
// that it is signed by the validator at committee position idx. The beacon block root is
// randomised on every call so that P2P gossip treats each message as distinct.
func (h *doubleAttestationHelper) getSlashableAttestation(idx uint64) (*eth.Attestation, error) {
	// msg must be unique so they are not filtered by P2P.
	randVal := make([]byte, 4)
	if _, err := rand.Read(randVal); err != nil {
		return nil, errors.Wrap(err, "error reading random val")
	}
	blockRoot := bytesutil.ToBytes32(append(randVal, []byte("muahahahaha evil validator")...))
	h.attData.BeaconBlockRoot = blockRoot[:]

	signingRoot, err := signing.ComputeSigningRoot(h.attData, h.domainData)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute signing root")
	}

	valIdx := h.validatorIndexAtCommitteeIndex(idx)

	attBitfield := bitfield.NewBitlist(uint64(len(h.committee)))
	attBitfield.SetBitAt(idx, true)
	att := &eth.Attestation{
		AggregationBits: attBitfield,
		Data:            h.attData,
		Signature:       h.privKeys[valIdx].Sign(signingRoot[:]).Marshal(),
	}
	return att, nil
}

// getBeaconCommittee fetches the full list of validator indices in a specific committee for the
// given slot and committee index. It calls:
//
//	GET /eth/v1/beacon/states/head/committees?slot={slot}&index={committeeIndex}
//
// and returns the Validators field of the matching entry.
func getBeaconCommittee(conn *e2etypes.NodeConnection, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) ([]primitives.ValidatorIndex, error) {
	url := fmt.Sprintf(
		"%s/eth/v1/beacon/states/head/committees?slot=%d&index=%d",
		conn.BaseURL, slot, committeeIndex,
	)
	resp, err := conn.Client.Get(url)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get beacon committees")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("beacon committees request failed with status %d: %s", resp.StatusCode, body)
	}

	result := &structs.GetCommitteesResponse{}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return nil, errors.Wrap(err, "failed to decode beacon committees response")
	}

	for _, c := range result.Data {
		ciRaw, err := strconv.ParseUint(c.Index, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not parse committee index %q: %w", c.Index, err)
		}
		slotRaw, err := strconv.ParseUint(c.Slot, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not parse committee slot %q: %w", c.Slot, err)
		}
		if primitives.CommitteeIndex(ciRaw) == committeeIndex && primitives.Slot(slotRaw) == slot {
			committee := make([]primitives.ValidatorIndex, len(c.Validators))
			for i, v := range c.Validators {
				vi, err := strconv.ParseUint(v, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("could not parse committee validator index %q: %w", v, err)
				}
				committee[i] = primitives.ValidatorIndex(vi)
			}
			return committee, nil
		}
	}

	return nil, fmt.Errorf("committee not found for slot %d index %d", slot, committeeIndex)
}
