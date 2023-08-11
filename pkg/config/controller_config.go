package config

import (
	"errors"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/golang/glog"
)

const (
	LatticeGatewayControllerName = "application-networking.k8s.aws/gateway-api-controller"
	defaultLogLevel              = "Info"
	UnknownInput                 = ""
)

const (
	NO_DEFAULT_SERVICE_NETWORK      = "NO_DEFAULT_SERVICE_NETWORK"
	REGION                          = "REGION"
	CLUSTER_VPC_ID                  = "CLUSTER_VPC_ID"
	CLUSTER_LOCAL_GATEWAY           = "CLUSTER_LOCAL_GATEWAY"
	AWS_ACCOUNT_ID                  = "AWS_ACCOUNT_ID"
	TARGET_GROUP_NAME_LEN_MODE      = "TARGET_GROUP_NAME_LEN_MODE"
	GATEWAY_API_CONTROLLER_LOGLEVEL = "GATEWAY_API_CONTROLLER_LOGLEVEL"
)

// GATEWAY_API_CONTROLLER_LOGLEVEL
func GetLogLevel() string {
	logLevel := os.Getenv(GATEWAY_API_CONTROLLER_LOGLEVEL)
	switch strings.ToLower(logLevel) {
	case "debug":
		return "10"
	case "info":
		return "2"
	}
	return "2"
}

func SetLogLevel(logLevel string) {
	os.Setenv(GATEWAY_API_CONTROLLER_LOGLEVEL, logLevel)
}

// CLUSTER_VPC_ID
func GetVpcID() string {
	sess, _ := session.NewSession()
	metadata := NewEC2Metadata(sess)

	var err error

	vpcID := os.Getenv(CLUSTER_VPC_ID)
	if vpcID != UnknownInput {
		glog.V(2).Infoln("CLUSTER_VPC_ID passed as input:", vpcID)
	} else {
		vpcID, err = metadata.VpcID()
		glog.V(2).Infoln("CLUSTER_VPC_ID from IMDS config discovery :", vpcID)
		if err != nil {
			glog.V(2).Infoln("IMDS config discovery for CLUSTER_VPC_ID is NOT AVAILABLE :", err)
		}
	}
	return vpcID
}

func SetVpcID(vpcId string) {
	os.Setenv(CLUSTER_VPC_ID, vpcId)
}

// REGION
func GetRegion() string {
	var err error

	sess, _ := session.NewSession()
	metadata := NewEC2Metadata(sess)

	awsRegion := os.Getenv(REGION)
	if awsRegion != UnknownInput {
		glog.V(2).Infoln("REGION passed as input:", awsRegion)
	} else {
		awsRegion, err = metadata.Region()
		glog.V(2).Infoln("REGION from IMDS config discovery :", awsRegion)
		if err != nil {
			glog.V(2).Infoln("IMDS config discovery for REGION is NOT AVAILABLE :", err)
		}
	}
	return awsRegion
}

// AWS_ACCOUNT_ID
func GetAccountID() string {
	var err error

	sess, _ := session.NewSession()
	metadata := NewEC2Metadata(sess)

	accountID := os.Getenv(AWS_ACCOUNT_ID)
	if accountID != UnknownInput {
		glog.V(2).Infoln("AWS_ACCOUNT_ID passed as input:", accountID)
	} else {
		accountID, err = metadata.AccountId()
		glog.V(2).Infoln("AWS_ACCOUNT_ID from IMDS config discovery :", accountID)
		if err != nil {
			glog.V(2).Infoln("IMDS config discovery for AWS_ACCOUNT_ID is NOT AVAILABLE :", err)
		}
	}
	return accountID
}

// CLUSTER_LOCAL_GATEWAY
func GetClusterLocalGateway() (string, error) {
	defaultServiceNetwork := os.Getenv(CLUSTER_LOCAL_GATEWAY)

	if defaultServiceNetwork == UnknownInput {
		glog.V(2).Infoln("No CLUSTER_LOCAL_GATEWAY")
		return UnknownInput, errors.New(NO_DEFAULT_SERVICE_NETWORK)
	} else {
		glog.V(2).Infoln("CLUSTER_LOCAL_GATEWAY", defaultServiceNetwork)
	}
	return defaultServiceNetwork, nil
}

// TARGET_GROUP_NAME_LEN_MODE
func UseLongTGName() bool {
	tgNameLengthMode := os.Getenv(TARGET_GROUP_NAME_LEN_MODE)
	glog.V(2).Infoln("TARGET_GROUP_NAME_LEN_MODE", tgNameLengthMode)

	if tgNameLengthMode == "long" {
		return true
	} else {
		return false
	}
}
