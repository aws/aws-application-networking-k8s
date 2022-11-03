package lattice

import (
	"context"
	"errors"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-sdk-go/service/mercury"
	"testing"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// target group does not exist, and is active after creation
func Test_CreateTargetGroup_TGNotExist_Active(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	tgSpec := latticemodel.TargetGroupSpec{
		Name: "test",
		Config: latticemodel.TargetGroupConfig{
			Port:            int32(8080),
			Protocol:        "HTTP",
			VpcID:           config.VpcID,
			EKSClusterName:  "",
			IsServiceImport: false,
		},
	}
	tgCreateInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	mockMercurySess := mocks.NewMockMercury(c)
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	tgStatus := mercury.TargetGroupStatusActive
	tgCreateOutput := &mercury.CreateTargetGroupOutput{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &tgStatus,
	}
	p := int64(8080)
	prot := "HTTP"
	emptystring := ""
	config := &mercury.TargetGroupConfig{
		Port:            &p,
		Protocol:        &prot,
		VpcIdentifier:   &config.VpcID,
		ProtocolVersion: &emptystring,
	}

	createTargetGroupInput := mercury.CreateTargetGroupInput{
		Config: config,
		Name:   &name,
		Type:   &emptystring,
	}

	listTgOutput := []*mercury.TargetGroupSummary{}

	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
	mockMercurySess.EXPECT().CreateTargetGroupWithContext(ctx, &createTargetGroupInput).Return(tgCreateOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)
	resp, err := tgManager.Create(ctx, &tgCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.TargetGroupARN, arn)
	assert.Equal(t, resp.TargetGroupID, id)
}

// target group status is failed, and is active after creation
func Test_CreateTargetGroup_TGFailed_Active(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	tgSpec := latticemodel.TargetGroupSpec{
		Name:   "test",
		Config: latticemodel.TargetGroupConfig{},
	}
	tgCreateInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
		Status:       nil,
	}
	mockMercurySess := mocks.NewMockMercury(c)
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	tgStatus := mercury.TargetGroupStatusActive
	tgCreateOutput := &mercury.CreateTargetGroupOutput{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &tgStatus,
	}

	beforeCreateStatus := mercury.TargetGroupStatusCreateFailed
	tgSummary := mercury.TargetGroupSummary{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &beforeCreateStatus,
	}
	listTgOutput := []*mercury.TargetGroupSummary{&tgSummary}

	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
	mockMercurySess.EXPECT().CreateTargetGroupWithContext(ctx, gomock.Any()).Return(tgCreateOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)
	resp, err := tgManager.Create(ctx, &tgCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.TargetGroupARN, arn)
	assert.Equal(t, resp.TargetGroupID, id)
}

// target group status is active before creation, no need to recreate
func Test_CreateTargetGroup_TGActive_ACTIVE(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	tgSpec := latticemodel.TargetGroupSpec{
		Name:   "test",
		Config: latticemodel.TargetGroupConfig{},
	}
	tgCreateInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	mockMercurySess := mocks.NewMockMercury(c)
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"

	beforeCreateStatus := mercury.TargetGroupStatusActive
	tgSummary := mercury.TargetGroupSummary{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &beforeCreateStatus,
	}
	listTgOutput := []*mercury.TargetGroupSummary{&tgSummary}

	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)
	resp, err := tgManager.Create(ctx, &tgCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.TargetGroupARN, arn)
	assert.Equal(t, resp.TargetGroupID, id)
}

// target group status is create-in-progress before creation, return Retry
func Test_CreateTargetGroup_TGCreateInProgress_Retry(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	tgSpec := latticemodel.TargetGroupSpec{
		Name:   "test",
		Config: latticemodel.TargetGroupConfig{},
	}
	tgCreateInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	mockMercurySess := mocks.NewMockMercury(c)
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"

	beforeCreateStatus := mercury.TargetGroupStatusCreateInProgress
	tgSummary := mercury.TargetGroupSummary{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &beforeCreateStatus,
	}
	listTgOutput := []*mercury.TargetGroupSummary{&tgSummary}

	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, errors.New(LATTICE_RETRY))
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)
	resp, err := tgManager.Create(ctx, &tgCreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.TargetGroupARN, "")
	assert.Equal(t, resp.TargetGroupID, "")
}

// target group status is delete-in-progress before creation, return Retry
func Test_CreateTargetGroup_TGDeleteInProgress_Retry(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	tgSpec := latticemodel.TargetGroupSpec{
		Name:   "test",
		Config: latticemodel.TargetGroupConfig{},
	}
	tgCreateInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	mockMercurySess := mocks.NewMockMercury(c)
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"

	beforeCreateStatus := mercury.TargetGroupStatusDeleteInProgress
	tgSummary := mercury.TargetGroupSummary{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &beforeCreateStatus,
	}
	listTgOutput := []*mercury.TargetGroupSummary{&tgSummary}

	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, errors.New(LATTICE_RETRY))
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)
	resp, err := tgManager.Create(ctx, &tgCreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.TargetGroupARN, "")
	assert.Equal(t, resp.TargetGroupID, "")
}

// target group is not in-progress before, get update-in-progress, should return retry
/*
func Test_CreateTargetGroup_TGNotExist_UpdateInProgress(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	tgSpec := latticemodel.TargetGroupSpec{
		Name:   "test",
		Config: latticemodel.TargetGroupConfig{},
	}
	tgCreateInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	mockMercurySess := mocks.NewMockMercury(c)
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	tgStatus := mercury.TargetGroupStatusUpdateInProgress
	tgCreateOutput := &mercury.CreateTargetGroupOutput{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &tgStatus,
	}

	listTgOutput := []*mercury.TargetGroupSummary{}

	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
	mockMercurySess.EXPECT().CreateTargetGroupWithContext(ctx, gomock.Any()).Return(tgCreateOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)
	resp, err := tgManager.Create(ctx, &tgCreateInput)

	assert.NotNil(t, err)
	assert.NotNil(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.TargetGroupARN, "")
	assert.Equal(t, resp.TargetGroupID, "")
}
*/

// target group is not in-progress before, get create-in-progress, should return retry
func Test_CreateTargetGroup_TGNotExist_CreateInProgress(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	tgSpec := latticemodel.TargetGroupSpec{
		Name:   "test",
		Config: latticemodel.TargetGroupConfig{},
	}
	tgCreateInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	mockMercurySess := mocks.NewMockMercury(c)
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	tgStatus := mercury.TargetGroupStatusCreateInProgress
	tgCreateOutput := &mercury.CreateTargetGroupOutput{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &tgStatus,
	}

	listTgOutput := []*mercury.TargetGroupSummary{}

	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
	mockMercurySess.EXPECT().CreateTargetGroupWithContext(ctx, gomock.Any()).Return(tgCreateOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)
	resp, err := tgManager.Create(ctx, &tgCreateInput)

	assert.NotNil(t, err)
	assert.NotNil(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.TargetGroupARN, "")
	assert.Equal(t, resp.TargetGroupID, "")
}

// target group is not in-progress before, get delete-in-progress, should return retry
func Test_CreateTargetGroup_TGNotExist_DeleteInProgress(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	tgSpec := latticemodel.TargetGroupSpec{
		Name:   "test",
		Config: latticemodel.TargetGroupConfig{},
	}
	tgCreateInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	mockMercurySess := mocks.NewMockMercury(c)
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	tgStatus := mercury.TargetGroupStatusDeleteInProgress
	tgCreateOutput := &mercury.CreateTargetGroupOutput{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &tgStatus,
	}

	listTgOutput := []*mercury.TargetGroupSummary{}

	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
	mockMercurySess.EXPECT().CreateTargetGroupWithContext(ctx, gomock.Any()).Return(tgCreateOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)
	resp, err := tgManager.Create(ctx, &tgCreateInput)

	assert.NotNil(t, err)
	assert.NotNil(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.TargetGroupARN, "")
	assert.Equal(t, resp.TargetGroupID, "")
}

// target group is not in-progress before, get failed, should return retry
func Test_CreateTargetGroup_TGNotExist_Failed(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	tgSpec := latticemodel.TargetGroupSpec{
		Name:   "test",
		Config: latticemodel.TargetGroupConfig{},
	}
	tgCreateInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	mockMercurySess := mocks.NewMockMercury(c)
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	tgStatus := mercury.TargetGroupStatusCreateFailed
	tgCreateOutput := &mercury.CreateTargetGroupOutput{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &tgStatus,
	}

	listTgOutput := []*mercury.TargetGroupSummary{}

	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
	mockMercurySess.EXPECT().CreateTargetGroupWithContext(ctx, gomock.Any()).Return(tgCreateOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)
	resp, err := tgManager.Create(ctx, &tgCreateInput)

	assert.NotNil(t, err)
	assert.NotNil(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.TargetGroupARN, "")
	assert.Equal(t, resp.TargetGroupID, "")
}

// Failed to list target group, should return error
func Test_CreateTargetGroup_ListTGError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	tgSpec := latticemodel.TargetGroupSpec{
		Name:   "test",
		Config: latticemodel.TargetGroupConfig{},
	}
	tgCreateInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	mockMercurySess := mocks.NewMockMercury(c)
	listTgOutput := []*mercury.TargetGroupSummary{}

	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, errors.New("test"))
	mockCloud.EXPECT().Mercury().Return(mockMercurySess)
	tgManager := NewTargetGroupManager(mockCloud)
	resp, err := tgManager.Create(ctx, &tgCreateInput)

	assert.NotNil(t, err)
	assert.NotNil(t, err, errors.New("test"))
	assert.Equal(t, resp.TargetGroupARN, "")
	assert.Equal(t, resp.TargetGroupID, "")
}

// Failed to create target group, should return error
func Test_CreateTargetGroup_CreateTGFailed(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	tgSpec := latticemodel.TargetGroupSpec{
		Name:   "test",
		Config: latticemodel.TargetGroupConfig{},
	}
	tgCreateInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	mockMercurySess := mocks.NewMockMercury(c)
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	tgStatus := mercury.TargetGroupStatusCreateFailed
	tgCreateOutput := &mercury.CreateTargetGroupOutput{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &tgStatus,
	}

	listTgOutput := []*mercury.TargetGroupSummary{}

	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
	mockMercurySess.EXPECT().CreateTargetGroupWithContext(ctx, gomock.Any()).Return(tgCreateOutput, errors.New("test"))
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)
	resp, err := tgManager.Create(ctx, &tgCreateInput)

	assert.NotNil(t, err)
	assert.NotNil(t, err, errors.New("test"))
	assert.Equal(t, resp.TargetGroupARN, "")
	assert.Equal(t, resp.TargetGroupID, "")
}

// Case1: Deregister targets and delete target group work perfectly fine
// Case2: Delete target group that no targets register on it
// Case3: While deleting target group, deregister targets fails
// Case4: While deleting target group, list targets fails
// Case5: While deleting target group, deregister targets unsuccessfully
// Case6: Delete target group fails

// Case1: Deregister targets and delete target group work perfectly fine
func Test_DeleteTG_DeRegisterTargets_DeleteTargetGroup(t *testing.T) {
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

	listTargetsOutput := []*mercury.TargetSummary{targetsList}
	successful := []*mercury.Target{targetsSuccessful}
	deRegisterTargetsOutput := &mercury.DeregisterTargetsOutput{
		Successful: successful,
	}
	deleteTargetGroupOutput := &mercury.DeleteTargetGroupOutput{}

	tgSpec := latticemodel.TargetGroupSpec{
		Name:      "test",
		Config:    latticemodel.TargetGroupConfig{},
		Type:      "IP",
		IsDeleted: false,
		LatticeID: "123",
	}
	tgDeleteInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess := mocks.NewMockMercury(c)
	mockMercurySess.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, nil)
	mockMercurySess.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(deRegisterTargetsOutput, nil)
	mockMercurySess.EXPECT().DeleteTargetGroupWithContext(ctx, gomock.Any()).Return(deleteTargetGroupOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.Nil(t, err)
}

// Case2: Delete target group that no targets register on it
func Test_DeleteTG_NoRegisteredTargets_DeleteTargetGroup(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targets := &mercury.TargetSummary{
		Id:   &sId,
		Port: &sPort,
	}
	listTargetsOutput := []*mercury.TargetSummary{targets}
	deRegisterTargetsOutput := &mercury.DeregisterTargetsOutput{}
	deleteTargetGroupOutput := &mercury.DeleteTargetGroupOutput{}

	tgSpec := latticemodel.TargetGroupSpec{
		Name:      "test",
		Config:    latticemodel.TargetGroupConfig{},
		Type:      "IP",
		IsDeleted: false,
		LatticeID: "123",
	}
	tgDeleteInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess := mocks.NewMockMercury(c)
	mockMercurySess.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, nil)
	mockMercurySess.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(deRegisterTargetsOutput, nil)
	mockMercurySess.EXPECT().DeleteTargetGroupWithContext(ctx, gomock.Any()).Return(deleteTargetGroupOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.Nil(t, err)
}

// Case3: While deleting target group, deregister targets fails
func Test_DeleteTG_DeRegisteredTargetsFailed(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targets := &mercury.TargetSummary{
		Id:   &sId,
		Port: &sPort,
	}
	listTargetsOutput := []*mercury.TargetSummary{targets}
	deRegisterTargetsOutput := &mercury.DeregisterTargetsOutput{}

	tgSpec := latticemodel.TargetGroupSpec{
		Name:      "test",
		Config:    latticemodel.TargetGroupConfig{},
		Type:      "IP",
		IsDeleted: false,
		LatticeID: "123",
	}
	tgDeleteInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess := mocks.NewMockMercury(c)
	mockMercurySess.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, nil)
	mockMercurySess.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(deRegisterTargetsOutput, errors.New("Deregister_failed"))
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("Deregister_failed"))
}

// Case4: While deleting target group, list targets fails
func Test_DeleteTG_ListTargetsFailed(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targets := &mercury.TargetSummary{
		Id:   &sId,
		Port: &sPort,
	}
	listTargetsOutput := []*mercury.TargetSummary{targets}

	tgSpec := latticemodel.TargetGroupSpec{
		Name:      "test",
		Config:    latticemodel.TargetGroupConfig{},
		Type:      "IP",
		IsDeleted: false,
		LatticeID: "123",
	}
	tgDeleteInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
		Status:       nil,
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess := mocks.NewMockMercury(c)
	mockMercurySess.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, errors.New("Listregister_failed"))
	mockCloud.EXPECT().Mercury().Return(mockMercurySess)
	tgManager := NewTargetGroupManager(mockCloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("Listregister_failed"))
}

// Case5: While deleting target group, deregister targets unsuccessfully
func Test_DeleteTG_DeRegisterTargetsUnsuccessfully(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targets := &mercury.TargetSummary{
		Id:   &sId,
		Port: &sPort,
	}
	listTargetsOutput := []*mercury.TargetSummary{targets}
	targetsFailure := &mercury.TargetFailure{
		FailureCode:    nil,
		FailureMessage: nil,
		Id:             &sId,
		Port:           &sPort,
	}
	unsuccessful := []*mercury.TargetFailure{targetsFailure}
	deRegisterTargetsOutput := &mercury.DeregisterTargetsOutput{
		Unsuccessful: unsuccessful,
	}

	tgSpec := latticemodel.TargetGroupSpec{
		Name:      "test",
		Config:    latticemodel.TargetGroupConfig{},
		Type:      "IP",
		IsDeleted: false,
		LatticeID: "123",
	}
	tgDeleteInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess := mocks.NewMockMercury(c)
	mockMercurySess.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, nil)
	mockMercurySess.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(deRegisterTargetsOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
}

// Case6: Delete target group fails
func Test_DeleteTG_DeRegisterTargets_DeleteTargetGroupFailed(t *testing.T) {
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

	listTargetsOutput := []*mercury.TargetSummary{targetsList}
	successful := []*mercury.Target{targetsSuccessful}
	deRegisterTargetsOutput := &mercury.DeregisterTargetsOutput{
		Successful: successful,
	}
	deleteTargetGroupOutput := &mercury.DeleteTargetGroupOutput{}

	tgSpec := latticemodel.TargetGroupSpec{
		Name:      "test",
		Config:    latticemodel.TargetGroupConfig{},
		Type:      "IP",
		IsDeleted: false,
		LatticeID: "123",
	}
	tgDeleteInput := latticemodel.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess := mocks.NewMockMercury(c)
	mockMercurySess.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, nil)
	mockMercurySess.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(deRegisterTargetsOutput, nil)
	mockMercurySess.EXPECT().DeleteTargetGroupWithContext(ctx, gomock.Any()).Return(deleteTargetGroupOutput, errors.New("DeleteTG_failed"))
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
	tgManager := NewTargetGroupManager(mockCloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("DeleteTG_failed"))
}

func Test_ListTG_TGsExist(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name1 := "test1"
	tg1 := &mercury.TargetGroupSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name1,
	}
	name2 := "test2"
	tg2 := &mercury.TargetGroupSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name2,
	}
	listTGOutput := []*mercury.TargetGroupSummary{tg1, tg2}

	config1 := &mercury.TargetGroupConfig{
		VpcIdentifier: &config.VpcID,
	}
	getTG1 := &mercury.GetTargetGroupOutput{
		Config: config1,
	}

	vpcid2 := "123456789"
	config2 := &mercury.TargetGroupConfig{
		VpcIdentifier: &vpcid2,
	}
	getTG2 := &mercury.GetTargetGroupOutput{
		Config: config2,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTGOutput, nil)
	mockMercurySess.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(getTG1, nil)
	mockMercurySess.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(getTG2, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess)

	tgManager := NewTargetGroupManager(mockCloud)
	tgList, err := tgManager.List(ctx)
	expect := []mercury.GetTargetGroupOutput{*getTG1}

	assert.Nil(t, err)
	assert.Equal(t, tgList, expect)
}

func Test_ListTG_NoTG(t *testing.T) {
	listTGOutput := []*mercury.TargetGroupSummary{}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTGOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess)

	tgManager := NewTargetGroupManager(mockCloud)
	tgList, err := tgManager.List(ctx)

	assert.Nil(t, err)
	assert.Equal(t, tgList, []mercury.GetTargetGroupOutput(nil))
}

func Test_Get(t *testing.T) {
	tests := []struct {
		wantErr        error
		tgId           string
		tgArn          string
		tgName         string
		input          *latticemodel.TargetGroup
		wantOutput     latticemodel.TargetGroupStatus
		randomArn      string
		randomId       string
		randomName     string
		tgStatus       string
		tgStatusFailed string
	}{
		{
			wantErr: nil,
			tgId:    "tg-id-012345",
			tgArn:   "tg-arn-123456",
			tgName:  "tg-test-1",
			input: &latticemodel.TargetGroup{
				ResourceMeta: core.ResourceMeta{},
				Spec: latticemodel.TargetGroupSpec{
					Name:      "tg-test-1",
					Config:    latticemodel.TargetGroupConfig{},
					Type:      "",
					IsDeleted: false,
					LatticeID: "",
				},
				Status: nil,
			},
			wantOutput:     latticemodel.TargetGroupStatus{TargetGroupARN: "tg-arn-123456", TargetGroupID: "tg-id-012345"},
			randomArn:      "random-tg-arn-12345",
			randomId:       "random-tg-id-12345",
			randomName:     "tgrandom-1",
			tgStatus:       mercury.TargetGroupStatusActive,
			tgStatusFailed: mercury.TargetGroupStatusCreateFailed,
		},
		{
			wantErr: errors.New("Non existing Target Group"),
			tgId:    "tg-id-012345",
			tgArn:   "tg-arn-123456",
			tgName:  "tg-test-1",
			input: &latticemodel.TargetGroup{
				ResourceMeta: core.ResourceMeta{},
				Spec: latticemodel.TargetGroupSpec{
					Name:      "tg-test-1",
					Config:    latticemodel.TargetGroupConfig{},
					Type:      "",
					IsDeleted: false,
					LatticeID: "",
				},
				Status: nil,
			},
			wantOutput:     latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""},
			randomArn:      "random-tg-arn-12345",
			randomId:       "random-tg-id-12345",
			randomName:     "tgrandom-1",
			tgStatus:       mercury.TargetGroupStatusCreateFailed,
			tgStatusFailed: mercury.TargetGroupStatusCreateFailed,
		},
		{
			wantErr: errors.New(LATTICE_RETRY),
			tgId:    "tg-id-012345",
			tgArn:   "tg-arn-123456",
			tgName:  "tg-test-1",
			input: &latticemodel.TargetGroup{
				ResourceMeta: core.ResourceMeta{},
				Spec: latticemodel.TargetGroupSpec{
					Name:      "tg-test-1",
					Config:    latticemodel.TargetGroupConfig{},
					Type:      "",
					IsDeleted: false,
					LatticeID: "",
				},
				Status: nil,
			},
			wantOutput:     latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""},
			randomArn:      "random-tg-arn-12345",
			randomId:       "random-tg-id-12345",
			randomName:     "tgrandom-1",
			tgStatus:       mercury.TargetGroupStatusDeleteInProgress,
			tgStatusFailed: mercury.TargetGroupStatusDeleteFailed,
		},
		{
			wantErr: errors.New("Non existing Target Group"),
			tgId:    "tg-id-012345",
			tgArn:   "tg-arn-123456",
			tgName:  "tg-test-not-exist",
			input: &latticemodel.TargetGroup{
				ResourceMeta: core.ResourceMeta{},
				Spec: latticemodel.TargetGroupSpec{
					Name:      "tg-test-2",
					Config:    latticemodel.TargetGroupConfig{},
					Type:      "",
					IsDeleted: false,
					LatticeID: "",
				},
				Status: nil,
			},
			wantOutput:     latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""},
			randomArn:      "random-tg-arn-12345",
			randomId:       "random-tg-id-12345",
			randomName:     "tgrandom-2",
			tgStatus:       mercury.TargetGroupStatusCreateFailed,
			tgStatusFailed: mercury.TargetGroupStatusCreateFailed,
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockMercurySess := mocks.NewMockMercury(c)
		mockCloud := mocks_aws.NewMockCloud(c)

		listTGinput := &mercury.ListTargetGroupsInput{}
		listTGOutput := []*mercury.TargetGroupSummary{
			&mercury.TargetGroupSummary{
				Arn:    &tt.randomArn,
				Id:     &tt.randomId,
				Name:   &tt.randomName,
				Status: &tt.tgStatusFailed,
				Type:   nil,
			},
			&mercury.TargetGroupSummary{
				Arn:    &tt.tgArn,
				Id:     &tt.tgId,
				Name:   &tt.tgName,
				Status: &tt.tgStatus,
				Type:   nil,
			}}

		mockMercurySess.EXPECT().ListTargetGroupsAsList(ctx, listTGinput).Return(listTGOutput, nil)

		mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()
		targetGroupManager := NewTargetGroupManager(mockCloud)

		resp, err := targetGroupManager.Get(ctx, tt.input)

		if tt.wantErr != nil {
			assert.NotNil(t, err)
			assert.Equal(t, err, tt.wantErr)
		} else {
			assert.Nil(t, err)
			assert.Equal(t, resp.TargetGroupID, tt.wantOutput.TargetGroupID)
			assert.Equal(t, resp.TargetGroupARN, tt.wantOutput.TargetGroupARN)
		}
	}
}
