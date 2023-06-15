package lattice

import (
	"context"
	"errors"
	"fmt"
	"log"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_SynthesizeService(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name          string
		httpRoute     *gateway_api.HTTPRoute
		serviceARN    string
		serviceID     string
		mgrErr        error
		wantErrIsNil  bool
		wantIsDeleted bool
	}{
		{
			name: "HttpRoute Creation trigger LatticeService Creation",
			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service1",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
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
			name: "Add LatticeService, if serviceManager return error, then Synthesize() should return error",

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service3",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
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
			name: "serviceSynthesizer.Synthesize() should ignore LatticeService deletion request",

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service2",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
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
	}

	for _, tt := range tests {
		log.Println("test case: ", tt.name)
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

func Test_PostSynthesizeService(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name          string
		httpRoute     *gateway_api.HTTPRoute
		serviceARN    string
		serviceID     string
		mgrErr        error
		wantErrIsNil  bool
		wantIsDeleted bool
	}{
		{
			name: "serviceSynthesizer.PostSynthesize() should ignore any LatticeService creation request",
			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service1",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
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
			name: "LatticeService deletion triggered by HttpRoute deletion, ok case",

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service2",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
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
			name: "LatticeService deletion return err since serviceManager returns error",

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service4",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
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
		log.Println("test case: ", tt.name)
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

		if tt.wantIsDeleted {
			mockSvcManager.EXPECT().Delete(ctx, latticeService).Return(tt.mgrErr)
			ds.AddLatticeService(spec.Name, spec.Namespace, tt.serviceARN, tt.serviceID, "mytest.com")
		}

		synthesizer := NewServiceSynthesizer(mockSvcManager, stack, ds)
		var err error

		err = synthesizer.PostSynthesize(ctx)

		if tt.wantErrIsNil {
			assert.Nil(t, err)
		} else {
			assert.NotNil(t, err)
		}

		if tt.wantIsDeleted {
			if tt.mgrErr == nil {
				// serviceManager delete latticeService successfully,
				//the entry in the dataStore should be deleted as well
				_, err := ds.GetLatticeService(spec.Name, spec.Namespace)
				assert.Equal(t, err, errors.New(latticestore.DATASTORE_SERVICE_NOT_EXIST))

			} else {
				//serviceManager returns error, delete failed,
				//latticeService should still exist in the dataStore
				serviceFromDS, err := ds.GetLatticeService(spec.Name, spec.Namespace)
				assert.Nil(t, err)
				assert.Equal(t, tt.serviceARN, serviceFromDS.ARN)
				assert.Equal(t, tt.serviceID, serviceFromDS.ID)
			}
		}
	}
}
