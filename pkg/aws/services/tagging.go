package services

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	tagtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

//go:generate mockgen -destination tagging_mocks.go -package services github.com/aws/aws-application-networking-k8s/pkg/aws/services Tagging

type ResourceType string

const (
	resourceTypePrefix = "vpc-lattice:"

	ResourceTypeTargetGroup ResourceType = resourceTypePrefix + "targetgroup"
	ResourceTypeService     ResourceType = resourceTypePrefix + "service"

	// https://docs.aws.amazon.com/resourcegroupstagging/latest/APIReference/API_GetResources.html#API_GetResources_RequestSyntax
	maxArnsPerGetResourcesApi = 100
)

type Tagging interface {
	// Receives a list of arns and returns arn-to-tags map.
	GetTagsForArns(ctx context.Context, arns []string) (map[string]Tags, error)

	// Finds one resource that matches the given set of tags.
	FindResourcesByTags(ctx context.Context, resourceType ResourceType, tags Tags) ([]string, error)

	// Updates tags for a given resource ARN
	UpdateTags(ctx context.Context, resourceArn string, additionalTags Tags, awsManagedTags Tags) error
}

type defaultTagging struct {
	client *resourcegroupstaggingapi.Client
}

type latticeTagging struct {
	Lattice
	vpcId string
}

func (t *defaultTagging) GetTagsForArns(ctx context.Context, arns []string) (map[string]Tags, error) {
	chunks := utils.Chunks(arns, maxArnsPerGetResourcesApi)
	result := make(map[string]Tags)

	for _, chunk := range chunks {
		paginator := resourcegroupstaggingapi.NewGetResourcesPaginator(t.client, &resourcegroupstaggingapi.GetResourcesInput{
			ResourceARNList: chunk,
		})
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return nil, err
			}
			for _, r := range page.ResourceTagMappingList {
				result[aws.ToString(r.ResourceARN)] = convertTags(r.Tags)
			}
		}
	}
	return result, nil
}

func (t *defaultTagging) FindResourcesByTags(ctx context.Context, resourceType ResourceType, tags Tags) ([]string, error) {
	var matchingArns []string
	paginator := resourcegroupstaggingapi.NewGetResourcesPaginator(t.client, &resourcegroupstaggingapi.GetResourcesInput{
		TagFilters:          convertTagsToFilter(tags),
		ResourceTypeFilters: []string{string(resourceType)},
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range page.ResourceTagMappingList {
			matchingArns = append(matchingArns, aws.ToString(r.ResourceARN))
		}
	}
	return matchingArns, nil
}

func NewDefaultTagging(cfg aws.Config, region string) *defaultTagging {
	return &defaultTagging{
		client: resourcegroupstaggingapi.NewFromConfig(cfg, func(o *resourcegroupstaggingapi.Options) {
			o.Region = region
		}),
	}
}

// Use VPC Lattice API instead of the Resource Groups Tagging API
func NewLatticeTagging(cfg aws.Config, acc string, region string, vpcId string) *latticeTagging {
	api := NewDefaultLattice(cfg, acc, region)
	return &latticeTagging{Lattice: api, vpcId: vpcId}
}

func (t *latticeTagging) GetTagsForArns(ctx context.Context, arns []string) (map[string]Tags, error) {
	result := map[string]Tags{}
	for _, arn := range arns {
		tags, err := t.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{ResourceArn: aws.String(arn)})
		if err != nil {
			return nil, err
		}
		result[arn] = tags.Tags
	}
	return result, nil
}

func (t *latticeTagging) FindResourcesByTags(ctx context.Context, resourceType ResourceType, tags Tags) ([]string, error) {
	if resourceType != ResourceTypeTargetGroup {
		return nil, fmt.Errorf("unsupported resource type %q for FindResourcesByTags", resourceType)
	}

	tgs, err := t.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{
		VpcIdentifier: aws.String(t.vpcId),
	})
	if err != nil {
		return nil, err
	}

	arns := make([]string, 0, len(tgs))
	for _, tg := range tgs {
		resp, err := t.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{ResourceArn: tg.Arn})
		if err != nil {
			return nil, err
		}
		if containsTags(resp.Tags, tags) {
			arns = append(arns, aws.ToString(tg.Arn))
		}
	}
	return arns, nil
}

func containsTags(source, check Tags) bool {
	for k, v := range check {
		sourceV, ok := source[k]
		if !ok || sourceV != v {
			return false
		}
	}
	return len(check) != 0
}

func convertTags(tags []tagtypes.Tag) Tags {
	out := make(Tags)
	for _, tag := range tags {
		out[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return out
}

func convertTagsToFilter(tags Tags) []tagtypes.TagFilter {
	filters := make([]tagtypes.TagFilter, 0, len(tags))
	for k, v := range tags {
		filters = append(filters, tagtypes.TagFilter{
			Key:    aws.String(k),
			Values: []string{v},
		})
	}
	return filters
}

func (t *defaultTagging) UpdateTags(ctx context.Context, resourceArn string, additionalTags Tags, awsManagedTags Tags) error {
	existingTags, err := t.GetTagsForArns(ctx, []string{resourceArn})
	if err != nil {
		return fmt.Errorf("failed to get existing tags: %w", err)
	}

	currentTags := k8s.GetNonAWSManagedTags(existingTags[resourceArn])
	filteredNewTags := k8s.GetNonAWSManagedTags(additionalTags)

	for key, value := range awsManagedTags {
		if existingValue, exists := existingTags[resourceArn][key]; exists {
			currentTags[key] = existingValue
		}
		filteredNewTags[key] = value
	}

	tagsToAdd, tagsToRemove := k8s.CalculateTagDifference(currentTags, filteredNewTags)

	if len(tagsToRemove) > 0 {
		_, err := t.client.UntagResources(ctx, &resourcegroupstaggingapi.UntagResourcesInput{
			ResourceARNList: []string{resourceArn},
			TagKeys:         tagsToRemove,
		})
		if err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	if len(tagsToAdd) > 0 {
		_, err := t.client.TagResources(ctx, &resourcegroupstaggingapi.TagResourcesInput{
			ResourceARNList: []string{resourceArn},
			Tags:            tagsToAdd,
		})
		if err != nil {
			return fmt.Errorf("failed to add/update tags: %w", err)
		}
	}

	return nil
}

func (t *latticeTagging) UpdateTags(ctx context.Context, resourceArn string, additionalTags Tags, awsManagedTags Tags) error {
	existingTags, err := t.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
		ResourceArn: aws.String(resourceArn),
	})
	if err != nil {
		return fmt.Errorf("failed to get existing tags: %w", err)
	}

	currentTags := k8s.GetNonAWSManagedTags(existingTags.Tags)
	filteredNewTags := k8s.GetNonAWSManagedTags(additionalTags)

	for key, value := range awsManagedTags {
		if existingValue, exists := existingTags.Tags[key]; exists {
			currentTags[key] = existingValue
		}
		filteredNewTags[key] = value
	}

	tagsToAdd, tagsToRemove := k8s.CalculateTagDifference(currentTags, filteredNewTags)

	if len(tagsToRemove) > 0 {
		_, err := t.UntagResource(ctx, &vpclattice.UntagResourceInput{
			ResourceArn: aws.String(resourceArn),
			TagKeys:     tagsToRemove,
		})
		if err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	if len(tagsToAdd) > 0 {
		_, err := t.TagResource(ctx, &vpclattice.TagResourceInput{
			ResourceArn: aws.String(resourceArn),
			Tags:        tagsToAdd,
		})
		if err != nil {
			return fmt.Errorf("failed to add/update tags: %w", err)
		}
	}
	return nil
}
