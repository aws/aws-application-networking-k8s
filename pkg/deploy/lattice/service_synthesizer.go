package lattice

import (
	"context"
	"github.com/golang/glog"

	"github.com/aws/aws-application-networking-k8s/pkg/deploy/externaldns"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func NewServiceSynthesizer(serviceManager ServiceManager, dnsEndpointManager externaldns.DnsEndpointManager,
	stack core.Stack, latticeDataStore *latticestore.LatticeDataStore) *serviceSynthesizer {
	return &serviceSynthesizer{
		serviceManager:     serviceManager,
		dnsEndpointManager: dnsEndpointManager,
		stack:              stack,
		latticeDataStore:   latticeDataStore,
	}
}

type serviceSynthesizer struct {
	serviceManager     ServiceManager
	dnsEndpointManager externaldns.DnsEndpointManager
	stack              core.Stack
	latticeDataStore   *latticestore.LatticeDataStore
}

func (s *serviceSynthesizer) Synthesize(ctx context.Context) error {
	var resServices []*latticemodel.Service

	s.stack.ListResources(&resServices)
	glog.V(6).Infof("Service-synthesize start: %v \n", resServices)

	// TODO
	for _, resService := range resServices {

		glog.V(6).Infof("Synthesize Service/HTTPRoute: %v\n", resService)
		if resService.Spec.IsDeleted {
			// handle service delete
			err := s.serviceManager.Delete(ctx, resService)

			if err == nil {
				glog.V(6).Infof("service - Synthesizer: finish deleting service %v\n", *resService)
				s.latticeDataStore.DelLatticeService(resService.Spec.Name, resService.Spec.Namespace)

				// also delete all listeners of this service
				listeners, err := s.latticeDataStore.GetAllListeners(resService.Spec.Name, resService.Spec.Namespace)

				glog.V(6).Infof("service synthesize -- need to delete listeners %v, err : %v\n", listeners, err)

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
				glog.V(6).Infof("Error on s.serviceManager.Create %v \n", err)
				return err
			}

			s.latticeDataStore.AddLatticeService(resService.Spec.Name, resService.Spec.Namespace,
				serviceStatus.ServiceARN, serviceStatus.ServiceID, serviceStatus.ServiceDNS)

			glog.V(6).Infof("Creating Service DNS")
			resService.Status = &serviceStatus
			err = s.dnsEndpointManager.Create(ctx, resService)
			if err != nil {
				glog.V(6).Infof("Failed creating service DNS: %v", err)
				return err
			}

			glog.V(6).Infof("serviceStatus %v, error = %v \n", serviceStatus, err)
		}
	}

	glog.V(6).Infof("Service-synthesize end %v \n", resServices)

	return nil
}

func (s *serviceSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
