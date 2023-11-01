package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/deploy/externaldns"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func NewServiceSynthesizer(
	log gwlog.Logger,
	serviceManager ServiceManager,
	dnsEndpointManager externaldns.DnsEndpointManager,
	stack core.Stack,
) *serviceSynthesizer {
	return &serviceSynthesizer{
		log:                log,
		serviceManager:     serviceManager,
		dnsEndpointManager: dnsEndpointManager,
		stack:              stack,
	}
}

type serviceSynthesizer struct {
	log                gwlog.Logger
	serviceManager     ServiceManager
	dnsEndpointManager externaldns.DnsEndpointManager
	stack              core.Stack
}

func (s *serviceSynthesizer) Synthesize(ctx context.Context) error {
	var resServices []*model.Service
	s.stack.ListResources(&resServices)

	var svcErr error
	for _, resService := range resServices {
		svcName := fmt.Sprintf("%s-%s", resService.Spec.RouteName, resService.Spec.RouteNamespace)
		s.log.Debugf("Synthesizing service: %s", svcName)
		if resService.IsDeleted {
			err := s.serviceManager.Delete(ctx, resService)
			if err != nil {
				svcErr = errors.Join(svcErr,
					fmt.Errorf("failed ServiceManager.Delete %s due to %s", svcName, err))
				continue
			}
		} else {
			serviceStatus, err := s.serviceManager.Upsert(ctx, resService)
			if err != nil {
				svcErr = errors.Join(svcErr,
					fmt.Errorf("failed ServiceManager.Upsert %s due to %s", svcName, err))
				continue
			}

			resService.Status = &serviceStatus
			err = s.dnsEndpointManager.Create(ctx, resService)
			if err != nil {
				svcErr = errors.Join(svcErr,
					fmt.Errorf("failed DnsEndpointManager.Create %s due to %s", svcName, err))

				svcErr = err
				continue
			}
		}
	}

	return svcErr
}

func (s *serviceSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
