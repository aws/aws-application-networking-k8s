package lattice

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_SynthesizeService(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name          string
		httpRoute     *v1alpha2.HTTPRoute
		serviceARN    string
		serviceID     string
		mgrErr        error
		wantErrIsNil  bool
		wantIsDeleted bool
	}{
		{
			name: "Add LatticeService",

			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service1",
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
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

			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service2",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
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

			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service3",
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
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

			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service4",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
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
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		ds := latticestore.NewLatticeDataStore()

		stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.httpRoute)))

		mockSvcManager := NewMockServiceManager(c)

		pro := "HTTP"
		protocols := []*string{&pro}
		spec := latticemodel.ServiceSpec{
			Name:      tt.httpRoute.Name,
			Namespace: tt.httpRoute.Namespace,
			Protocols: protocols,
		}

		if tt.httpRoute.DeletionTimestamp.IsZero() {
			spec.IsDeleted = false
		} else {
			spec.IsDeleted = true
		}

		latticeService := latticemodel.NewLatticeService(stack, "", spec)
		fmt.Printf("latticeService :%v\n", latticeService)

		if tt.httpRoute.DeletionTimestamp.IsZero() {
			mockSvcManager.EXPECT().Create(ctx, latticeService).Return(latticemodel.ServiceStatus{ServiceARN: tt.serviceARN, ServiceID: tt.serviceID}, tt.mgrErr)
		} else {
			mockSvcManager.EXPECT().Delete(ctx, latticeService).Return(tt.mgrErr)
		}

		synthesizer := NewServiceSynthesizer(mockSvcManager, stack, ds)

		err := synthesizer.Synthesize(ctx)

		if tt.wantErrIsNil {
			assert.Nil(t, err)
			if tt.httpRoute.DeletionTimestamp.IsZero() {
				svc, err := ds.GetLatticeService(spec.Name, spec.Namespace)
				assert.Nil(t, err)
				assert.Equal(t, tt.serviceARN, svc.ARN)
				assert.Equal(t, tt.serviceID, svc.ID)
			}
		} else {
			assert.NotNil(t, err)
		}

	}
}
