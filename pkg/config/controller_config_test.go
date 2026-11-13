package config

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
)

type ec2MetadataUnavaialble struct {
}

func (m ec2MetadataUnavaialble) Region() (string, error) {
	return "", errors.New("EC2 metadata not available in this test on purpose")
}

func (m ec2MetadataUnavaialble) VpcID() (string, error) {
	return "", errors.New("EC2 metadata not available in this test on purpose")
}

func (m ec2MetadataUnavaialble) AccountId() (string, error) {
	return "", errors.New("EC2 metadata not available in this test on purpose")
}

func ec2MetadataUnavailable() EC2Metadata {
	return ec2MetadataUnavaialble{}
}

func Test_config_init_with_partial_env_var(t *testing.T) {
	// Test variable
	testRegion := "us-west-2"
	testClusterVpcId := "vpc-123456"
	testClusterLocalGateway := "default"

	os.Setenv(REGION, testRegion)
	os.Setenv(CLUSTER_VPC_ID, testClusterVpcId)
	os.Setenv(DEFAULT_SERVICE_NETWORK, testClusterLocalGateway)
	os.Unsetenv(AWS_ACCOUNT_ID)
	err := configInit(aws.Config{}, ec2MetadataUnavailable())
	assert.NotNil(t, err)
}

func Test_config_init_no_env_var(t *testing.T) {
	os.Unsetenv(REGION)
	os.Unsetenv(CLUSTER_VPC_ID)
	os.Unsetenv(DEFAULT_SERVICE_NETWORK)
	os.Unsetenv(AWS_ACCOUNT_ID)
	os.Unsetenv(ROUTE_MAX_CONCURRENT_RECONCILES)
	err := configInit(aws.Config{}, ec2MetadataUnavailable())
	assert.NotNil(t, err)

}

func Test_config_init_with_all_env_var(t *testing.T) {
	// Test variable
	testRegion := "us-west-2"
	testClusterVpcId := "vpc-123456"
	testClusterLocalGateway := "default"
	testAwsAccountId := "12345678"
	testClusterName := "cluster-name"
	testMaxRouteReconciles := "5"
	testMaxRouteReconcilesInt := 5

	os.Setenv(REGION, testRegion)
	os.Setenv(CLUSTER_VPC_ID, testClusterVpcId)
	os.Setenv(DEFAULT_SERVICE_NETWORK, testClusterLocalGateway)
	os.Setenv(AWS_ACCOUNT_ID, testAwsAccountId)
	os.Setenv(CLUSTER_NAME, testClusterName)
	os.Setenv(ROUTE_MAX_CONCURRENT_RECONCILES, testMaxRouteReconciles)
	err := configInit(aws.Config{}, ec2MetadataUnavailable())
	assert.Nil(t, err)
	assert.Equal(t, testRegion, Region)
	assert.Equal(t, testClusterVpcId, VpcID)
	assert.Equal(t, testAwsAccountId, AccountID)
	assert.Equal(t, testClusterLocalGateway, DefaultServiceNetwork)
	assert.Equal(t, testClusterName, ClusterName)
	assert.Equal(t, testMaxRouteReconcilesInt, RouteMaxConcurrentReconciles)
}

func Test_bad_reconcile_value(t *testing.T) {
	// Test variable
	maxReconciles := "FOO"

	os.Setenv(ROUTE_MAX_CONCURRENT_RECONCILES, maxReconciles)
	err := configInit(aws.Config{}, ec2MetadataUnavailable())
	assert.NotNil(t, err)
}

func Test_make_before_break_targets_env(t *testing.T) {
	t.Setenv(REGION, "us-west-2")
	t.Setenv(CLUSTER_VPC_ID, "vpc-123456")
	t.Setenv(AWS_ACCOUNT_ID, "12345678")
	t.Setenv(CLUSTER_NAME, "cluster-name")
	t.Setenv(ROUTE_MAX_CONCURRENT_RECONCILES, "1")
	t.Setenv(RECONCILE_DEFAULT_RESYNC_SECONDS, "")
	defer func() {
		MakeBeforeBreakTargets = false
		MakeBeforeBreakTargetsTimeout = defaultMakeBeforeBreakTargetsTimeout
	}()

	t.Setenv(ENABLE_MAKE_BEFORE_BREAK_TARGETS, "")
	t.Setenv(MAKE_BEFORE_BREAK_TARGETS_TIMEOUT, "")
	assert.NoError(t, configInit(aws.Config{}, ec2MetadataUnavailable()))
	assert.False(t, MakeBeforeBreakTargets)
	assert.Equal(t, 5*time.Minute, MakeBeforeBreakTargetsTimeout)

	t.Setenv(ENABLE_MAKE_BEFORE_BREAK_TARGETS, "True")
	t.Setenv(MAKE_BEFORE_BREAK_TARGETS_TIMEOUT, "90s")
	assert.NoError(t, configInit(aws.Config{}, ec2MetadataUnavailable()))
	assert.True(t, MakeBeforeBreakTargets)
	assert.Equal(t, 90*time.Second, MakeBeforeBreakTargetsTimeout)

	for _, bad := range []string{"5", "-1m", "0s", "soon"} {
		t.Setenv(MAKE_BEFORE_BREAK_TARGETS_TIMEOUT, bad)
		assert.Error(t, configInit(aws.Config{}, ec2MetadataUnavailable()), bad)
	}
}
