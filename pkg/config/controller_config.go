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

var VpcID = UnknownInput
var AccountID = UnknownInput
var Region = UnknownInput
var logLevel = defaultLogLevel
var DefaultServiceNetwork = UnknownInput
var UseLongTGName = false

func GetLogLevel() string {
	logLevel = os.Getenv(GATEWAY_API_CONTROLLER_LOGLEVEL)
	switch strings.ToLower(logLevel) {
	case "debug":
		return "10"
	case "info":
		return "2"
	}
	return "2"
}

func GetClusterLocalGateway() (string, error) {
	if DefaultServiceNetwork == UnknownInput {
		return UnknownInput, errors.New(NO_DEFAULT_SERVICE_NETWORK)
	}

	return DefaultServiceNetwork, nil
}

func ConfigInit() {

	sess, _ := session.NewSession()
	metadata := NewEC2Metadata(sess)
	var err error

	// CLUSTER_VPC_ID
	VpcID = os.Getenv(CLUSTER_VPC_ID)
	if VpcID != UnknownInput {
		glog.V(2).Infoln("CLUSTER_VPC_ID passed as input:", VpcID)
	} else {
		VpcID, err = metadata.VpcID()
		glog.V(2).Infoln("CLUSTER_VPC_ID from IMDS config discovery :", VpcID)
		if err != nil {
			glog.V(2).Infoln("IMDS config discovery for CLUSTER_VPC_ID is NOT AVAILABLE :", err)
		}
	}

	// REGION
	Region = os.Getenv(REGION)
	if Region != UnknownInput {
		glog.V(2).Infoln("REGION passed as input:", Region)
	} else {
		Region, err = metadata.Region()
		glog.V(2).Infoln("REGION from IMDS config discovery :", Region)
		if err != nil {
			glog.V(2).Infoln("IMDS config discovery for REGION is NOT AVAILABLE :", err)
		}
	}

	// AWS_ACCOUNT_ID
	AccountID = os.Getenv(AWS_ACCOUNT_ID)
	if AccountID != UnknownInput {
		glog.V(2).Infoln("AWS_ACCOUNT_ID passed as input:", AccountID)
	} else {
		AccountID, err = metadata.AccountId()
		glog.V(2).Infoln("AWS_ACCOUNT_ID from IMDS config discovery :", AccountID)
		if err != nil {
			glog.V(2).Infoln("IMDS config discovery for AWS_ACCOUNT_ID is NOT AVAILABLE :", err)
		}
	}

	// GATEWAY_API_CONTROLLER_LOGLEVEL
	logLevel = os.Getenv(GATEWAY_API_CONTROLLER_LOGLEVEL)
	glog.V(2).Infoln("Logging Level:", os.Getenv(GATEWAY_API_CONTROLLER_LOGLEVEL))

	// CLUSTER_LOCAL_GATEWAY
	DefaultServiceNetwork = os.Getenv(CLUSTER_LOCAL_GATEWAY)
	if DefaultServiceNetwork == UnknownInput {
		glog.V(2).Infoln("No CLUSTER_LOCAL_GATEWAY")
	} else {
		glog.V(2).Infoln("CLUSTER_LOCAL_GATEWAY", DefaultServiceNetwork)
	}

	// TARGET_GROUP_NAME_LEN_MODE
	tgNameLengthMode := os.Getenv(TARGET_GROUP_NAME_LEN_MODE)
	glog.V(2).Infoln("TARGET_GROUP_NAME_LEN_MODE", tgNameLengthMode)

	if tgNameLengthMode == "long" {
		UseLongTGName = true
	} else {
		UseLongTGName = false
	}
}
