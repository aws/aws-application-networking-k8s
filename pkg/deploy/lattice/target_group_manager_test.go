package lattice

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

// target group does not exist, and is active after creation
func Test_CreateTargetGroup_TGNotExist_Active(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	config.VpcID = "vpc-id"
	config.ClusterName = "cluster-name"
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

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
			tgSpec.K8SClusterName = config.ClusterName
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
			tgSpec.K8SClusterName = config.ClusterName
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
		expectedTags[model.K8SClusterNameKey] = &tgSpec.K8SClusterName
		expectedTags[model.K8SProtocolVersionKey] = &tgSpec.ProtocolVersion

		if tgType == "by-serviceexport" {
			value := string(model.SourceTypeSvcExport)
			expectedTags[model.K8SSourceTypeKey] = &value
		} else if tgType == "by-backendref" {
			value := string(model.SourceTypeHTTPRoute)
			expectedTags[model.K8SSourceTypeKey] = &value
			expectedTags[model.K8SRouteNameKey] = &tgSpec.K8SRouteName
			expectedTags[model.K8SRouteNamespaceKey] = &tgSpec.K8SRouteNamespace
		}

		mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
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
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

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
	tgOutput := vpclattice.GetTargetGroupOutput{
		Arn:    &arn,
		Id:     &id,
		Name:   &name,
		Status: &beforeCreateStatus,
		Config: &vpclattice.TargetGroupConfig{},
	}

	mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return([]string{arn}, nil)
	mockLattice.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(&tgOutput, nil)

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
			mockTagging := mocks.NewMockTagging(c)
			cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

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

			tgOutput := vpclattice.GetTargetGroupOutput{
				Arn:    &arn,
				Id:     &id,
				Name:   aws.String("test-https-http1"),
				Status: aws.String(vpclattice.TargetGroupStatusActive),
				Config: &vpclattice.TargetGroupConfig{
					Port:            aws.Int64(80),
					Protocol:        aws.String(vpclattice.TargetGroupProtocolHttps),
					ProtocolVersion: aws.String(vpclattice.TargetGroupProtocolVersionHttp1),
				},
			}

			mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return([]string{arn}, nil)
			mockLattice.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(&tgOutput, nil)

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

func Test_CreateTargetGroup_TGActive_HealthCheckSame(t *testing.T) {
	ctx := context.TODO()
	c := gomock.NewController(t)
	defer c.Finish()

	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	hcConfig := &vpclattice.HealthCheckConfig{
		Enabled:                    aws.Bool(true),
		HealthCheckIntervalSeconds: aws.Int64(3),
		HealthCheckTimeoutSeconds:  aws.Int64(3),
		HealthyThresholdCount:      aws.Int64(3),
		Matcher:                    &vpclattice.Matcher{HttpCode: aws.String("200")},
		Path:                       aws.String("/"),
		Port:                       nil,
		Protocol:                   aws.String(vpclattice.TargetGroupProtocolHttps),
		ProtocolVersion:            aws.String(vpclattice.TargetGroupProtocolVersionHttp1),
		UnhealthyThresholdCount:    aws.Int64(3),
	}
	tgSpec := model.TargetGroupSpec{
		Port:              80,
		Protocol:          vpclattice.TargetGroupProtocolHttps,
		ProtocolVersion:   vpclattice.TargetGroupProtocolVersionHttp1,
		HealthCheckConfig: hcConfig,
	}

	tgCreateInput := model.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}

	tgOutput := vpclattice.GetTargetGroupOutput{
		Arn:    aws.String("arn"),
		Id:     aws.String("id"),
		Name:   aws.String("test-https-http1"),
		Status: aws.String(vpclattice.TargetGroupStatusActive),
		Config: &vpclattice.TargetGroupConfig{
			Port:            aws.Int64(80),
			HealthCheck:     hcConfig,
			Protocol:        aws.String(vpclattice.TargetGroupProtocolHttps),
			ProtocolVersion: aws.String(vpclattice.TargetGroupProtocolVersionHttp1),
		},
	}

	mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return([]string{"arn"}, nil)
	mockLattice.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(&tgOutput, nil)
	mockLattice.EXPECT().UpdateTargetGroupWithContext(ctx, gomock.Any()).Times(0)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
	resp, err := tgManager.Upsert(ctx, &tgCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, "arn", resp.Arn)
	assert.Equal(t, "id", resp.Id)
}

// target group status is create-in-progress before creation, return Retry
func Test_CreateTargetGroup_ExistingTG_Status_Retry(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	tgSpec := model.TargetGroupSpec{
		Port:            80,
		Protocol:        "HTTP",
		ProtocolVersion: "HTTP1",
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
			tgOutput := vpclattice.GetTargetGroupOutput{
				Arn:    &arn,
				Id:     &id,
				Name:   &name,
				Status: &beforeCreateStatus,
				Config: &vpclattice.TargetGroupConfig{
					Port:            aws.Int64(80),
					Protocol:        aws.String("HTTP"),
					ProtocolVersion: aws.String("HTTP1"),
				},
			}

			mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return([]string{arn}, nil)
			mockLattice.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(&tgOutput, nil)

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
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

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

			mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
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
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	tgSpec := model.TargetGroupSpec{
		Port:     80,
		Protocol: "HTTP",
	}
	tgCreateInput := model.TargetGroup{
		ResourceMeta: core.ResourceMeta{},
		Spec:         tgSpec,
	}

	// search error
	mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return(nil, errors.New("test"))

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
	_, err := tgManager.Upsert(ctx, &tgCreateInput)

	assert.Equal(t, errors.New("test"), err)

	// create error
	mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
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
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

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
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.Nil(t, err)
}

func Test_DeleteTG_WithExistingTG(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	tgOutput := vpclattice.GetTargetGroupOutput{
		Arn:    aws.String("existing-tg-arn"),
		Id:     aws.String("existing-tg-id"),
		Name:   aws.String("name"),
		Status: aws.String(vpclattice.TargetGroupStatusActive),
		Config: &vpclattice.TargetGroupConfig{
			Port:            aws.Int64(80),
			Protocol:        aws.String(vpclattice.TargetGroupProtocolHttps),
			ProtocolVersion: aws.String(vpclattice.HealthCheckProtocolVersionHttp1),
		},
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

	mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return([]string{*tgOutput.Arn}, nil)
	mockLattice.EXPECT().GetTargetGroupWithContext(ctx, gomock.Any()).Return(&tgOutput, nil)

	mockLattice.EXPECT().ListTargetsAsList(ctx, gomock.Any()).Return(listTargetsOutput, nil)

	dtgInput := &vpclattice.DeleteTargetGroupInput{TargetGroupIdentifier: tgOutput.Id}
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
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	tgSpec := model.TargetGroupSpec{
		Port:            80,
		Protocol:        "HTTPS",
		ProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp1,
	}
	tgDeleteInput := model.TargetGroup{
		Spec:   tgSpec,
		Status: nil,
	}

	mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)

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
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

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
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

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
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

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
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)

	err := tgManager.Delete(ctx, &tgDeleteInput)

	assert.NotNil(t, err)
}

func Test_ListTG_TGsExist(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name1 := "test1"
	config.VpcID = "vpc-id"
	externalVpc := "external-vpc-id"

	config.ClusterName = "cluster-name"
	tgType := vpclattice.TargetGroupTypeIp
	tg1 := &vpclattice.TargetGroupSummary{
		Arn:           &arn,
		Id:            &id,
		Name:          &name1,
		VpcIdentifier: &config.VpcID,
		Type:          &tgType,
	}
	name2 := "test2"
	tg2 := &vpclattice.TargetGroupSummary{
		Arn:           &arn,
		Id:            &id,
		Name:          &name2,
		VpcIdentifier: &externalVpc,
		Type:          &tgType,
	}
	listTGOutput := []*vpclattice.TargetGroupSummary{tg1, tg2}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTGOutput, nil)

	// assume no tags
	mockTagging := mocks.NewMockTagging(c)
	mockTagging.EXPECT().GetTagsForArns(ctx, gomock.Any()).Return(nil, nil)

	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
	tgList, err := tgManager.List(ctx)
	expect := []tgListOutput{
		{
			tgSummary: tg1,
			tags:      nil,
		},
		{
			tgSummary: tg2,
			tags:      nil,
		},
	}

	assert.Nil(t, err)
	assert.ElementsMatch(t, tgList, expect)
}

func Test_ListTG_NoTG(t *testing.T) {
	listTGOutput := []*vpclattice.TargetGroupSummary{}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(listTGOutput, nil)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	tgManager := NewTargetGroupManager(gwlog.FallbackLogger, cloud)
	tgList, err := tgManager.List(ctx)
	expectTgList := []tgListOutput(nil)

	assert.Nil(t, err)
	assert.Equal(t, tgList, expectTgList)
}

func Test_defaultTargetGroupManager_getDefaultHealthCheckConfig(t *testing.T) {
	var (
		defaultMatcher = &vpclattice.Matcher{
			HttpCode: aws.String("200"),
		}
		defaultPath                       = aws.String("/")
		defaultProtocol                   = aws.String(vpclattice.TargetGroupProtocolHttp)
		defaultHealthCheckIntervalSeconds = aws.Int64(30)
		defaultHealthCheckTimeoutSeconds  = aws.Int64(5)
		defaultHealthyThresholdCount      = aws.Int64(5)
		defaultUnhealthyThresholdCount    = aws.Int64(2)
	)

	type args struct {
		targetGroupProtocol        string
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
				targetGroupProtocol:        vpclattice.TargetGroupProtocolHttp,
				targetGroupProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp1,
			},
			want: &vpclattice.HealthCheckConfig{
				Enabled:                    aws.Bool(true),
				HealthCheckIntervalSeconds: defaultHealthCheckIntervalSeconds,
				HealthCheckTimeoutSeconds:  defaultHealthCheckTimeoutSeconds,
				HealthyThresholdCount:      defaultHealthyThresholdCount,
				UnhealthyThresholdCount:    defaultUnhealthyThresholdCount,
				Matcher:                    defaultMatcher,
				Path:                       defaultPath,
				Port:                       nil,
				Protocol:                   defaultProtocol,
				ProtocolVersion:            aws.String(vpclattice.HealthCheckProtocolVersionHttp1),
			},
		},
		{
			name: "HTTPS TargetGroup default health check config",
			args: args{
				targetGroupProtocol:        vpclattice.TargetGroupProtocolHttps,
				targetGroupProtocolVersion: "",
			},
			want: &vpclattice.HealthCheckConfig{
				Enabled:                    aws.Bool(true),
				HealthCheckIntervalSeconds: defaultHealthCheckIntervalSeconds,
				HealthCheckTimeoutSeconds:  defaultHealthCheckTimeoutSeconds,
				HealthyThresholdCount:      defaultHealthyThresholdCount,
				UnhealthyThresholdCount:    defaultUnhealthyThresholdCount,
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
				targetGroupProtocol:        vpclattice.TargetGroupProtocolHttp,
				targetGroupProtocolVersion: "",
			},
			want: &vpclattice.HealthCheckConfig{
				Enabled:                    aws.Bool(true),
				HealthCheckIntervalSeconds: defaultHealthCheckIntervalSeconds,
				HealthCheckTimeoutSeconds:  defaultHealthCheckTimeoutSeconds,
				HealthyThresholdCount:      defaultHealthyThresholdCount,
				UnhealthyThresholdCount:    defaultUnhealthyThresholdCount,
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
				targetGroupProtocol:        vpclattice.TargetGroupProtocolHttp,
				targetGroupProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp2,
			},
			want: &vpclattice.HealthCheckConfig{
				Enabled:                    aws.Bool(false),
				HealthCheckIntervalSeconds: defaultHealthCheckIntervalSeconds,
				HealthCheckTimeoutSeconds:  defaultHealthCheckTimeoutSeconds,
				HealthyThresholdCount:      defaultHealthyThresholdCount,
				UnhealthyThresholdCount:    defaultUnhealthyThresholdCount,
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
				targetGroupProtocol:        vpclattice.TargetGroupProtocolHttp,
				targetGroupProtocolVersion: vpclattice.TargetGroupProtocolVersionGrpc,
			},
			want: &vpclattice.HealthCheckConfig{
				Enabled:                    aws.Bool(false),
				HealthCheckIntervalSeconds: defaultHealthCheckIntervalSeconds,
				HealthCheckTimeoutSeconds:  defaultHealthCheckTimeoutSeconds,
				HealthyThresholdCount:      defaultHealthyThresholdCount,
				UnhealthyThresholdCount:    defaultUnhealthyThresholdCount,
				Matcher:                    defaultMatcher,
				Path:                       defaultPath,
				Port:                       nil,
				Protocol:                   defaultProtocol,
				ProtocolVersion:            aws.String(vpclattice.HealthCheckProtocolVersionHttp1),
			},
		},
		{
			name: "TCP TargetGroup default health check config",
			args: args{
				targetGroupProtocol:        vpclattice.TargetGroupProtocolTcp,
				targetGroupProtocolVersion: "",
			},
			want: &vpclattice.HealthCheckConfig{
				Enabled: aws.Bool(false),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()

			cloud := pkg_aws.NewDefaultCloud(nil, TestCloudConfig)

			s := NewTargetGroupManager(gwlog.FallbackLogger, cloud)

			if got := s.getDefaultHealthCheckConfig(tt.args.targetGroupProtocol, tt.args.targetGroupProtocolVersion); !reflect.DeepEqual(got, tt.want) {
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
			tags:      &model.TargetGroupTagFields{K8SClusterName: "foo"},
		},
		{
			name:           "tags equal",
			expectedResult: true,
			wantErr:        false,
			modelTg: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					Port:            443,
					ProtocolVersion: "HTTP1",
					TargetGroupTagFields: model.TargetGroupTagFields{
						K8SClusterName: "cluster",
					},
				},
			},
			latticeTg: &vpclattice.TargetGroupSummary{Port: aws.Int64(443)},
			tags: &model.TargetGroupTagFields{
				K8SClusterName: "cluster",
			},
		},
		{
			name:           "tags not provided",
			expectedResult: true,
			wantErr:        false,
			modelTg: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					Port:            443,
					ProtocolVersion: "HTTP1",
					TargetGroupTagFields: model.TargetGroupTagFields{
						K8SClusterName: "cluster",
					},
				},
			},
			latticeTg: &vpclattice.TargetGroupSummary{Port: aws.Int64(443)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			mockLattice := mocks.NewMockLattice(c)
			mockTagging := mocks.NewMockTagging(c)
			cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

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

func Test_ResolveRuleTgIds(t *testing.T) {
	config.VpcID = "vpc-id"
	config.ClusterName = "cluster-name"

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	mockCloud := pkg_aws.NewMockCloud(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()
	mockCloud.EXPECT().Tagging().Return(mockTagging).AnyTimes()
	mockTagging.EXPECT().GetTagsForArns(ctx, gomock.Any()).Return(
		map[string]map[string]*string{
			"svc-export-tg-arn": {
				model.K8SServiceNameKey:      aws.String("svc-name"),
				model.K8SServiceNamespaceKey: aws.String("ns"),
				model.K8SClusterNameKey:      aws.String("cluster-name"),
				model.K8SSourceTypeKey:       aws.String(string(model.SourceTypeSvcExport)),
			},
		}, nil)
	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(
		[]*vpclattice.TargetGroupSummary{
			{
				Arn:           aws.String("svc-export-tg-arn"),
				VpcIdentifier: aws.String("vpc-id"),
				Id:            aws.String("svc-export-tg-id"),
				Name:          aws.String("svc-export-tg-name"),
			},
		}, nil)

	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})

	stackTg := &model.TargetGroup{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "stack-tg-id"),
		Status:       &model.TargetGroupStatus{Id: "tg-id"},
	}
	assert.NoError(t, stack.AddResource(stackTg))
	stackRule := &model.Rule{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Rule", "rule-id"),
		Spec: model.RuleSpec{
			Action: model.RuleAction{
				TargetGroups: []*model.RuleTargetGroup{
					{
						SvcImportTG: &model.SvcImportTargetGroup{
							K8SClusterName:      "cluster-name",
							K8SServiceName:      "svc-name",
							K8SServiceNamespace: "ns",
							VpcId:               "vpc-id",
						},
					},
					{
						StackTargetGroupId: "stack-tg-id",
					},
					{
						StackTargetGroupId: model.InvalidBackendRefTgId,
					},
				},
			},
		},
	}
	assert.NoError(t, stack.AddResource(stackRule))

	s := NewTargetGroupManager(gwlog.FallbackLogger, mockCloud)
	assert.NoError(t, s.ResolveRuleTgIds(ctx, &stackRule.Spec.Action, stack))

	assert.Equal(t, "svc-export-tg-id", stackRule.Spec.Action.TargetGroups[0].LatticeTgId)
	assert.Equal(t, "tg-id", stackRule.Spec.Action.TargetGroups[1].LatticeTgId)
	assert.Equal(t, model.InvalidBackendRefTgId, stackRule.Spec.Action.TargetGroups[2].LatticeTgId)
}
