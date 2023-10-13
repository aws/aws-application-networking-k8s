package lattice

import (
	"context"
	"errors"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/externaldns"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	"testing"
)

func Test_SynthesizeService(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name          string
		httpRoute     *gwv1beta1.HTTPRoute
		serviceARN    string
		serviceID     string
		mgrErr        error
		dnsErr        error
		wantErrIsNil  bool
		wantIsDeleted bool
	}{
		{
			name: "Add LatticeService",

			httpRoute: &gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service1",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
				},
			},
			serviceARN:    "arn1234",
			serviceID:     "56789",
			mgrErr:        nil,
			wantIsDeleted: false,
			wantErrIsNil:  true,
		},
		{
			name: "Delete LatticeService",

			httpRoute: &gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service2",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name: "gateway2",
							},
						},
					},
				},
			},
			serviceARN:    "arn1234",
			serviceID:     "56789",
			mgrErr:        nil,
			wantIsDeleted: true,
			wantErrIsNil:  true,
		},
		{
			name: "Add LatticeService, return error need to retry",

			httpRoute: &gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service3",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
				},
			},
			serviceARN:    "arn1234",
			serviceID:     "56789",
			mgrErr:        errors.New("Need-to-Retry"),
			wantIsDeleted: false,
			wantErrIsNil:  false,
		},
		{
			name: "Delete LatticeService, but need retry",

			httpRoute: &gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service4",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name: "gateway2",
							},
						},
					},
				},
			},
			serviceARN:    "arn1234",
			serviceID:     "56789",
			mgrErr:        errors.New("need-to-retry-delete"),
			wantIsDeleted: true,
			wantErrIsNil:  false,
		},
		{
			name: "Add LatticeService, getting error registering DNS",

			httpRoute: &gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service3",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
				},
			},
			serviceARN:    "arn1234",
			serviceID:     "56789",
			dnsErr:        errors.New("Failed registering DNS"),
			wantIsDeleted: false,
			wantErrIsNil:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.httpRoute)))

			mockSvcManager := NewMockServiceManager(c)
			mockDnsManager := externaldns.NewMockDnsEndpointManager(c)

			spec := model.ServiceSpec{
				Name:      tt.httpRoute.Name,
				Namespace: tt.httpRoute.Namespace,
			}

			latticeService, err := model.NewLatticeService(stack, spec)
			assert.Nil(t, err)
			latticeService.IsDeleted = !tt.httpRoute.DeletionTimestamp.IsZero()

			if latticeService.IsDeleted {
				mockSvcManager.EXPECT().Delete(ctx, latticeService).Return(tt.mgrErr)
			} else {
				mockSvcManager.EXPECT().Upsert(ctx, latticeService).Return(model.ServiceStatus{Arn: tt.serviceARN, Id: tt.serviceID}, tt.mgrErr)
			}

			if !latticeService.IsDeleted && tt.mgrErr == nil {
				mockDnsManager.EXPECT().Create(ctx, gomock.Any()).Return(tt.dnsErr)
			}

			synthesizer := NewServiceSynthesizer(gwlog.FallbackLogger, mockSvcManager, mockDnsManager, stack)

			err = synthesizer.Synthesize(ctx)
			if tt.wantErrIsNil {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
			}
		})
	}
}
