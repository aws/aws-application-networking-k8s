package lattice

import (
	"context"
	"errors"
	"fmt"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

// target group does not exist, and is active after creation
func Test_CreateTargetGroup_TGNotExist_Active(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	config.VpcID = "vpc-id"
	config.ClusterName = "cluster-name"
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgTypes := [2]string{"by-backendref", "by-serviceexport"}

	for _, tgType := range tgTypes {
		var tgSpec model.TargetGroupSpec

		if tgType == "by-serviceexport" {
			// testing targetgroup for serviceexport
			tgSpec = model.TargetGroupSpec{
				Port:            int32(8080),
				Protocol:        "HTTP",
				ProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp1,
			}
			tgSpec.VpcId = config.VpcID
			tgSpec.ClusterName = config.ClusterName
			tgSpec.K8SSourceType = model.SourceTypeSvcExport
			tgSpec.K8SServiceName = "exportsvc1"
			tgSpec.K8SServiceNamespace = "default"
			tgSpec.Type = model.TargetGroupTypeIP
		} else if tgType == "by-backendref" {
			// testing targetgroup for backendref
			tgSpec = model.TargetGroupSpec{
				Port:            int32(8080),
				Protocol:        "HTTP",
				ProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp1,
			}
			tgSpec.VpcId = config.VpcID
			tgSpec.ClusterName = config.ClusterName
			tgSpec.K8SSourceType = model.SourceTypeHTTPRoute
			tgSpec.K8SServiceName = "backend-svc1"
			tgSpec.K8SServiceNamespace = "default"
			tgSpec.K8SRouteName = "httproute1"
			tgSpec.K8SRouteNamespace = "default"
			tgSpec.Type = model.TargetGroupTypeIP
		}
		tgCreateInput := model.TargetGroup{
			ResourceMeta: core.ResourceMeta{},
			Spec:         tgSpec,
		}

		expectedTags := cloud.DefaultTags()
		expectedTags[model.K8SServiceNameKey] = &tgSpec.K8SServiceName
		expectedTags[model.K8SServiceNamespaceKey] = &tgSpec.K8SServiceNamespace
		expectedTags[model.EKSClusterNameKey] = &tgSpec.ClusterName

		if tgType == "by-serviceexport" {
			value := string(model.SourceTypeSvcExport)
			expectedTags[model.K8SSourceTypeKey] = &value
		} else if tgType == "by-backendref" {
			value := string(model.SourceTypeHTTPRoute)
			expectedTags[model.K8SSourceTypeKey] = &value
			expectedTags[model.K8SRouteNameKey] = &tgSpec.K8SRouteName
			expectedTags[model.K8SRouteNamespaceKey] = &tgSpec.K8SRouteNamespace
		}

		var listTgOutput []*vpclattice.TargetGroupSummary

		mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
		mockLattice.EXPECT().CreateTargetGroupWithContext(ctx, gomock.Any()).DoAndReturn(
			func(ctx context.Context, input *vpclattice.CreateTargetGroupInput, arg3 ...interface{}) (*vpclattice.CreateTargetGroupOutput, error) {
				assert.Equal(t, aws.Int64(int64(tgSpec.Port)), input.Config.Port)
				assert.Equal(t, tgSpec.Protocol, *input.Config.Protocol)
				assert.Equal(t, tgSpec.ProtocolVersion, *input.Config.ProtocolVersion)
				assert.Equal(t, expectedTags, input.Tags)
				assert.Equal(t, tgSpec.VpcId, *input.Config.VpcIdentifier)

				return &vpclattice.CreateTargetGroupOutput{
					Arn:    aws.String("tg-arn-1"),
					Id:     aws.String("tg-id-1"),
					Name:   aws.String("tg-name-1"),
					Status: aws.String(vpclattice.TargetGroupStatusActive),
				}, nil
			},
		)

		tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
		resp, err := tgManager.Upsert(ctx, &tgCreateInput)

		assert.Nil(t, err)
		assert.Equal(t, "tg-arn-1", resp.Arn)
		assert.Equal(t, "tg-id-1", resp.Id)
		assert.Equal(t, "tg-name-1", resp.Name)
	}
}

// target group status is failed, and is active after creation
func Test_CreateTargetGroup_TGFailed_Active(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgSpec := model.TargetGroupSpec{}
	tgSpec.K8SRouteName = "route1"
	tgSpec.K8SRouteNamespace = "ns1"
	tgCreateInput := model.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}

	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	tgStatus := vpclattice.TargetGroupStatusActive
	tgCreateOutput := &vpclattice.CreateTargetGroupOutput{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &tgStatus,
	}

	beforeCreateStatus := vpclattice.TargetGroupStatusCreateFailed
	tgSummary := vpclattice.TargetGroupSummary{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &beforeCreateStatus,
	}
	listTgOutput := []*vpclattice.TargetGroupSummary{&tgSummary}

	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
	mockLattice.EXPECT().CreateTargetGroupWithContext(ctx, gomock.Any()).Return(tgCreateOutput, nil)
	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
	resp, err := tgManager.Upsert(ctx, &tgCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, arn, resp.Arn)
	assert.Equal(t, id, resp.Id)
}

// target group status is active before creation, no need to recreate
func Test_CreateTargetGroup_TGActive_UpdateHealthCheck(t *testing.T) {
	tests := []struct {
		name              string
		healthCheckConfig *vpclattice.HealthCheckConfig
		wantErr           bool
	}{
		{
			name: "includes health check",
			healthCheckConfig: &vpclattice.HealthCheckConfig{
				Enabled: aws.Bool(false),
			},
			wantErr: false,
		},
		{
			name:              "health check nil",
			healthCheckConfig: nil,
			wantErr:           false,
		},
		{
			name:    "health check missing",
			wantErr: true,
		},
	}

	ctx := context.TODO()

	arn := "12345678912345678912"
	id := "12345678912345678912"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()

			mockLattice := mocks.NewMockLattice(c)
			cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

			tgSpec := model.TargetGroupSpec{
				Port:              80,
				Protocol:          vpclattice.TargetGroupProtocolHttps,
				ProtocolVersion:   vpclattice.TargetGroupProtocolVersionHttp1,
				HealthCheckConfig: tt.healthCheckConfig,
			}

			tgCreateInput := model.TargetGroup{
				ResourceMeta: core.ResourceMeta{},
				Spec:         tgSpec,
			}

			tgSummary := vpclattice.TargetGroupSummary{
				Arn:      &arn,
				Id:       &id,
				Name:     aws.String("test-https-http1"),
				Status:   aws.String(vpclattice.TargetGroupStatusActive),
				Port:     aws.Int64(80),
				Protocol: aws.String(vpclattice.TargetGroupProtocolHttps),
			}

			listTgOutput := []*vpclattice.TargetGroupSummary{&tgSummary}

			mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)

			// empty tags should be OK and should match since all tag values on the spec are empty
			mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(
				&vpclattice.ListTagsForResourceOutput{}, nil)

			// we use Get to do a last check on the protocol version
			mockLattice.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(
				&vpclattice.GetTargetGroupOutput{
					Config: &vpclattice.TargetGroupConfig{
						ProtocolVersion: aws.String(tgSpec.ProtocolVersion),
					},
				}, nil)

			if tt.wantErr {
				mockLattice.EXPECT().UpdateTargetGroupWithContext(ctx, gomock.Any()).Return(nil, errors.New("error"))
			} else {
				mockLattice.EXPECT().UpdateTargetGroupWithContext(ctx, gomock.Any()).Return(nil, nil)
			}

			tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
			resp, err := tgManager.Upsert(ctx, &tgCreateInput)

			if tt.wantErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, arn, resp.Arn)
				assert.Equal(t, id, resp.Id)
			}
		})
	}
}

// target group status is create-in-progress before creation, return Retry
func Test_CreateTargetGroup_ExistingTG_Status_Retry(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgSpec := model.TargetGroupSpec{
		Port:     80,
		Protocol: "HTTP",
	}
	tgCreateInput := model.TargetGroup{
		Spec: tgSpec,
	}
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"

	retryStatuses := []string{
		vpclattice.TargetGroupStatusCreateInProgress,
		vpclattice.TargetGroupStatusDeleteInProgress,
	}

	for _, retryStatus := range retryStatuses {
		t.Run(fmt.Sprintf("retry on status %s", retryStatus), func(t *testing.T) {
			beforeCreateStatus := retryStatus
			tgSummary := vpclattice.TargetGroupSummary{
				Arn:      &arn,
				Id:       &id,
				Name:     &name,
				Status:   &beforeCreateStatus,
				Port:     aws.Int64(80),
				Protocol: aws.String("HTTP"),
			}
			listTgOutput := []*vpclattice.TargetGroupSummary{&tgSummary}

			mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
			mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(
				&vpclattice.ListTagsForResourceOutput{}, nil)
			mockLattice.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(
				&vpclattice.GetTargetGroupOutput{
					Config: &vpclattice.TargetGroupConfig{
						ProtocolVersion: aws.String(tgSpec.ProtocolVersion),
					},
				}, nil)

			tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
			_, err := tgManager.Upsert(ctx, &tgCreateInput)

			assert.Equal(t, errors.New(LATTICE_RETRY), err)
		})
	}
}

// target group is not in-progress before, get create-in-progress, should return retry
func Test_CreateTargetGroup_NewTG_RetryStatus(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgSpec := model.TargetGroupSpec{
		Port:     80,
		Protocol: "HTTP",
	}
	tgCreateInput := model.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"

	var listTgOutput []*vpclattice.TargetGroupSummary

	retryStatuses := []string{
		vpclattice.TargetGroupStatusDeleteInProgress,
		vpclattice.TargetGroupStatusCreateFailed,
		vpclattice.TargetGroupStatusDeleteFailed,
		vpclattice.TargetGroupStatusDeleteInProgress,
	}
	for _, retryStatus := range retryStatuses {
		t.Run(fmt.Sprintf("retry on status %s", retryStatus), func(t *testing.T) {

			tgStatus := retryStatus
			tgCreateOutput := &vpclattice.CreateTargetGroupOutput{
				Arn:    &arn,
				Id:     &id,
				Name:   &name,
				Status: &tgStatus,
			}

			mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
			mockLattice.EXPECT().CreateTargetGroupWithContext(ctx, gomock.Any()).Return(tgCreateOutput, nil)

			tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
			_, err := tgManager.Upsert(ctx, &tgCreateInput)

			assert.Equal(t, errors.New(LATTICE_RETRY), err)
		})
	}
}

func Test_Lattice_API_Errors(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgSpec := model.TargetGroupSpec{
		Port:     80,
		Protocol: "HTTP",
	}
	tgCreateInput := model.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}
	var listTgOutput []*vpclattice.TargetGroupSummary

	// list error
	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, errors.New("test"))

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
	_, err := tgManager.Upsert(ctx, &tgCreateInput)

	assert.Equal(t, errors.New("test"), err)

	// create error
	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
	mockLattice.EXPECT().CreateTargetGroupWithContext(ctx, gomock.Any()).Return(nil, errors.New("test"))

	tgManager = NewTargetGroupManager(gwlog.FallbackLogger, cloud)
	_, err = tgManager.Upsert(ctx, &tgCreateInput)
	assert.NotNil(t, err)
}

// Deregister unused status targets and delete target group work perfectly fine
func Test_DeleteTG_DeRegisterTargets_DeleteTargetGroup(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targetStatus := vpclattice.TargetStatusUnused
	targetsList := &vpclattice.TargetSummary{
		Id:     &sId,
		Port:   &sPort,
		Status: &targetStatus,
	}
	targetsSuccessful := &vpclattice.Target{
		Id:   &sId,
		Port: &sPort,
	}

	listTargetsOutput := []*vpclattice.TargetSummary{targetsList}
	successful := []*vpclattice.Target{targetsSuccessful}
	deRegisterTargetsOutput := &vpclattice.DeregisterTargetsOutput{
		Successful: successful,
	}
	deleteTargetGroupOutput := &vpclattice.DeleteTargetGroupOutput{}

	tgSpec := model.TargetGroupSpec{
		Type: "IP",
	}
	tgDeleteInput := model.TargetGroup{
		Spec: tgSpec,
		Status: &model.TargetGroupStatus{
			Name: "name",
			Arn:  "arn",
			Id:   "id",
		},
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, nil)
	mockLattice.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(deRegisterTargetsOutput, nil)
	mockLattice.EXPECT().DeleteTargetGroupWithContext(ctx, gomock.Any()).Return(deleteTargetGroupOutput, nil)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.Nil(t, err)
}

// Delete target group that no targets register on it
func Test_DeleteTG_NoRegisteredTargets_DeleteTargetGroup(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targets := &vpclattice.TargetSummary{
		Id:   &sId,
		Port: &sPort,
	}
	listTargetsOutput := []*vpclattice.TargetSummary{targets}
	deRegisterTargetsOutput := &vpclattice.DeregisterTargetsOutput{}
	deleteTargetGroupOutput := &vpclattice.DeleteTargetGroupOutput{}

	tgSpec := model.TargetGroupSpec{
		Type: "IP",
	}
	tgDeleteInput := model.TargetGroup{
		Spec: tgSpec,
		Status: &model.TargetGroupStatus{
			Name: "name",
			Arn:  "arn",
			Id:   "id",
		},
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, nil)
	mockLattice.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(deRegisterTargetsOutput, nil)
	mockLattice.EXPECT().DeleteTargetGroupWithContext(ctx, gomock.Any()).Return(deleteTargetGroupOutput, nil)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.Nil(t, err)
}

func Test_DeleteTG_WithExistingTG(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgSummary := vpclattice.TargetGroupSummary{
		Arn:      aws.String("existing-tg-arn"),
		Id:       aws.String("existing-tg-id"),
		Name:     aws.String("name"),
		Status:   aws.String(vpclattice.TargetGroupStatusActive),
		Port:     aws.Int64(80),
		Protocol: aws.String(vpclattice.TargetGroupProtocolHttps),
	}

	tgSpec := model.TargetGroupSpec{
		Port:            80,
		Protocol:        "HTTPS",
		ProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp1,
	}
	tgDeleteInput := model.TargetGroup{
		Spec:   tgSpec,
		Status: nil,
	}
	var listTargetsOutput []*vpclattice.TargetSummary
	listTgOutput := []*vpclattice.TargetGroupSummary{&tgSummary}

	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)
	mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListTagsForResourceOutput{}, nil)

	mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, nil)

	mockLattice.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(
		&vpclattice.GetTargetGroupOutput{
			Config: &vpclattice.TargetGroupConfig{
				ProtocolVersion: aws.String(tgSpec.ProtocolVersion),
			},
		}, nil)

	dtgInput := &vpclattice.DeleteTargetGroupInput{TargetGroupIdentifier: tgSummary.Id}
	dtgOutput := &vpclattice.DeleteTargetGroupOutput{}
	mockLattice.EXPECT().DeleteTargetGroupWithContext(ctx, dtgInput).Return(dtgOutput, nil)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.Nil(t, err)
}

func Test_DeleteTG_NothingToDelete(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgSummary := vpclattice.TargetGroupSummary{
		Arn:      aws.String("existing-tg-arn"),
		Id:       aws.String("existing-tg-id"),
		Name:     aws.String("name"),
		Status:   aws.String(vpclattice.TargetGroupStatusActive),
		Port:     aws.Int64(443), // <-- important difference, so not a match
		Protocol: aws.String(vpclattice.TargetGroupProtocolHttps),
	}

	tgSpec := model.TargetGroupSpec{
		Port:            80,
		Protocol:        "HTTPS",
		ProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp1,
	}
	tgDeleteInput := model.TargetGroup{
		Spec:   tgSpec,
		Status: nil,
	}
	listTgOutput := []*vpclattice.TargetGroupSummary{&tgSummary}

	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTgOutput, nil)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.Nil(t, err)
}

// While deleting target group, deregister targets fails
func Test_DeleteTG_DeRegisteredTargetsFailed(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targets := &vpclattice.TargetSummary{
		Id:   &sId,
		Port: &sPort,
	}
	listTargetsOutput := []*vpclattice.TargetSummary{targets}
	deRegisterTargetsOutput := &vpclattice.DeregisterTargetsOutput{}

	tgSpec := model.TargetGroupSpec{
		Type: "IP",
	}
	tgDeleteInput := model.TargetGroup{
		Spec: tgSpec,
		Status: &model.TargetGroupStatus{
			Name: "name",
			Arn:  "arn",
			Id:   "id",
		},
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, nil)
	mockLattice.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(deRegisterTargetsOutput, errors.New("Deregister_failed"))
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)
	assert.NotNil(t, err)
}

// While deleting target group, list targets fails
func Test_DeleteTG_ListTargetsFailed(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targets := &vpclattice.TargetSummary{
		Id:   &sId,
		Port: &sPort,
	}
	listTargetsOutput := []*vpclattice.TargetSummary{targets}

	tgSpec := model.TargetGroupSpec{
		Type: "IP",
	}
	tgDeleteInput := model.TargetGroup{
		Spec: tgSpec,
		Status: &model.TargetGroupStatus{
			Name: "name",
			Arn:  "arn",
			Id:   "id",
		},
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, errors.New("Listregister_failed"))
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)
	assert.NotNil(t, err)
}

// While deleting target group, deregister targets unsuccessfully
func Test_DeleteTG_DeRegisterTargetsUnsuccessfully(t *testing.T) {
	sId := "123.456.7.890"
	sPort := int64(80)
	targets := &vpclattice.TargetSummary{
		Id:   &sId,
		Port: &sPort,
	}
	listTargetsOutput := []*vpclattice.TargetSummary{targets}
	targetsFailure := &vpclattice.TargetFailure{
		FailureCode:    nil,
		FailureMessage: nil,
		Id:             &sId,
		Port:           &sPort,
	}
	unsuccessful := []*vpclattice.TargetFailure{targetsFailure}
	deRegisterTargetsOutput := &vpclattice.DeregisterTargetsOutput{
		Unsuccessful: unsuccessful,
	}

	tgSpec := model.TargetGroupSpec{
		Type: "IP",
	}
	tgDeleteInput := model.TargetGroup{
		Spec: tgSpec,
		Status: &model.TargetGroupStatus{
			Name: "name",
			Arn:  "arn",
			Id:   "id",
		},
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, nil)
	mockLattice.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(deRegisterTargetsOutput, nil)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.NotNil(t, err)
}

// Delete target group fails
func Test_DeleteTG_DeRegisterTargets_DeleteTargetGroupFailed(t *testing.T) {
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

	listTargetsOutput := []*vpclattice.TargetSummary{targetsList}
	successful := []*vpclattice.Target{targetsSuccessful}
	deRegisterTargetsOutput := &vpclattice.DeregisterTargetsOutput{
		Successful: successful,
	}
	deleteTargetGroupOutput := &vpclattice.DeleteTargetGroupOutput{}

	tgSpec := model.TargetGroupSpec{
		Type: "IP",
	}
	tgDeleteInput := model.TargetGroup{
		Spec: tgSpec,
		Status: &model.TargetGroupStatus{
			Name: "name",
			Arn:  "arn",
			Id:   "id",
		},
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, nil)
	mockLattice.EXPECT().DeregisterTargetsWithContext(ctx, gomock.Any()).Return(deRegisterTargetsOutput, nil)
	mockLattice.EXPECT().DeleteTargetGroupWithContext(ctx, gomock.Any()).Return(deleteTargetGroupOutput, errors.New("DeleteTG_failed"))
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.NotNil(t, err)
}

func Test_ListTG_TGsExist(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name1 := "test1"
	tg1 := &vpclattice.TargetGroupSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name1,
	}
	name2 := "test2"
	tg2 := &vpclattice.TargetGroupSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name2,
	}
	listTGOutput := []*vpclattice.TargetGroupSummary{tg1, tg2}

	config1 := &vpclattice.TargetGroupConfig{
		VpcIdentifier: &config.VpcID,
	}
	getTG1 := &vpclattice.GetTargetGroupOutput{
		Config: config1,
	}

	vpcid2 := "123456789"
	config2 := &vpclattice.TargetGroupConfig{
		VpcIdentifier: &vpcid2,
	}
	getTG2 := &vpclattice.GetTargetGroupOutput{
		Config: config2,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTGOutput, nil)
	mockLattice.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(getTG1, nil)
	// assume no tags
	mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(nil, errors.New("no tags"))
	mockLattice.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(getTG2, nil)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
	tgList, err := tgManager.List(ctx)
	expect := []tgListOutput{
		{
			getTargetGroupOutput: *getTG1,
			targetGroupTags:      nil,
		},
	}

	assert.Nil(t, err)
	assert.Equal(t, tgList, expect)
}

func Test_ListTG_NoTG(t *testing.T) {
	listTGOutput := []*vpclattice.TargetGroupSummary{}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTGOutput, nil)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
	tgList, err := tgManager.List(ctx)
	expectTgList := []tgListOutput(nil)

	assert.Nil(t, err)
	assert.Equal(t, tgList, expectTgList)
}

func Test_defaultTargetGroupManager_getDefaultHealthCheckConfig(t *testing.T) {
	var (
		resetValue     = aws.Int64(0)
		defaultMatcher = &vpclattice.Matcher{
			HttpCode: aws.String("200"),
		}
		defaultPath     = aws.String("/")
		defaultProtocol = aws.String(vpclattice.TargetGroupProtocolHttp)
	)

	type args struct {
		targetGroupProtocolVersion string
	}

	tests := []struct {
		name string
		args args
		want *vpclattice.HealthCheckConfig
	}{
		{
			name: "HTTP1 default health check config",
			args: args{
				targetGroupProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp1,
			},
			want: &vpclattice.HealthCheckConfig{
				Enabled:                    aws.Bool(true),
				HealthCheckIntervalSeconds: resetValue,
				HealthCheckTimeoutSeconds:  resetValue,
				HealthyThresholdCount:      resetValue,
				UnhealthyThresholdCount:    resetValue,
				Matcher:                    defaultMatcher,
				Path:                       defaultPath,
				Port:                       nil,
				Protocol:                   defaultProtocol,
				ProtocolVersion:            aws.String(vpclattice.HealthCheckProtocolVersionHttp1),
			},
		},
		{
			name: "empty target group protocol version default health check config",
			args: args{
				targetGroupProtocolVersion: "",
			},
			want: &vpclattice.HealthCheckConfig{
				Enabled:                    aws.Bool(true),
				HealthCheckIntervalSeconds: resetValue,
				HealthCheckTimeoutSeconds:  resetValue,
				HealthyThresholdCount:      resetValue,
				UnhealthyThresholdCount:    resetValue,
				Matcher:                    defaultMatcher,
				Path:                       defaultPath,
				Port:                       nil,
				Protocol:                   defaultProtocol,
				ProtocolVersion:            aws.String(vpclattice.HealthCheckProtocolVersionHttp1),
			},
		},
		{
			name: "HTTP2 default health check config",
			args: args{
				targetGroupProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp2,
			},
			want: &vpclattice.HealthCheckConfig{
				Enabled:                    aws.Bool(false),
				HealthCheckIntervalSeconds: resetValue,
				HealthCheckTimeoutSeconds:  resetValue,
				HealthyThresholdCount:      resetValue,
				UnhealthyThresholdCount:    resetValue,
				Matcher:                    defaultMatcher,
				Path:                       defaultPath,
				Port:                       nil,
				Protocol:                   defaultProtocol,
				ProtocolVersion:            aws.String(vpclattice.HealthCheckProtocolVersionHttp2),
			},
		},
		{
			name: "GRPC default health check config",
			args: args{
				targetGroupProtocolVersion: vpclattice.TargetGroupProtocolVersionGrpc,
			},
			want: &vpclattice.HealthCheckConfig{
				Enabled:                    aws.Bool(false),
				HealthCheckIntervalSeconds: resetValue,
				HealthCheckTimeoutSeconds:  resetValue,
				HealthyThresholdCount:      resetValue,
				UnhealthyThresholdCount:    resetValue,
				Matcher:                    defaultMatcher,
				Path:                       defaultPath,
				Port:                       nil,
				Protocol:                   defaultProtocol,
				ProtocolVersion:            aws.String(vpclattice.HealthCheckProtocolVersionHttp1),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()

			cloud := pkg_aws.NewDefaultCloud(nil, TestCloudConfig)

			s := NewTargetGroupManager(gwlog.FallbackLogger, cloud)

			if got := s.getDefaultHealthCheckConfig(tt.args.targetGroupProtocolVersion); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("defaultTargetGroupManager.getDefaultHealthCheckConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_IsTargetGroupMatch(t *testing.T) {
	tests := []struct {
		name           string
		expectedResult bool
		wantErr        bool
		modelTg        *model.TargetGroup
		latticeTg      *vpclattice.TargetGroupSummary
		tags           *model.TargetGroupTagFields
		listTagsOut    *vpclattice.ListTagsForResourceOutput
		getTgOut       *vpclattice.GetTargetGroupOutput
	}{
		{
			name:           "port not equal",
			expectedResult: false,
			wantErr:        false,
			modelTg: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					Port: 8080,
				},
			},
			latticeTg: &vpclattice.TargetGroupSummary{Port: aws.Int64(443)},
			tags:      &model.TargetGroupTagFields{},
		},
		{
			name:           "tags not equal",
			expectedResult: false,
			wantErr:        false,
			modelTg: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					Port: 443,
				},
			},
			latticeTg: &vpclattice.TargetGroupSummary{Port: aws.Int64(443)},
			tags:      &model.TargetGroupTagFields{ClusterName: "foo"},
		},
		{
			name:           "fetch tags not equal",
			expectedResult: false,
			wantErr:        false,
			modelTg: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					Port: 443,
				},
			},
			latticeTg: &vpclattice.TargetGroupSummary{Port: aws.Int64(443)},
			listTagsOut: &vpclattice.ListTagsForResourceOutput{
				Tags: map[string]*string{
					model.EKSClusterNameKey: aws.String("foo"),
				},
			},
		},
		{
			name:           "protocol version not equal",
			expectedResult: false,
			wantErr:        false,
			modelTg: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					Port:            443,
					ProtocolVersion: "HTTP1",
				},
			},
			latticeTg: &vpclattice.TargetGroupSummary{Port: aws.Int64(443)},
			tags:      &model.TargetGroupTagFields{},
			getTgOut: &vpclattice.GetTargetGroupOutput{
				Config: &vpclattice.TargetGroupConfig{
					ProtocolVersion: aws.String("HTTP2"),
				},
			},
		},
		{
			name:           "equal with existing tags",
			expectedResult: true,
			wantErr:        false,
			modelTg: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					Port:            443,
					ProtocolVersion: "HTTP1",
					TargetGroupTagFields: model.TargetGroupTagFields{
						ClusterName: "cluster",
					},
				},
			},
			latticeTg: &vpclattice.TargetGroupSummary{Port: aws.Int64(443)},
			tags: &model.TargetGroupTagFields{
				ClusterName: "cluster",
			},
			getTgOut: &vpclattice.GetTargetGroupOutput{
				Config: &vpclattice.TargetGroupConfig{
					ProtocolVersion: aws.String("HTTP1"),
				},
			},
		},
		{
			name:           "equal with fetched tags",
			expectedResult: true,
			wantErr:        false,
			modelTg: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					Port:            443,
					ProtocolVersion: "HTTP1",
					TargetGroupTagFields: model.TargetGroupTagFields{
						ClusterName: "cluster",
					},
				},
			},
			latticeTg: &vpclattice.TargetGroupSummary{Port: aws.Int64(443)},
			listTagsOut: &vpclattice.ListTagsForResourceOutput{
				Tags: map[string]*string{
					model.EKSClusterNameKey: aws.String("cluster"),
				},
			},
			getTgOut: &vpclattice.GetTargetGroupOutput{
				Config: &vpclattice.TargetGroupConfig{
					ProtocolVersion: aws.String("HTTP1"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			mockLattice := mocks.NewMockLattice(c)
			cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

			if tt.listTagsOut != nil {
				mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(tt.listTagsOut, nil)
			}
			if tt.getTgOut != nil {
				mockLattice.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(tt.getTgOut, nil)
			}

			s := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
			result, err := s.IsTargetGroupMatch(ctx, tt.modelTg, tt.latticeTg, tt.tags)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
