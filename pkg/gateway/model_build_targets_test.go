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
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Test_Targets(t *testing.T) {
	tests := []struct {
		name               string
		srvExportName      string
		srvExportNamespace string
		port               int32
		endPoints          []corev1.Endpoints
		svc                corev1.Service
		serviceExport      mcs_api.ServiceExport
		inDataStore        bool
		refByServiceExport bool
		refByService       bool
		wantErrIsNil       bool
		expectedTargetList []latticemodel.Target
		route              core.Route
	}{
		{
			name:               "Add all endpoints to build spec",
			srvExportName:      "export1",
			srvExportNamespace: "ns1",
			port:               0,
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
			inDataStore:  true,
			refByService: true,
			wantErrIsNil: true,
			expectedTargetList: []latticemodel.Target{
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
			name:               "Add endpoints with matching service port to build spec",
			srvExportName:      "export1",
			srvExportNamespace: "ns1",
			port:               80,
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
			inDataStore:  true,
			refByService: true,
			wantErrIsNil: true,
			expectedTargetList: []latticemodel.Target{
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
			name:               "Add all endpoints to build spec with port annotation",
			srvExportName:      "export1",
			srvExportNamespace: "ns1",
			port:               3090,
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
			serviceExport: mcs_api.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         "ns1",
					Name:              "export1",
					DeletionTimestamp: nil,
					Annotations:       map[string]string{"multicluster.x-k8s.io/port": "81"},
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
			port:               0,
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
			port:               0,
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
			port:               0,
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
			port:               0,
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
			port:               0,
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
			port:               0,
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
			name:               "Only add endpoints for port 8675 to build spec",
			srvExportName:      "export6",
			srvExportNamespace: "ns1",
			port:               8675,
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
			inDataStore:        true,
			refByServiceExport: true,
			wantErrIsNil:       true,
			expectedTargetList: []latticemodel.Target{
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
			name:               "BackendRef port does not match service port",
			srvExportName:      "export7",
			srvExportNamespace: "ns1",
			port:               8750,
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
			inDataStore:        false,
			refByService:       true,
			refByServiceExport: true,
			wantErrIsNil:       false,
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
			client:         k8sClient,
			tgName:         tt.srvExportName,
			tgNamespace:    tt.srvExportNamespace,
			datastore:      ds,
			backendRefPort: tt.port,
			stack:          core.NewDefaultStack(core.StackID(srvName)),
			route:          tt.route,
		}
		err := targetTask.buildLatticeTargets(ctx)
		if tt.wantErrIsNil {
			assert.Nil(t, err)

			fmt.Printf("t.latticeTargets %v \n", targetTask.latticeTargets)
			assert.Equal(t, tt.srvExportName, targetTask.latticeTargets.Spec.Name)
			assert.Equal(t, tt.srvExportNamespace, targetTask.latticeTargets.Spec.Namespace)

			// verify targets, ports are built correctly
			assert.Equal(t, tt.expectedTargetList, targetTask.latticeTargets.Spec.TargetIPList)

		} else {
			assert.NotNil(t, err)
		}

	}
}
