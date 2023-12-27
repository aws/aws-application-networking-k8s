package config

import (
	"errors"
	"fmt"
	"os"

	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

const (
	LatticeGatewayControllerName = "application-networking.k8s.aws/gateway-api-controller"
	defaultLogLevel              = "Info"
)

const (
	REGION                          = "REGION"
	CLUSTER_VPC_ID                  = "CLUSTER_VPC_ID"
	CLUSTER_NAME                    = "CLUSTER_NAME"
	DEFAULT_SERVICE_NETWORK         = "DEFAULT_SERVICE_NETWORK"
	ENABLE_SERVICE_NETWORK_OVERRIDE = "ENABLE_SERVICE_NETWORK_OVERRIDE"
	AWS_ACCOUNT_ID                  = "AWS_ACCOUNT_ID"
)

var VpcID = ""
var AccountID = ""
var Region = ""
var DefaultServiceNetwork = ""
var ClusterName = ""

var ServiceNetworkOverrideMode = false

func ConfigInit() error {
	sess, _ := session.NewSession()
	metadata := NewEC2Metadata(sess)
	return configInit(sess, metadata)
}

func configInit(sess *session.Session, metadata EC2Metadata) error {
	var err error

	// CLUSTER_VPC_ID
	VpcID = os.Getenv(CLUSTER_VPC_ID)
	if VpcID == "" {
		VpcID, err = metadata.VpcID()
		if err != nil {
			return fmt.Errorf("vpcId is not specified: %s", err)
		}
	}

	// REGION
	Region = os.Getenv(REGION)
	if Region == "" {
		Region, err = metadata.Region()
		if err != nil {
			return fmt.Errorf("region is not specified: %s", err)
		}
	}

	// AWS_ACCOUNT_ID
	AccountID = os.Getenv(AWS_ACCOUNT_ID)
	if AccountID == "" {
		AccountID, err = metadata.AccountId()
		if err != nil {
			return fmt.Errorf("account is not specified: %s", err)
		}
	}

	// DEFAULT_SERVICE_NETWORK
	DefaultServiceNetwork = os.Getenv(DEFAULT_SERVICE_NETWORK)

	overrideFlag := os.Getenv(ENABLE_SERVICE_NETWORK_OVERRIDE)
	if strings.ToLower(overrideFlag) == "true" && DefaultServiceNetwork != "" {
		ServiceNetworkOverrideMode = true
	}

	// CLUSTER_NAME
	ClusterName, err = getClusterName(sess)
	if err != nil {
		return fmt.Errorf("cannot get cluster name: %s", err)
	}

	return nil
}

// try to find cluster name, search in env then in ec2 instance tags
func getClusterName(sess *session.Session) (string, error) {
	cn := os.Getenv(CLUSTER_NAME)
	if cn != "" {
		return cn, nil
	}
	// fallback to ec2 instance tags
	meta := ec2metadata.New(sess)
	doc, err := meta.GetInstanceIdentityDocument()
	if err != nil {
		return "", err
	}
	instanceId := doc.InstanceID
	region, err := meta.Region()
	if err != nil {
		return "", err
	}
	ec2Client := ec2.New(sess, &aws.Config{Region: aws.String(region)})
	tagReq := &ec2.DescribeTagsInput{Filters: []*ec2.Filter{{
		Name:   aws.String("resource-id"),
		Values: []*string{aws.String(instanceId)},
	}}}
	tagRes, err := ec2Client.DescribeTags(tagReq)
	if err != nil {
		return "", err
	}
	for _, tag := range tagRes.Tags {
		if *tag.Key == "aws:eks:cluster-name" {
			return *tag.Value, nil
		}
	}
	return "", errors.New("not found in env and metadata")
}
