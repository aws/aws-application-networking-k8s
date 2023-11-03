package externaldns

import (
	"context"
	"errors"
	"testing"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

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
		service          model.Service
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
			service: model.Service{
				Spec: model.ServiceSpec{
					ServiceTagFields: model.ServiceTagFields{
						RouteName:      "service",
						RouteNamespace: "default",
					},
					CustomerDomainName: "",
				},
				Status: &model.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			errIsNil: true,
		},
		{
			name: "No service dns - skips creation",
			service: model.Service{
				Spec: model.ServiceSpec{
					ServiceTagFields: model.ServiceTagFields{
						RouteName:      "service",
						RouteNamespace: "default",
					},
					CustomerDomainName: "custom-domain",
				},
				Status: &model.ServiceStatus{
					Dns: "",
				},
			},
			errIsNil: true,
		},
		{
			name: "No parent route - skips creation",
			service: model.Service{
				Spec: model.ServiceSpec{
					ServiceTagFields: model.ServiceTagFields{
						RouteName:      "service",
						RouteNamespace: "default",
					},
					CustomerDomainName: "custom-domain",
				},
				Status: &model.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			routeGetErr: errors.New("No HTTPRoute found"),
			errIsNil:    true,
		},
		{
			name: "Create new DNSEndpoint if not existing already",
			service: model.Service{
				Spec: model.ServiceSpec{
					ServiceTagFields: model.ServiceTagFields{
						RouteName:      "service",
						RouteNamespace: "default",
					},
					CustomerDomainName: "custom-domain",
				},
				Status: &model.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			dnsGetErr: apierrors.NewNotFound(schema.GroupResource{}, ""),
			created:   true,
			errIsNil:  true,
		},
		{
			name: "Return error on creation failure",
			service: model.Service{
				Spec: model.ServiceSpec{
					ServiceTagFields: model.ServiceTagFields{
						RouteName:      "service",
						RouteNamespace: "default",
					},
					CustomerDomainName: "custom-domain",
				},
				Status: &model.ServiceStatus{
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
			service: model.Service{
				Spec: model.ServiceSpec{
					ServiceTagFields: model.ServiceTagFields{
						RouteName:      "service",
						RouteNamespace: "default",
					},
					CustomerDomainName: "custom-domain",
				},
				Status: &model.ServiceStatus{
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
			service: model.Service{
				Spec: model.ServiceSpec{
					ServiceTagFields: model.ServiceTagFields{
						RouteName:      "service",
						RouteNamespace: "default",
					},
					CustomerDomainName: "custom-domain",
				},
				Status: &model.ServiceStatus{
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
			service: model.Service{
				Spec: model.ServiceSpec{
					ServiceTagFields: model.ServiceTagFields{
						RouteName:      "service",
						RouteNamespace: "default",
					},
					CustomerDomainName: "custom-domain",
				},
				Status: &model.ServiceStatus{
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
			service: model.Service{
				Spec: model.ServiceSpec{
					ServiceTagFields: model.ServiceTagFields{
						RouteName:      "service",
						RouteNamespace: "default",
					},
					CustomerDomainName: "custom-domain",
				},
				Status: &model.ServiceStatus{
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
			service: model.Service{
				Spec: model.ServiceSpec{
					ServiceTagFields: model.ServiceTagFields{
						RouteName:      "service",
						RouteNamespace: "default",
					},
					CustomerDomainName: "custom-domain",
				},
				Status: &model.ServiceStatus{
					Dns: "lattice-internal-domain",
				},
			},
			dnsGetErr: errors.New("Unhandled exception"),
			errIsNil:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mock_client.NewMockClient(c)
			mgr := NewDnsEndpointManager(gwlog.FallbackLogger, mockClient)

			mockClient.EXPECT().Scheme().Return(runtime.NewScheme()).AnyTimes()

			mockClient.EXPECT().Get(gomock.Any(), gomock.Eq(types.NamespacedName{
				Namespace: tt.service.Spec.RouteNamespace,
				Name:      tt.service.Spec.RouteName,
			}), gomock.Any()).Return(tt.routeGetErr).AnyTimes()

			mockClient.EXPECT().Get(gomock.Any(), gomock.Eq(types.NamespacedName{
				Namespace: tt.service.Spec.RouteNamespace,
				Name:      tt.service.Spec.RouteName + "-dns",
			}), gomock.Any()).DoAndReturn(func(ctx context.Context, name types.NamespacedName, ep *endpoint.DNSEndpoint, _ ...interface{}) error {
				tt.existingEndpoint.DeepCopyInto(ep)
				return tt.dnsGetErr
			}).AnyTimes()

			createCall := mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).Return(tt.dnsCreateErr)
			if tt.created {
				createCall.Times(1)
			} else {
				createCall.Times(0)
			}
			patchCall := mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.dnsUpdateErr)
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
		})
	}
}
