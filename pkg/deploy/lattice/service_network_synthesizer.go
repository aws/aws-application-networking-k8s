package lattice

import (
	"context"
	"errors"
	"github.com/golang/glog"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func NewServiceNetworkSynthesizer(client client.Client, serviceNetworkManager ServiceNetworkManager, stack core.Stack) *serviceNetworkSynthesizer {
	return &serviceNetworkSynthesizer{
		Client:                client,
		serviceNetworkManager: serviceNetworkManager,
		stack:                 stack,
	}
}

type serviceNetworkSynthesizer struct {
	client.Client

	serviceNetworkManager ServiceNetworkManager
	stack                 core.Stack
}

func (s *serviceNetworkSynthesizer) Synthesize(ctx context.Context) error {
	var ret = ""

	glog.V(6).Infof("Start synthesizing ServiceNetworks/Gateways ...  \n")

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
		glog.V(6).Infof("Finish synthesizing ServiceNetworks/Gateways ...  \n")
		return nil
	}

}

func (s *serviceNetworkSynthesizer) synthesizeTriggeredGateways(ctx context.Context) error {
	var serviceNetworks []*latticemodel.ServiceNetwork
	var ret = "" // only tracks the last error encountered, others are in the logs

	s.stack.ListResources(&serviceNetworks)
	glog.V(6).Infof("Start synthesizing Triggered ServiceNetworks/Gateways ...%v \n", serviceNetworks)

	for _, resServiceNetwork := range serviceNetworks {
		if resServiceNetwork.Spec.IsDeleted {
			deleteRet := s.deleteServiceNetwork(ctx, resServiceNetwork)
			if deleteRet != "" {
				ret = deleteRet
			}
		} else {
			glog.V(6).Infof("Synthesizing Gateway: Add %v\n", resServiceNetwork.Spec.Name)
			serviceNetworkStatus, err := s.serviceNetworkManager.Create(ctx, resServiceNetwork)

			if err != nil {
				glog.V(6).Infof("Synthesizing Gateway failed for gateway %v error =%v\n ", resServiceNetwork.Spec.Name, err)
				ret = LATTICE_RETRY
			} else {
				glog.V(6).Infof("Synthesizing Gateway succeeded for gateway %v, status %v \n", resServiceNetwork.Spec.Name, serviceNetworkStatus)
			}
		}
	}

	if ret != "" {
		return errors.New(ret)
	} else {
		return nil
	}
}

func (s *serviceNetworkSynthesizer) deleteServiceNetwork(ctx context.Context, resServiceNetwork *latticemodel.ServiceNetwork) string {
	glog.V(6).Infof("Synthesizing Gateway: Del %v\n", resServiceNetwork.Spec.Name)

	// TODO need to check if service network is referenced by gateway in other namespace
	gwList := &gateway_api.GatewayList{}
	s.Client.List(ctx, gwList)
	snUsedByGateway := false
	for _, gw := range gwList.Items {
		if gw.Name == resServiceNetwork.Spec.Name &&
			gw.Namespace != resServiceNetwork.Spec.Namespace {
			glog.V(6).Infof("Skip deleting gw %v, namespace %v, since it is still used", gw.Name, gw.Namespace)
			snUsedByGateway = true
			break
		}
	}

	if snUsedByGateway {
		glog.V(6).Infof("Skipping deleting gw: %v since it is still used by gateway(s)",
			resServiceNetwork.Spec.Name)

		return ""
	}

	err := s.serviceNetworkManager.Delete(ctx, resServiceNetwork.Spec.Name)
	if err != nil {
		glog.V(6).Infof("Synthesizing Gateway delete failed for gateway %v error =%v\n ", resServiceNetwork.Spec.Name, err)
		return LATTICE_RETRY
	} else {
		glog.V(6).Infof("Synthesizing Gateway: successfully deleted gateway %v\n", resServiceNetwork.Spec.Name)
	}

	return ""
}

func (s *serviceNetworkSynthesizer) synthesizeSDKServiceNetworks(ctx context.Context) error {
	var ret = ""
	sdkServiceNetworks, err := s.serviceNetworkManager.List(ctx)
	if err != nil {
		glog.V(2).Infof("Synthesize failed on List lattice serviceNetworkes %v\n", err)
		return err
	}
	glog.V(6).Infof("SDK List: %v \n", sdkServiceNetworks)

	// handling delete those gateway in lattice DB, but not in K8S DB
	// check local K8S cache
	gwList := &gateway_api.GatewayList{}
	s.Client.List(context.TODO(), gwList)

	for _, sdkServiceNetwork := range sdkServiceNetworks {
		glog.V(6).Infof("Synthesizing Gateway: checking if sdkServiceNetwork %v needed to be deleted \n", sdkServiceNetwork)

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

			glog.V(2).Infof("Synthesizing Gateway: Delete stale sdkServiceNetwork %v\n", sdkServiceNetwork)
			err := s.serviceNetworkManager.Delete(ctx, sdkServiceNetwork)

			if err != nil {
				glog.V(6).Infof("Need to retry synthesizing err %v", err)
				ret = LATTICE_RETRY
			}
		} else {
			glog.V(6).Infof("Skip deleting sdkServiceNetwork %v since some gateway(s) still reference it",
				sdkServiceNetwork)

		}

	}
	if ret != "" {
		glog.V(2).Infof("Need to retry synthesizing, reg = %v", ret)
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
