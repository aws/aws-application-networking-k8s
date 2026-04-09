package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
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
	DISABLE_TAGGING_SERVICE_API     = "DISABLE_TAGGING_SERVICE_API"
	ENABLE_SERVICE_NETWORK_OVERRIDE = "ENABLE_SERVICE_NETWORK_OVERRIDE"
	AWS_ACCOUNT_ID                  = "AWS_ACCOUNT_ID"
	DEV_MODE                        = "DEV_MODE"
	WEBHOOK_ENABLED                 = "WEBHOOK_ENABLED"
	ROUTE_MAX_CONCURRENT_RECONCILES = "ROUTE_MAX_CONCURRENT_RECONCILES"
)

var VpcID = ""
var AccountID = ""
var Region = ""
var DefaultServiceNetwork = ""
var ClusterName = ""
var DevMode = ""
var WebhookEnabled = ""

var DisableTaggingServiceAPI = false
var ServiceNetworkOverrideMode = false
var RouteMaxConcurrentReconciles = 1

func ConfigInit() error {
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO())
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	metadata := NewEC2Metadata(cfg)
	return configInit(cfg, metadata)
}

func configInit(cfg aws.Config, metadata EC2Metadata) error {
	var err error

	DevMode = os.Getenv(DEV_MODE)
	WebhookEnabled = os.Getenv(WEBHOOK_ENABLED)

	VpcID = os.Getenv(CLUSTER_VPC_ID)
	if VpcID == "" {
		VpcID, err = metadata.VpcID()
		if err != nil {
			return fmt.Errorf("vpcId is not specified: %s", err)
		}
	}

	Region = os.Getenv(REGION)
	if Region == "" {
		Region, err = metadata.Region()
		if err != nil {
			return fmt.Errorf("region is not specified: %s", err)
		}
	}

	AccountID = os.Getenv(AWS_ACCOUNT_ID)
	if AccountID == "" {
		AccountID, err = metadata.AccountId()
		if err != nil {
			return fmt.Errorf("account is not specified: %s", err)
		}
	}

	DefaultServiceNetwork = os.Getenv(DEFAULT_SERVICE_NETWORK)

	overrideFlag := os.Getenv(ENABLE_SERVICE_NETWORK_OVERRIDE)
	if strings.ToLower(overrideFlag) == "true" && DefaultServiceNetwork != "" {
		ServiceNetworkOverrideMode = true
	}

	disableTaggingAPI := os.Getenv(DISABLE_TAGGING_SERVICE_API)

	if strings.ToLower(disableTaggingAPI) == "true" {
		DisableTaggingServiceAPI = true
	}

	ClusterName, err = getClusterName(cfg)
	if err != nil {
		return fmt.Errorf("cannot get cluster name: %s", err)
	}

	routeMaxConcurrentReconciles := os.Getenv(ROUTE_MAX_CONCURRENT_RECONCILES)
	if routeMaxConcurrentReconciles != "" {
		routeMaxConcurrentReconcilesInt, err := strconv.Atoi(routeMaxConcurrentReconciles)
		if err != nil {
			return fmt.Errorf("invalid value for ROUTE_MAX_CONCURRENT_RECONCILES: %s", err)
		}
		RouteMaxConcurrentReconciles = routeMaxConcurrentReconcilesInt
	}

	return nil
}

// try to find cluster name, search in env then in ec2 instance tags
func getClusterName(cfg aws.Config) (string, error) {
	cn := os.Getenv(CLUSTER_NAME)
	if cn != "" {
		return cn, nil
	}

	// fallback to ec2 instance tags
	ctx := context.TODO()
	imdsClient := imds.NewFromConfig(cfg)

	doc, err := imdsClient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		return "", err
	}
	instanceId := doc.InstanceID
	region := doc.Region

	ec2Client := ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		o.Region = region
	})

	tagRes, err := ec2Client.DescribeTags(ctx, &ec2.DescribeTagsInput{
		Filters: []ec2types.Filter{{
			Name:   aws.String("resource-id"),
			Values: []string{instanceId},
		}},
	})
	if err != nil {
		return "", err
	}
	for _, tag := range tagRes.Tags {
		if aws.ToString(tag.Key) == "aws:eks:cluster-name" {
			return aws.ToString(tag.Value), nil
		}
	}
	return "", errors.New("not found in env and metadata")
}
