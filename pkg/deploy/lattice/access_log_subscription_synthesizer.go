package lattice

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type accessLogSubscriptionSynthesizer struct {
	log                          gwlog.Logger
	client                       client.Client
	accessLogSubscriptionManager AccessLogSubscriptionManager
	stack                        core.Stack
}

func NewAccessLogSubscriptionSynthesizer(
	log gwlog.Logger,
	client client.Client,
	accessLogSubscriptionManager AccessLogSubscriptionManager,
	stack core.Stack,
) *accessLogSubscriptionSynthesizer {
	return &accessLogSubscriptionSynthesizer{
		log:                          log,
		client:                       client,
		accessLogSubscriptionManager: accessLogSubscriptionManager,
		stack:                        stack,
	}
}

func (s *accessLogSubscriptionSynthesizer) Synthesize(ctx context.Context) error {
	var accessLogSubscriptions []*model.AccessLogSubscription
	err := s.stack.ListResources(&accessLogSubscriptions)
	if err != nil {
		return err
	}

	for _, als := range accessLogSubscriptions {
		switch als.Spec.EventType {
		case core.CreateEvent:
			s.log.Debugf("Started creating Access Log Subscription %s", als.ID())
			alsStatus, err := s.accessLogSubscriptionManager.Create(ctx, als)
			if err != nil {
				return err
			}
			als.Status = alsStatus
		case core.UpdateEvent:
			s.log.Debugf("Started updating Access Log Subscription %s", als.ID())
			// TODO
			return nil
		case core.DeleteEvent:
			s.log.Debugf("Started deleting Access Log Subscription %s", als.ID())
			if als.Status == nil {
				s.log.Debugf("Ignoring deletion of Access Log Subscription because als %s has no ARN", als.ID())
				return nil
			}
			err := s.accessLogSubscriptionManager.Delete(ctx, als)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *accessLogSubscriptionSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
