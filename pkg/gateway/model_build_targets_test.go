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
	"github.com/aws/aws-sdk-go/aws"
	discoveryv1 "k8s.io/api/discovery/v1"
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
		endpointSlice      []discoveryv1.EndpointSlice
		svc                corev1.Service
		serviceExport      anv1alpha1.ServiceExport
		refByServiceExport bool
		refByService       bool
		wantErrIsNil       bool
		expectedTargetList []model.Target
	}{
		{
			name: "Add all endpoints with readiness to build spec",
			port: 0,
			endpointSlice: []discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export1",
						Labels:    map[string]string{discoveryv1.LabelServiceName: "export1"},
					},
					Ports: []discoveryv1.EndpointPort{
						{Port: aws.Int32(8675)},
					},
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"10.10.1.1"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: aws.Bool(true),
							},
							TargetRef: &corev1.ObjectReference{
								Namespace: "ns1",
								Name:      "pod1",
								Kind:      "Pod",
							},
						},
						{
							Addresses: []string{"10.10.2.2"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: aws.Bool(false),
							},
							TargetRef: &corev1.ObjectReference{
								Namespace: "ns1",
								Name:      "pod2",
								Kind:      "Pod",
							},
						},
						{
							Addresses: []string{"10.10.3.3"},
							Conditions: discoveryv1.EndpointConditions{
								Ready:       aws.Bool(false),
								Terminating: aws.Bool(true),
							},
							TargetRef: &corev1.ObjectReference{
								Namespace: "ns1",
								Name:      "pod3",
								Kind:      "Pod",
							},
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
					TargetIP:  "10.10.1.1",
					Port:      8675,
					Ready:     true,
					TargetRef: types.NamespacedName{Namespace: "ns1", Name: "pod1"},
				},
				{
					TargetIP:  "10.10.2.2",
					Port:      8675,
					Ready:     false,
					TargetRef: types.NamespacedName{Namespace: "ns1", Name: "pod2"},
				},
			},
		},
		{
			name: "Add endpoints with matching service port to build spec",
			port: 80,
			endpointSlice: []discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export1",
						Labels:    map[string]string{discoveryv1.LabelServiceName: "export1"},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Name: aws.String("a"),
							Port: aws.Int32(8675),
						},
						{
							Name: aws.String("b"),
							Port: aws.Int32(309),
						},
					},
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"10.10.1.1", "10.10.2.2"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: aws.Bool(true),
							},
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
					Ready:    true,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     8675,
					Ready:    true,
				},
			},
		},
		{
			name: "Add all endpoints to build spec with port annotation",
			port: 3090,
			endpointSlice: []discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export1",
						Labels:    map[string]string{discoveryv1.LabelServiceName: "export1"},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Name: aws.String("a"),
							Port: aws.Int32(8675),
						},
						{
							Name: aws.String("b"),
							Port: aws.Int32(3090),
						},
					},
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"10.10.1.1", "10.10.2.2"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: aws.Bool(true),
							},
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
					Ready:    true,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     3090,
					Ready:    true,
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
			endpointSlice: []discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export5",
						Labels:    map[string]string{discoveryv1.LabelServiceName: "export5"},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Name: aws.String("a"),
							Port: aws.Int32(8675),
						},
						{
							Name: aws.String("b"),
							Port: aws.Int32(309),
						},
					},
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"10.10.1.1", "10.10.2.2"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: aws.Bool(true),
							},
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
					Ready:    true,
				},
				{
					TargetIP: "10.10.1.1",
					Port:     309,
					Ready:    true,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     8675,
					Ready:    true,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     309,
					Ready:    true,
				},
			},
		},
		{
			name: "Only add endpoints for port 8675 to build spec",
			port: 8675,
			endpointSlice: []discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export6",
						Labels:    map[string]string{discoveryv1.LabelServiceName: "export6"},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Name: aws.String("a"),
							Port: aws.Int32(8675),
						},
					},
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"10.10.1.1", "10.10.2.2"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: aws.Bool(true),
							},
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
					Ready:    true,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     8675,
					Ready:    true,
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
			endpointSlice: []discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export7",
						Labels:    map[string]string{discoveryv1.LabelServiceName: "export7"},
					},
					Ports: []discoveryv1.EndpointPort{
						{
							Name: aws.String("a"),
							Port: aws.Int32(8675),
						},
						{
							Name: aws.String("b"),
							Port: aws.Int32(309),
						},
					},
					Endpoints: []discoveryv1.Endpoint{
						{
							Addresses: []string{"10.10.1.1", "10.10.2.2"},
							Conditions: discoveryv1.EndpointConditions{
								Ready: aws.Bool(true),
							},
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
			discoveryv1.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

			if !reflect.DeepEqual(tt.serviceExport, anv1alpha1.ServiceExport{}) {
				assert.NoError(t, k8sClient.Create(ctx, tt.serviceExport.DeepCopy()))
			}

			if len(tt.endpointSlice) > 0 {
				assert.NoError(t, k8sClient.Create(ctx, tt.endpointSlice[0].DeepCopy()))
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
			assert.ElementsMatch(t, tt.expectedTargetList, st.Spec.TargetList)
		})
	}
}
