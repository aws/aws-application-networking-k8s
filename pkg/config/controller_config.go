package config

import (
	"github.com/golang/glog"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
)

const (
	LatticeGatewayControllerName = "application-networking.k8s.aws/gateway-api-controller"
	defaultLogLevel              = "Info"
)

// TODO endpoint, region
var VpcID = "vpc-xxxx"
var AccountID = "yyyyyy"
var Region = "us-east-2"
var logLevel = defaultLogLevel

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

func ConfigInit() {
	// discover VPC using environment first
	VpcID = os.Getenv("CLUSTER_VPC_ID")
	glog.V(2).Infoln("CLUSTER_VPC_ID: ", os.Getenv("CLUSTER_VPC_ID"))

	// discover Account
	AccountID = os.Getenv("ACCOUNT_ID")
	glog.V(2).Infoln("ACCOUNT_ID:", os.Getenv("ACCOUNT_ID"))

	// discover Region
	Region = os.Getenv("REGION")
	glog.V(2).Infoln("REGION:", os.Getenv("REGION"))

	logLevel = os.Getenv("GATEWAY_API_CONTROLLER_LOGLEVEL")
	glog.V(2).Infoln("Logging Level:", os.Getenv("GATEWAY_API_CONTROLLER_LOGLEVEL"))

	sess, _ := session.NewSession()
	metadata := NewEC2Metadata(sess)

	var err error
	if ifRunningInCluster() {
		VpcID, err = metadata.VpcID()
		if err != nil {
			return
		}
		Region, err = metadata.Region()
		if err != nil {
			return
		}
		AccountID, err = metadata.AccountId()
		if err != nil {
			return
		}
		glog.V(2).Infoln("INSIDE CLUSTER CLUSTER_VPC_ID: ", VpcID)
		glog.V(2).Infoln("INSIDE CLUSTER  REGION: ", Region)
		glog.V(2).Infoln("INSIDE CLUSTER ACCOUNT_ID: ", AccountID)
	}
}

func ifRunningInCluster() bool {
	_, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount")
	if err == nil {
		glog.V(2).Infoln("Controller is running inside cluster")
		return true
	}

	if os.IsNotExist(err) {
		glog.V(2).Infoln("Controller is NOT running inside cluster")
		return false
	}

	glog.V(2).Infoln("Controller is NOT running inside cluster")
	return false
}
