package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

/*
case1: register targets Successfully
case2: target group does not exist
case3: failed register targets
case4: register targets Unsuccessfully
*/

func Test_RegisterTargets_RegisterSuccessfully(t *testing.T) {
	targets := model.Target{
		TargetIP: "123.456.78",
		Port:     int64(8080),
	}
	targetsSpec := model.TargetsSpec{
		Name:          "test",
		TargetGroupID: "123456789",
		TargetIPList:  []model.Target{targets},
	}
	createInput := model.Targets{
		ResourceMeta: core.ResourceMeta{},
		Spec:         targetsSpec,
	}

	id := "123456789"
	ip := "123.456.78"
	port := int64(8080)
	targetInput := &vpclattice.Target{
		Id:   &ip,
		Port: &port,
	}
	registerTargetsInput := &vpclattice.RegisterTargetsInput{
		TargetGroupIdentifier: &id,
		Targets:               []*vpclattice.Target{targetInput},
	}

	tgCreateOutput := &vpclattice.RegisterTargetsOutput{}
	listTargetOutput := []*vpclattice.TargetSummary{}

	latticeDataStore := latticestore.NewLatticeDataStore()
	tgName := latticestore.TargetGroupName("test", "")
	//TODO routename
	latticeDataStore.AddTargetGroup(tgName, "vpc-123456789", "123456789", "123456789", false, "")
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)

	mockLattice.EXPECT().RegisterTargetsWithContext(ctx, registerTargetsInput).Return(tgCreateOutput, nil)
	mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud, latticeDataStore)
	err := targetsManager.Create(ctx, &createInput)

	assert.Nil(t, err)
}

// Target group does not exist, should return Retry
func Test_RegisterTargets_TGNotExist(t *testing.T) {
	targetsSpec := model.TargetsSpec{
		Name:          "test",
		TargetGroupID: "123456789",
	}
	createInput := model.Targets{
		ResourceMeta: core.ResourceMeta{},
		Spec:         targetsSpec,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	latticeDataStore := latticestore.NewLatticeDataStore()
	mockCloud := mocks_aws.NewMockCloud(c)

	targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud, latticeDataStore)
	err := targetsManager.Create(ctx, &createInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
}

// case3: api call to register target fails
func Test_RegisterTargets_Registerfailed(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targetsList := &vpclattice.TargetSummary{
		Id:   &sId,
		Port: &sPort,
	}
	targetsSuccessful := &vpclattice.Target{
		Id:   &sId,
		Port: &sPort,
	}
	successful := []*vpclattice.Target{targetsSuccessful}
	deRegisterTargetsOutput := &vpclattice.DeregisterTargetsOutput{
		Successful: successful,
	}

	listTargetOutput := []*vpclattice.TargetSummary{targetsList}

	targetsSpec := model.TargetsSpec{
		Name:          "test",
		Namespace:     "",
		TargetGroupID: "123456789",
		TargetIPList: []model.Target{
			{
				TargetIP: "123.456.7.891",
				Port:     sPort,
			},
		},
	}

	planToRegister := model.Targets{
		ResourceMeta: core.ResourceMeta{},
		Spec:         targetsSpec,
	}

	registerTargetsOutput := &vpclattice.RegisterTargetsOutput{}

	latticeDataStore := latticestore.NewLatticeDataStore()
	tgName := latticestore.TargetGroupName("test", "")
	// routename
	latticeDataStore.AddTargetGroup(tgName, "vpc-123456789", "123456789", "123456789", false, "")
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)

	mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetOutput, nil)
	mockLattice.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(deRegisterTargetsOutput, nil)
	mockLattice.EXPECT().RegisterTargetsWithContext(ctx, gomock.Any()).Return(registerTargetsOutput, errors.New("Register_Targets_Failed"))
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud, latticeDataStore)
	err := targetsManager.Create(ctx, &planToRegister)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("Register_Targets_Failed"))
}

// case4: register targets Unsuccessfully
func Test_RegisterTargets_RegisterUnsuccessfully(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targetsList := &vpclattice.TargetSummary{
		Id:   &sId,
		Port: &sPort,
	}
	targetsSuccessful := &vpclattice.Target{
		Id:   &sId,
		Port: &sPort,
	}
	successful := []*vpclattice.Target{targetsSuccessful}
	deRegisterTargetsOutput := &vpclattice.DeregisterTargetsOutput{
		Successful: successful,
	}
	listTargetOutput := []*vpclattice.TargetSummary{targetsList}

	tgId := "123456789"
	deRegisterTargetsInput := &vpclattice.DeregisterTargetsInput{
		TargetGroupIdentifier: &tgId,
		Targets:               []*vpclattice.Target{targetsSuccessful},
	}

	targetToRegister := model.Target{
		TargetIP: "123.456.78",
		Port:     int64(8080),
	}
	targetsSpec := model.TargetsSpec{
		Name:          "test",
		Namespace:     "",
		TargetGroupID: tgId,
		TargetIPList:  []model.Target{targetToRegister},
	}
	planToRegister := model.Targets{
		ResourceMeta: core.ResourceMeta{},
		Spec:         targetsSpec,
	}

	ip := "123.456.78"
	port := int64(8080)
	targetToRegisterInput := &vpclattice.Target{
		Id:   &ip,
		Port: &port,
	}
	registerTargetsInput := vpclattice.RegisterTargetsInput{
		TargetGroupIdentifier: &tgId,
		Targets:               []*vpclattice.Target{targetToRegisterInput},
	}

	unsuccessfulId := "123.456.78"
	unsuccessfulPort := int64(8080)
	targetsUnsuccessful := &vpclattice.TargetFailure{
		Id:   &unsuccessfulId,
		Port: &unsuccessfulPort,
	}
	unsuccessful := []*vpclattice.TargetFailure{targetsUnsuccessful}

	registerTargetsOutput := &vpclattice.RegisterTargetsOutput{
		Unsuccessful: unsuccessful,
	}

	latticeDataStore := latticestore.NewLatticeDataStore()
	tgName := latticestore.TargetGroupName("test", "")
	//routename
	latticeDataStore.AddTargetGroup(tgName, "vpc-123456789", "123456789", "123456789", false, "")
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)

	mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetOutput, nil)
	mockLattice.EXPECT().DeregisterTargetsWithContext(ctx, deRegisterTargetsInput).Return(deRegisterTargetsOutput, nil)
	mockLattice.EXPECT().RegisterTargetsWithContext(ctx, &registerTargetsInput).Return(registerTargetsOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud, latticeDataStore)
	err := targetsManager.Create(ctx, &planToRegister)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
}

func Test_RegisterTargets_NoTargets_NoCallRegisterTargets(t *testing.T) {
	planToRegister := model.Targets{
		ResourceMeta: core.ResourceMeta{},
		Spec: model.TargetsSpec{
			Name:          "test",
			Namespace:     "",
			TargetGroupID: "123456789",
			TargetIPList:  []model.Target{},
		},
	}

	latticeDataStore := latticestore.NewLatticeDataStore()
	tgName := latticestore.TargetGroupName("test", "")

	listTargetOutput := []*vpclattice.TargetSummary{}

	// routename
	latticeDataStore.AddTargetGroup(tgName, "vpc-123456789", "123456789", "123456789", false, "")

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)

	mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetOutput, nil)
	// Expect not to call RegisterTargets
	mockLattice.EXPECT().RegisterTargetsWithContext(ctx, gomock.Any()).MaxTimes(0)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	targetsManager := NewTargetsManager(gwlog.FallbackLogger, mockCloud, latticeDataStore)
	err := targetsManager.Create(ctx, &planToRegister)

	assert.Nil(t, err)
}
