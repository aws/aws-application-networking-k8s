package aws

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/smithy-go/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"maps"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/metrics"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	TagBase      = "application-networking.k8s.aws/"
	TagManagedBy = TagBase + "ManagedBy"
)

//go:generate mockgen -destination cloud_mocks.go -package aws github.com/aws/aws-application-networking-k8s/pkg/aws Cloud

type CloudConfig struct {
	VpcId                     string
	AccountId                 string
	Region                    string
	ClusterName               string
	TaggingServiceAPIDisabled bool
}

type Cloud interface {
	Config() CloudConfig
	Lattice() services.Lattice
	Tagging() services.Tagging
	ACM() services.ACM

	// creates lattice tags with default values populated
	DefaultTags() services.Tags

	// creates lattice tags with default values populated and merges them with provided tags
	DefaultTagsMergedWith(services.Tags) services.Tags

	// check if managedBy tag set for lattice resource
	IsArnManaged(ctx context.Context, arn string) (bool, error)

	// check ownership and acquire if it is not owned by anyone.
	TryOwn(ctx context.Context, arn string) (bool, error)
	TryOwnFromTags(ctx context.Context, arn string, tags services.Tags) (bool, error)

	// MergeTags creates a new tag map by merging baseTags and additionalTags.
	// BaseTags will override additionalTags for any duplicate keys.
	MergeTags(baseTags services.Tags, additionalTags services.Tags) services.Tags

	// GetManagedByFromTags extracts the ManagedBy tag value from a tags map
	GetManagedByFromTags(tags services.Tags) string
}

// NewCloud constructs new Cloud implementation.
func NewCloud(log gwlog.Logger, cfg CloudConfig, metricsRegisterer prometheus.Registerer) (Cloud, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, err
	}

	// Add logging middleware
	awsCfg.APIOptions = append(awsCfg.APIOptions, func(stack *middleware.Stack) error {
		return stack.Initialize.Add(&loggingMiddleware{log: log}, middleware.After)
	})

	// Add metrics middleware
	if metricsRegisterer != nil {
		metricsCollector, err := metrics.NewCollector(metricsRegisterer)
		if err != nil {
			return nil, err
		}
		awsCfg.APIOptions = append(awsCfg.APIOptions, metricsCollector.APIOptions()...)
	}

	lattice := services.NewDefaultLattice(awsCfg, cfg.AccountId, cfg.Region)
	var tagging services.Tagging

	if cfg.TaggingServiceAPIDisabled {
		tagging = services.NewLatticeTagging(awsCfg, cfg.AccountId, cfg.Region, cfg.VpcId)
	} else {
		tagging = services.NewDefaultTagging(awsCfg, cfg.Region)
	}

	acmClient := services.NewDefaultACM(awsCfg, cfg.Region)

	return &defaultCloud{
		cfg:          cfg,
		lattice:      lattice,
		tagging:      tagging,
		acm:          acmClient,
		managedByTag: getManagedByTag(cfg),
	}, nil
}

// Used in testing and mocks
func NewDefaultCloud(lattice services.Lattice, cfg CloudConfig) Cloud {
	return &defaultCloud{
		cfg:          cfg,
		lattice:      lattice,
		managedByTag: getManagedByTag(cfg),
	}
}

func NewDefaultCloudWithTagging(lattice services.Lattice, tagging services.Tagging, cfg CloudConfig) Cloud {
	return &defaultCloud{
		cfg:          cfg,
		lattice:      lattice,
		tagging:      tagging,
		managedByTag: getManagedByTag(cfg),
	}
}

type defaultCloud struct {
	cfg          CloudConfig
	lattice      services.Lattice
	tagging      services.Tagging
	acm          services.ACM
	managedByTag string
}

func (c *defaultCloud) Lattice() services.Lattice {
	return c.lattice
}

func (c *defaultCloud) Tagging() services.Tagging {
	return c.tagging
}

func (c *defaultCloud) ACM() services.ACM {
	return c.acm
}

func (c *defaultCloud) Config() CloudConfig {
	return c.cfg
}

func (c *defaultCloud) DefaultTags() services.Tags {
	return services.Tags{
		TagManagedBy: c.managedByTag,
	}
}

func (c *defaultCloud) DefaultTagsMergedWith(tags services.Tags) services.Tags {
	newTags := c.DefaultTags()
	maps.Copy(newTags, tags)
	return newTags
}

func (c *defaultCloud) MergeTags(baseTags services.Tags, additionalTags services.Tags) services.Tags {
	result := make(services.Tags)
	if additionalTags != nil {
		maps.Copy(result, additionalTags)
	}
	if baseTags != nil {
		maps.Copy(result, baseTags)
	}
	return result
}

func (c *defaultCloud) getTags(ctx context.Context, arn string) (services.Tags, error) {
	resp, err := c.lattice.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{ResourceArn: &arn})
	if err != nil {
		return nil, err
	}
	return resp.Tags, nil
}

func (c *defaultCloud) GetManagedByFromTags(tags services.Tags) string {
	tag, ok := tags[TagManagedBy]
	if !ok {
		return ""
	}
	return tag
}

func (c *defaultCloud) IsArnManaged(ctx context.Context, arn string) (bool, error) {
	tags, err := c.getTags(ctx, arn)
	if err != nil {
		return false, err
	}
	return c.isOwner(c.GetManagedByFromTags(tags)), nil
}

func (c *defaultCloud) TryOwn(ctx context.Context, arn string) (bool, error) {
	// For resources that need backwards compatibility - not having managedBy is considered as owned by controller.
	tags, err := c.getTags(ctx, arn)
	if err != nil {
		return false, err
	}
	return c.TryOwnFromTags(ctx, arn, tags)
}

func (c *defaultCloud) TryOwnFromTags(ctx context.Context, arn string, tags services.Tags) (bool, error) {
	// For resources that need backwards compatibility - not having managedBy is considered as owned by controller.
	managedBy := c.GetManagedByFromTags(tags)
	if managedBy == "" {
		err := c.ownResource(ctx, arn)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return c.isOwner(managedBy), nil
}

func (c *defaultCloud) ownResource(ctx context.Context, arn string) error {
	_, err := c.Lattice().TagResource(ctx, &vpclattice.TagResourceInput{
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

// loggingMiddleware logs API call results
type loggingMiddleware struct {
	log gwlog.Logger
}

func (m *loggingMiddleware) ID() string { return "ControllerLogging" }

func (m *loggingMiddleware) HandleInitialize(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (
	out middleware.InitializeOutput, metadata middleware.Metadata, err error,
) {
	out, metadata, err = next.HandleInitialize(ctx, in)

	service := middleware.GetServiceID(ctx)
	operation := middleware.GetOperationName(ctx)

	if err != nil {
		m.log.Debugw(ctx, "error",
			"error", err.Error(),
			"serviceName", service,
			"operation", operation,
		)
	} else {
		m.log.Debugw(ctx, "response",
			"serviceName", service,
			"operation", operation,
		)
	}
	return out, metadata, err
}
