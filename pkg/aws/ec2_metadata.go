package aws

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
)

type EC2Metadata interface {
	Region() (string, error)
	VpcID() (string, error)
	AccountId() (string, error)
}

// NewEC2Metadata constructs new EC2Metadata implementation.
func NewEC2Metadata(session *session.Session) EC2Metadata {
	return &defaultEC2Metadata{
		EC2Metadata: ec2metadata.New(session),
	}
}

type defaultEC2Metadata struct {
	*ec2metadata.EC2Metadata
}

func (c *defaultEC2Metadata) VpcID() (string, error) {
	mac, err := c.GetMetadata("mac")
	if err != nil {
		return "", err
	}
	vpcID, err := c.GetMetadata(fmt.Sprintf("network/interfaces/macs/%s/vpc-id", mac))
	if err != nil {
		return "", err
	}
	return vpcID, nil
}

func (c *defaultEC2Metadata) Region() (string, error) {
	region, err := c.GetMetadata(fmt.Sprintf("placement/region"))
	if err != nil {
		return "", err
	}
	return region, nil
}

func (c *defaultEC2Metadata) AccountId() (string, error) {
	ec2Info, err := c.GetMetadata(fmt.Sprintf("identity-credentials/ec2/info"))
	type accountInfo struct {
		Code        string `json:"code"`
		LastUpdated string `json:"LastUpdated"`
		AccountId   string `json:"AccountId"`
	}

	var acc accountInfo
	json.Unmarshal([]byte(ec2Info), &acc)
	if err != nil {
		return "", err
	}
	return acc.AccountId, nil
}
