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
	NoDefaultServiceNetwork      = ""
	NO_DEFAULT_SERVICE_NETWORK   = "NO_DEFAULT_SERVICE_NETWORK"
)

// TODO endpoint, region
var VpcID = "vpc-xxxx"
var AccountID = "yyyyyy"
var Region = "us-west-2"
var logLevel = defaultLogLevel
var DefaultServiceNetwork = NoDefaultServiceNetwork
var UseLongTGName = false

func GetLogLevel() string {
	logLevel = os.Getenv("GATEWAY_API_CONTROLLER_LOGLEVEL")
	switch strings.ToLower(logLevel) {
	case "debug":
		return "10"
	case "info":
		return "2"
	}
	return "2"
}

func GetClusterLocalGateway() (string, error) {
	if DefaultServiceNetwork == NoDefaultServiceNetwork {
		return NoDefaultServiceNetwork, errors.New(NO_DEFAULT_SERVICE_NETWORK)
	}

	return DefaultServiceNetwork, nil
}

func ConfigInit() {

	sess, _ := session.NewSession()
	metadata := NewEC2Metadata(sess)
	var err error

	// VpcId
	VpcID = os.Getenv("CLUSTER_VPC_ID")
	if VpcID != "" {
		glog.V(2).Infoln("CLUSTER_VPC_ID passed as input:", VpcID)
	} else {
		VpcID, err = metadata.VpcID()
		glog.V(2).Infoln("CLUSTER_VPC_ID from IMDS config discovery :", VpcID)
		if err != nil {
			glog.V(2).Infoln("IMDS config discovery for CLUSTER_VPC_ID is NOT AVAILABLE :", err)
			return
		}
	}

	// Region
	Region = os.Getenv("REGION")
	if Region != "" {
		glog.V(2).Infoln("REGION passed as input:", Region)
	} else {
		Region, err = metadata.Region()
		glog.V(2).Infoln("REGION from IMDS config discovery :", Region)
		if err != nil {
			glog.V(2).Infoln("IMDS config discovery for REGION is NOT AVAILABLE :", err)
			return
		}
	}

	// AccountId
	AccountID = os.Getenv("AWS_ACCOUNT_ID")
	if AccountID != "" {
		glog.V(2).Infoln("AWS_ACCOUNT_ID passed as input:", AccountID)
	} else {
		AccountID, err = metadata.AccountId()
		glog.V(2).Infoln("AWS_ACCOUNT_ID from IMDS config discovery :", AccountID)
		if err != nil {
			glog.V(2).Infoln("IMDS config discovery for AWS_ACCOUNT_ID is NOT AVAILABLE :", err)
			return
		}
	}

	logLevel = os.Getenv("GATEWAY_API_CONTROLLER_LOGLEVEL")
	glog.V(2).Infoln("Logging Level:", os.Getenv("GATEWAY_API_CONTROLLER_LOGLEVEL"))

	DefaultServiceNetwork = os.Getenv("CLUSTER_LOCAL_GATEWAY")

	if DefaultServiceNetwork == NoDefaultServiceNetwork {
		glog.V(2).Infoln("No CLUSTER_LOCAL_GATEWAY")
	} else {

		glog.V(2).Infoln("CLUSTER_LOCAL_GATEWAY", DefaultServiceNetwork)
	}

	tgNameLengthMode := os.Getenv("TARGET_GROUP_NAME_LEN_MODE")

	glog.V(2).Infoln("TARGET_GROUP_NAME_LEN_MODE", tgNameLengthMode)

	if tgNameLengthMode == "long" {
		UseLongTGName = true
	} else {
		UseLongTGName = false
	}

}

func ifRunningInCluster() bool {
	_, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount")
	if err == nil {
		return true
	}
	return false
}
