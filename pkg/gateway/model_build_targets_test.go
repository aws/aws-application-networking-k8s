package gateway

import (
	"context"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func Test_Targets(t *testing.T) {
	namespacePtr := func(ns string) *gwv1beta1.Namespace {
		p := gwv1beta1.Namespace(ns)
		return &p
	}
	kindPtr := func(k string) *gwv1beta1.Kind {
		p := gwv1beta1.Kind(k)
		return &p
	}

	tests := []struct {
		name               string
		port               int32
		endPoints          []corev1.Endpoints
		svc                corev1.Service
		serviceExport      anv1alpha1.ServiceExport
		refByServiceExport bool
		refByService       bool
		wantErrIsNil       bool
		expectedTargetList []model.Target
	}{
		{
			name: "Add all endpoints to build spec",
			port: 0,
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export1",
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{{IP: "10.10.1.1"}, {IP: "10.10.2.2"}},
							Ports:     []corev1.EndpointPort{{Port: 8675}},
						},
					},
				},
			},
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "export1",
					DeletionTimestamp: nil,
				},
			},
			refByService: true,
			wantErrIsNil: true,
			expectedTargetList: []model.Target{
				{
					TargetIP: "10.10.1.1",
					Port:     8675,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     8675,
				},
			},
		},
		{
			name: "Add endpoints with matching service port to build spec",
			port: 80,
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export1",
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{{IP: "10.10.1.1"}, {IP: "10.10.2.2"}},
							Ports:     []corev1.EndpointPort{{Name: "a", Port: 8675}, {Name: "b", Port: 309}},
						},
					},
				},
			},
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "export1",
					DeletionTimestamp: nil,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "a",
							Port:       80,
							TargetPort: intstr.FromInt(8675),
						},
						{
							Name:       "b",
							Port:       81,
							TargetPort: intstr.FromInt(309),
						},
					},
				},
			},
			refByService: true,
			wantErrIsNil: true,
			expectedTargetList: []model.Target{
				{
					TargetIP: "10.10.1.1",
					Port:     8675,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     8675,
				},
			},
		},
		{
			name: "Add all endpoints to build spec with port annotation",
			port: 3090,
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export1",
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{{IP: "10.10.1.1"}, {IP: "10.10.2.2"}},
							Ports:     []corev1.EndpointPort{{Name: "a", Port: 8675}, {Name: "b", Port: 3090}},
						},
					},
				},
			},
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "export1",
					DeletionTimestamp: nil,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "a",
							Port:       80,
							TargetPort: intstr.FromInt(8675),
						},
						{
							Name:       "b",
							Port:       81,
							TargetPort: intstr.FromInt(3090),
						},
					},
				},
			},
			serviceExport: anv1alpha1.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "export1",
					DeletionTimestamp: nil,
					Annotations:       map[string]string{"application-networking.k8s.aws/port": "81"},
				},
			},
			refByServiceExport: true,
			wantErrIsNil:       true,
			expectedTargetList: []model.Target{
				{
					TargetIP: "10.10.1.1",
					Port:     3090,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     3090,
				},
			},
		},
		{
			name:               "Endpoints does NOT exists",
			port:               0,
			refByServiceExport: true,
			wantErrIsNil:       false,
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "export1",
					DeletionTimestamp: nil,
				},
			},
		},
		{
			name: "Add all endpoints to build spec",
			port: 0,
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export5",
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{{IP: "10.10.1.1"}, {IP: "10.10.2.2"}},
							Ports:     []corev1.EndpointPort{{Name: "a", Port: 8675}, {Name: "b", Port: 309}},
						},
					},
				},
			},
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "export5",
					DeletionTimestamp: nil,
				},
			},
			refByService: true,
			wantErrIsNil: true,
			expectedTargetList: []model.Target{
				{
					TargetIP: "10.10.1.1",
					Port:     8675,
				},
				{
					TargetIP: "10.10.1.1",
					Port:     309,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     8675,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     309,
				},
			},
		},
		{
			name: "Only add endpoints for port 8675 to build spec",
			port: 8675,
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export6",
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{{IP: "10.10.1.1"}, {IP: "10.10.2.2"}},
							Ports:     []corev1.EndpointPort{{Name: "a", Port: 8675}},
						},
					},
				},
			},
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "export6",
					DeletionTimestamp: nil,
				},
			},
			refByServiceExport: true,
			wantErrIsNil:       true,
			expectedTargetList: []model.Target{
				{
					TargetIP: "10.10.1.1",
					Port:     8675,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     8675,
				},
			},
			serviceExport: anv1alpha1.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "export6",
					DeletionTimestamp: nil,
				},
			},
		},
		{
			name: "BackendRef port does not match service port",
			port: 8750,
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export7",
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{{IP: "10.10.1.1"}, {IP: "10.10.2.2"}},
							Ports:     []corev1.EndpointPort{{Name: "a", Port: 8675}, {Name: "b", Port: 309}},
						},
					},
				},
			},
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "export7",
					DeletionTimestamp: nil,
				},
			},
			refByService:       true,
			refByServiceExport: true,
			wantErrIsNil:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			k8sSchema := runtime.NewScheme()
			k8sSchema.AddKnownTypes(anv1alpha1.SchemeGroupVersion, &anv1alpha1.ServiceExport{})
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)

			if !reflect.DeepEqual(tt.serviceExport, anv1alpha1.ServiceExport{}) {
				assert.NoError(t, k8sClient.Create(ctx, tt.serviceExport.DeepCopy()))
			}

			if len(tt.endPoints) > 0 {
				assert.NoError(t, k8sClient.Create(ctx, tt.endPoints[0].DeepCopy()))
			}

			assert.NoError(t, k8sClient.Create(ctx, tt.svc.DeepCopy()))

			br := gwv1beta1.HTTPBackendRef{}
			br.Name = "name"
			br.Namespace = namespacePtr("ns")
			br.Kind = kindPtr("Service")
			br.Port = PortNumberPtr(int(tt.port))
			corebr := core.NewHTTPBackendRef(br)

			nsn := types.NamespacedName{
				Name:      "stack",
				Namespace: "ns",
			}
			stack := core.NewDefaultStack(core.StackID(nsn))
			builder := NewTargetsBuilder(gwlog.FallbackLogger, k8sClient, stack)

			var err error
			if tt.refByServiceExport {
				_, err = builder.BuildForServiceExport(ctx, &tt.serviceExport, "tg-id")
			} else {
				_, err = builder.Build(ctx, &tt.svc, &corebr, "tg-id")
			}

			if !tt.wantErrIsNil {
				assert.NotNil(t, err)
				return
			}
			assert.Nil(t, err)

			var stackTargets []*model.Targets
			_ = stack.ListResources(&stackTargets)
			assert.Equal(t, 1, len(stackTargets))
			st := stackTargets[0]

			assert.Equal(t, "tg-id", st.Spec.StackTargetGroupId)
			assert.Equal(t, tt.expectedTargetList, st.Spec.TargetList)
		})
	}
}
