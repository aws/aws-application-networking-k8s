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
	os.Setenv(DEFAULT_SERVICE_NETWORK, testClusterLocalGateway)
	os.Unsetenv(AWS_ACCOUNT_ID)
	err := configInit(nil, ec2MetadataUnavailable())
	assert.NotNil(t, err)
}

func Test_config_init_no_env_var(t *testing.T) {
	os.Unsetenv(REGION)
	os.Unsetenv(CLUSTER_VPC_ID)
	os.Unsetenv(DEFAULT_SERVICE_NETWORK)
	os.Unsetenv(AWS_ACCOUNT_ID)
	err := configInit(nil, ec2MetadataUnavailable())
	assert.NotNil(t, err)

}

func Test_config_init_with_all_env_var(t *testing.T) {
	// Test variable
	testRegion := "us-west-2"
	testClusterVpcId := "vpc-123456"
	testClusterLocalGateway := "default"
	testAwsAccountId := "12345678"
	testClusterName := "cluster-name"

	os.Setenv(REGION, testRegion)
	os.Setenv(CLUSTER_VPC_ID, testClusterVpcId)
	os.Setenv(DEFAULT_SERVICE_NETWORK, testClusterLocalGateway)
	os.Setenv(AWS_ACCOUNT_ID, testAwsAccountId)
	os.Setenv(CLUSTER_NAME, testClusterName)
	configInit(nil, ec2MetadataUnavailable())
	assert.Equal(t, Region, testRegion)
	assert.Equal(t, VpcID, testClusterVpcId)
	assert.Equal(t, AccountID, testAwsAccountId)
	assert.Equal(t, DefaultServiceNetwork, testClusterLocalGateway)
	assert.Equal(t, testClusterName, ClusterName)
}
