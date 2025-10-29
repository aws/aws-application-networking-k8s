package services

import (
	"context"
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	taggingapi "github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	taggingapiiface "github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi/resourcegroupstaggingapiiface"
	"github.com/aws/aws-sdk-go/service/vpclattice"
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

type Tags = map[string]*string

type Tagging interface {
	// Receives a list of arns and returns arn-to-tags map.
	GetTagsForArns(ctx context.Context, arns []string) (map[string]Tags, error)

	// Finds one resource that matches the given set of tags.
	FindResourcesByTags(ctx context.Context, resourceType ResourceType, tags Tags) ([]string, error)

	// Updates tags for a given resource ARN
	UpdateTags(ctx context.Context, resourceArn string, additionalTags Tags, awsManagedTags Tags) error
}

type defaultTagging struct {
	taggingapiiface.ResourceGroupsTaggingAPIAPI
}

type latticeTagging struct {
	Lattice
	vpcId string
}

func (t *defaultTagging) GetTagsForArns(ctx context.Context, arns []string) (map[string]Tags, error) {
	chunks := utils.Chunks(utils.SliceMap(arns, aws.String), maxArnsPerGetResourcesApi)
	result := make(map[string]Tags)

	for _, chunk := range chunks {
		input := &taggingapi.GetResourcesInput{
			ResourceARNList: chunk,
		}
		err := t.GetResourcesPagesWithContext(ctx, input, func(page *taggingapi.GetResourcesOutput, lastPage bool) bool {
			for _, r := range page.ResourceTagMappingList {
				result[*r.ResourceARN] = convertTags(r.Tags)
			}
			return true
		})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (t *defaultTagging) FindResourcesByTags(ctx context.Context, resourceType ResourceType, tags Tags) ([]string, error) {
	input := &taggingapi.GetResourcesInput{
		TagFilters:          convertTagsToFilter(tags),
		ResourceTypeFilters: []*string{aws.String(string(resourceType))},
	}
	resp, err := t.GetResourcesWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	matchingArns := utils.SliceMap(resp.ResourceTagMappingList, func(t *taggingapi.ResourceTagMapping) string {
		return aws.StringValue(t.ResourceARN)
	})
	return matchingArns, nil
}

func NewDefaultTagging(sess *session.Session, region string) *defaultTagging {
	api := taggingapi.New(sess, &aws.Config{Region: aws.String(region)})
	return &defaultTagging{ResourceGroupsTaggingAPIAPI: api}
}

// Use VPC Lattice API instead of the Resource Groups Tagging API
func NewLatticeTagging(sess *session.Session, acc string, region string, vpcId string) *latticeTagging {
	api := NewDefaultLattice(sess, acc, region)
	return &latticeTagging{Lattice: api, vpcId: vpcId}
}

func (t *latticeTagging) GetTagsForArns(ctx context.Context, arns []string) (map[string]Tags, error) {
	result := map[string]Tags{}

	for _, arn := range arns {
		tags, err := t.ListTagsForResourceWithContext(ctx,
			&vpclattice.ListTagsForResourceInput{ResourceArn: aws.String(arn)},
		)
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
		resp, err := t.ListTagsForResourceWithContext(ctx,
			&vpclattice.ListTagsForResourceInput{ResourceArn: tg.Arn},
		)
		if err != nil {
			return nil, err
		}

		if containsTags(resp.Tags, tags) {
			arns = append(arns, aws.StringValue(tg.Arn))
		}
	}

	return arns, nil
}

func containsTags(source, check Tags) bool {
	for k, v := range check {
		sourceV, ok := source[k]
		if !ok || aws.StringValue(sourceV) != aws.StringValue(v) {
			return false
		}
	}
	return len(check) != 0
}

func convertTags(tags []*taggingapi.Tag) Tags {
	out := make(Tags)
	for _, tag := range tags {
		out[*tag.Key] = tag.Value
	}
	return out
}

func convertTagsToFilter(tags Tags) []*taggingapi.TagFilter {
	filters := make([]*taggingapi.TagFilter, 0, len(tags))
	for k, v := range tags {
		filters = append(filters, &taggingapi.TagFilter{
			Key:    aws.String(k),
			Values: []*string{v},
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
		_, err := t.UntagResourcesWithContext(ctx, &taggingapi.UntagResourcesInput{
			ResourceARNList: []*string{aws.String(resourceArn)},
			TagKeys:         tagsToRemove,
		})
		if err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	if len(tagsToAdd) > 0 {
		_, err := t.TagResourcesWithContext(ctx, &taggingapi.TagResourcesInput{
			ResourceARNList: []*string{aws.String(resourceArn)},
			Tags:            tagsToAdd,
		})
		if err != nil {
			return fmt.Errorf("failed to add/update tags: %w", err)
		}
	}

	return nil
}

func (t *latticeTagging) UpdateTags(ctx context.Context, resourceArn string, additionalTags Tags, awsManagedTags Tags) error {
	existingTags, err := t.ListTagsForResourceWithContext(ctx, &vpclattice.ListTagsForResourceInput{
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
		_, err := t.UntagResourceWithContext(ctx, &vpclattice.UntagResourceInput{
			ResourceArn: aws.String(resourceArn),
			TagKeys:     tagsToRemove,
		})
		if err != nil {
			return fmt.Errorf("failed to remove tags: %w", err)
		}
	}

	if len(tagsToAdd) > 0 {
		_, err := t.TagResourceWithContext(ctx, &vpclattice.TagResourceInput{
			ResourceArn: aws.String(resourceArn),
			Tags:        tagsToAdd,
		})
		if err != nil {
			return fmt.Errorf("failed to add/update tags: %w", err)
		}
	}
	return nil
}
