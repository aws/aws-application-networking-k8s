package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

type EC2Metadata interface {
	Region() (string, error)
	VpcID() (string, error)
	AccountId() (string, error)
}

// NewEC2Metadata constructs new EC2Metadata implementation.
func NewEC2Metadata(cfg aws.Config) EC2Metadata {
	return &defaultEC2Metadata{
		client: imds.NewFromConfig(cfg),
	}
}

type defaultEC2Metadata struct {
	client *imds.Client
}

func (c *defaultEC2Metadata) getMetadataString(path string) (string, error) {
	out, err := c.client.GetMetadata(context.TODO(), &imds.GetMetadataInput{Path: path})
	if err != nil {
		return "", err
	}
	defer out.Content.Close()
	b, err := io.ReadAll(out.Content)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *defaultEC2Metadata) VpcID() (string, error) {
	mac, err := c.getMetadataString("mac")
	if err != nil {
		return "", err
	}
	return c.getMetadataString(fmt.Sprintf("network/interfaces/macs/%s/vpc-id", mac))
}

func (c *defaultEC2Metadata) Region() (string, error) {
	return c.getMetadataString("placement/region")
}

func (c *defaultEC2Metadata) AccountId() (string, error) {
	ec2Info, err := c.getMetadataString("identity-credentials/ec2/info")
	if err != nil {
		return "", err
	}
	type accountInfo struct {
		Code        string `json:"code"`
		LastUpdated string `json:"LastUpdated"`
		AccountId   string `json:"AccountId"`
	}
	var acc accountInfo
	if err := json.Unmarshal([]byte(ec2Info), &acc); err != nil {
		return "", err
	}
	return acc.AccountId, nil
}
