package lattice

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"testing"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func TestTargetsManager(t *testing.T) {
	targets := model.Target{
		TargetIP: "192.0.2.10",
		Port:     int64(8080),
	}
	targetsSpec := model.TargetsSpec{
		StackTargetGroupId: "tg-stack-id",
		TargetList:         []model.Target{targets},
	}
	modelTargets := model.Targets{
		Spec: targetsSpec,
	}

	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})
	modelTg := model.TargetGroup{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "tg-stack-id"),
		Status: &model.TargetGroupStatus{
			Name: "tg-name",
			Arn:  "tg-arn",
			Id:   "tg-id",
		},
	}

	targetInput := &vpclattice.Target{
		Id:   aws.String(targets.TargetIP),
		Port: aws.Int64(targets.Port),
	}
	registerTargetsInput := &vpclattice.RegisterTargetsInput{
		TargetGroupIdentifier: aws.String("tg-id"),
		Targets:               []*vpclattice.Target{targetInput},
	}

	registerTargetsOutput := &vpclattice.RegisterTargetsOutput{}
	var emptyListTargetOutput []*vpclattice.TargetSummary

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	t.Run("success - no current targets", func(t *testing.T) {
		mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(emptyListTargetOutput, nil)
		mockLattice.EXPECT().RegisterTargetsWithContext(ctx, registerTargetsInput).Return(registerTargetsOutput, nil)

		targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud)
		err := targetsManager.Update(ctx, &modelTargets, &modelTg)

		assert.Nil(t, err)
	})

	t.Run("success - deregister targets, no target overlap", func(t *testing.T) {
		existingTarget := &vpclattice.TargetSummary{
			Id:   aws.String("192.0.2.250"),
			Port: aws.Int64(80),
		}
		existingTargets := []*vpclattice.TargetSummary{existingTarget}

		deregisterTargets := []*vpclattice.Target{
			{
				Id:   existingTarget.Id,
				Port: existingTarget.Port,
			},
		}
		deregisterInput := &vpclattice.DeregisterTargetsInput{
			TargetGroupIdentifier: aws.String(modelTg.Status.Id),
			Targets:               deregisterTargets,
		}
		deregisterOutput := &vpclattice.DeregisterTargetsOutput{
			Successful: deregisterTargets,
		}

		mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(existingTargets, nil)
		mockLattice.EXPECT().DeregisterTargetsWithContext(ctx, deregisterInput).Return(deregisterOutput, nil)
		mockLattice.EXPECT().RegisterTargetsWithContext(ctx, registerTargetsInput).Return(registerTargetsOutput, nil)

		targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud)
		err := targetsManager.Update(ctx, &modelTargets, &modelTg)

		assert.Nil(t, err)
	})

	t.Run("failures", func(t *testing.T) {
		// error on ListTargetsAsList
		mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(nil, errors.New("List_Targets_Failed"))

		targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud)
		err := targetsManager.Update(ctx, &modelTargets, &modelTg)
		assert.NotNil(t, err)

		// error on RegisterTargetsWithContext
		mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(emptyListTargetOutput, nil)
		mockLattice.EXPECT().RegisterTargetsWithContext(ctx, gomock.Any()).Return(registerTargetsOutput, errors.New("Register_Targets_Failed"))

		targetsManager = NewTargetsManager(gwlog.FallbackLogger, mockCloud)
		err = targetsManager.Update(ctx, &modelTargets, &modelTg)
		assert.NotNil(t, err)

		// error on DeregisterTargetsWithContext
		existingTarget := &vpclattice.TargetSummary{
			Id:   aws.String("192.0.2.250"),
			Port: aws.Int64(80),
		}
		existingTargets := []*vpclattice.TargetSummary{existingTarget}
		mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(existingTargets, nil)
		mockLattice.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(nil, errors.New("Deregister_Targets_Failed"))
		mockLattice.EXPECT().RegisterTargetsWithContext(ctx, gomock.Any()).Return(registerTargetsOutput, nil)

		targetsManager = NewTargetsManager(gwlog.FallbackLogger, mockCloud)
		err = targetsManager.Update(ctx, &modelTargets, &modelTg)
		assert.NotNil(t, err)
	})

	t.Run("basic validation", func(t *testing.T) {
		targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud)

		missingStatusTg := model.TargetGroup{}
		err := targetsManager.Update(ctx, &modelTargets, &missingStatusTg)
		assert.NotNil(t, err)

		missingStatusTg = model.TargetGroup{Status: &model.TargetGroupStatus{}}
		err = targetsManager.Update(ctx, &modelTargets, &missingStatusTg)
		assert.NotNil(t, err)

		mismatchedId := "not-the-same-stack-id"
		mismatchedTg := model.TargetGroup{
			ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", mismatchedId),
			Status:       &model.TargetGroupStatus{Id: "tg-id"},
		}
		err = targetsManager.Update(ctx, &modelTargets, &mismatchedTg)
		assert.NotNil(t, err)
	})

	t.Run("register unsuccessful returns error", func(t *testing.T) {
		unsuccessful := []*vpclattice.TargetFailure{
			{
				Id:   aws.String(targets.TargetIP),
				Port: aws.Int64(targets.Port),
			},
		}
		unsuccessfulRTO := &vpclattice.RegisterTargetsOutput{
			Unsuccessful: unsuccessful,
		}

		mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(emptyListTargetOutput, nil)
		mockLattice.EXPECT().RegisterTargetsWithContext(ctx, registerTargetsInput).Return(unsuccessfulRTO, nil)

		targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud)
		err := targetsManager.Update(ctx, &modelTargets, &modelTg)

		assert.NotNil(t, err)
	})

	t.Run("no targets does not register", func(t *testing.T) {
		emptyTargetsSpec := model.TargetsSpec{
			StackTargetGroupId: "tg-stack-id",
			TargetList:         []model.Target{},
		}
		emptyModelTargets := model.Targets{
			Spec: emptyTargetsSpec,
		}

		mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(emptyListTargetOutput, nil)

		targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud)
		err := targetsManager.Update(ctx, &emptyModelTargets, &modelTg)

		assert.Nil(t, err)
	})

	t.Run("overlapping target sets does the right thing", func(t *testing.T) {
		mt1 := model.Target{
			TargetIP: "192.0.2.10",
			Port:     int64(8080),
		}
		mt2 := model.Target{
			TargetIP: "192.0.2.20",
			Port:     int64(8080),
		}
		mt3 := model.Target{
			TargetIP: "192.0.2.30",
			Port:     int64(8080),
		}

		existingTargets := []*vpclattice.TargetSummary{
			{
				Id:   aws.String(mt1.TargetIP),
				Port: aws.Int64(mt1.Port),
			},
			{
				Id:   aws.String(mt2.TargetIP),
				Port: aws.Int64(mt2.Port),
			},
		}

		newTargets := model.Targets{
			Spec: model.TargetsSpec{
				StackTargetGroupId: "tg-stack-id",
				TargetList:         []model.Target{mt2, mt3},
			},
		}

		deregisterTargets := []*vpclattice.Target{
			{
				Id:   aws.String(mt1.TargetIP),
				Port: aws.Int64(mt1.Port),
			},
		}

		deregisterInput := &vpclattice.DeregisterTargetsInput{
			TargetGroupIdentifier: aws.String(modelTg.Status.Id),
			Targets:               deregisterTargets,
		}
		deregisterOutput := &vpclattice.DeregisterTargetsOutput{
			Successful: deregisterTargets,
		}

		registerInput := &vpclattice.RegisterTargetsInput{
			TargetGroupIdentifier: aws.String("tg-id"),
			Targets: []*vpclattice.Target{
				{Id: aws.String(mt2.TargetIP), Port: aws.Int64(mt2.Port)},
				{Id: aws.String(mt3.TargetIP), Port: aws.Int64(mt3.Port)},
			},
		}

		mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(existingTargets, nil)
		mockLattice.EXPECT().RegisterTargetsWithContext(ctx, registerInput).Return(registerTargetsOutput, nil)
		mockLattice.EXPECT().DeregisterTargetsWithContext(ctx, deregisterInput).Return(deregisterOutput, nil)

		targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud)
		err := targetsManager.Update(ctx, &newTargets, &modelTg)

		assert.Nil(t, err)

	})

	t.Run("port difference handled correctly", func(t *testing.T) {
		existingTarget := &vpclattice.TargetSummary{
			Id:   aws.String(targets.TargetIP),
			Port: aws.Int64(targets.Port + 1), // <-- the important bit
		}
		existingTargets := []*vpclattice.TargetSummary{existingTarget}

		deregisterTargets := []*vpclattice.Target{
			{
				Id:   existingTarget.Id,
				Port: existingTarget.Port,
			},
		}
		deregisterInput := &vpclattice.DeregisterTargetsInput{
			TargetGroupIdentifier: aws.String(modelTg.Status.Id),
			Targets:               deregisterTargets,
		}
		deregisterOutput := &vpclattice.DeregisterTargetsOutput{
			Successful: deregisterTargets,
		}

		mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(existingTargets, nil)
		mockLattice.EXPECT().DeregisterTargetsWithContext(ctx, deregisterInput).Return(deregisterOutput, nil)
		mockLattice.EXPECT().RegisterTargetsWithContext(ctx, registerTargetsInput).Return(registerTargetsOutput, nil)

		targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud)
		err := targetsManager.Update(ctx, &modelTargets, &modelTg)

		assert.Nil(t, err)
	})
}
