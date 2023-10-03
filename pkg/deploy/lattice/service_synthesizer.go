package lattice

import (
	"context"

	"github.com/aws/aws-application-networking-k8s/pkg/deploy/externaldns"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func NewServiceSynthesizer(
	log gwlog.Logger,
	serviceManager ServiceManager,
	dnsEndpointManager externaldns.DnsEndpointManager,
	stack core.Stack,
	latticeDataStore *latticestore.LatticeDataStore,
) *serviceSynthesizer {
	return &serviceSynthesizer{
		log:                log,
		serviceManager:     serviceManager,
		dnsEndpointManager: dnsEndpointManager,
		stack:              stack,
		latticeDataStore:   latticeDataStore,
	}
}

type serviceSynthesizer struct {
	log                gwlog.Logger
	serviceManager     ServiceManager
	dnsEndpointManager externaldns.DnsEndpointManager
	stack              core.Stack
	latticeDataStore   *latticestore.LatticeDataStore
}

func (s *serviceSynthesizer) Synthesize(ctx context.Context) error {
	var resServices []*model.Service
	s.stack.ListResources(&resServices)

	for _, resService := range resServices {
		s.log.Debugf("Synthesizing service: %s-%s", resService.Spec.Name, resService.Spec.Namespace)
		if resService.Spec.IsDeleted {
			// handle service delete
			err := s.serviceManager.Delete(ctx, resService)

			if err == nil {
				s.log.Debugf("Successfully synthesized service deletion %s-%s", resService.Spec.Name, resService.Spec.Namespace)

				// Also delete all listeners of this service
				listeners, err := s.latticeDataStore.GetAllListeners(resService.Spec.Name, resService.Spec.Namespace)
				if err != nil {
					return err
				}

				for _, l := range listeners {
					err := s.latticeDataStore.DelListener(resService.Spec.Name, resService.Spec.Namespace,
						l.Key.Port, l.Key.Protocol)
					if err != nil {
						s.log.Errorf("Error deleting listener for service %s-%s, port %d, protocol %s: %s",
							resService.Spec.Name, resService.Spec.Namespace, l.Key.Port, l.Key.Protocol, err)
					}
				}
				// Deleting DNSEndpoint is not required, as it has ownership relation.
			}
			return err
		} else {
			serviceStatus, err := s.serviceManager.Create(ctx, resService)
			if err != nil {
				return err
			}

			resService.Status = &serviceStatus
			err = s.dnsEndpointManager.Create(ctx, resService)
			if err != nil {
				return err
			}

			s.log.Debugf("Successfully created service %s-%s with status %s",
				resService.Spec.Name, resService.Spec.Namespace, serviceStatus)
		}
	}

	return nil
}

func (s *serviceSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
