package evaluators

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// OptimisticSyncEnabled checks that the node is in an optimistic state.
var OptimisticSyncEnabled = types.Evaluator{
	Name:       "optimistic_sync_at_epoch_%d",
	Policy:     policies.AllEpochs,
	Evaluation: optimisticSyncEnabled,
}

func optimisticSyncEnabled(_ *types.EvaluationContext, conns ...*types.NodeConnection) error {
	for _, conn := range conns {
		path := conn.BaseURL + "/eth/v1/beacon/blinded_blocks/head"
		resp := structs.GetBlockV2Response{}
		httpResp, err := conn.Client.Get(path)
		if err != nil {
			return err
		}
		if httpResp.StatusCode != http.StatusOK {
			e := httputil.DefaultJsonError{}
			if err = json.NewDecoder(httpResp.Body).Decode(&e); err != nil {
				return err
			}
			return fmt.Errorf("%s (status code %d)", e.Message, e.Code)
		}
		if err = json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
			return err
		}
		headSlot, err := retrieveHeadSlot(&resp)
		if err != nil {
			return err
		}
		currEpoch := slots.ToEpoch(primitives.Slot(headSlot))
		startSlot, err := slots.EpochStart(currEpoch)
		if err != nil {
			return err
		}
		for i := startSlot; i <= primitives.Slot(headSlot); i++ {
			path = fmt.Sprintf("%s/eth/v1/beacon/blinded_blocks/%d", conn.BaseURL, i)
			resp = structs.GetBlockV2Response{}
			httpResp, err = conn.Client.Get(path)
			if err != nil {
				return err
			}
			if httpResp.StatusCode == http.StatusNotFound {
				// Continue in the event of non-existent blocks.
				continue
			}
			if httpResp.StatusCode != http.StatusOK {
				e := httputil.DefaultJsonError{}
				if err = json.NewDecoder(httpResp.Body).Decode(&e); err != nil {
					return err
				}
				return fmt.Errorf("%s (status code %d)", e.Message, e.Code)
			}
			if err = json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
				return err
			}
			if !resp.ExecutionOptimistic {
				return errors.New("expected block to be optimistic, but it is not")
			}
		}
	}
	return nil
}

func retrieveHeadSlot(resp *structs.GetBlockV2Response) (uint64, error) {
	var headSlot uint64
	var err error
	switch resp.Version {
	case version.String(version.Phase0):
		b := &structs.BeaconBlock{}
		if err := json.Unmarshal(resp.Data.Message, b); err != nil {
			return 0, err
		}
		headSlot, err = strconv.ParseUint(b.Slot, 10, 64)
		if err != nil {
			return 0, err
		}
	case version.String(version.Altair):
		b := &structs.BeaconBlockAltair{}
		if err := json.Unmarshal(resp.Data.Message, b); err != nil {
			return 0, err
		}
		headSlot, err = strconv.ParseUint(b.Slot, 10, 64)
		if err != nil {
			return 0, err
		}
	case version.String(version.Bellatrix):
		b := &structs.BeaconBlockBellatrix{}
		if err := json.Unmarshal(resp.Data.Message, b); err != nil {
			return 0, err
		}
		headSlot, err = strconv.ParseUint(b.Slot, 10, 64)
		if err != nil {
			return 0, err
		}
	case version.String(version.Capella):
		b := &structs.BeaconBlockCapella{}
		if err := json.Unmarshal(resp.Data.Message, b); err != nil {
			return 0, err
		}
		headSlot, err = strconv.ParseUint(b.Slot, 10, 64)
		if err != nil {
			return 0, err
		}
	case version.String(version.Deneb):
		b := &structs.BeaconBlockDeneb{}
		if err := json.Unmarshal(resp.Data.Message, b); err != nil {
			return 0, err
		}
		headSlot, err = strconv.ParseUint(b.Slot, 10, 64)
		if err != nil {
			return 0, err
		}
	case version.String(version.Electra):
		b := &structs.BeaconBlockElectra{}
		if err := json.Unmarshal(resp.Data.Message, b); err != nil {
			return 0, err
		}
		headSlot, err = strconv.ParseUint(b.Slot, 10, 64)
		if err != nil {
			return 0, err
		}
	default:
		return 0, errors.New("no valid block type retrieved")
	}
	return headSlot, nil
}
