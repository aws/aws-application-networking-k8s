package externaldns

import (
	"context"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/golang/glog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/external-dns/endpoint"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type DnsEndpointManager interface {
	Create(ctx context.Context, service *latticemodel.Service) error
}

type defaultDnsEndpointManager struct {
	k8sClient client.Client
}

func NewDnsEndpointManager(k8sClient client.Client) *defaultDnsEndpointManager {
	return &defaultDnsEndpointManager{
		k8sClient: k8sClient,
	}
}

func (s *defaultDnsEndpointManager) Create(ctx context.Context, service *latticemodel.Service) error {
	namespacedName := types.NamespacedName{
		Namespace: service.Spec.Namespace,
		Name:      service.Spec.Name + "-dns",
	}
	if service.Spec.CustomerDomainName == "" {
		glog.V(2).Infof("Skipping creation of %s: detected no custom domain", namespacedName.String())
		return nil
	}
	if service.Status == nil || service.Status.ServiceDNS == "" {
		glog.V(2).Infof("Skipping creation of %s: DNS target not ready in svc status", namespacedName.String())
		return nil
	}

	httproute := &gateway_api.HTTPRoute{}
	routeNamespacedName := types.NamespacedName{
		Namespace: service.Spec.Namespace,
		Name:      service.Spec.Name,
	}
	if err := s.k8sClient.Get(ctx, routeNamespacedName, httproute); err != nil {
		glog.V(2).Infof("Skipping creation of %s: Could not find corresponding route", namespacedName.String())
		return nil
	}

	ep := &endpoint.DNSEndpoint{}
	if err := s.k8sClient.Get(ctx, namespacedName, ep); err != nil {
		if apierrors.IsNotFound(err) {
			glog.V(2).Infof("Attempting creation of DNSEndpoint for %s - %s -> %s",
				namespacedName.String(), service.Spec.CustomerDomainName, service.Status.ServiceDNS)
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
								service.Status.ServiceDNS,
							},
							RecordType: "CNAME",
							RecordTTL:  300,
						},
					},
				},
			}
			controllerutil.SetControllerReference(httproute, ep, s.k8sClient.Scheme())
			if err = s.k8sClient.Create(ctx, ep); err != nil {
				glog.V(2).Infof("Failed creating DNSEndpoint: %s", err.Error())
				return err
			}
		} else if meta.IsNoMatchError(err) {
			glog.V(2).Infof("DNSEndpoint CRD not supported, skipping")
			return nil
		} else {
			glog.V(2).Infof("Failed lookup of DNSEndpoint: %s", err.Error())
			return err
		}
	} else {
		glog.V(2).Infof("Attempting update of DNSEndpoint for %s - %s -> %s",
			namespacedName.String(), service.Spec.CustomerDomainName, service.Status.ServiceDNS)
		old := ep.DeepCopy()
		ep.Spec.Endpoints = []*endpoint.Endpoint{
			{
				DNSName: service.Spec.CustomerDomainName,
				Targets: []string{
					service.Status.ServiceDNS,
				},
				RecordType: "CNAME",
				RecordTTL:  300,
			},
		}
		if !reflect.DeepEqual(ep.Spec.Endpoints, old.Spec.Endpoints) {
			if err = s.k8sClient.Patch(ctx, ep, client.MergeFrom(old)); err != nil {
				glog.V(2).Infof("Failed updating DNSEndpoint: %s", err.Error())
				return err
			}
		}
	}
	return nil
}
