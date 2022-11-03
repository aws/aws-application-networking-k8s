package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/service/mercury"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

/*
case1: register targets Successfully
case2: target group does not exist
case3: failed register targets
case4: register targets Unsuccessfully
*/

func Test_RegisterTargets_RegisterSuccessfully(t *testing.T) {
	targets := latticemodel.Target{
		TargetIP: "123.456.78",
		Port:     int64(8080),
	}
	targetsSpec := latticemodel.TargetsSpec{
		Name:          "test",
		TargetGroupID: "123456789",
		TargetIPList:  []latticemodel.Target{targets},
	}
	createInput := latticemodel.Targets{
		ResourceMeta: core.ResourceMeta{},
		Spec:         targetsSpec,
	}

	id := "123456789"
	ip := "123.456.78"
	port := int64(8080)
	targetInput := &mercury.Target{
		Id:   &ip,
		Port: &port,
	}
	registerTargetsInput := &mercury.RegisterTargetsInput{
		TargetGroupIdentifier: &id,
		Targets:               []*mercury.Target{targetInput},
	}

	tgCreateOutput := &mercury.RegisterTargetsOutput{}
	listTargetOutput := []*mercury.TargetSummary{}

	latticeDataStore := latticestore.NewLatticeDataStore()
	tgName := latticestore.TargetGroupName("test", "")
	latticeDataStore.AddTargetGroup(tgName, "vpc-123456789", "123456789", "123456789", false)
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess := mocks.NewMockMercury(c)

	mockMercurySess.EXPECT().RegisterTargetsWithContext(ctx, registerTargetsInput).Return(tgCreateOutput, nil)
	mockMercurySess.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	targetsManager := NewTargetsManager(mockCloud, latticeDataStore)
	err := targetsManager.Create(ctx, &createInput)

	assert.Nil(t, err)
}

// Target group does not exist, should return Retry
func Test_RegisterTargets_TGNotExist(t *testing.T) {
	targetsSpec := latticemodel.TargetsSpec{
		Name:          "test",
		TargetGroupID: "123456789",
	}
	createInput := latticemodel.Targets{
		ResourceMeta: core.ResourceMeta{},
		Spec:         targetsSpec,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	latticeDataStore := latticestore.NewLatticeDataStore()
	mockCloud := mocks_aws.NewMockCloud(c)

	targetsManager := NewTargetsManager(mockCloud, latticeDataStore)
	err := targetsManager.Create(ctx, &createInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
}

// case3: api call to register target fails
func Test_RegisterTargets_Registerfailed(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targetsList := &mercury.TargetSummary{
		Id:   &sId,
		Port: &sPort,
	}
	targetsSuccessful := &mercury.Target{
		Id:   &sId,
		Port: &sPort,
	}
	successful := []*mercury.Target{targetsSuccessful}
	deRegisterTargetsOutput := &mercury.DeregisterTargetsOutput{
		Successful: successful,
	}

	listTargetOutput := []*mercury.TargetSummary{targetsList}

	targetsSpec := latticemodel.TargetsSpec{
		Name:          "test",
		Namespace:     "",
		TargetGroupID: "123456789",
		TargetIPList:  nil,
	}
	planToRegister := latticemodel.Targets{
		ResourceMeta: core.ResourceMeta{},
		Spec:         targetsSpec,
	}
	registerTargetsOutput := &mercury.RegisterTargetsOutput{}

	latticeDataStore := latticestore.NewLatticeDataStore()
	tgName := latticestore.TargetGroupName("test", "")
	latticeDataStore.AddTargetGroup(tgName, "vpc-123456789", "123456789", "123456789", false)
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess := mocks.NewMockMercury(c)

	mockMercurySess.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetOutput, nil)
	mockMercurySess.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(deRegisterTargetsOutput, nil)
	mockMercurySess.EXPECT().RegisterTargetsWithContext(ctx, gomock.Any()).Return(registerTargetsOutput, errors.New("Register_Targets_Failed"))
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	targetsManager := NewTargetsManager(mockCloud, latticeDataStore)
	err := targetsManager.Create(ctx, &planToRegister)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("Register_Targets_Failed"))
}

// case4: register targets Unsuccessfully
func Test_RegisterTargets_RegisterUnsuccessfully(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targetsList := &mercury.TargetSummary{
		Id:   &sId,
		Port: &sPort,
	}
	targetsSuccessful := &mercury.Target{
		Id:   &sId,
		Port: &sPort,
	}
	successful := []*mercury.Target{targetsSuccessful}
	deRegisterTargetsOutput := &mercury.DeregisterTargetsOutput{
		Successful: successful,
	}
	listTargetOutput := []*mercury.TargetSummary{targetsList}

	tgId := "123456789"
	deRegisterTargetsInput := &mercury.DeregisterTargetsInput{
		TargetGroupIdentifier: &tgId,
		Targets:               []*mercury.Target{targetsSuccessful},
	}

	targetToRegister := latticemodel.Target{
		TargetIP: "123.456.78",
		Port:     int64(8080),
	}
	targetsSpec := latticemodel.TargetsSpec{
		Name:          "test",
		Namespace:     "",
		TargetGroupID: tgId,
		TargetIPList:  []latticemodel.Target{targetToRegister},
	}
	planToRegister := latticemodel.Targets{
		ResourceMeta: core.ResourceMeta{},
		Spec:         targetsSpec,
	}

	ip := "123.456.78"
	port := int64(8080)
	targetToRegisterInput := &mercury.Target{
		Id:   &ip,
		Port: &port,
	}
	registerTargetsInput := mercury.RegisterTargetsInput{
		TargetGroupIdentifier: &tgId,
		Targets:               []*mercury.Target{targetToRegisterInput},
	}

	unsuccessfulId := "123.456.78"
	unsuccessfulPort := int64(8080)
	targetsUnsuccessful := &mercury.TargetFailure{
		Id:   &unsuccessfulId,
		Port: &unsuccessfulPort,
	}
	unsuccessful := []*mercury.TargetFailure{targetsUnsuccessful}

	registerTargetsOutput := &mercury.RegisterTargetsOutput{
		Unsuccessful: unsuccessful,
	}

	latticeDataStore := latticestore.NewLatticeDataStore()
	tgName := latticestore.TargetGroupName("test", "")
	latticeDataStore.AddTargetGroup(tgName, "vpc-123456789", "123456789", "123456789", false)
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess := mocks.NewMockMercury(c)

	mockMercurySess.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetOutput, nil)
	mockMercurySess.EXPECT().DeregisterTargetsWithContext(ctx, deRegisterTargetsInput).Return(deRegisterTargetsOutput, nil)
	mockMercurySess.EXPECT().RegisterTargetsWithContext(ctx, &registerTargetsInput).Return(registerTargetsOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	targetsManager := NewTargetsManager(mockCloud, latticeDataStore)
	err := targetsManager.Create(ctx, &planToRegister)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
}
