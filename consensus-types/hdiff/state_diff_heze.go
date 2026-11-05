package hdiff

import (
	"bytes"
	"encoding/binary"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

// pqKeyDiffLength is the serialized size of one pqKeyDiff: registrant(8) + xmss_public_key(32) + vrf_root(32) + change_epoch(8).
const pqKeyDiffLength = 8 + 2*fieldparams.RootLength + 8

// pqKeyDiff is a changed or appended EIP-8450 post-quantum key entry.
// The registrant is a validator index, or a builder index with BuilderIndexFlag set.
type pqKeyDiff struct {
	registrant  primitives.ValidatorIndex
	key         *ethpb.PQKeyMaterial
	changeEpoch primitives.Epoch
}

// diffPQKeys records every target entry that differs from the source, in increasing index order,
// validators first. A pre-Heze source has no entries, so every target entry is recorded.
func diffPQKeys(diff *stateDiff, source, target state.ReadOnlyBeaconState) error {
	tKeys, tEpochs, tBuilderKeys, tBuilderEpochs, err := target.PQKeyListsReadOnly()
	if err != nil {
		return errors.Wrap(err, "failed to get target post-quantum keys")
	}
	var sKeys, sBuilderKeys []*ethpb.PQKeyMaterial
	var sEpochs, sBuilderEpochs []primitives.Epoch
	if source.Version() >= version.Heze {
		sKeys, sEpochs, sBuilderKeys, sBuilderEpochs, err = source.PQKeyListsReadOnly()
		if err != nil {
			return errors.Wrap(err, "failed to get source post-quantum keys")
		}
	}
	diff.pqKeyDiffs = nil
	appendDiffs := func(sKeys, tKeys []*ethpb.PQKeyMaterial, sEpochs, tEpochs []primitives.Epoch, flag uint64) {
		for i := range tKeys {
			if i < len(sKeys) && i < len(sEpochs) && sEpochs[i] == tEpochs[i] &&
				bytes.Equal(sKeys[i].XmssPublicKey, tKeys[i].XmssPublicKey) && bytes.Equal(sKeys[i].VrfRoot, tKeys[i].VrfRoot) {
				continue
			}
			diff.pqKeyDiffs = append(diff.pqKeyDiffs, pqKeyDiff{
				registrant:  primitives.ValidatorIndex(uint64(i) | flag),
				key:         tKeys[i],
				changeEpoch: tEpochs[i],
			})
		}
	}
	appendDiffs(sKeys, tKeys, sEpochs, tEpochs, 0)
	appendDiffs(sBuilderKeys, tBuilderKeys, sBuilderEpochs, tBuilderEpochs, primitives.BuilderIndexFlag)
	return nil
}

func serializePQKeyDiffs(ret []byte, diffs []pqKeyDiff) []byte {
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(diffs)))
	for _, d := range diffs {
		ret = binary.LittleEndian.AppendUint64(ret, uint64(d.registrant))
		ret = append(ret, d.key.XmssPublicKey...)
		ret = append(ret, d.key.VrfRoot...)
		ret = binary.LittleEndian.AppendUint64(ret, uint64(d.changeEpoch))
	}
	return ret
}

func (ret *stateDiff) readPQKeyDiffs(data *[]byte) error {
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "pqKeyDiffs count")
	}
	count := binary.LittleEndian.Uint64((*data)[:8])
	*data = (*data)[8:]
	if count > uint64(len(*data)/pqKeyDiffLength) {
		return errors.Wrap(errDataSmall, "pqKeyDiffs data")
	}
	ret.pqKeyDiffs = make([]pqKeyDiff, count)
	for i := range ret.pqKeyDiffs {
		d := (*data)[:pqKeyDiffLength]
		ret.pqKeyDiffs[i] = pqKeyDiff{
			registrant: primitives.ValidatorIndex(binary.LittleEndian.Uint64(d[:8])),
			key: &ethpb.PQKeyMaterial{
				XmssPublicKey: bytes.Clone(d[8:40]),
				VrfRoot:       bytes.Clone(d[40:72]),
			},
			changeEpoch: primitives.Epoch(binary.LittleEndian.Uint64(d[72:80])),
		}
		*data = (*data)[pqKeyDiffLength:]
	}
	return nil
}
