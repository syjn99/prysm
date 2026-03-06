// Package evaluators contains REST helper functions for E2E evaluators.
// These delegate to rest.Handler (embedded in NodeConnection) for all HTTP
// communication, replacing the former ad-hoc getJSON / postJSON plumbing.
package evaluators

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// postJSON marshals body to JSON and POSTs it via rest.Handler, decoding the response into result.
// If result is nil, the response body is discarded.
func postJSON(conn *e2etypes.NodeConnection, path string, body any, result any) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return errors.Wrap(err, "failed to marshal request body")
	}
	return conn.Post(context.Background(), path, nil, bytes.NewBuffer(jsonBody), result)
}

// --- Domain helpers ---

// getChainHead fetches the chain head from the Prysm-specific REST endpoint.
func getChainHead(conn *e2etypes.NodeConnection) (*structs.ChainHead, error) {
	result := &structs.ChainHead{}
	if err := conn.Get(context.Background(), "/prysm/v1/beacon/chain_head", result); err != nil {
		return nil, errors.Wrap(err, "failed to get chain head")
	}
	return result, nil
}

// getGenesis fetches genesis info from the standard Beacon API.
func getGenesis(conn *e2etypes.NodeConnection) (*structs.Genesis, error) {
	result := &structs.GetGenesisResponse{}
	if err := conn.Get(context.Background(), "/eth/v1/beacon/genesis", result); err != nil {
		return nil, errors.Wrap(err, "failed to get genesis")
	}
	return result.Data, nil
}

// getGenesisTime fetches genesis and returns the time.
func getGenesisTime(conn *e2etypes.NodeConnection) (time.Time, error) {
	genesis, err := getGenesis(conn)
	if err != nil {
		return time.Time{}, err
	}
	genesisTimeSec, err := strconv.ParseInt(genesis.GenesisTime, 10, 64)
	if err != nil {
		return time.Time{}, errors.Wrap(err, "failed to parse genesis time")
	}
	return time.Unix(genesisTimeSec, 0), nil
}

// getSyncStatus fetches sync status from the standard Beacon API.
func getSyncStatus(conn *e2etypes.NodeConnection) (*structs.SyncStatusResponseData, error) {
	result := &structs.SyncStatusResponse{}
	if err := conn.Get(context.Background(), "/eth/v1/node/syncing", result); err != nil {
		return nil, errors.Wrap(err, "failed to get sync status")
	}
	return result.Data, nil
}

// getNodePeers fetches peers from the standard Beacon API.
func getNodePeers(conn *e2etypes.NodeConnection) (*structs.GetPeersResponse, error) {
	result := &structs.GetPeersResponse{}
	if err := conn.Get(context.Background(), "/eth/v1/node/peers", result); err != nil {
		return nil, errors.Wrap(err, "failed to get peers")
	}
	return result, nil
}

// getValidators fetches validators from the standard Beacon API.
func getValidators(conn *e2etypes.NodeConnection, stateID string, statuses []string) (*structs.GetValidatorsResponse, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("/eth/v1/beacon/states/%s/validators", stateID))
	sep := "?"
	for _, s := range statuses {
		sb.WriteString(sep + "status=" + s)
		sep = "&"
	}
	path := sb.String()
	result := &structs.GetValidatorsResponse{}
	if err := conn.Get(context.Background(), path, result); err != nil {
		return nil, errors.Wrap(err, "failed to get validators")
	}
	return result, nil
}

// getValidator fetches a single validator from the standard Beacon API.
func getValidator(conn *e2etypes.NodeConnection, stateID, validatorID string) (*structs.GetValidatorResponse, error) {
	path := fmt.Sprintf("/eth/v1/beacon/states/%s/validators/%s", stateID, validatorID)
	result := &structs.GetValidatorResponse{}
	if err := conn.Get(context.Background(), path, result); err != nil {
		return nil, errors.Wrap(err, "failed to get validator")
	}
	return result, nil
}

// getValidatorParticipation fetches validator participation from the Prysm-specific REST endpoint.
func getValidatorParticipation(conn *e2etypes.NodeConnection) (*structs.GetValidatorParticipationResponse, error) {
	result := &structs.GetValidatorParticipationResponse{}
	if err := conn.Get(context.Background(), "/prysm/v1/validators/head/participation", result); err != nil {
		return nil, errors.Wrap(err, "failed to get validator participation")
	}
	return result, nil
}

// getBlockSSZ fetches a block as SSZ and returns it as a ReadOnlySignedBeaconBlock.
func getBlockSSZ(conn *e2etypes.NodeConnection, blockID string) (interfaces.ReadOnlySignedBeaconBlock, error) {
	path := fmt.Sprintf("/eth/v2/beacon/blocks/%s", blockID)
	body, headers, err := conn.GetSSZ(context.Background(), path)
	if err != nil {
		var jsonErr *httputil.DefaultJsonError
		if errors.As(err, &jsonErr) && jsonErr.Code == http.StatusNotFound {
			return nil, nil // missed slot
		}
		return nil, errors.Wrap(err, "failed to get block SSZ")
	}
	ver := headers.Get("Eth-Consensus-Version")
	return unmarshalBlockSSZ(ver, body)
}

// getBlock fetches a block as JSON from the standard Beacon API.
func getBlock(conn *e2etypes.NodeConnection, blockID string) (*structs.GetBlockV2Response, error) {
	path := fmt.Sprintf("/eth/v2/beacon/blocks/%s", blockID)
	result := &structs.GetBlockV2Response{}
	if err := conn.Get(context.Background(), path, result); err != nil {
		return nil, errors.Wrap(err, "failed to get block")
	}
	return result, nil
}

// getBeaconState fetches a beacon state as JSON from the debug endpoint.
func getBeaconState(conn *e2etypes.NodeConnection, stateID string) (*structs.GetBeaconStateV2Response, error) {
	path := fmt.Sprintf("/eth/v2/debug/beacon/states/%s", stateID)
	result := &structs.GetBeaconStateV2Response{}
	if err := conn.Get(context.Background(), path, result); err != nil {
		return nil, errors.Wrap(err, "failed to get beacon state")
	}
	return result, nil
}

func unmarshalBlockSSZ(ver string, sszBytes []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	switch ver {
	case version.String(version.Phase0):
		pb := &ethpb.SignedBeaconBlock{}
		if err := pb.UnmarshalSSZ(sszBytes); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal phase0 block")
		}
		return blocks.NewSignedBeaconBlock(pb)
	case version.String(version.Altair):
		pb := &ethpb.SignedBeaconBlockAltair{}
		if err := pb.UnmarshalSSZ(sszBytes); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal altair block")
		}
		return blocks.NewSignedBeaconBlock(pb)
	case version.String(version.Bellatrix):
		pb := &ethpb.SignedBeaconBlockBellatrix{}
		if err := pb.UnmarshalSSZ(sszBytes); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal bellatrix block")
		}
		return blocks.NewSignedBeaconBlock(pb)
	case version.String(version.Capella):
		pb := &ethpb.SignedBeaconBlockCapella{}
		if err := pb.UnmarshalSSZ(sszBytes); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal capella block")
		}
		return blocks.NewSignedBeaconBlock(pb)
	case version.String(version.Deneb):
		pb := &ethpb.SignedBeaconBlockDeneb{}
		if err := pb.UnmarshalSSZ(sszBytes); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal deneb block")
		}
		return blocks.NewSignedBeaconBlock(pb)
	case version.String(version.Electra):
		pb := &ethpb.SignedBeaconBlockElectra{}
		if err := pb.UnmarshalSSZ(sszBytes); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal electra block")
		}
		return blocks.NewSignedBeaconBlock(pb)
	case version.String(version.Fulu):
		pb := &ethpb.SignedBeaconBlockFulu{}
		if err := pb.UnmarshalSSZ(sszBytes); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal fulu block")
		}
		return blocks.NewSignedBeaconBlock(pb)
	default:
		return nil, fmt.Errorf("unknown block version: %s", ver)
	}
}

// getBlocksForEpoch fetches all blocks in an epoch via REST SSZ endpoint.
func getBlocksForEpoch(conn *e2etypes.NodeConnection, epoch primitives.Epoch) ([]interfaces.ReadOnlySignedBeaconBlock, error) {
	startSlot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, err
	}
	endSlot := startSlot + params.BeaconConfig().SlotsPerEpoch

	var result []interfaces.ReadOnlySignedBeaconBlock
	for slot := startSlot; slot < endSlot; slot++ {
		blk, err := getBlockSSZ(conn, fmt.Sprintf("%d", slot))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get block at slot %d", slot)
		}
		if blk != nil { // nil means missed slot
			result = append(result, blk)
		}
	}
	return result, nil
}

// getHeadBlock fetches the head block via REST SSZ endpoint.
func getHeadBlock(conn *e2etypes.NodeConnection) (interfaces.ReadOnlySignedBeaconBlock, error) {
	return getBlockSSZ(conn, "head")
}

// getBeaconStateSSZ fetches a beacon state as SSZ bytes from the debug endpoint.
func getBeaconStateSSZ(conn *e2etypes.NodeConnection, stateID string) ([]byte, error) {
	path := fmt.Sprintf("/eth/v2/debug/beacon/states/%s", stateID)
	body, _, err := conn.GetSSZ(context.Background(), path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get beacon state SSZ")
	}
	return body, nil
}

// submitVoluntaryExit submits a signed voluntary exit via REST.
func submitVoluntaryExit(conn *e2etypes.NodeConnection, exit *structs.SignedVoluntaryExit) error {
	return postJSON(conn, "/eth/v1/beacon/pool/voluntary_exits", exit, nil)
}

// submitAttestation submits an attestation via REST.
func submitAttestation(conn *e2etypes.NodeConnection, att *ethpb.Attestation) error {
	jsonAtt := structs.AttFromConsensus(att)
	return postJSON(conn, "/eth/v1/beacon/pool/attestations", []*structs.Attestation{jsonAtt}, nil)
}

// publishBlock publishes a signed beacon block via REST using SSZ encoding.
func publishBlock(conn *e2etypes.NodeConnection, blk interfaces.ReadOnlySignedBeaconBlock) error {
	sszBytes, err := blk.MarshalSSZ()
	if err != nil {
		return errors.Wrap(err, "failed to marshal block to SSZ")
	}
	headers := map[string]string{
		"Eth-Consensus-Version": version.String(blk.Version()),
	}
	_, _, err = conn.PostSSZ(context.Background(), "/eth/v2/beacon/blocks", headers, bytes.NewBuffer(sszBytes))
	return err
}

// getAttestationData fetches attestation data from the REST API.
func getAttestationData(conn *e2etypes.NodeConnection, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) (*structs.AttestationData, error) {
	path := fmt.Sprintf("/eth/v1/validator/attestation_data?slot=%d&committee_index=%d", slot, committeeIndex)
	result := &structs.GetAttestationDataResponse{}
	if err := conn.Get(context.Background(), path, result); err != nil {
		return nil, errors.Wrap(err, "failed to get attestation data")
	}
	return result.Data, nil
}

// getProposerDuties fetches proposer duties for an epoch.
func getProposerDuties(conn *e2etypes.NodeConnection, epoch primitives.Epoch) (*structs.GetProposerDutiesResponse, error) {
	path := fmt.Sprintf("/eth/v1/validator/duties/proposer/%d", epoch)
	result := &structs.GetProposerDutiesResponse{}
	if err := conn.Get(context.Background(), path, result); err != nil {
		return nil, errors.Wrap(err, "failed to get proposer duties")
	}
	return result, nil
}

// getAttesterDuties fetches attester duties for an epoch and set of validator indices.
func getAttesterDuties(conn *e2etypes.NodeConnection, epoch primitives.Epoch, indices []string) (*structs.GetAttesterDutiesResponse, error) {
	path := fmt.Sprintf("/eth/v1/validator/duties/attester/%d", epoch)
	result := &structs.GetAttesterDutiesResponse{}
	if err := postJSON(conn, path, indices, result); err != nil {
		return nil, errors.Wrap(err, "failed to get attester duties")
	}
	return result, nil
}

// computeDomainData computes the signing domain locally (replacing the gRPC DomainData call).
func computeDomainData(conn *e2etypes.NodeConnection, epoch primitives.Epoch, domainType [4]byte) ([]byte, error) {
	genesis, err := getGenesis(conn)
	if err != nil {
		return nil, err
	}
	genesisValidatorsRoot, err := bytesutil.DecodeHexWithLength(genesis.GenesisValidatorsRoot, 32)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode genesis validators root")
	}
	cfg := params.BeaconConfig()
	fork := params.ForkFromConfig(cfg, epoch)
	// EIP-7044: from Deneb onward, voluntary exits are always signed with Capella fork version.
	if domainType == cfg.DomainVoluntaryExit && epoch >= cfg.DenebForkEpoch {
		fork = &ethpb.Fork{
			PreviousVersion: cfg.CapellaForkVersion,
			CurrentVersion:  cfg.CapellaForkVersion,
			Epoch:           cfg.CapellaForkEpoch,
		}
	}
	domain, err := signing.Domain(fork, epoch, domainType, genesisValidatorsRoot)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute domain")
	}
	return domain, nil
}

// pollForBlock polls the head block until one is found at or after the given slot.
// This replaces gRPC StreamBlocksAltair for fork transition evaluators.
func pollForBlock(ctx context.Context, conn *e2etypes.NodeConnection, minSlot primitives.Slot) (interfaces.ReadOnlySignedBeaconBlock, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			blk, err := getHeadBlock(conn)
			if err != nil {
				continue // retry
			}
			if blk == nil || blk.IsNil() {
				continue
			}
			if blk.Block().Slot() >= minSlot {
				return blk, nil
			}
		}
	}
}

// chainHeadEpoch is a helper that returns the head epoch from the chain head.
func chainHeadEpoch(ch *structs.ChainHead) (primitives.Epoch, error) {
	e, err := strconv.ParseUint(ch.HeadEpoch, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse head epoch")
	}
	return primitives.Epoch(e), nil
}

// chainHeadSlot is a helper that returns the head slot from the chain head.
func chainHeadSlot(ch *structs.ChainHead) (primitives.Slot, error) {
	s, err := strconv.ParseUint(ch.HeadSlot, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse head slot")
	}
	return primitives.Slot(s), nil
}

// chainHeadFinalizedEpoch returns the finalized epoch from chain head.
func chainHeadFinalizedEpoch(ch *structs.ChainHead) (primitives.Epoch, error) {
	e, err := strconv.ParseUint(ch.FinalizedEpoch, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse finalized epoch")
	}
	return primitives.Epoch(e), nil
}
