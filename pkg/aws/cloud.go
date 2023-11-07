package aws

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"golang.org/x/exp/maps"

	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	TagBase      = "application-networking.k8s.aws/"
	TagManagedBy = TagBase + "ManagedBy"
)

//go:generate mockgen -destination cloud_mocks.go -package aws github.com/aws/aws-application-networking-k8s/pkg/aws Cloud

type CloudConfig struct {
	VpcId       string
	AccountId   string
	Region      string
	ClusterName string
}

type Cloud interface {
	Config() CloudConfig
	Lattice() services.Lattice

	// creates lattice tags with default values populated
	DefaultTags() services.Tags

	// creates lattice tags with default values populated and merges them with provided tags
	DefaultTagsMergedWith(services.Tags) services.Tags

	// Retrieve tags and check if tags map has managedBy tag
	ContainsManagedBy(tags services.Tags) bool

	// check if managedBy tag set for lattice resource
	IsArnManaged(ctx context.Context, arn string) (bool, error)

	// check ownership and acquire if it is not owned by anyone.
	CheckAndAcquireOwnership(ctx context.Context, arn string) (bool, error)
	CheckAndAcquireOwnershipFromTags(ctx context.Context, arn string, tags services.Tags) (bool, error)
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

	lattice := services.NewDefaultLattice(sess, cfg.Region)
	cl := NewDefaultCloud(lattice, cfg)
	return cl, nil
}

// Used in testing and mocks
func NewDefaultCloud(lattice services.Lattice, cfg CloudConfig) Cloud {
	return &defaultCloud{
		cfg:          cfg,
		lattice:      lattice,
		managedByTag: getManagedByTag(cfg),
	}
}

type defaultCloud struct {
	cfg          CloudConfig
	lattice      services.Lattice
	managedByTag string
}

func (c *defaultCloud) Lattice() services.Lattice {
	return c.lattice
}

func (c *defaultCloud) Config() CloudConfig {
	return c.cfg
}

func (c *defaultCloud) DefaultTags() services.Tags {
	tags := services.Tags{}
	tags[TagManagedBy] = &c.managedByTag
	return tags
}

func (c *defaultCloud) DefaultTagsMergedWith(tags services.Tags) services.Tags {
	newTags := c.DefaultTags()
	maps.Copy(newTags, tags)
	return newTags
}

func (c *defaultCloud) getManagedByFromTags(tags services.Tags) string {
	tag, ok := tags[TagManagedBy]
	if !ok || tag == nil {
		return ""
	}
	return *tag
}

func (c *defaultCloud) getManagedBy(ctx context.Context, arn string) (string, error) {
	tagsReq := &vpclattice.ListTagsForResourceInput{ResourceArn: &arn}
	resp, err := c.lattice.ListTagsForResourceWithContext(ctx, tagsReq)
	if err != nil {
		return "", err
	}
	return c.getManagedByFromTags(resp.Tags), nil
}

func (c *defaultCloud) ContainsManagedBy(tags services.Tags) bool {
	return c.isOwner(c.getManagedByFromTags(tags))
}

func (c *defaultCloud) IsArnManaged(ctx context.Context, arn string) (bool, error) {
	managedBy, err := c.getManagedBy(ctx, arn)
	if err != nil {
		return false, nil
	}
	return c.isOwner(managedBy), nil
}

func (c *defaultCloud) CheckAndAcquireOwnership(ctx context.Context, arn string) (bool, error) {
	// For resources that need backwards compatibility - not having managedBy is considered as owned by controller.
	managedBy, err := c.getManagedBy(ctx, arn)
	if err != nil {
		return false, err
	}
	if managedBy == "" {
		err := c.acquireOwnership(ctx, arn)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return c.isOwner(managedBy), nil
}

func (c *defaultCloud) CheckAndAcquireOwnershipFromTags(ctx context.Context, arn string, tags services.Tags) (bool, error) {
	// For resources that need backwards compatibility - not having managedBy is considered as owned by controller.
	managedBy := c.getManagedByFromTags(tags)
	if managedBy == "" {
		err := c.acquireOwnership(ctx, arn)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return c.isOwner(managedBy), nil
}

func (c *defaultCloud) acquireOwnership(ctx context.Context, arn string) error {
	_, err := c.Lattice().TagResourceWithContext(ctx, &vpclattice.TagResourceInput{
		ResourceArn: &arn,
		Tags:        c.DefaultTags(),
	})
	return err
}

func (c *defaultCloud) isOwner(managedBy string) bool {
	return managedBy == c.managedByTag
}

func getManagedByTag(cfg CloudConfig) string {
	return fmt.Sprintf("%s/%s/%s", cfg.AccountId, cfg.ClusterName, cfg.VpcId)
}
