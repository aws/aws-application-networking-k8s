package services

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	taggingapi "github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	taggingapiiface "github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi/resourcegroupstaggingapiiface"
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
	taggingapiiface.ResourceGroupsTaggingAPIAPI

	// Receives a list of arns and returns arn-to-tags map.
	GetTagsForArns(ctx context.Context, arns []string) (map[string]Tags, error)

	// Finds one resource that matches the given set of tags.
	FindResourcesByTags(ctx context.Context, resourceType ResourceType, tags Tags) ([]string, error)
}

type defaultTagging struct {
	taggingapiiface.ResourceGroupsTaggingAPIAPI
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
