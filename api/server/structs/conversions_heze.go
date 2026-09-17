package structs

import (
	"fmt"

	beaconState "github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// ----------------------------------------------------------------------------
// Heze
// ----------------------------------------------------------------------------

func ROExecutionPayloadBidHezeFromConsensus(b interfaces.ROExecutionPayloadBid) *ExecutionPayloadBidHeze {
	if b == nil {
		return nil
	}

	pbh := b.ParentBlockHash()
	pbr := b.ParentBlockRoot()
	bh := b.BlockHash()
	pr := b.PrevRandao()
	fr := b.FeeRecipient()
	commitments := b.BlobKzgCommitments()
	blobKzgCommitments := make([]string, 0, len(commitments))
	for _, commitment := range commitments {
		blobKzgCommitments = append(blobKzgCommitments, hexutil.Encode(commitment))
	}
	erRoot := b.ExecutionRequestsRoot()
	return &ExecutionPayloadBidHeze{
		ParentBlockHash:       hexutil.Encode(pbh[:]),
		ParentBlockRoot:       hexutil.Encode(pbr[:]),
		BlockHash:             hexutil.Encode(bh[:]),
		PrevRandao:            hexutil.Encode(pr[:]),
		FeeRecipient:          hexutil.Encode(fr[:]),
		GasLimit:              fmt.Sprintf("%d", b.GasLimit()),
		BuilderIndex:          fmt.Sprintf("%d", b.BuilderIndex()),
		Slot:                  fmt.Sprintf("%d", b.Slot()),
		Value:                 fmt.Sprintf("%d", b.Value()),
		ExecutionPayment:      fmt.Sprintf("%d", b.ExecutionPayment()),
		BlobKzgCommitments:    blobKzgCommitments,
		ExecutionRequestsRoot: hexutil.Encode(erRoot[:]),
		InclusionListBits:     hexutil.Encode(b.InclusionListBits()),
	}
}

func BeaconStateHezeFromConsensus(st beaconState.BeaconState) (*BeaconStateHeze, error) {
	srcBr := st.BlockRoots()
	br := make([]string, len(srcBr))
	for i, r := range srcBr {
		br[i] = hexutil.Encode(r)
	}
	srcSr := st.StateRoots()
	sr := make([]string, len(srcSr))
	for i, r := range srcSr {
		sr[i] = hexutil.Encode(r)
	}
	srcHr := st.HistoricalRoots()
	hr := make([]string, len(srcHr))
	for i, r := range srcHr {
		hr[i] = hexutil.Encode(r)
	}
	srcVotes := st.Eth1DataVotes()
	votes := make([]*Eth1Data, len(srcVotes))
	for i, e := range srcVotes {
		votes[i] = Eth1DataFromConsensus(e)
	}
	srcVals := st.Validators()
	vals := make([]*Validator, len(srcVals))
	for i, v := range srcVals {
		vals[i] = ValidatorFromConsensus(v)
	}
	srcBals := st.Balances()
	bals := make([]string, len(srcBals))
	for i, b := range srcBals {
		bals[i] = fmt.Sprintf("%d", b)
	}
	srcRm := st.RandaoMixes()
	rm := make([]string, len(srcRm))
	for i, m := range srcRm {
		rm[i] = hexutil.Encode(m)
	}
	srcSlashings := st.Slashings()
	slashings := make([]string, len(srcSlashings))
	for i, s := range srcSlashings {
		slashings[i] = fmt.Sprintf("%d", s)
	}
	srcPrevPart, err := st.PreviousEpochParticipation()
	if err != nil {
		return nil, err
	}
	prevPart := make([]string, len(srcPrevPart))
	for i, p := range srcPrevPart {
		prevPart[i] = fmt.Sprintf("%d", p)
	}
	srcCurrPart, err := st.CurrentEpochParticipation()
	if err != nil {
		return nil, err
	}
	currPart := make([]string, len(srcCurrPart))
	for i, p := range srcCurrPart {
		currPart[i] = fmt.Sprintf("%d", p)
	}
	srcIs, err := st.InactivityScores()
	if err != nil {
		return nil, err
	}
	is := make([]string, len(srcIs))
	for i, s := range srcIs {
		is[i] = fmt.Sprintf("%d", s)
	}
	currSc, err := st.CurrentSyncCommittee()
	if err != nil {
		return nil, err
	}
	nextSc, err := st.NextSyncCommittee()
	if err != nil {
		return nil, err
	}
	srcHs, err := st.HistoricalSummaries()
	if err != nil {
		return nil, err
	}
	hs := make([]*HistoricalSummary, len(srcHs))
	for i, s := range srcHs {
		hs[i] = HistoricalSummaryFromConsensus(s)
	}
	nwi, err := st.NextWithdrawalIndex()
	if err != nil {
		return nil, err
	}
	nwvi, err := st.NextWithdrawalValidatorIndex()
	if err != nil {
		return nil, err
	}
	drsi, err := st.DepositRequestsStartIndex()
	if err != nil {
		return nil, err
	}
	dbtc, err := st.DepositBalanceToConsume()
	if err != nil {
		return nil, err
	}
	ebtc, err := st.ExitBalanceToConsume()
	if err != nil {
		return nil, err
	}
	eee, err := st.EarliestExitEpoch()
	if err != nil {
		return nil, err
	}
	cbtc, err := st.ConsolidationBalanceToConsume()
	if err != nil {
		return nil, err
	}
	ece, err := st.EarliestConsolidationEpoch()
	if err != nil {
		return nil, err
	}
	pbd, err := st.PendingDeposits()
	if err != nil {
		return nil, err
	}
	ppw, err := st.PendingPartialWithdrawals()
	if err != nil {
		return nil, err
	}
	pc, err := st.PendingConsolidations()
	if err != nil {
		return nil, err
	}
	srcLookahead, err := st.ProposerLookahead()
	if err != nil {
		return nil, err
	}
	lookahead := make([]string, len(srcLookahead))
	for i, v := range srcLookahead {
		lookahead[i] = fmt.Sprintf("%d", uint64(v))
	}
	// Gloas-specific fields
	lepb, err := st.LatestExecutionPayloadBid()
	if err != nil {
		return nil, err
	}
	builders, err := st.Builders()
	if err != nil {
		return nil, err
	}
	nwbi, err := st.NextWithdrawalBuilderIndex()
	if err != nil {
		return nil, err
	}
	epa, err := st.ExecutionPayloadAvailabilityVector()
	if err != nil {
		return nil, err
	}
	bpp, err := st.BuilderPendingPayments()
	if err != nil {
		return nil, err
	}
	bpw, err := st.BuilderPendingWithdrawals()
	if err != nil {
		return nil, err
	}
	lbh, err := st.LatestBlockHash()
	if err != nil {
		return nil, err
	}
	pew, err := st.PayloadExpectedWithdrawals()
	if err != nil {
		return nil, err
	}
	ptcWindow, err := st.PTCWindow()
	if err != nil {
		return nil, err
	}

	return &BeaconStateHeze{
		GenesisTime:                   fmt.Sprintf("%d", st.GenesisTime().Unix()),
		GenesisValidatorsRoot:         hexutil.Encode(st.GenesisValidatorsRoot()),
		Slot:                          fmt.Sprintf("%d", st.Slot()),
		Fork:                          ForkFromConsensus(st.Fork()),
		LatestBlockHeader:             BeaconBlockHeaderFromConsensus(st.LatestBlockHeader()),
		BlockRoots:                    br,
		StateRoots:                    sr,
		HistoricalRoots:               hr,
		Eth1Data:                      Eth1DataFromConsensus(st.Eth1Data()),
		Eth1DataVotes:                 votes,
		Eth1DepositIndex:              fmt.Sprintf("%d", st.Eth1DepositIndex()),
		Validators:                    vals,
		Balances:                      bals,
		RandaoMixes:                   rm,
		Slashings:                     slashings,
		PreviousEpochParticipation:    prevPart,
		CurrentEpochParticipation:     currPart,
		JustificationBits:             hexutil.Encode(st.JustificationBits()),
		PreviousJustifiedCheckpoint:   CheckpointFromConsensus(st.PreviousJustifiedCheckpoint()),
		CurrentJustifiedCheckpoint:    CheckpointFromConsensus(st.CurrentJustifiedCheckpoint()),
		FinalizedCheckpoint:           CheckpointFromConsensus(st.FinalizedCheckpoint()),
		InactivityScores:              is,
		CurrentSyncCommittee:          SyncCommitteeFromConsensus(currSc),
		NextSyncCommittee:             SyncCommitteeFromConsensus(nextSc),
		NextWithdrawalIndex:           fmt.Sprintf("%d", nwi),
		NextWithdrawalValidatorIndex:  fmt.Sprintf("%d", nwvi),
		HistoricalSummaries:           hs,
		DepositRequestsStartIndex:     fmt.Sprintf("%d", drsi),
		DepositBalanceToConsume:       fmt.Sprintf("%d", dbtc),
		ExitBalanceToConsume:          fmt.Sprintf("%d", ebtc),
		EarliestExitEpoch:             fmt.Sprintf("%d", eee),
		ConsolidationBalanceToConsume: fmt.Sprintf("%d", cbtc),
		EarliestConsolidationEpoch:    fmt.Sprintf("%d", ece),
		PendingDeposits:               PendingDepositsFromConsensus(pbd),
		PendingPartialWithdrawals:     PendingPartialWithdrawalsFromConsensus(ppw),
		PendingConsolidations:         PendingConsolidationsFromConsensus(pc),
		ProposerLookahead:             lookahead,
		LatestExecutionPayloadBid:     ROExecutionPayloadBidHezeFromConsensus(lepb),
		Builders:                      BuildersFromConsensus(builders),
		NextWithdrawalBuilderIndex:    fmt.Sprintf("%d", nwbi),
		ExecutionPayloadAvailability:  hexutil.Encode(epa),
		BuilderPendingPayments:        BuilderPendingPaymentsFromConsensus(bpp),
		BuilderPendingWithdrawals:     BuilderPendingWithdrawalsFromConsensus(bpw),
		LatestBlockHash:               hexutil.Encode(lbh[:]),
		PayloadExpectedWithdrawals:    WithdrawalsFromConsensus(pew),
		PtcWindow:                     PTCWindowFromConsensus(ptcWindow),
	}, nil
}
