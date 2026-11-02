package sync

import (
	"context"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func (s *Service) pqKeyRegistrationSubscriber(_ context.Context, msg proto.Message) error {
	r, ok := msg.(*ethpb.SignedPQKeyRegistration)
	if !ok {
		return errors.Errorf("incorrect type of message received, wanted %T but got %T", &ethpb.SignedPQKeyRegistration{}, msg)
	}
	s.cfg.pqKeyPool.InsertRegistration(r)
	return nil
}
