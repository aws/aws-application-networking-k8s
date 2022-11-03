package gateway

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_Targets(t *testing.T) {
	tests := []struct {
		name               string
		srvExportName      string
		srvExportNamespace string
		endPoints          []corev1.Endpoints
		inDataStore        bool
		refByServiceExport bool
		refByService       bool
		wantErrIsNil       bool
		expectedTargetList []latticemodel.Target
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
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		k8sSchema := runtime.NewScheme()
		clientgoscheme.AddToScheme(k8sSchema)
		k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)

		if len(tt.endPoints) > 0 {
			assert.NoError(t, k8sClient.Create(ctx, tt.endPoints[0].DeepCopy()))
		}

		ds := latticestore.NewLatticeDataStore()

		if tt.inDataStore {
			tgName := latticestore.TargetGroupName(tt.srvExportName, tt.srvExportNamespace)
			err := ds.AddTargetGroup(tgName, "", "", "", false)
			assert.Nil(t, err)
			if tt.refByServiceExport {
				ds.SetTargetGroupByServiceExport(tgName, false, true)
			}
			if tt.refByService {
				ds.SetTargetGroupByBackendRef(tgName, false, true)
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
