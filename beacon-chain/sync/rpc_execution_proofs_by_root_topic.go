package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	libp2pcore "github.com/libp2p/go-libp2p/core"
)

// SendExecutionProofsByRootRequest sends ExecutionProofsByRoot and returns fetched execution proofs, if any.
func SendExecutionProofsByRootRequest(
	ctx context.Context,
	blk blocks.ROBlock,
	alreadyHave []primitives.ExecutionProofId,
) ([]*ethpb.ExecutionProof, error) {
	// Return error if we already have enough proofs.
	alreadyHaveCount := uint64(len(alreadyHave))
	countNeeded := params.BeaconConfig().MinProofsRequired
	if alreadyHaveCount >= countNeeded {
		return nil, fmt.Errorf("already have enough proofs: have %d, need %d", len(alreadyHave), countNeeded)
	}
	countNeeded -= alreadyHaveCount

	// Construct the request.
	blockRoot := blk.Root()
	req := &ethpb.ExecutionProofsByRootRequest{
		BlockRoot:   blockRoot[:],
		CountNeeded: countNeeded,
		AlreadyHave: alreadyHave,
	}
	fmt.Printf("Req.BlockRoot: %#x\n", req.BlockRoot)

	return []*ethpb.ExecutionProof{}, nil
}

// executionProofsByRootRPCHandler looks up the request blocks from the database from the given block roots.
func (s *Service) executionProofsByRootRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	_, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	// log := log.WithField("handler", "execution_proof_by_root")

	_, ok := msg.(*ethpb.ExecutionProofsByRootRequest)
	if !ok {
		return errors.New("message is not type ExecutionProofsByRootRequest")
	}
	// blockRoots := *rawMsg
	// if err := s.rateLimiter.validateRequest(stream, uint64(len(blockRoots))); err != nil {
	// 	return err
	// }
	// if len(blockRoots) == 0 {
	// 	// Add to rate limiter in the event no
	// 	// roots are requested.
	// 	s.rateLimiter.add(stream, 1)
	// 	s.writeErrorResponseToStream(responseCodeInvalidRequest, "no block roots provided in request", stream)
	// 	return errors.New("no block roots provided")
	// }

	// s.rateLimiter.add(stream, int64(len(blockRoots)))

	// for _, root := range blockRoots {
	// 	blk, err := s.cfg.beaconDB.Block(ctx, root)
	// 	if err != nil {
	// 		log.WithError(err).Debug("Could not fetch block")
	// 		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
	// 		return err
	// 	}
	// 	if err := blocks.BeaconBlockIsNil(blk); err != nil {
	// 		continue
	// 	}

	// 	if blk.Block().IsBlinded() {
	// 		blk, err = s.cfg.executionReconstructor.ReconstructFullBlock(ctx, blk)
	// 		if err != nil {
	// 			if errors.Is(err, execution.ErrEmptyBlockHash) {
	// 				log.WithError(err).Warn("Could not reconstruct block from header with syncing execution client. Waiting to complete syncing")
	// 			} else {
	// 				log.WithError(err).Error("Could not get reconstruct full block from blinded body")
	// 			}
	// 			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
	// 			return err
	// 		}
	// 	}

	// 	if err := s.chunkBlockWriter(stream, blk); err != nil {
	// 		return err
	// 	}
	// }

	// closeStream(stream, log)
	return nil
}
