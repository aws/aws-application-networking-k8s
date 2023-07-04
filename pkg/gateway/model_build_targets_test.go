package gateway

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_Targets(t *testing.T) {

	var httpSectionName gateway_api.SectionName = "http"
	var serviceKind gateway_api.Kind = "Service"
	var namespace = gateway_api.Namespace("ns1")

	tests := []struct {
		name               string
		srvExportName      string
		srvExportNamespace string
		endPoints          []corev1.Endpoints
		svc                corev1.Service
		serviceExport      mcs_api.ServiceExport
		inDataStore        bool
		refByServiceExport bool
		refByService       bool
		wantErrIsNil       bool
		expectedTargetList []latticemodel.Target
		httpRoute          *gateway_api.HTTPRoute
	}{
		{
			name:               "Add all endpoints to build spec",
			srvExportName:      "export1",
			srvExportNamespace: "ns1",
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
			},
			inDataStore:        true,
			refByServiceExport: true,
			wantErrIsNil:       true,
			expectedTargetList: []latticemodel.Target{
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
			name:               "Add all endpoints to build spec with port annotation",
			srvExportName:      "export1",
			srvExportNamespace: "ns1",
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
			},
			serviceExport: mcs_api.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "export1",
					DeletionTimestamp: nil,
					Annotations:       map[string]string{"multicluster.x-k8s.io/port": "3090"},
				},
			},
			inDataStore:        true,
			refByServiceExport: true,
			wantErrIsNil:       true,
			expectedTargetList: []latticemodel.Target{
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
			name:               "Delete svc and all endpoints to build spec",
			srvExportName:      "export1",
			srvExportNamespace: "ns1",
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
					Namespace: "ns1",
					Name:      "export1",
					DeletionTimestamp: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
			inDataStore:        true,
			refByServiceExport: true,
			wantErrIsNil:       true,
			expectedTargetList: nil,
		},
		{
			name:               "Delete svc and no endpoints to build spec",
			srvExportName:      "export1",
			srvExportNamespace: "ns1",
			endPoints:          []corev1.Endpoints{},
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "export1",
					DeletionTimestamp: &metav1.Time{
						Time: time.Now(),
					},
				},
			},
			inDataStore:        true,
			refByServiceExport: true,
			wantErrIsNil:       true,
			expectedTargetList: nil,
		},
		{
			name:               "Endpoints without TargetGroup",
			srvExportName:      "export2",
			srvExportNamespace: "ns1",
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
			},
			inDataStore:        false,
			refByServiceExport: true,
			wantErrIsNil:       false,
		},
		{
			name:               "Endpoints's TargetGroup is NOT referenced by serviceexport",
			srvExportName:      "export3",
			srvExportNamespace: "ns1",
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
			},
			inDataStore:        true,
			refByServiceExport: false,
			refByService:       false,
			wantErrIsNil:       false,
		},
		{
			name:               "Endpoints does NOT exists",
			srvExportName:      "export4",
			srvExportNamespace: "ns1",
			inDataStore:        false,
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
			name:               "Add all endpoints to build spec",
			srvExportName:      "export5",
			srvExportNamespace: "ns1",
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
			inDataStore:  true,
			refByService: true,
			wantErrIsNil: true,
			expectedTargetList: []latticemodel.Target{
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
			name: "backendRef port is defined",
			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "export6",
					Namespace: string(namespace),
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: gateway_api.BackendRef{
										BackendObjectReference: gateway_api.BackendObjectReference{
											Name:      "targetgroup1",
											Namespace: &namespace,
											Kind:      &serviceKind,
											Port:      PortNumberPtr(8070),
										},
									},
								},
							},
						},
					},
				},
			},
			srvExportName:      "export6",
			srvExportNamespace: string(namespace),
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: string(namespace),
						Name:      "export6",
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{{IP: "10.10.1.1"}, {IP: "10.10.2.2"}},
							Ports:     []corev1.EndpointPort{{Name: "a", Port: 8070}, {Name: "b", Port: 3090}},
						},
					},
				},
			},
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         string(namespace),
					Name:              "export6",
					DeletionTimestamp: nil,
				},
			},
			inDataStore:  true,
			refByService: true,
			wantErrIsNil: true,
			expectedTargetList: []latticemodel.Target{
				{
					TargetIP: "10.10.1.1",
					Port:     8070,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     8070,
				},
			},
		},
		{
			name: "backendRef port is NOT defined",
			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: string(namespace),
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: gateway_api.BackendRef{
										BackendObjectReference: gateway_api.BackendObjectReference{
											Name:      "targetgroup2",
											Namespace: &namespace,
											Kind:      &serviceKind,
										},
									},
								},
							},
						},
					},
				},
			},
			srvExportName:      "export7",
			srvExportNamespace: string(namespace),
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: string(namespace),
						Name:      "export7",
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{{IP: "10.10.1.1"}, {IP: "10.10.2.2"}},
							Ports:     []corev1.EndpointPort{{Name: "a", Port: 8070}, {Name: "b", Port: 3090}},
						},
					},
				},
			},
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         string(namespace),
					Name:              "export7",
					DeletionTimestamp: nil,
				},
			},
			inDataStore:  true,
			refByService: true,
			wantErrIsNil: true,
			expectedTargetList: []latticemodel.Target{
				{
					TargetIP: "10.10.1.1",
					Port:     8070,
				},
				{
					TargetIP: "10.10.1.1",
					Port:     3090,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     8070,
				},
				{
					TargetIP: "10.10.2.2",
					Port:     3090,
				},
			},
		},
		{
			name: "backendRef defined port does not mathch any of the service ports",
			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: string(namespace),
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: gateway_api.BackendRef{
										BackendObjectReference: gateway_api.BackendObjectReference{
											Name:      "targetgroup2",
											Namespace: &namespace,
											Kind:      &serviceKind,
											Port:      PortNumberPtr(123),
										},
									},
								},
							},
						},
					},
				},
			},
			srvExportName:      "export7",
			srvExportNamespace: string(namespace),
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: string(namespace),
						Name:      "export7",
					},
					Subsets: []corev1.EndpointSubset{
						{
							Addresses: []corev1.EndpointAddress{{IP: "10.10.1.1"}, {IP: "10.10.2.2"}},
							Ports:     []corev1.EndpointPort{{Name: "a", Port: 8070}, {Name: "b", Port: 3090}},
						},
					},
				},
			},
			svc: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         string(namespace),
					Name:              "export7",
					DeletionTimestamp: nil,
				},
			},
			inDataStore:        true,
			refByService:       true,
			wantErrIsNil:       true,
			expectedTargetList: nil,
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		k8sSchema := runtime.NewScheme()
		k8sSchema.AddKnownTypes(mcs_api.SchemeGroupVersion, &mcs_api.ServiceExport{})
		clientgoscheme.AddToScheme(k8sSchema)
		k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)

		if !reflect.DeepEqual(tt.serviceExport, mcs_api.ServiceExport{}) {
			assert.NoError(t, k8sClient.Create(ctx, tt.serviceExport.DeepCopy()))
		}

		if len(tt.endPoints) > 0 {
			assert.NoError(t, k8sClient.Create(ctx, tt.endPoints[0].DeepCopy()))
		}

		assert.NoError(t, k8sClient.Create(ctx, tt.svc.DeepCopy()))

		ds := latticestore.NewLatticeDataStore()

		if tt.inDataStore {
			tgName := latticestore.TargetGroupName(tt.srvExportName, tt.srvExportNamespace)
			err := ds.AddTargetGroup(tgName, "", "", "", false, "")
			assert.Nil(t, err)
			if tt.refByServiceExport {
				ds.SetTargetGroupByServiceExport(tgName, false, true)
			}
			if tt.refByService {
				ds.SetTargetGroupByBackendRef(tgName, "", false, true)
			}

		}

		srvName := types.NamespacedName{
			Name:      tt.srvExportName,
			Namespace: tt.srvExportNamespace,
		}
		targetTask := &latticeTargetsModelBuildTask{
			Client:      k8sClient,
			tgName:      tt.srvExportName,
			tgNamespace: tt.srvExportNamespace,
			datastore:   ds,
			stack:       core.NewDefaultStack(core.StackID(srvName)),
			httpRoute:   tt.httpRoute,
		}
		err := targetTask.buildLatticeTargets(ctx)
		if tt.wantErrIsNil {
			fmt.Printf("t.latticeTargets %v \n", targetTask.latticeTargets)
			assert.Equal(t, tt.srvExportName, targetTask.latticeTargets.Spec.Name)
			assert.Equal(t, tt.srvExportNamespace, targetTask.latticeTargets.Spec.Namespace)

			// verify targets, ports are built correctly
			assert.Equal(t, tt.expectedTargetList, targetTask.latticeTargets.Spec.TargetIPList)
			assert.Nil(t, err)

		} else {
			assert.NotNil(t, err)
		}

	}
}
