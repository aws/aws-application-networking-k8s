package config

import (
	"errors"
	"os"
	"testing"

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
	os.Setenv(CLUSTER_LOCAL_GATEWAY, testClusterLocalGateway)
	os.Unsetenv(AWS_ACCOUNT_ID)
	os.Unsetenv(TARGET_GROUP_NAME_LEN_MODE)
	err := configInit(ec2MetadataUnavailable())
	assert.NotNil(t, err)
}

func Test_config_init_no_env_var(t *testing.T) {
	os.Unsetenv(REGION)
	os.Unsetenv(CLUSTER_VPC_ID)
	os.Unsetenv(CLUSTER_LOCAL_GATEWAY)
	os.Unsetenv(AWS_ACCOUNT_ID)
	os.Unsetenv(TARGET_GROUP_NAME_LEN_MODE)
	err := configInit(ec2MetadataUnavailable())
	assert.NotNil(t, err)

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
	configInit(ec2MetadataUnavailable())
	assert.Equal(t, Region, testRegion)
	assert.Equal(t, VpcID, testClusterVpcId)
	assert.Equal(t, AccountID, testAwsAccountId)
	assert.Equal(t, DefaultServiceNetwork, testClusterLocalGateway)
	assert.Equal(t, UseLongTGName, true)
}
