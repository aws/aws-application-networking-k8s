package lattice

import (
	"context"
	"github.com/golang/glog"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func NewServiceSynthesizer(serviceManager ServiceManager, stack core.Stack, latticeDataStore *latticestore.LatticeDataStore) *serviceSynthesizer {
	return &serviceSynthesizer{
		serviceManager:   serviceManager,
		stack:            stack,
		latticeDataStore: latticeDataStore,
	}
}

type serviceSynthesizer struct {
	serviceManager   ServiceManager
	stack            core.Stack
	latticeDataStore *latticestore.LatticeDataStore
}

func (s *serviceSynthesizer) SynthesizeCreation(ctx context.Context) error {
	var resServices []*latticemodel.Service

	s.stack.ListResources(&resServices)
	glog.V(6).Infof("Service-synthesize start: %v \n", resServices)

	// TODO
	for _, resService := range resServices {

		glog.V(6).Infof("SynthesizeCreation Service/HTTPRoute: %v\n", resService)
		if resService.Spec.IsDeleted {
			// handle latticeService creation request in SynthesizeCreation() only, will handle the deletion request in SynthesizeDeletion()
			continue
		}

		serviceStatus, err := s.serviceManager.Create(ctx, resService)

		if err != nil {
			glog.V(6).Infof("Error on s.serviceManager.Create %v \n", err)
			return err
		}

		s.latticeDataStore.AddLatticeService(resService.Spec.Name, resService.Spec.Namespace,
			serviceStatus.ServiceARN, serviceStatus.ServiceID, serviceStatus.ServiceDNS)

		glog.V(6).Infof("serviceStatus %v, error = %v \n", serviceStatus, err)
	}

	glog.V(6).Infof("Service-synthesize end %v \n", resServices)

	return nil
}

func (s *serviceSynthesizer) SynthesizeDeletion(ctx context.Context) error {
	var resServices []*latticemodel.Service

	s.stack.ListResources(&resServices)
	glog.V(6).Infof("Service SynthesizeDeletion(Deletion) start: %v \n", resServices)

	for _, resService := range resServices {
		if resService.Spec.IsDeleted {
			// In serviceSynthesizer.SynthesizeDeletion(), we only handle the deletion request,
			// ignore the creation request which is handled in SynthesizeCreation()
			if err := s.serviceManager.Delete(ctx, resService); err == nil {
				glog.V(6).Infof("service  SynthesizeDeletion: finish deleting service %v\n", *resService)
				s.latticeDataStore.DelLatticeService(resService.Spec.Name, resService.Spec.Namespace)
			} else {
				return err
			}
		}
	}
	return nil
}
