package lattice

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func Test_SynthesizeTargets(t *testing.T) {
	tests := []struct {
		name               string
		srvExportName      string
		srvExportNamespace string
		targetList         []latticemodel.Target
		expectedTargetList []latticestore.Target
	}{
		{
			name:               "Add all endpoints to build spec",
			srvExportName:      "export1",
			srvExportNamespace: "ns1",
			targetList: []latticemodel.Target{
				{
					TargetIP: "10.10.1.1",
					Port:     8675,
				},
				{
					TargetIP: "10.10.1.1",
					Port:     309,
				},
				{
					TargetIP: "10.10.1.2",
					Port:     8675,
				},
				{
					TargetIP: "10.10.1.2",
					Port:     309,
				},
			},
			expectedTargetList: []latticestore.Target{
				{
					TargetIP:   "10.10.1.1",
					TargetPort: 8675,
				},
				{
					TargetIP:   "10.10.1.1",
					TargetPort: 309,
				},
				{
					TargetIP:   "10.10.1.2",
					TargetPort: 8675,
				},
				{
					TargetIP:   "10.10.1.2",
					TargetPort: 309,
				},
			},
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		ds := latticestore.NewLatticeDataStore()

		tgName := latticestore.TargetGroupName(tt.srvExportName, tt.srvExportNamespace)
		// TODO routename
		err := ds.AddTargetGroup(tgName, "", "", "", false, "")
		assert.Nil(t, err)
		ds.SetTargetGroupByServiceExport(tgName, false, true)

		mockTargetsManager := NewMockTargetsManager(c)
		tgNamespacedName := types.NamespacedName{
			Namespace: tt.srvExportNamespace,
			Name:      tt.srvExportName,
		}

		mockStack := core.NewDefaultStack(core.StackID(tgNamespacedName))

		targetsSynthesizer := NewTargetsSynthesizer(gwlog.FallbackLogger, nil, mockTargetsManager, mockStack, ds)

		targetsSpec := latticemodel.TargetsSpec{
			Name:         tt.srvExportName,
			Namespace:    tt.srvExportNamespace,
			TargetIPList: tt.targetList,
		}
		modelTarget := latticemodel.Targets{
			Spec: targetsSpec,
		}

		resTargetsList := []*latticemodel.Targets{}

		resTargetsList = append(resTargetsList, &modelTarget)

		mockTargetsManager.EXPECT().Create(ctx, gomock.Any()).Return(nil)

		err = targetsSynthesizer.SynthesizeTargets(ctx, resTargetsList)
		assert.Nil(t, err)

		// TODO routename
		dsTG, err := ds.GetTargetGroup(tgName, "", false)
		assert.Equal(t, tt.expectedTargetList, dsTG.EndPoints)

		assert.Nil(t, err)
		fmt.Printf("dsTG: %v \n", dsTG)

		assert.Nil(t, err)

	}

}
