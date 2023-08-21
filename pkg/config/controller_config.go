package config

/*
import (
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws/session"
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

type ControllerConfig struct {
	vpcID                 string
	region                string
	accountID             string
	defaultServiceNetwork string
	logLevel              string
	useLongTGName         bool
}

func NewControllerConfig() *ControllerConfig {
	var config ControllerConfig

	config.ConfigInit()

	return &config
}

func (lgc *ControllerConfig) GetLogLevel() string {
	return lgc.logLevel
}

func (lgc *ControllerConfig) SetLogLevel(logLevel string) {
	lgc.logLevel = logLevel
}

func (lgc *ControllerConfig) GetVpcID() string {
	return lgc.vpcID
}

func (lgc *ControllerConfig) SetVpcID(vpcID string) {
	lgc.vpcID = vpcID
}

func (lgc *ControllerConfig) GetRegion() string {
	return lgc.region
}

func (lgc *ControllerConfig) GetAccountID() string {
	return lgc.accountID
}

func (lgc *ControllerConfig) GetClusterLocalGateway() (string, error) {
	if lgc.defaultServiceNetwork == UnknownInput {
		return UnknownInput, errors.New(NO_DEFAULT_SERVICE_NETWORK)
	}
	return lgc.defaultServiceNetwork, nil
}

func (lgc *ControllerConfig) UseLongTGName() bool {
	return lgc.useLongTGName
}

var VpcID = ""
var AccountID = ""
var Region = ""
var logLevel = defaultLogLevel
var DefaultServiceNetwork = ""
var UseLongTGName = false

	func GetClusterLocalGateway() (string, error) {
		if DefaultServiceNetwork == UnknownInput {
			return UnknownInput, errors.New(NO_DEFAULT_SERVICE_NETWORK)
		}
		return DefaultServiceNetwork, nil
	}

func (lgc *ControllerConfig) ConfigInit() []error {
	sess, _ := session.NewSession()
	metadata := NewEC2Metadata(sess)
	errs := make([]error, 0)
	var err error

	// CLUSTER_VPC_ID
	lgc.vpcID = os.Getenv(CLUSTER_VPC_ID)
	if lgc.vpcID == "" {
		lgc.vpcID, err = metadata.VpcID()
		if err != nil {
			errs = append(errs, fmt.Errorf("vpcId is not specified: %s", err))
		}
	}

	// REGION
	lgc.region = os.Getenv(REGION)
	if lgc.region == "" {
		lgc.region, err = metadata.Region()
		if err != nil {
			errs = append(errs, fmt.Errorf("region is not specified: %s", err))
		}
	}

	// AWS_ACCOUNT_ID
	lgc.accountID = os.Getenv(AWS_ACCOUNT_ID)
	if lgc.accountID == "" {
		lgc.accountID, err = metadata.AccountId()
		if err != nil {
			errs = append(errs, fmt.Errorf("account is not specified: %s", err))
		}
	}

	// CLUSTER_LOCAL_GATEWAY
	lgc.defaultServiceNetwork = os.Getenv(CLUSTER_LOCAL_GATEWAY)

	// TARGET_GROUP_NAME_LEN_MODE
	tgNameLengthMode := os.Getenv(TARGET_GROUP_NAME_LEN_MODE)

	if tgNameLengthMode == "long" {
		lgc.useLongTGName = true
	} else {
		lgc.useLongTGName = false
	}

	return errs
}
*/
