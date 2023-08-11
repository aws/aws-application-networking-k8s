package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_config_init_with_partial_env_var(t *testing.T) {
	// Test variable
	testRegion := "us-west-2"
	testClusterVpcId := "vpc-123456"
	testClusterLocalGateway := "default"

	os.Setenv(REGION, testRegion)
	os.Setenv(CLUSTER_VPC_ID, testClusterVpcId)
	os.Setenv(CLUSTER_LOCAL_GATEWAY, testClusterLocalGateway)
	os.Unsetenv(AWS_ACCOUNT_ID)
	os.Unsetenv(TARGET_GROUP_NAME_LEN_MODE)
	clusterLocalGateway, _ := GetClusterLocalGateway()
	assert.Equal(t, GetRegion(), testRegion)
	assert.Equal(t, GetVpcID(), testClusterVpcId)
	assert.Equal(t, GetAccountID(), UnknownInput)
	assert.Equal(t, clusterLocalGateway, testClusterLocalGateway)
	assert.Equal(t, UseLongTGName(), false)
}

func Test_config_init_no_env_var(t *testing.T) {
	os.Unsetenv(REGION)
	os.Unsetenv(CLUSTER_VPC_ID)
	os.Unsetenv(CLUSTER_LOCAL_GATEWAY)
	os.Unsetenv(AWS_ACCOUNT_ID)
	os.Unsetenv(TARGET_GROUP_NAME_LEN_MODE)
	clusterLocalGateway, _ := GetClusterLocalGateway()
	assert.Equal(t, GetRegion(), UnknownInput)
	assert.Equal(t, GetVpcID(), UnknownInput)
	assert.Equal(t, GetAccountID(), UnknownInput)
	assert.Equal(t, clusterLocalGateway, UnknownInput)
	assert.Equal(t, UseLongTGName(), false)
}

func Test_config_init_with_all_env_var(t *testing.T) {
	// Test variable
	testRegion := "us-west-2"
	testClusterVpcId := "vpc-123456"
	testClusterLocalGateway := "default"
	testTargetGroupNameLenMode := "long"
	testAwsAccountId := "12345678"

	os.Setenv(REGION, testRegion)
	os.Setenv(CLUSTER_VPC_ID, testClusterVpcId)
	os.Setenv(CLUSTER_LOCAL_GATEWAY, testClusterLocalGateway)
	os.Setenv(AWS_ACCOUNT_ID, testAwsAccountId)
	os.Setenv(TARGET_GROUP_NAME_LEN_MODE, testTargetGroupNameLenMode)
	clusterLocalGateway, _ := GetClusterLocalGateway()
	assert.Equal(t, GetRegion(), testRegion)
	assert.Equal(t, GetVpcID(), testClusterVpcId)
	assert.Equal(t, GetAccountID(), testAwsAccountId)
	assert.Equal(t, clusterLocalGateway, testClusterLocalGateway)
	assert.Equal(t, UseLongTGName(), true)
}
