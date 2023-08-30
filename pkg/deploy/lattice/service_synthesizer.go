package lattice

import (
	"context"

	"github.com/aws/aws-application-networking-k8s/pkg/deploy/externaldns"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
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
	var resServices []*latticemodel.Service

	s.stack.ListResources(&resServices)
	s.log.Infof("Service-synthesize start: %v", resServices)

	// TODO
	for _, resService := range resServices {

		s.log.Infof("Synthesize Service/HTTPRoute: %v", resService)
		if resService.Spec.IsDeleted {
			// handle service delete
			err := s.serviceManager.Delete(ctx, resService)

			if err == nil {
				s.log.Infof("service - Synthesizer: finish deleting service %v", *resService)
				s.latticeDataStore.DelLatticeService(resService.Spec.Name, resService.Spec.Namespace)

				// also delete all listeners of this service
				listeners, err := s.latticeDataStore.GetAllListeners(resService.Spec.Name, resService.Spec.Namespace)

				s.log.Infof("service synthesize -- need to delete listeners %v, err : %v", listeners, err)

				for _, l := range listeners {
					s.latticeDataStore.DelListener(resService.Spec.Name, resService.Spec.Namespace,
						l.Key.Port, l.Key.Protocol)
				}

				// Deleting DNSEndpoint is not required, as it has ownership relation.
			}
			return err
		} else {

			serviceStatus, err := s.serviceManager.Create(ctx, resService)

			if err != nil {
				s.log.Infof("Error on s.serviceManager.Create %v", err)
				return err
			}

			s.latticeDataStore.AddLatticeService(resService.Spec.Name, resService.Spec.Namespace,
				serviceStatus.Arn, serviceStatus.Id, serviceStatus.Dns)

			s.log.Infof("Creating Service DNS")
			resService.Status = &serviceStatus
			err = s.dnsEndpointManager.Create(ctx, resService)
			if err != nil {
				s.log.Debugf("Failed creating service DNS: %v", err)
				return err
			}

			s.log.Infof("serviceStatus %v, error = %v", serviceStatus, err)
		}
	}

	s.log.Infof("Service-synthesize end %v", resServices)

	return nil
}

func (s *serviceSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
