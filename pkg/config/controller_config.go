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

func ConfigInit(vpcId string, region string, accountId string) {

	sess, _ := session.NewSession()
	metadata := NewEC2Metadata(sess)
	var err error

	// Check if controller running inside the k8s pod
	configDiscoveryNeeded := ifRunningInCluster()

	// VpcId
	if vpcId != "" {
		VpcID = vpcId
		glog.V(2).Infoln("CLUSTER_VPC_ID passed as input:", VpcID)
	} else {
		if configDiscoveryNeeded {
			VpcID, err = metadata.VpcID()
			glog.V(2).Infoln("CLUSTER_VPC_ID from IMDS config discovery :", VpcID)
			if err != nil {
				glog.V(2).Infoln("IMDS config discovery is NOT AVAILABLE :", err)
				return
			}
		} else {
			VpcID = os.Getenv("CLUSTER_VPC_ID")
			glog.V(2).Infoln("CLUSTER_VPC_ID from local dev environment: ", VpcID)
		}
	}

	// Region
	if region != "" {
		Region = region
		glog.V(2).Infoln("REGION passed as input:", Region)
	} else {
		if configDiscoveryNeeded {
			Region, err = metadata.Region()
			glog.V(2).Infoln("REGION from IMDS config discovery :", Region)
			if err != nil {
				return
			}
		} else {
			Region = os.Getenv("REGION")
			glog.V(2).Infoln("REGION from local dev environment: ", Region)
		}
	}

	// AccountId
	if accountId != "" {
		AccountID = accountId
		glog.V(2).Infoln("AWS_ACCOUNT_ID passed as input:", AccountID)
	} else {
		if configDiscoveryNeeded {
			AccountID, err = metadata.AccountId()
			glog.V(2).Infoln("AWS_ACCOUNT_ID from IMDS config discovery :", AccountID)
			if err != nil {
				return
			}
		} else {
			AccountID = os.Getenv("AWS_ACCOUNT_ID")
			glog.V(2).Infoln("AWS_ACCOUNT_ID from local dev environment: ", AccountID)
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
