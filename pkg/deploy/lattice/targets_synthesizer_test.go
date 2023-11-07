package lattice

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_SynthesizeTargets(t *testing.T) {

	targetList := []model.Target{
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
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockTargetsManager := NewMockTargetsManager(c)

	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})

	modelTg := model.TargetGroup{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "tg-stack-id"),
		Status: &model.TargetGroupStatus{
			Name: "tg-name",
			Arn:  "tg-arn",
			Id:   "tg-id",
		},
	}
	assert.NoError(t, stack.AddResource(&modelTg))

	targetsSpec := model.TargetsSpec{
		StackTargetGroupId: modelTg.ID(),
		TargetList:         targetList,
	}
	model.NewTargets(stack, targetsSpec)

	mockTargetsManager.EXPECT().Update(ctx, gomock.Any(), gomock.Any()).Return(nil)

	synthesizer := NewTargetsSynthesizer(gwlog.FallbackLogger, nil, mockTargetsManager, stack)
	err := synthesizer.Synthesize(ctx)
	assert.Nil(t, err)
}
