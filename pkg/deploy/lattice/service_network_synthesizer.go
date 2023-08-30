package lattice

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func NewServiceNetworkSynthesizer(
	log gwlog.Logger,
	client client.Client,
	serviceNetworkManager ServiceNetworkManager,
	stack core.Stack,
	latticeDataStore *latticestore.LatticeDataStore,
) *serviceNetworkSynthesizer {
	return &serviceNetworkSynthesizer{
		log:                   log,
		client:                client,
		serviceNetworkManager: serviceNetworkManager,
		stack:                 stack,
		latticeDataStore:      latticeDataStore,
	}
}

type serviceNetworkSynthesizer struct {
	log                   gwlog.Logger
	client                client.Client
	serviceNetworkManager ServiceNetworkManager
	stack                 core.Stack
	latticeDataStore      *latticestore.LatticeDataStore
}

func (s *serviceNetworkSynthesizer) Synthesize(ctx context.Context) error {
	var ret = ""

	s.log.Infof("Start synthesizing ServiceNetworks/Gateways ... ")

	if err := s.synthesizeTriggeredGateways(ctx); err != nil {
		ret = LATTICE_RETRY
	}

	if err := s.synthesizeSDKServiceNetworks(ctx); err != nil {
		ret = LATTICE_RETRY
	}

	//TODO
	// https://github.com/kubernetes-sigs/aws-load-balancer-controller/blob/main/pkg/deploy/elbv2/load_balancer_synthesizer.go#L46
	if ret != "" {
		return errors.New(ret)
	} else {
		s.log.Infof("Finish synthesizing ServiceNetworks/Gateways ... ")
		return nil
	}

}

func (s *serviceNetworkSynthesizer) synthesizeTriggeredGateways(ctx context.Context) error {
	var resServiceNetworks []*latticemodel.ServiceNetwork
	var ret = ""

	s.stack.ListResources(&resServiceNetworks)
	s.log.Infof("Start synthesizing Triggered ServiceNetworks/Gateways ...%v", resServiceNetworks)

	// handling add
	for _, resServiceNetwork := range resServiceNetworks {
		if resServiceNetwork.Spec.IsDeleted {
			s.log.Infof("Synthersing Gateway: Del %v", resServiceNetwork.Spec.Name)

			// TODO need to check if servicenetwork is referenced by gateway in other namespace
			gwList := &gateway_api.GatewayList{}
			s.client.List(context.TODO(), gwList)
			snUsedByGateway := false
			for _, gw := range gwList.Items {
				if gw.Name == resServiceNetwork.Spec.Name &&
					gw.Namespace != resServiceNetwork.Spec.Namespace {
					s.log.Infof("Skip deleting gw %v, namespace %v, since it is still used", gw.Name, gw.Namespace)
					snUsedByGateway = true
					break
				}
			}

			if snUsedByGateway {
				s.log.Infof("Skiping deleting gw: %v since it is still used by gateway(s)",
					resServiceNetwork.Spec.Name)

				continue
			}

			err := s.serviceNetworkManager.Delete(ctx, resServiceNetwork.Spec.Name)
			if err != nil {
				ret = LATTICE_RETRY
			} else {
				s.log.Infof("Synthersing Gateway: successfully deleted gateway %v", resServiceNetwork.Spec.Name)
				s.latticeDataStore.DelServiceNetwork(resServiceNetwork.Spec.Name, resServiceNetwork.Spec.Account)
			}
			continue

		} else {
			s.log.Infof("Synthersing Gateway: Add %v", resServiceNetwork.Spec.Name)
			serviceNetworkStatus, err := s.serviceNetworkManager.Create(ctx, resServiceNetwork)

			if err != nil {
				s.log.Infof("Synthersizing Gateway failed for gateway %v error =%v ", resServiceNetwork.Spec.Name, err)
				// update data store with status
				s.latticeDataStore.AddServiceNetwork(resServiceNetwork.Spec.Name, resServiceNetwork.Spec.Account, serviceNetworkStatus.ServiceNetworkARN, serviceNetworkStatus.ServiceNetworkID, latticestore.DATASTORE_SERVICE_NETWORK_CREATE_IN_PROGRESS)
				ret = LATTICE_RETRY

				continue
			}

			s.log.Infof("Synthersizing Gateway succeeded for gateway %v, status %v", resServiceNetwork.Spec.Name, serviceNetworkStatus)
			s.latticeDataStore.AddServiceNetwork(resServiceNetwork.Spec.Name, resServiceNetwork.Spec.Account, serviceNetworkStatus.ServiceNetworkARN, serviceNetworkStatus.ServiceNetworkID, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
		}
	}

	if ret != "" {
		return errors.New(ret)
	} else {
		return nil
	}

}

func (s *serviceNetworkSynthesizer) synthesizeSDKServiceNetworks(ctx context.Context) error {
	var ret = ""
	sdkServiceNetworks, err := s.serviceNetworkManager.List(ctx)
	if err != nil {
		s.log.Debugf("Synthesize failed on List lattice serviceNetworkes %v", err)
		return err
	}
	s.log.Infof("SDK List: %v", sdkServiceNetworks)

	// handling delete those gateway in lattice DB, but not in K8S DB
	// check local K8S cache
	gwList := &gateway_api.GatewayList{}
	s.client.List(context.TODO(), gwList)

	for _, sdkServiceNetwork := range sdkServiceNetworks {
		s.log.Infof("Synthersizing Gateway: checking if sdkServiceNetwork %v needed to be deleted", sdkServiceNetwork)

		toBeDeleted := false

		snUsedByGateway := false
		for _, gw := range gwList.Items {
			if gw.Name == sdkServiceNetwork {
				snUsedByGateway = true
				break
			}
		}

		if !snUsedByGateway {
			toBeDeleted = true
		}

		if toBeDeleted {

			s.log.Debugf("Synthesizing Gateway: Delete stale sdkServiceNetwork %v", sdkServiceNetwork)
			err := s.serviceNetworkManager.Delete(ctx, sdkServiceNetwork)

			if err != nil {
				s.log.Infof("Need to retry synthesizing err %v", err)
				ret = LATTICE_RETRY
			}
		} else {
			s.log.Infof("Skip deleting sdkServiceNetwork %v since some gateway(s) still reference it",
				sdkServiceNetwork)

		}

	}
	if ret != "" {
		s.log.Debugf("Need to retry synthesizing, reg = %v", ret)
		return errors.New(ret)
	} else {
		return nil
	}
}

func mapResServiceNetworkByResourceID(resServiceNetworks []*latticemodel.ServiceNetwork) map[string]*latticemodel.ServiceNetwork {
	resServiceNetworkByID := make(map[string]*latticemodel.ServiceNetwork, len(resServiceNetworks))

	for _, resServiceNetwork := range resServiceNetworks {
		resServiceNetworkByID[resServiceNetwork.ID()] = resServiceNetwork
	}
	return resServiceNetworkByID
}

func (s *serviceNetworkSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
