package blockproduction

import (
	"context"
	"fmt"
	"sync"

	builderapi "github.com/OffchainLabs/prysm/v7/api/client/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	blockfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/block"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// BlockProposerDeps holds the networking dependencies needed for block proposal
// (broadcasting and receiving). These are separate from block production deps.
type BlockProposerDeps struct {
	P2P                p2p.Broadcaster
	BlockReceiver      blockchain.BlockReceiver
	BlobReceiver       blockchain.BlobReceiver
	DataColumnReceiver blockchain.DataColumnReceiver
	BlockNotifier      blockfeed.Notifier
	OperationNotifier  operation.Notifier
	BlockBuilder       builder.BlockBuilder
}

// ProposeBeaconBlock handles the proposal of beacon blocks: broadcasting and
// receiving a signed block and its sidecars.
func (b *BlockProducer) ProposeBeaconBlock(ctx context.Context, req *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error) {
	var (
		blobSidecars       []*ethpb.BlobSidecar
		dataColumnSidecars []blocks.RODataColumn
	)

	ctx, span := trace.StartSpan(ctx, "BlockProducer.ProposeBeaconBlock")
	defer span.End()

	if req == nil {
		return nil, fmt.Errorf("empty request")
	}

	block, err := blocks.NewSignedBeaconBlock(req.Block)
	if err != nil {
		return nil, fmt.Errorf("decode block failed: %v", err)
	}
	root, err := block.Block().HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("could not hash tree root: %v", err)
	}

	// For post-Fulu blinded blocks, submit to relay and return early
	if block.IsBlinded() && slots.ToEpoch(block.Block().Slot()) >= params.BeaconConfig().FuluForkEpoch {
		err := b.Proposer.BlockBuilder.SubmitBlindedBlockPostFulu(ctx, block)
		if err != nil {
			return nil, fmt.Errorf("could not submit blinded block post-Fulu: %v", err)
		}
		return &ethpb.ProposeResponse{BlockRoot: root[:]}, nil
	}

	rob, err := blocks.NewROBlockWithRoot(block, root)
	if block.IsBlinded() {
		block, blobSidecars, err = b.handleBlindedBlock(ctx, block)
		if errors.Is(err, builderapi.ErrBadGateway) {
			log.WithError(err).Info("Optimistically proposed block - builder relay temporarily unavailable, block may arrive over P2P")
			return &ethpb.ProposeResponse{BlockRoot: root[:]}, nil
		}
	} else if block.Version() >= version.Deneb {
		blobSidecars, dataColumnSidecars, err = b.handleUnblindedBlock(rob, req)
	}
	if err != nil {
		return nil, fmt.Errorf("handle block failed: %v", err)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	wg.Add(1)
	go func() {
		if err := b.broadcastReceiveBlock(ctx, &wg, block, root); err != nil {
			errChan <- errors.Wrap(err, "broadcast/receive block failed")
			return
		}
		errChan <- nil
	}()

	wg.Wait()

	if err := b.broadcastAndReceiveSidecars(ctx, block, root, blobSidecars, dataColumnSidecars); err != nil {
		return nil, fmt.Errorf("could not broadcast/receive sidecars: %v", err)
	}
	if err := <-errChan; err != nil {
		return nil, fmt.Errorf("could not broadcast/receive block: %v", err)
	}

	return &ethpb.ProposeResponse{BlockRoot: root[:]}, nil
}

// broadcastAndReceiveSidecars broadcasts and receives sidecars.
func (b *BlockProducer) broadcastAndReceiveSidecars(
	ctx context.Context,
	block interfaces.SignedBeaconBlock,
	root [fieldparams.RootLength]byte,
	blobSidecars []*ethpb.BlobSidecar,
	dataColumnSidecars []blocks.RODataColumn,
) error {
	if block.Version() >= version.Fulu {
		if err := b.broadcastAndReceiveDataColumns(ctx, dataColumnSidecars); err != nil {
			return errors.Wrap(err, "broadcast and receive data columns")
		}
		return nil
	}

	if err := b.broadcastAndReceiveBlobs(ctx, blobSidecars, root); err != nil {
		return errors.Wrap(err, "broadcast and receive blobs")
	}

	return nil
}

// handleBlindedBlock processes blinded beacon blocks (pre-Fulu only).
func (b *BlockProducer) handleBlindedBlock(ctx context.Context, block interfaces.SignedBeaconBlock) (interfaces.SignedBeaconBlock, []*ethpb.BlobSidecar, error) {
	if block.Version() < version.Bellatrix {
		return nil, nil, errors.New("pre-Bellatrix blinded block")
	}

	if b.Proposer.BlockBuilder == nil || !b.Proposer.BlockBuilder.Configured() {
		return nil, nil, errors.New("unconfigured block builder")
	}

	copiedBlock, err := block.Copy()
	if err != nil {
		return nil, nil, err
	}

	payload, bundle, err := b.Proposer.BlockBuilder.SubmitBlindedBlock(ctx, block)
	if err != nil {
		return nil, nil, errors.Wrap(err, "submit blinded block failed")
	}

	if err := copiedBlock.Unblind(payload); err != nil {
		return nil, nil, errors.Wrap(err, "unblind failed")
	}

	sidecars, err := UnblindBlobsSidecars(copiedBlock, bundle)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unblind blobs sidecars: commitment value doesn't match block")
	}

	return copiedBlock, sidecars, nil
}

func (b *BlockProducer) handleUnblindedBlock(
	block blocks.ROBlock,
	req *ethpb.GenericSignedBeaconBlock,
) ([]*ethpb.BlobSidecar, []blocks.RODataColumn, error) {
	rawBlobs, proofs, err := blobsAndProofs(req)
	if err != nil {
		return nil, nil, err
	}

	if block.Version() >= version.Fulu {
		cellsPerBlob, proofsPerBlob, err := peerdas.ComputeCellsAndProofsFromFlat(rawBlobs, proofs)
		if err != nil {
			return nil, nil, errors.Wrap(err, "compute cells and proofs")
		}

		roDataColumnSidecars, err := peerdas.DataColumnSidecars(cellsPerBlob, proofsPerBlob, peerdas.PopulateFromBlock(block))
		if err != nil {
			return nil, nil, errors.Wrap(err, "data column sidcars")
		}

		return nil, roDataColumnSidecars, nil
	}

	blobSidecars, err := BuildBlobSidecars(block, rawBlobs, proofs)
	if err != nil {
		return nil, nil, errors.Wrap(err, "build blob sidecars")
	}

	return blobSidecars, nil, nil
}

// broadcastReceiveBlock broadcasts a block and handles its reception.
func (b *BlockProducer) broadcastReceiveBlock(ctx context.Context, wg *sync.WaitGroup, block interfaces.SignedBeaconBlock, root [fieldparams.RootLength]byte) error {
	if err := b.broadcastBlock(ctx, wg, block, root); err != nil {
		return errors.Wrap(err, "broadcast block")
	}

	b.Proposer.BlockNotifier.BlockFeed().Send(&feed.Event{
		Type: blockfeed.ReceivedBlock,
		Data: &blockfeed.ReceivedBlockData{SignedBlock: block},
	})

	if err := b.Proposer.BlockReceiver.ReceiveBlock(ctx, block, root, nil); err != nil {
		return errors.Wrap(err, "receive block")
	}

	return nil
}

func (b *BlockProducer) broadcastBlock(ctx context.Context, wg *sync.WaitGroup, block interfaces.SignedBeaconBlock, root [fieldparams.RootLength]byte) error {
	defer wg.Done()

	protoBlock, err := block.Proto()
	if err != nil {
		return errors.Wrap(err, "protobuf conversion failed")
	}
	if err := b.Proposer.P2P.Broadcast(ctx, protoBlock); err != nil {
		return errors.Wrap(err, "broadcast failed")
	}

	log.WithFields(logrus.Fields{
		"slot": block.Block().Slot(),
		"root": fmt.Sprintf("%#x", root),
	}).Debug("Broadcasted block")

	return nil
}

// broadcastAndReceiveBlobs handles the broadcasting and reception of blob sidecars.
func (b *BlockProducer) broadcastAndReceiveBlobs(ctx context.Context, sidecars []*ethpb.BlobSidecar, root [fieldparams.RootLength]byte) error {
	eg, eCtx := errgroup.WithContext(ctx)
	for subIdx, sc := range sidecars {
		eg.Go(func() error {
			if err := b.Proposer.P2P.BroadcastBlob(eCtx, uint64(subIdx), sc); err != nil {
				return errors.Wrap(err, "broadcast blob failed")
			}
			readOnlySc, err := blocks.NewROBlobWithRoot(sc, root)
			if err != nil {
				return errors.Wrap(err, "ROBlob creation failed")
			}
			verifiedBlob := blocks.NewVerifiedROBlob(readOnlySc)
			if err := b.Proposer.BlobReceiver.ReceiveBlob(ctx, verifiedBlob); err != nil {
				return errors.Wrap(err, "receive blob failed")
			}
			b.Proposer.OperationNotifier.OperationFeed().Send(&feed.Event{
				Type: operation.BlobSidecarReceived,
				Data: &operation.BlobSidecarReceivedData{Blob: &verifiedBlob},
			})
			return nil
		})
	}
	return eg.Wait()
}

// broadcastAndReceiveDataColumns handles the broadcasting and reception of data columns sidecars.
func (b *BlockProducer) broadcastAndReceiveDataColumns(ctx context.Context, roSidecars []blocks.RODataColumn) error {
	verifiedSidecars := make([]blocks.VerifiedRODataColumn, 0, len(roSidecars))
	for _, sidecar := range roSidecars {
		verifiedSidecar := blocks.NewVerifiedRODataColumn(sidecar)
		verifiedSidecars = append(verifiedSidecars, verifiedSidecar)
	}

	if err := b.Proposer.P2P.BroadcastDataColumnSidecars(ctx, verifiedSidecars); err != nil {
		return errors.Wrap(err, "broadcast data column sidecars")
	}

	if err := b.Proposer.DataColumnReceiver.ReceiveDataColumns(verifiedSidecars); err != nil {
		return errors.Wrap(err, "receive data columns")
	}

	return nil
}

func blobsAndProofs(req *ethpb.GenericSignedBeaconBlock) ([][]byte, [][]byte, error) {
	switch {
	case req.GetDeneb() != nil:
		dbBlockContents := req.GetDeneb()
		return dbBlockContents.Blobs, dbBlockContents.KzgProofs, nil
	case req.GetElectra() != nil:
		dbBlockContents := req.GetElectra()
		return dbBlockContents.Blobs, dbBlockContents.KzgProofs, nil
	case req.GetFulu() != nil:
		dbBlockContents := req.GetFulu()
		return dbBlockContents.Blobs, dbBlockContents.KzgProofs, nil
	default:
		return nil, nil, errors.Errorf("unknown request type provided: %T", req)
	}
}
