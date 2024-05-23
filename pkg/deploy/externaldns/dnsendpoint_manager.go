package externaldns

import (
	"context"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/external-dns/endpoint"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

//go:generate mockgen -destination dnsendpoint_manager_mock.go -package externaldns github.com/aws/aws-application-networking-k8s/pkg/deploy/externaldns DnsEndpointManager

type DnsEndpointManager interface {
	Create(ctx context.Context, service *latticemodel.Service) error
}

type defaultDnsEndpointManager struct {
	log       gwlog.Logger
	k8sClient client.Client
}

func NewDnsEndpointManager(log gwlog.Logger, k8sClient client.Client) *defaultDnsEndpointManager {
	return &defaultDnsEndpointManager{
		log:       log,
		k8sClient: k8sClient,
	}
}

func (s *defaultDnsEndpointManager) Create(ctx context.Context, service *latticemodel.Service) error {
	namespacedName := types.NamespacedName{
		Namespace: service.Spec.RouteNamespace,
		Name:      service.Spec.RouteName + "-dns",
	}
	if service.Spec.CustomerDomainName == "" {
		s.log.Debugf("Skipping creation of %s: detected no custom domain", namespacedName)
		return nil
	}
	if service.Status == nil || service.Status.Dns == "" {
		s.log.Debugf("Skipping creation of %s: DNS target not ready in svc status", namespacedName)
		return nil
	}

	var (
		route core.Route
		err   error
	)
	routeNamespacedName := types.NamespacedName{
		Namespace: service.Spec.RouteNamespace,
		Name:      service.Spec.RouteName,
	}
	if service.Spec.RouteType == core.GrpcRouteType {
		route, err = core.GetGRPCRoute(ctx, s.k8sClient, routeNamespacedName)
	} else if service.Spec.RouteType == core.TlsRouteType {
		route, err = core.GetTLSRoute(ctx, s.k8sClient, routeNamespacedName)
	} else {
		route, err = core.GetHTTPRoute(ctx, s.k8sClient, routeNamespacedName)
	}
	if err != nil {
		s.log.Debugf("Skipping creation of %s: Could not find corresponding route", namespacedName.String())
		return nil
	}

	ep := &endpoint.DNSEndpoint{}
	if err := s.k8sClient.Get(ctx, namespacedName, ep); err != nil {
		if apierrors.IsNotFound(err) {
			s.log.Debugf("Attempting creation of DNSEndpoint for %s - %s -> %s",
				namespacedName.String(), service.Spec.CustomerDomainName, service.Status.Dns)
			ep = &endpoint.DNSEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespacedName.Name,
					Namespace: namespacedName.Namespace,
				},
				Spec: endpoint.DNSEndpointSpec{
					Endpoints: []*endpoint.Endpoint{
						{
							DNSName: service.Spec.CustomerDomainName,
							Targets: []string{
								service.Status.Dns,
							},
							RecordType: "CNAME",
							RecordTTL:  300,
						},
					},
				},
			}
			controllerutil.SetControllerReference(route.K8sObject(), ep, s.k8sClient.Scheme())
			if err = s.k8sClient.Create(ctx, ep); err != nil {
				return err
			}
		} else if meta.IsNoMatchError(err) {
			s.log.Debugf("DNSEndpoint CRD not supported, skipping")
			return nil
		} else {
			return err
		}
	} else {
		s.log.Debugf("Attempting update of DNSEndpoint for %s - %s -> %s",
			namespacedName.String(), service.Spec.CustomerDomainName, service.Status.Dns)
		old := ep.DeepCopy()
		ep.Spec.Endpoints = []*endpoint.Endpoint{
			{
				DNSName: service.Spec.CustomerDomainName,
				Targets: []string{
					service.Status.Dns,
				},
				RecordType: "CNAME",
				RecordTTL:  300,
			},
		}
		if !reflect.DeepEqual(ep.Spec.Endpoints, old.Spec.Endpoints) {
			if err = s.k8sClient.Patch(ctx, ep, client.MergeFrom(old)); err != nil {
				return err
			}
		}
	}
	return nil
}
