package evaluators

import (
	"bytes"
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/interop"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/components"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var FeeRecipientIsPresent = types.Evaluator{
	Name: "fee_recipient_is_present_%d",
	Policy: func(e primitives.Epoch) bool {
		fEpoch := params.BeaconConfig().BellatrixForkEpoch
		return policies.AfterNthEpoch(fEpoch)(e)
	},
	Evaluation: feeRecipientIsPresent,
}

func lhKeyMap() (map[string]bool, error) {
	if e2e.TestParams.LighthouseBeaconNodeCount == 0 {
		return nil, nil
	}
	pry, lh := e2e.TestParams.BeaconNodeCount, e2e.TestParams.LighthouseBeaconNodeCount
	valPerNode := int(params.BeaconConfig().MinGenesisActiveValidatorCount) / (pry + lh)
	lhOff := valPerNode * pry
	_, keys, err := interop.DeterministicallyGenerateKeys(uint64(lhOff), uint64(valPerNode*lh))
	if err != nil {
		return nil, err
	}

	km := make(map[string]bool)
	for _, k := range keys {
		km[hexutil.Encode(k.Marshal())] = true
	}
	return km, nil
}

func valKeyMap() (map[string]bool, error) {
	nvals := params.BeaconConfig().MinGenesisActiveValidatorCount
	// matches validator start in validator component + validators used for deposits
	_, pubs, err := interop.DeterministicallyGenerateKeys(0, nvals+e2e.DepositCount)
	if err != nil {
		return nil, err
	}
	km := make(map[string]bool)
	for _, k := range pubs {
		km[hexutil.Encode(k.Marshal())] = true
	}
	return km, nil
}

func feeRecipientIsPresent(_ *types.EvaluationContext, conns ...*types.NodeConnection) error {
	conn := conns[0]
	chainHead, err := getChainHead(conn)
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	epoch, err := chainHeadEpoch(chainHead)
	if err != nil {
		return errors.Wrap(err, "failed to parse head epoch")
	}
	if epoch > 0 {
		epoch--
	}

	blks, err := getBlocksForEpoch(conn, epoch)
	if err != nil {
		return errors.Wrap(err, "failed to list blocks")
	}

	rpcclient, err := rpc.DialHTTP(fmt.Sprintf("http://127.0.0.1:%d", e2e.TestParams.Ports.Eth1RPCPort))
	if err != nil {
		return err
	}
	defer rpcclient.Close()

	valkeys, err := valKeyMap()
	if err != nil {
		return err
	}
	lhkeys, err := lhKeyMap()
	if err != nil {
		return err
	}

	for _, blk := range blks {
		if blk == nil || blk.IsNil() {
			continue
		}

		execPayload, err := blk.Block().Body().Execution()
		if err != nil {
			// Pre-Bellatrix blocks have no execution payload; skip them.
			continue
		}

		blockHash := execPayload.BlockHash()
		// If the beacon chain has transitioned to Bellatrix, but the EL hasn't hit TTD, we could see a few slots
		// of blocks with empty payloads.
		if bytes.Equal(blockHash, make([]byte, 32)) {
			continue
		}

		feeRecipientBytes := execPayload.FeeRecipient()
		if len(feeRecipientBytes) == 0 || hexutil.Encode(feeRecipientBytes) == params.BeaconConfig().EthBurnAddressHex {
			log.WithField("proposerIndex", blk.Block().ProposerIndex()).
				WithField("slot", blk.Block().Slot()).
				Error("Fee recipient eval bug")
			return errors.New("fee recipient is not set")
		}

		fr := common.BytesToAddress(feeRecipientBytes)
		proposerIndex := blk.Block().ProposerIndex()
		valResp, err := getValidator(conn, "head", fmt.Sprintf("%d", proposerIndex))
		if err != nil {
			return errors.Wrap(err, "failed to get validators")
		}
		pk := valResp.Data.Validator.Pubkey

		if _, ok := lhkeys[pk]; ok {
			// Don't check lighthouse keys.
			continue
		}

		// In e2e we generate deterministic keys by validator index, and then use a slice of their public key bytes
		// as the fee recipient, so that this will also be deterministic, so this test can statelessly verify it.
		// These should be the only keys we see.
		// Otherwise, something has changed in e2e and this test needs to be updated.
		_, knownKey := valkeys[pk]
		if !knownKey {
			log.WithField("pubkey", pk).
				WithField("slot", blk.Block().Slot()).
				WithField("proposerIndex", proposerIndex).
				WithField("feeRecipient", fr.Hex()).
				Warn("Unknown key observed, not a deterministically generated key")
			return errors.New("unknown key observed, not a deterministically generated key")
		}

		if components.FeeRecipientFromPubkey(pk) != fr.Hex() {
			return fmt.Errorf("publickey %s, fee recipient %s does not match the proposer settings fee recipient %s",
				pk, fr.Hex(), components.FeeRecipientFromPubkey(pk))
		}

		parentHash := execPayload.ParentHash()
		if err := checkRecipientBalance(rpcclient, common.BytesToHash(blockHash), common.BytesToHash(parentHash), fr); err != nil {
			return err
		}
	}

	return nil
}

func checkRecipientBalance(c *rpc.Client, block, parent common.Hash, account common.Address) error {
	web3 := ethclient.NewClient(c)
	ctx := context.Background()
	b, err := web3.BlockByHash(ctx, block)
	if err != nil {
		return err
	}

	bal, err := web3.BalanceAt(ctx, account, b.Number())
	if err != nil {
		return err
	}
	pBlock, err := web3.BlockByHash(ctx, parent)
	if err != nil {
		return err
	}
	pBal, err := web3.BalanceAt(ctx, account, pBlock.Number())
	if err != nil {
		return err
	}
	if b.GasUsed() > 0 && bal.Uint64() <= pBal.Uint64() {
		return errors.Errorf("account balance didn't change after applying fee recipient for account: %s", account.Hex())
	}

	return nil
}
