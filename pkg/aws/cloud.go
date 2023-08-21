package aws

import (
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/vpclattice"
)

//go:generate mockgen -destination cloud_mocks.go -package aws github.com/aws/aws-application-networking-k8s/pkg/aws Cloud

const (
	TagManagedBy = "ManagedBy"
)

type Tags = map[string]*string

type CloudConfig struct {
	VpcId     string
	AccountId string
}

type Cloud interface {
	Config() CloudConfig
	Lattice() services.Lattice
	EKS() services.EKS

	// Create an empty tags map
	NewTags() Tags

	// Create tags map and add managed by controller tag
	NewTagsWithManagedBy() Tags

	// Check for tag indicating it's managed by controller
	IsTagManagedBy(Tags) bool

	// Check if ARN has tag ManagedBy
	IsArnManaged(arn *string) (bool, error)
}

// NewCloud constructs new Cloud implementation.
func NewCloud(log gwlog.Logger, cfg CloudConfig) (Cloud, error) {
	sess, err := session.NewSession()
	if err != nil {
		return nil, err
	}

	sess.Handlers.Complete.PushFront(func(r *request.Request) {
		if r.Error != nil {
			log.Debugw("error",
				"error", r.Error.Error(),
				"serviceName", r.ClientInfo.ServiceName,
				"operation", r.Operation.Name,
				"params", r.Params,
			)
		} else {
			log.Debugw("response",
				"serviceName", r.ClientInfo.ServiceName,
				"operation", r.Operation.Name,
				"params", r.Params,
			)
		}
	})

	lattice := services.NewDefaultLattice(sess, config.Region)
	eks := services.NewDefaultEKS(sess, config.Region)
	cl := NewDefaultCloud(lattice, eks, cfg)
	return cl, nil
}

// Used in testing and mocks
func NewDefaultCloud(l services.Lattice, e services.EKS, cfg CloudConfig) Cloud {
	return &defaultCloud{
		gwapiTag: gatewayApiUniqTag(cfg.VpcId),
		cfg:      cfg,
		lattice:  l,
		eks:      e,
	}
}

type defaultCloud struct {
	gwapiTag string
	cfg      CloudConfig
	lattice  services.Lattice
	eks      services.EKS
}

func (c *defaultCloud) Lattice() services.Lattice {
	return c.lattice
}

func (c *defaultCloud) EKS() services.EKS {
	return c.eks
}

func (c *defaultCloud) Config() CloudConfig {
	return c.cfg
}

func (d *defaultCloud) GetEKSClusterVPC(name string) string {
	input := &eks.DescribeClusterInput{
		Name: aws.String(name),
	}

	result, err := d.eks.DescribeCluster(input)

	if err != nil {
		return ""
	}
	return result.String()
}

func (c *defaultCloud) NewTags() Tags {
	return map[string]*string{}
}

func (c *defaultCloud) NewTagsWithManagedBy() Tags {
	tags := c.NewTags()
	tags[TagManagedBy] = &c.gwapiTag
	return tags
}

func (c *defaultCloud) IsTagManagedBy(tags Tags) bool {
	if tags == nil {
		return false
	}
	t, ok := tags[TagManagedBy]
	if ok && t != nil {
		return *t == c.gwapiTag
	}
	return false
}

func (c *defaultCloud) IsArnManaged(arn *string) (bool, error) {
	tagsReq := &vpclattice.ListTagsForResourceInput{ResourceArn: arn}
	tagsResp, err := c.lattice.ListTagsForResource(tagsReq)
	if err != nil {
		return false, err
	}
	return c.IsTagManagedBy(tagsResp.Tags), nil
}

// a unique identifier for ManagedBy tag that controller uses
func gatewayApiUniqTag(vpcid string) string {
	return fmt.Sprintf("k8s-gwapi-%s", vpcid)
}
