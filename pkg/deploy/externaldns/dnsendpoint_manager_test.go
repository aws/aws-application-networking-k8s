package externaldns

import (
	"context"
	"errors"
	"fmt"
	"testing"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/external-dns/endpoint"
)

func TestCreateDnsEndpoint(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	tests := []struct {
		name             string
		service          latticemodel.Service
		existingEndpoint endpoint.DNSEndpoint
		routeGetErr      error
		dnsGetErr        error
		dnsCreateErr     error
		dnsUpdateErr     error
		created          bool
		updated          bool
		errIsNil         bool
	}{
		{
			name: "No customer domain name - skips creation",
			service: latticemodel.Service{
				Spec: latticemodel.ServiceSpec{
					Name:               "service",
					Namespace:          "default",
					CustomerDomainName: "",
				},
				Status: &latticemodel.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			errIsNil: true,
		},
		{
			name: "No service dns - skips creation",
			service: latticemodel.Service{
				Spec: latticemodel.ServiceSpec{
					Name:               "service",
					Namespace:          "default",
					CustomerDomainName: "custom-domain",
				},
				Status: &latticemodel.ServiceStatus{
					Dns: "",
				},
			},
			errIsNil: true,
		},
		{
			name: "No parent route - skips creation",
			service: latticemodel.Service{
				Spec: latticemodel.ServiceSpec{
					Name:               "service",
					Namespace:          "default",
					CustomerDomainName: "custom-domain",
				},
				Status: &latticemodel.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			routeGetErr: errors.New("No HTTPRoute found"),
			errIsNil:    true,
		},
		{
			name: "Create new DNSEndpoint if not existing already",
			service: latticemodel.Service{
				Spec: latticemodel.ServiceSpec{
					Name:               "service",
					Namespace:          "default",
					CustomerDomainName: "custom-domain",
				},
				Status: &latticemodel.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			dnsGetErr: apierrors.NewNotFound(schema.GroupResource{}, ""),
			created:   true,
			errIsNil:  true,
		},
		{
			name: "Return error on creation failure",
			service: latticemodel.Service{
				Spec: latticemodel.ServiceSpec{
					Name:               "service",
					Namespace:          "default",
					CustomerDomainName: "custom-domain",
				},
				Status: &latticemodel.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			dnsGetErr:    apierrors.NewNotFound(schema.GroupResource{}, ""),
			created:      true,
			dnsCreateErr: errors.New("DNS creation failed"),
			errIsNil:     false,
		},
		{
			name: "Update DNSEndpoint if existing already",
			service: latticemodel.Service{
				Spec: latticemodel.ServiceSpec{
					Name:               "service",
					Namespace:          "default",
					CustomerDomainName: "custom-domain",
				},
				Status: &latticemodel.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			existingEndpoint: endpoint.DNSEndpoint{
				Spec: endpoint.DNSEndpointSpec{
					Endpoints: []*endpoint.Endpoint{
						{
							DNSName:    "custom-domain",
							Targets:    []string{"lattice-internal-domain-but-different"},
							RecordType: "CNAME",
							RecordTTL:  300,
						},
					},
				},
			},
			updated:  true,
			errIsNil: true,
		},
		{
			name: "DNSEndpoint existing already, but skip if it is the same",
			service: latticemodel.Service{
				Spec: latticemodel.ServiceSpec{
					Name:               "service",
					Namespace:          "default",
					CustomerDomainName: "custom-domain",
				},
				Status: &latticemodel.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			existingEndpoint: endpoint.DNSEndpoint{
				Spec: endpoint.DNSEndpointSpec{
					Endpoints: []*endpoint.Endpoint{
						{
							DNSName:    "custom-domain",
							Targets:    []string{"lattice-internal-domain"},
							RecordType: "CNAME",
							RecordTTL:  300,
						},
					},
				},
			},
			updated:  false,
			errIsNil: true,
		},
		{
			name: "Return error on update failure",
			service: latticemodel.Service{
				Spec: latticemodel.ServiceSpec{
					Name:               "service",
					Namespace:          "default",
					CustomerDomainName: "custom-domain",
				},
				Status: &latticemodel.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			existingEndpoint: endpoint.DNSEndpoint{
				Spec: endpoint.DNSEndpointSpec{
					Endpoints: []*endpoint.Endpoint{
						{
							DNSName:    "custom-domain",
							Targets:    []string{"lattice-internal-domain-but-different"},
							RecordType: "CNAME",
							RecordTTL:  300,
						},
					},
				},
			},
			updated:      true,
			dnsUpdateErr: errors.New("DNS update failed"),
			errIsNil:     false,
		},
		{
			name: "Skips creation when DNSEndpoint CRD is not found",
			service: latticemodel.Service{
				Spec: latticemodel.ServiceSpec{
					Name:               "service",
					Namespace:          "default",
					CustomerDomainName: "custom-domain",
				},
				Status: &latticemodel.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			dnsGetErr: &meta.NoKindMatchError{
				GroupKind:        schema.GroupKind{},
				SearchedVersions: []string{},
			},
			errIsNil: true,
		},
		{
			name: "Return error on unexpected lookup failure",
			service: latticemodel.Service{
				Spec: latticemodel.ServiceSpec{
					Name:               "service",
					Namespace:          "default",
					CustomerDomainName: "custom-domain",
				},
				Status: &latticemodel.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			dnsGetErr: errors.New("Unhandled exception"),
			errIsNil:  false,
		},
	}

	for _, tt := range tests {
		fmt.Printf("Testing >>>>> %v\n", tt.name)
		client := mock_client.NewMockClient(c)
		mgr := NewDnsEndpointManager(client)

		client.EXPECT().Scheme().Return(runtime.NewScheme()).AnyTimes()

		client.EXPECT().Get(gomock.Any(), gomock.Eq(types.NamespacedName{
			Namespace: tt.service.Spec.Namespace,
			Name:      tt.service.Spec.Name,
		}), gomock.Any()).Return(tt.routeGetErr).AnyTimes()

		client.EXPECT().Get(gomock.Any(), gomock.Eq(types.NamespacedName{
			Namespace: tt.service.Spec.Namespace,
			Name:      tt.service.Spec.Name + "-dns",
		}), gomock.Any()).DoAndReturn(func(ctx context.Context, name types.NamespacedName, ep *endpoint.DNSEndpoint, _ ...interface{}) error {
			tt.existingEndpoint.DeepCopyInto(ep)
			return tt.dnsGetErr
		}).AnyTimes()

		createCall := client.EXPECT().Create(gomock.Any(), gomock.Any()).Return(tt.dnsCreateErr)
		if tt.created {
			createCall.Times(1)
		} else {
			createCall.Times(0)
		}
		patchCall := client.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.dnsUpdateErr)
		if tt.updated {
			patchCall.Times(1)
		} else {
			patchCall.Times(0)
		}

		err := mgr.Create(context.Background(), &tt.service)
		if tt.errIsNil {
			assert.Nil(t, err)
		} else {
			assert.Error(t, err)
		}
	}
}
