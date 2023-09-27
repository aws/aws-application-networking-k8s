package lattice

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func NewServiceNetworkSynthesizer(
	log gwlog.Logger,
	client client.Client,
	serviceNetworkManager ServiceNetworkManager,
	stack core.Stack,
) *serviceNetworkSynthesizer {
	return &serviceNetworkSynthesizer{
		log:                   log,
		client:                client,
		serviceNetworkManager: serviceNetworkManager,
		stack:                 stack,
	}
}

type serviceNetworkSynthesizer struct {
	log                   gwlog.Logger
	client                client.Client
	serviceNetworkManager ServiceNetworkManager
	stack                 core.Stack
}

func (s *serviceNetworkSynthesizer) Synthesize(ctx context.Context) error {
	var ret = ""

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
		return nil
	}
}

func (s *serviceNetworkSynthesizer) synthesizeTriggeredGateways(ctx context.Context) error {
	var serviceNetworks []*latticemodel.ServiceNetwork
	var ret = "" // only tracks the last error encountered, others are in the logs

	s.stack.ListResources(&serviceNetworks)

	for _, resServiceNetwork := range serviceNetworks {
		s.log.Debugf("Start synthesizing service network %s", resServiceNetwork.Spec.Name)
		if resServiceNetwork.Spec.IsDeleted {
			deleteRet := s.deleteServiceNetwork(ctx, resServiceNetwork)
			if deleteRet != "" {
				ret = deleteRet
			}
		} else {
			s.log.Debugf("Synthesizing Gateway %s", resServiceNetwork.Spec.Name)
			serviceNetworkStatus, err := s.serviceNetworkManager.Create(ctx, resServiceNetwork)
			if err != nil {
				s.log.Debugf("Synthesizing Gateway failed for gateway %s due to %s",
					resServiceNetwork.Spec.Name, err)
				ret = LATTICE_RETRY
			} else {
				s.log.Debugf("Synthesizing Gateway succeeded for gateway %s, status %s",
					resServiceNetwork.Spec.Name, serviceNetworkStatus)
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
	s.log.Debugf("Synthesizing Gateway deletion for service network %s", resServiceNetwork.Spec.Name)

	// TODO need to check if service network is referenced by gateway in other namespace
	gwList := &gateway_api.GatewayList{}
	s.client.List(ctx, gwList)
	snUsedByGateway := false
	for _, gw := range gwList.Items {
		if gw.Name == resServiceNetwork.Spec.Name &&
			gw.Namespace != resServiceNetwork.Spec.Namespace {
			s.log.Debugf("Skip deleting gateway %s-%s, since it is still used", gw.Name, gw.Namespace)
			snUsedByGateway = true
			break
		}
	}

	if snUsedByGateway {
		return ""
	}

	err := s.serviceNetworkManager.Delete(ctx, resServiceNetwork.Spec.Name)
	if err != nil {
		s.log.Debugf("Synthesizing Gateway delete failed for gateway %s due to %s",
			resServiceNetwork.Spec.Name, err)
		return LATTICE_RETRY
	} else {
		s.log.Debugf("Synthesizing gateway deletion for service network %s succeeded", resServiceNetwork.Spec.Name)
	}

	return ""
}

func (s *serviceNetworkSynthesizer) synthesizeSDKServiceNetworks(ctx context.Context) error {
	var ret = ""
	sdkServiceNetworks, err := s.serviceNetworkManager.List(ctx)
	if err != nil {
		return err
	}

	// handling delete those gateway in lattice DB, but not in K8S DB
	// check local K8S cache
	gwList := &gateway_api.GatewayList{}
	s.client.List(context.TODO(), gwList)

	for _, sdkServiceNetwork := range sdkServiceNetworks {
		s.log.Debugf("Checking if service network %s needs to be deleted during gateway synthesis", sdkServiceNetwork)

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
			s.log.Debugf("Deleting stale service network %s during gateway synthesis", sdkServiceNetwork)
			err := s.serviceNetworkManager.Delete(ctx, sdkServiceNetwork)
			if err != nil {
				s.log.Debugf("Need to retry synthesizing service network %s due to err %s", sdkServiceNetwork, err)
				ret = LATTICE_RETRY
			}
		} else {
			s.log.Debugf("Skip deleting service network %s since some gateway(s) still reference it",
				sdkServiceNetwork)
		}

	}
	if ret != "" {
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
