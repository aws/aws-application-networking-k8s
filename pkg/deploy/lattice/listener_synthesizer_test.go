package lattice

import (
	"context"
	//"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

// PortNumberPtr translates an int to a *PortNumber
func PortNumberPtr(p int) *v1alpha2.PortNumber {
	result := v1alpha2.PortNumber(p)
	return &result
}
func Test_SynthesizeListener(t *testing.T) {

	tests := []struct {
		name           string
		gwListenerPort v1alpha2.PortNumber
		gwProtocol     string
		httpRoute      *v1alpha2.HTTPRoute
		listenerARN    string
		listenerID     string
		serviceARN     string
		serviceID      string
		mgrErr         error
		wantErrIsNil   bool
		wantIsDeleted  bool
	}{
		{
			name:           "Add Listener",
			gwListenerPort: *PortNumberPtr(80),
			gwProtocol:     "HTTP",
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
			listenerARN:   "arn1234",
			listenerID:    "1234",
			serviceARN:    "arn56789",
			serviceID:     "56789",
			mgrErr:        nil,
			wantIsDeleted: false,
			wantErrIsNil:  true,
		},
		{
			name:           "Delete Listener",
			gwListenerPort: *PortNumberPtr(80),
			gwProtocol:     "HTTP",
			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service2",
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
			listenerARN:   "arn1234",
			listenerID:    "1234",
			serviceARN:    "arn56789",
			serviceID:     "56789",
			mgrErr:        nil,
			wantIsDeleted: true,
			wantErrIsNil:  true,
		},
	}
	var protocol = "HTTP"

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		ds := latticestore.NewLatticeDataStore()

		stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.httpRoute)))

		mockListenerManager := NewMockListenerManager(c)

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

		action := latticemodel.DefaultAction{
			Is_Import:               false,
			BackendServiceName:      "test",
			BackendServiceNamespace: "default",
		}

		latticemodel.NewLatticeService(stack, "", spec)

		port := int64(tt.gwListenerPort)

		mockListenerManager.EXPECT().List(ctx, tt.serviceID).Return(
			[]*vpclattice.ListenerSummary{
				{
					Arn:      &tt.listenerARN,
					Id:       &tt.listenerID,
					Name:     &tt.httpRoute.Name,
					Port:     &port,
					Protocol: &protocol,
				},
			}, tt.mgrErr)

		ds.AddLatticeService(tt.httpRoute.Name, tt.httpRoute.Namespace, tt.serviceARN, tt.serviceID, "dns")
		if !tt.wantIsDeleted {
			listenerResourceName := fmt.Sprintf("%s-%s-%d-%s", tt.httpRoute.Name, tt.httpRoute.Namespace,
				tt.gwListenerPort, protocol)
			listener := latticemodel.NewListener(stack, listenerResourceName, int64(tt.gwListenerPort), tt.gwProtocol,
				tt.httpRoute.Name, tt.httpRoute.Namespace, action)

			mockListenerManager.EXPECT().Create(ctx, listener).Return(latticemodel.ListenerStatus{
				Name:        tt.httpRoute.Name,
				Namespace:   tt.httpRoute.Namespace,
				ListenerARN: tt.listenerARN,
				ListenerID:  tt.listenerID,
				ServiceID:   tt.serviceID,
				Port:        int64(tt.gwListenerPort),
				Protocol:    tt.gwProtocol}, tt.mgrErr)

		} else {
			mockListenerManager.EXPECT().Delete(ctx, tt.listenerID, tt.serviceID).Return(tt.mgrErr)

		}

		synthesizer := NewListenerSynthesizer(mockListenerManager, stack, ds)

		err := synthesizer.Synthesize(ctx)

		if tt.wantErrIsNil {
			assert.Nil(t, err)
		}
		if !tt.wantIsDeleted {
			listener, err := ds.GetlListener(spec.Name, spec.Namespace, int64(tt.gwListenerPort), tt.gwProtocol)
			assert.Nil(t, err)
			fmt.Printf("listener: %v \n", listener)
			assert.Equal(t, listener.ARN, tt.listenerARN)
			assert.Equal(t, listener.ID, tt.listenerID)
			assert.Equal(t, listener.Key.Name, tt.httpRoute.Name)
			assert.Equal(t, listener.Key.Namespace, tt.httpRoute.Namespace)
			assert.Equal(t, listener.Key.Port, int64(tt.gwListenerPort))

		} else {
			assert.Nil(t, err)

			// make sure listener is also deleted from datastore
			_, err := ds.GetlListener(spec.Name, spec.Namespace, int64(tt.gwListenerPort), tt.gwProtocol)
			assert.NotNil(t, err)

		}
	}

}
