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

	var ret error = nil
	for _, als := range accessLogSubscriptions {
		if !als.Spec.IsDeleted {
			s.log.Debugf("Started creating or updating access log subscription %s", als.ID())
			_, err := s.accessLogSubscriptionManager.Create(ctx, als)
			if err != nil {
				s.log.Debugf("Synthesizing access log subscription %s failed due to %s", als.ID(), err)
				ret = err
			}
		} else {
			s.log.Debugf("Started deleting access log subscription %s", als.ID())
			// TODO
		}
	}

	return ret
}

func (s *accessLogSubscriptionSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
