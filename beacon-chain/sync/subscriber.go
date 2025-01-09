package sync

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/altair"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/peers"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/container/slice"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/messagehandler"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

const pubsubMessageTimeout = 30 * time.Second

// wrappedVal represents a gossip validator which also returns an error along with the result.
type wrappedVal func(context.Context, peer.ID, *pubsub.Message) (pubsub.ValidationResult, error)

// subHandler represents handler for a given subscription.
type subHandler func(context.Context, proto.Message) error

// noopValidator is a no-op that only decodes the message, but does not check its contents.
func (s *Service) noopValidator(_ context.Context, _ peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		log.WithError(err).Debug("Could not decode message")
		return pubsub.ValidationReject, nil
	}
	msg.ValidatorData = m
	return pubsub.ValidationAccept, nil
}

func sliceFromCount(count uint64) []uint64 {
	result := make([]uint64, 0, count)

	for item := range count {
		result = append(result, item)
	}

	return result
}

func (s *Service) activeSyncSubnetIndices(currentSlot primitives.Slot) []uint64 {
	if flags.Get().SubscribeToAllSubnets {
		return sliceFromCount(params.BeaconConfig().SyncCommitteeSubnetCount)
	}

	// Get the current epoch.
	currentEpoch := slots.ToEpoch(currentSlot)

	// Retrieve the subnets we want to subscribe to.
	subs := cache.SyncSubnetIDs.GetAllSubnets(currentEpoch)

	return slice.SetUint64(subs)
}

// Register PubSub subscribers
func (s *Service) registerSubscribers(epoch primitives.Epoch, digest [4]byte) {
	s.subscribe(
		p2p.BlockSubnetTopicFormat,
		s.validateBeaconBlockPubSub,
		s.beaconBlockSubscriber,
		digest,
	)
	s.subscribe(
		p2p.AggregateAndProofSubnetTopicFormat,
		s.validateAggregateAndProof,
		s.beaconAggregateProofSubscriber,
		digest,
	)
	s.subscribe(
		p2p.ExitSubnetTopicFormat,
		s.validateVoluntaryExit,
		s.voluntaryExitSubscriber,
		digest,
	)
	s.subscribe(
		p2p.ProposerSlashingSubnetTopicFormat,
		s.validateProposerSlashing,
		s.proposerSlashingSubscriber,
		digest,
	)
	s.subscribe(
		p2p.AttesterSlashingSubnetTopicFormat,
		s.validateAttesterSlashing,
		s.attesterSlashingSubscriber,
		digest,
	)
	s.subscribeWithParameters(
		p2p.AttestationSubnetTopicFormat,
		s.validateCommitteeIndexBeaconAttestation,
		s.committeeIndexBeaconAttestationSubscriber,
		digest,
		s.persistentAndAggregatorSubnetIndices,
		s.attesterSubnetIndices,
	)
	// Altair fork version
	if params.BeaconConfig().AltairForkEpoch <= epoch {
		s.subscribe(
			p2p.SyncContributionAndProofSubnetTopicFormat,
			s.validateSyncContributionAndProof,
			s.syncContributionAndProofSubscriber,
			digest,
		)
		s.subscribeWithParameters(
			p2p.SyncCommitteeSubnetTopicFormat,
			s.validateSyncCommitteeMessage,
			s.syncCommitteeMessageSubscriber,
			digest,
			s.activeSyncSubnetIndices,
			func(currentSlot primitives.Slot) []uint64 { return []uint64{} },
		)
	}

	// New gossip topic in Capella
	if params.BeaconConfig().CapellaForkEpoch <= epoch {
		s.subscribe(
			p2p.BlsToExecutionChangeSubnetTopicFormat,
			s.validateBlsToExecutionChange,
			s.blsToExecutionChangeSubscriber,
			digest,
		)
	}

	// New gossip topic in Deneb, modified in Electra
	if params.BeaconConfig().DenebForkEpoch <= epoch && epoch < params.BeaconConfig().ElectraForkEpoch {
		s.subscribeWithParameters(
			p2p.BlobSubnetTopicFormat,
			s.validateBlob,
			s.blobSubscriber,
			digest,
			func(currentSlot primitives.Slot) []uint64 {
				return sliceFromCount(params.BeaconConfig().BlobsidecarSubnetCount)
			},
			func(currentSlot primitives.Slot) []uint64 { return []uint64{} },
		)
	}

	// Modified gossip topic in Electra
	if params.BeaconConfig().ElectraForkEpoch <= epoch {
		s.subscribeWithParameters(
			p2p.BlobSubnetTopicFormat,
			s.validateBlob,
			s.blobSubscriber,
			digest,
			func(currentSlot primitives.Slot) []uint64 {
				return sliceFromCount(params.BeaconConfig().BlobsidecarSubnetCountElectra)
			},
			func(currentSlot primitives.Slot) []uint64 { return []uint64{} },
		)
	}
}

// subscribe to a given topic with a given validator and subscription handler.
// The base protobuf message is used to initialize new messages for decoding.
func (s *Service) subscribe(topic string, validator wrappedVal, handle subHandler, digest [4]byte) *pubsub.Subscription {
	genRoot := s.cfg.clock.GenesisValidatorsRoot()
	_, e, err := forks.RetrieveForkDataFromDigest(digest, genRoot[:])
	if err != nil {
		// Impossible condition as it would mean digest does not exist.
		panic(err)
	}
	base := p2p.GossipTopicMappings(topic, e)
	if base == nil {
		// Impossible condition as it would mean topic does not exist.
		panic(fmt.Sprintf("%s is not mapped to any message in GossipTopicMappings", topic))
	}
	return s.subscribeWithBase(s.addDigestToTopic(topic, digest), validator, handle)
}

func (s *Service) subscribeWithBase(topic string, validator wrappedVal, handle subHandler) *pubsub.Subscription {
	topic += s.cfg.p2p.Encoding().ProtocolSuffix()
	log := log.WithField("topic", topic)

	// Do not resubscribe already seen subscriptions.
	ok := s.subHandler.topicExists(topic)
	if ok {
		log.WithField("topic", topic).Debug("Provided topic already has an active subscription running")
		return nil
	}

	if err := s.cfg.p2p.PubSub().RegisterTopicValidator(s.wrapAndReportValidation(topic, validator)); err != nil {
		log.WithError(err).Error("Could not register validator for topic")
		return nil
	}

	sub, err := s.cfg.p2p.SubscribeToTopic(topic)
	if err != nil {
		// Any error subscribing to a PubSub topic would be the result of a misconfiguration of
		// libp2p PubSub library or a subscription request to a topic that fails to match the topic
		// subscription filter.
		log.WithError(err).Error("Could not subscribe topic")
		return nil
	}

	s.subHandler.addTopic(sub.Topic(), sub)

	// Pipeline decodes the incoming subscription data, runs the validation, and handles the
	// message.
	pipeline := func(msg *pubsub.Message) {
		ctx, cancel := context.WithTimeout(s.ctx, pubsubMessageTimeout)
		defer cancel()

		ctx, span := trace.StartSpan(ctx, "sync.pubsub")
		defer span.End()

		defer func() {
			if r := recover(); r != nil {
				tracing.AnnotateError(span, fmt.Errorf("panic occurred: %v", r))
				log.WithField("error", r).
					WithField("recoveredAt", "subscribeWithBase").
					WithField("stack", string(debug.Stack())).
					Error("Panic occurred")
			}
		}()

		span.SetAttributes(trace.StringAttribute("topic", topic))

		if msg.ValidatorData == nil {
			log.Error("Received nil message on pubsub")
			messageFailedProcessingCounter.WithLabelValues(topic).Inc()
			return
		}

		if err := handle(ctx, msg.ValidatorData.(proto.Message)); err != nil {
			tracing.AnnotateError(span, err)
			log.WithError(err).Error("Could not handle p2p pubsub")
			messageFailedProcessingCounter.WithLabelValues(topic).Inc()
			return
		}
	}

	// The main message loop for receiving incoming messages from this subscription.
	messageLoop := func() {
		for {
			msg, err := sub.Next(s.ctx)
			if err != nil {
				// This should only happen when the context is cancelled or subscription is cancelled.
				if !errors.Is(err, pubsub.ErrSubscriptionCancelled) { // Only log a warning on unexpected errors.
					log.WithError(err).Warn("Subscription next failed")
				}
				// Cancel subscription in the event of an error, as we are
				// now exiting topic event loop.
				sub.Cancel()
				return
			}

			if msg.ReceivedFrom == s.cfg.p2p.PeerID() {
				continue
			}

			go pipeline(msg)
		}
	}

	go messageLoop()
	log.WithField("topic", topic).Info("Subscribed to")
	return sub
}

// Wrap the pubsub validator with a metric monitoring function. This function increments the
// appropriate counter if the particular message fails to validate.
func (s *Service) wrapAndReportValidation(topic string, v wrappedVal) (string, pubsub.ValidatorEx) {
	return topic, func(ctx context.Context, pid peer.ID, msg *pubsub.Message) (res pubsub.ValidationResult) {
		defer messagehandler.HandlePanic(ctx, msg)
		// Default: ignore any message that panics.
		res = pubsub.ValidationIgnore // nolint:wastedassign
		ctx, cancel := context.WithTimeout(ctx, pubsubMessageTimeout)
		defer cancel()
		messageReceivedCounter.WithLabelValues(topic).Inc()
		if msg.Topic == nil {
			messageFailedValidationCounter.WithLabelValues(topic).Inc()
			return pubsub.ValidationReject
		}
		// Ignore any messages received before chainstart.
		if s.chainStarted.IsNotSet() {
			messageIgnoredValidationCounter.WithLabelValues(topic).Inc()
			return pubsub.ValidationIgnore
		}
		retDigest, err := p2p.ExtractGossipDigest(topic)
		if err != nil {
			log.WithField("topic", topic).Errorf("Invalid topic format of pubsub topic: %v", err)
			return pubsub.ValidationIgnore
		}
		currDigest, err := s.currentForkDigest()
		if err != nil {
			log.WithField("topic", topic).Errorf("Unable to retrieve fork data: %v", err)
			return pubsub.ValidationIgnore
		}
		if currDigest != retDigest {
			log.WithField("topic", topic).Debugf("Received message from outdated fork digest %#x", retDigest)
			return pubsub.ValidationIgnore
		}
		b, err := v(ctx, pid, msg)
		// We do not penalize peers if we are hitting pubsub timeouts
		// trying to process those messages.
		if b == pubsub.ValidationReject && ctx.Err() != nil {
			b = pubsub.ValidationIgnore
		}
		if b == pubsub.ValidationReject {
			fields := logrus.Fields{
				"topic":        topic,
				"multiaddress": multiAddr(pid, s.cfg.p2p.Peers()),
				"peerID":       pid.String(),
				"agent":        agentString(pid, s.cfg.p2p.Host()),
				"gossipScore":  s.cfg.p2p.Peers().Scorers().GossipScorer().Score(pid),
			}
			if features.Get().EnableFullSSZDataLogging {
				fields["message"] = hexutil.Encode(msg.Data)
			}
			log.WithError(err).WithFields(fields).Debugf("Gossip message was rejected")
			messageFailedValidationCounter.WithLabelValues(topic).Inc()
		}
		if b == pubsub.ValidationIgnore {
			if err != nil && !errorIsIgnored(err) {
				log.WithError(err).WithFields(logrus.Fields{
					"topic":        topic,
					"multiaddress": multiAddr(pid, s.cfg.p2p.Peers()),
					"peerID":       pid.String(),
					"agent":        agentString(pid, s.cfg.p2p.Host()),
					"gossipScore":  fmt.Sprintf("%.2f", s.cfg.p2p.Peers().Scorers().GossipScorer().Score(pid)),
				}).Debug("Gossip message was ignored")
			}
			messageIgnoredValidationCounter.WithLabelValues(topic).Inc()
		}
		return b
	}
}

// reValidateSubscriptions unsubscribe from topics we are currently subscribed to but that are
// not in the list of wanted subnets.
// TODO: Rename this functions as it does not only revalidate subscriptions.
func (s *Service) reValidateSubscriptions(
	subscriptions map[uint64]*pubsub.Subscription,
	wantedSubs []uint64,
	topicFormat string,
	digest [4]byte,
) {
	for k, v := range subscriptions {
		var wanted bool
		for _, idx := range wantedSubs {
			if k == idx {
				wanted = true
				break
			}
		}

		if !wanted && v != nil {
			v.Cancel()
			fullTopic := fmt.Sprintf(topicFormat, digest, k) + s.cfg.p2p.Encoding().ProtocolSuffix()
			s.unSubscribeFromTopic(fullTopic)
			delete(subscriptions, k)
		}
	}
}

// searchForPeers searches for peers in the given subnets.
func (s *Service) searchForPeers(
	ctx context.Context,
	topicFormat string,
	digest [4]byte,
	currentSlot primitives.Slot,
	getSubnetsToSubscribe func(currentSlot primitives.Slot) []uint64,
	getSubnetsToFindPeersOnly func(currentSlot primitives.Slot) []uint64,
) {
	// Retrieve the subnets we want to subscribe to.
	subnetsToSubscribeIndex := getSubnetsToSubscribe(currentSlot)

	// Retrieve the subnets we want to find peers for.
	subnetsToFindPeersOnlyIndex := getSubnetsToFindPeersOnly(currentSlot)

	// Combine the subnets to subscribe and the subnets to find peers for.
	subnetsToFindPeersIndex := slice.SetUint64(append(subnetsToSubscribeIndex, subnetsToFindPeersOnlyIndex...))

	// Find new peers for wanted subnets if needed.
	for _, subnetIndex := range subnetsToFindPeersIndex {
		topic := fmt.Sprintf(topicFormat, digest, subnetIndex)

		// Check if we have enough peers in the subnet. Skip if we do.
		if s.enoughPeersAreConnected(topic) {
			continue
		}

		// Not enough peers in the subnet, we need to search for more.
		_, err := s.cfg.p2p.FindPeersWithSubnet(ctx, topic, subnetIndex, flags.Get().MinimumPeersPerSubnet)
		if err != nil {
			log.WithError(err).Debug("Could not search for peers")
		}
	}
}

// subscribeToSubnets subscribes to needed subnets, unsubscribe from unneeded ones and search for more peers if needed.
// Returns `true` if the digest is valid (wrt. the current epoch), `false` otherwise.
func (s *Service) subscribeToSubnets(
	topicFormat string,
	digest [4]byte,
	genesisValidatorsRoot [fieldparams.RootLength]byte,
	genesisTime time.Time,
	subscriptions map[uint64]*pubsub.Subscription,
	currentSlot primitives.Slot,
	validate wrappedVal,
	handle subHandler,
	getSubnetsToSubscribe func(currentSlot primitives.Slot) []uint64,
	getSubnetsToFindPeersOnly func(currentSlot primitives.Slot) []uint64,
) bool {
	// Do not subscribe if not synced.
	if s.chainStarted.IsSet() && s.cfg.initialSync.Syncing() {
		return true
	}

	// Check the validity of the digest.
	valid, err := isDigestValid(digest, genesisTime, genesisValidatorsRoot)
	if err != nil {
		log.Error(err)
		return true
	}

	// Unsubscribe from all subnets if the digest is not valid. It's likely to be the case after a hard fork.
	if !valid {
		description := topicFormat
		if pos := strings.LastIndex(topicFormat, "/"); pos != -1 {
			description = topicFormat[pos+1:]
		}

		if pos := strings.LastIndex(description, "_"); pos != -1 {
			description = description[:pos]
		}

		log.WithFields(logrus.Fields{
			"digest":  fmt.Sprintf("%#x", digest),
			"subnets": description,
		}).Debug("Subnets with this digest are no longer valid, unsubscribing from all of them")
		s.reValidateSubscriptions(subscriptions, []uint64{}, topicFormat, digest)
		return false
	}

	// Retrieve the subnets we want to subscribe to.
	subnetsToSubscribeIndex := getSubnetsToSubscribe(currentSlot)

	// Remove subscriptions that are no longer wanted.
	s.reValidateSubscriptions(subscriptions, subnetsToSubscribeIndex, topicFormat, digest)

	// Subscribe to wanted subnets.
	for _, subnetIndex := range subnetsToSubscribeIndex {
		subnetTopic := fmt.Sprintf(topicFormat, digest, subnetIndex)

		// Check if subscription exists.
		if _, exists := subscriptions[subnetIndex]; exists {
			continue
		}

		// We need to subscribe to the subnet.
		subscription := s.subscribeWithBase(subnetTopic, validate, handle)
		subscriptions[subnetIndex] = subscription
	}
	return true
}

// subscribeWithParameters subscribes to a list of subnets.
func (s *Service) subscribeWithParameters(
	topicFormat string,
	validate wrappedVal,
	handle subHandler,
	digest [4]byte,
	getSubnetsToSubscribe func(currentSlot primitives.Slot) []uint64,
	getSubnetsToFindPeersOnly func(currentSlot primitives.Slot) []uint64,
) {
	// Initialize the subscriptions map.
	subscriptions := make(map[uint64]*pubsub.Subscription)

	// Retrieve the genesis validators root.
	genesisValidatorsRoot := s.cfg.clock.GenesisValidatorsRoot()

	// Retrieve the epoch of the fork corresponding to the digest.
	_, epoch, err := forks.RetrieveForkDataFromDigest(digest, genesisValidatorsRoot[:])
	if err != nil {
		panic(err)
	}

	// Retrieve the base protobuf message.
	base := p2p.GossipTopicMappings(topicFormat, epoch)
	if base == nil {
		panic(fmt.Sprintf("%s is not mapped to any message in GossipTopicMappings", topicFormat))
	}

	// Retrieve the genesis time.
	genesisTime := s.cfg.clock.GenesisTime()

	// Define a ticker ticking every slot.
	secondsPerSlot := params.BeaconConfig().SecondsPerSlot
	ticker := slots.NewSlotTicker(genesisTime, secondsPerSlot)

	// Retrieve the current slot.
	currentSlot := s.cfg.clock.CurrentSlot()

	// Subscribe to subnets.
	s.subscribeToSubnets(topicFormat, digest, genesisValidatorsRoot, genesisTime, subscriptions, currentSlot, validate, handle, getSubnetsToSubscribe, getSubnetsToFindPeersOnly)

	// Derive a new context and cancel function.
	ctx, cancel := context.WithCancel(s.ctx)

	go func() {
		// Search for peers.
		s.searchForPeers(ctx, topicFormat, digest, currentSlot, getSubnetsToSubscribe, getSubnetsToFindPeersOnly)

		for {
			select {
			case currentSlot := <-ticker.C():
				isDigestValid := s.subscribeToSubnets(topicFormat, digest, genesisValidatorsRoot, genesisTime, subscriptions, currentSlot, validate, handle, getSubnetsToSubscribe, getSubnetsToFindPeersOnly)

				// Stop the ticker if the digest is not valid. Likely to happen after a hard fork.
				if !isDigestValid {
					ticker.Done()
					return
				}

				// Search for peers.
				s.searchForPeers(ctx, topicFormat, digest, currentSlot, getSubnetsToSubscribe, getSubnetsToFindPeersOnly)

			case <-s.ctx.Done():
				cancel()
				ticker.Done()
				return
			}
		}
	}()
}

func (s *Service) unSubscribeFromTopic(topic string) {
	log.WithField("topic", topic).Info("Unsubscribed from")
	if err := s.cfg.p2p.PubSub().UnregisterTopicValidator(topic); err != nil {
		log.WithError(err).Error("Could not unregister topic validator")
	}
	sub := s.subHandler.subForTopic(topic)
	if sub != nil {
		sub.Cancel()
	}
	s.subHandler.removeTopic(topic)
	if err := s.cfg.p2p.LeaveTopic(topic); err != nil {
		log.WithError(err).Error("Unable to leave topic")
	}
}

// enoughPeersAreConnected checks if we have enough peers which are subscribed to the same subnet.
func (s *Service) enoughPeersAreConnected(subnetTopic string) bool {
	topic := subnetTopic + s.cfg.p2p.Encoding().ProtocolSuffix()
	threshold := flags.Get().MinimumPeersPerSubnet

	peersWithSubnet := s.cfg.p2p.PubSub().ListPeers(topic)
	peersWithSubnetCount := len(peersWithSubnet)

	return peersWithSubnetCount >= threshold
}

func (s *Service) persistentAndAggregatorSubnetIndices(currentSlot primitives.Slot) []uint64 {
	if flags.Get().SubscribeToAllSubnets {
		return sliceFromCount(params.BeaconConfig().AttestationSubnetCount)
	}

	persistentSubnetIndices := s.persistentSubnetIndices()
	aggregatorSubnetIndices := s.aggregatorSubnetIndices(currentSlot)

	// Combine subscriptions to get all requested subscriptions.
	return slice.SetUint64(append(persistentSubnetIndices, aggregatorSubnetIndices...))
}

// filters out required peers for the node to function, not
// pruning peers who are in our attestation subnets.
func (s *Service) filterNeededPeers(pids []peer.ID) []peer.ID {
	// Exit early if nothing to filter.
	if len(pids) == 0 {
		return pids
	}
	digest, err := s.currentForkDigest()
	if err != nil {
		log.WithError(err).Error("Could not compute fork digest")
		return pids
	}
	currSlot := s.cfg.clock.CurrentSlot()
	wantedSubs := s.persistentAndAggregatorSubnetIndices(currSlot)
	wantedSubs = slice.SetUint64(append(wantedSubs, s.attesterSubnetIndices(currSlot)...))
	topic := p2p.GossipTypeMapping[reflect.TypeOf(&ethpb.Attestation{})]

	// Map of peers in subnets
	peerMap := make(map[peer.ID]bool)

	for _, sub := range wantedSubs {
		subnetTopic := fmt.Sprintf(topic, digest, sub) + s.cfg.p2p.Encoding().ProtocolSuffix()
		ps := s.cfg.p2p.PubSub().ListPeers(subnetTopic)
		if len(ps) > flags.Get().MinimumPeersPerSubnet {
			// In the event we have more than the minimum, we can
			// mark the remaining as viable for pruning.
			ps = ps[:flags.Get().MinimumPeersPerSubnet]
		}
		// Add peer to peer map.
		for _, p := range ps {
			// Even if the peer id has
			// already been seen we still set
			// it, as the outcome is the same.
			peerMap[p] = true
		}
	}

	// Clear out necessary peers from the peers to prune.
	newPeers := make([]peer.ID, 0, len(pids))

	for _, pid := range pids {
		if peerMap[pid] {
			continue
		}
		newPeers = append(newPeers, pid)
	}
	return newPeers
}

// Add fork digest to topic.
func (*Service) addDigestToTopic(topic string, digest [4]byte) string {
	if !strings.Contains(topic, "%x") {
		log.Error("Topic does not have appropriate formatter for digest")
	}
	return fmt.Sprintf(topic, digest)
}

// Add the digest and index to subnet topic.
func (*Service) addDigestAndIndexToTopic(topic string, digest [4]byte, idx uint64) string {
	if !strings.Contains(topic, "%x") {
		log.Error("Topic does not have appropriate formatter for digest")
	}
	return fmt.Sprintf(topic, digest, idx)
}

func (s *Service) currentForkDigest() ([4]byte, error) {
	genRoot := s.cfg.clock.GenesisValidatorsRoot()
	return forks.CreateForkDigest(s.cfg.clock.GenesisTime(), genRoot[:])
}

// Checks if the provided digest matches up with the current supposed digest.
func isDigestValid(digest [4]byte, genesis time.Time, genValRoot [32]byte) (bool, error) {
	retDigest, err := forks.CreateForkDigest(genesis, genValRoot[:])
	if err != nil {
		return false, err
	}
	isNextEpoch, err := forks.IsForkNextEpoch(genesis, genValRoot[:])
	if err != nil {
		return false, err
	}
	// In the event there is a fork the next epoch,
	// we skip the check, as we subscribe subnets an
	// epoch in advance.
	if isNextEpoch {
		return true, nil
	}
	return retDigest == digest, nil
}

func agentString(pid peer.ID, hst host.Host) string {
	rawVersion, storeErr := hst.Peerstore().Get(pid, "AgentVersion")
	agString, ok := rawVersion.(string)
	if storeErr != nil || !ok {
		agString = ""
	}
	return agString
}

func multiAddr(pid peer.ID, stat *peers.Status) string {
	addrs, err := stat.Address(pid)
	if err != nil || addrs == nil {
		return ""
	}
	return addrs.String()
}

func errorIsIgnored(err error) bool {
	if errors.Is(err, helpers.ErrTooLate) {
		return true
	}
	if errors.Is(err, altair.ErrTooLate) {
		return true
	}
	return false
}
