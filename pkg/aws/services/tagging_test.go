package services

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestContainsTags(t *testing.T) {
	tests := []struct {
		contains   bool
		findTags   Tags
		sourceTags Tags
		testName   string
	}{
		{
			contains: true,
			findTags: Tags{
				"Key1": aws.String("Value1"),
			},
			sourceTags: Tags{
				"Key1": aws.String("Value1"),
			},
			testName: "exact match with one tag",
		},
		{
			contains: true,
			findTags: Tags{
				"Key1": aws.String("Value1"),
				"Key2": aws.String("Value2"),
			},
			sourceTags: Tags{
				"Key1": aws.String("Value1"),
				"Key2": aws.String("Value2"),
			},
			testName: "exact match with multiple tags",
		},
		{
			contains: true,
			findTags: Tags{
				"Key1": aws.String("Value1"),
			},
			sourceTags: Tags{
				"Key1": aws.String("Value1"),
				"Key2": aws.String("Value2"),
			},
			testName: "match one tag from source's multiple tags",
		},
		{
			contains: true,
			findTags: Tags{
				"Key1": aws.String("Value1"),
				"Key2": aws.String("Value2"),
			},
			sourceTags: Tags{
				"Key3": aws.String("Value3"),
				"Key2": aws.String("Value2"),
				"Key1": aws.String("Value1"),
			},
			testName: "match subset of tags from source's multiple tags",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": aws.String("Value1"),
			},
			sourceTags: Tags{
				"Key2": aws.String("Value2"),
			},
			testName: "no match for tag with different key and value",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": aws.String("Value1"),
			},
			sourceTags: Tags{
				"Key1": aws.String("Value2"),
			},
			testName: "no match for tag with same key, different value",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": aws.String("Value1"),
			},
			sourceTags: Tags{
				"Key1": aws.String("value1"),
			},
			testName: "no match for tag with same key, different value case",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": aws.String("Value1"),
			},
			sourceTags: Tags{
				"key1": aws.String("Value1"),
			},
			testName: "no match for tag with different key case",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": aws.String("Value"),
			},
			sourceTags: Tags{},
			testName:   "no match for tag when source has no tags",
		},
		{
			contains: false,
			findTags: Tags{},
			sourceTags: Tags{
				"Key1": aws.String("Value1"),
				"Key2": aws.String("Value2"),
				"Key3": aws.String("Value3"),
			},
			testName: "no match when searching for empty tags",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": aws.String(""),
			},
			sourceTags: Tags{
				"Key1": aws.String("Value1"),
			},
			testName: "no match for tag with empty check value",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": aws.String(""),
			},
			sourceTags: Tags{},
			testName:   "no match for tag with empty check value when source has no tags",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": aws.String("Value1"),
			},
			sourceTags: Tags{
				"Key1": aws.String(""),
			},
			testName: "no match for tag with empty source value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			assert.Equal(t, containsTags(tt.sourceTags, tt.findTags), tt.contains)
		})
	}
}

func Test_latticeTagging_FindResourcesByTags_nonTargetGroupTypeError(t *testing.T) {
	c := gomock.NewController(t)
	ctx := context.TODO()
	mockLattice := NewMockLattice(c)
	lt := &latticeTagging{Lattice: mockLattice}

	nonTargetGroupResourceType := ResourceTypeService

	_, err := lt.FindResourcesByTags(ctx, nonTargetGroupResourceType, Tags{})
	assert.Error(t, err)
}

func Test_latticeTagging_FindResourcesByTags_ListTagsError(t *testing.T) {
	c := gomock.NewController(t)
	ctx := context.TODO()
	mockLattice := NewMockLattice(c)

	lt := &latticeTagging{
		Lattice: mockLattice,
	}

	inputTags := Tags{
		"Key1": aws.String("Value1"),
	}

	targetGroups := []*vpclattice.TargetGroupSummary{
		{
			Arn:  aws.String("tg-arn-1"),
			Id:   aws.String("tg-id-1"),
			Name: aws.String("tg-name-1"),
		},
	}

	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).
		Return(targetGroups, nil).Times(1)

	mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).
		Return(nil, ErrInternal).Times(len(targetGroups))

	_, err := lt.FindResourcesByTags(ctx, ResourceTypeTargetGroup, inputTags)

	assert.Error(t, err)
}

func Test_latticeTagging_FindResourcesByTags(t *testing.T) {
	tests := []struct {
		ctx          context.Context
		resourceType ResourceType
		inputTags    Tags
		testName     string
		targetGroups []*vpclattice.TargetGroupSummary
		tgTags       map[string]Tags
		want         []string
	}{
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeService,
			inputTags: Tags{
				"Key1": aws.String("Value1"),
			},
			testName:     "no results for ListTargetGroups returns empty list",
			targetGroups: []*vpclattice.TargetGroupSummary{},
			tgTags:       map[string]Tags{},
			want:         []string{},
		},
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeTargetGroup,
			inputTags: Tags{
				"Key1": aws.String("Value1"),
			},
			testName: "no resources found if target groups do not have tags",
			targetGroups: []*vpclattice.TargetGroupSummary{
				{
					Arn:  aws.String("tg-arn-1"),
					Id:   aws.String("tg-id-1"),
					Name: aws.String("tg-name-1"),
				},
				{
					Arn:  aws.String("tg-arn-2"),
					Id:   aws.String("tg-id-2"),
					Name: aws.String("tg-name-2"),
				},
			},
			tgTags: map[string]Tags{
				"tg-arn-1": {},
				"tg-arn-2": {},
			},
			want: []string{},
		},
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeTargetGroup,
			inputTags: Tags{
				"Key1": aws.String("Value1"),
			},
			testName: "one item with tags is found",
			targetGroups: []*vpclattice.TargetGroupSummary{
				{
					Arn:  aws.String("tg-arn-1"),
					Id:   aws.String("tg-id-1"),
					Name: aws.String("tg-name-1"),
				},
				{
					Arn:  aws.String("tg-arn-2"),
					Id:   aws.String("tg-id-2"),
					Name: aws.String("tg-name-2"),
				},
			},
			tgTags: map[string]Tags{
				"tg-arn-1": {
					"Key1": aws.String("Value1"),
				},
				"tg-arn-2": {
					"Key2": aws.String("Value2"),
				},
			},
			want: []string{"tg-arn-1"},
		},
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeTargetGroup,
			inputTags: Tags{
				"Key1": aws.String("Value1"),
			},
			testName: "multiple items with tags are found",
			targetGroups: []*vpclattice.TargetGroupSummary{
				{
					Arn:  aws.String("tg-arn-1"),
					Id:   aws.String("tg-id-1"),
					Name: aws.String("tg-name-1"),
				},
				{
					Arn:  aws.String("tg-arn-2"),
					Id:   aws.String("tg-id-2"),
					Name: aws.String("tg-name-2"),
				},
				{
					Arn:  aws.String("tg-arn-3"),
					Id:   aws.String("tg-id-3"),
					Name: aws.String("tg-name-3"),
				},
			},
			tgTags: map[string]Tags{
				"tg-arn-1": {
					"Key1": aws.String("Value1"),
				},
				"tg-arn-2": {
					"Key1": aws.String("Value1"),
				},
				"tg-arn-3": {
					"Key2": aws.String("Value2"),
				},
			},
			want: []string{"tg-arn-1", "tg-arn-2"},
		},
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeTargetGroup,
			inputTags: Tags{
				"Key1": aws.String("Value1"),
			},
			testName: "no resources found if no target groups have matching tags",
			targetGroups: []*vpclattice.TargetGroupSummary{
				{
					Arn:  aws.String("tg-arn-1"),
					Id:   aws.String("tg-id-1"),
					Name: aws.String("tg-name-1"),
				},
				{
					Arn:  aws.String("tg-arn-2"),
					Id:   aws.String("tg-id-2"),
					Name: aws.String("tg-name-2"),
				},
			},
			tgTags: map[string]Tags{
				"tg-arn-1": {
					"Key2": aws.String("Value2"),
				},
				"tg-arn-2": {
					"Key2": aws.String("Value2"),
				},
			},
			want: []string{},
		},
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeTargetGroup,
			inputTags: Tags{
				"Key1": aws.String("Value1"),
			},
			testName: "no resources found with tags case mismatch",
			targetGroups: []*vpclattice.TargetGroupSummary{
				{
					Arn:  aws.String("tg-arn-1"),
					Id:   aws.String("tg-id-1"),
					Name: aws.String("tg-name-1"),
				},
				{
					Arn:  aws.String("tg-arn-2"),
					Id:   aws.String("tg-id-2"),
					Name: aws.String("tg-name-2"),
				},
				{
					Arn:  aws.String("tg-arn-3"),
					Id:   aws.String("tg-id-3"),
					Name: aws.String("tg-name-3"),
				},
			},
			tgTags: map[string]Tags{
				"tg-arn-1": {
					"Key1": aws.String("VALUE1"),
				},
				"tg-arn-2": {
					"Key1": aws.String("value1"),
				},
				"tg-arn-3": {
					"key1": aws.String("Value1"),
				},
			},
			want: []string{},
		},
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeTargetGroup,
			inputTags: Tags{
				"Key1": aws.String("Value1"),
				"Key2": aws.String("Value2"),
			},
			testName: "no resources found when only a subset of tags match",
			targetGroups: []*vpclattice.TargetGroupSummary{
				{
					Arn:  aws.String("tg-arn-1"),
					Id:   aws.String("tg-id-1"),
					Name: aws.String("tg-name-1"),
				},
			},
			tgTags: map[string]Tags{
				"tg-arn-1": {
					"Key1": aws.String("Value1"),
				},
			},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			c := gomock.NewController(t)
			mockLattice := NewMockLattice(c)

			lt := &latticeTagging{
				Lattice: mockLattice,
			}

			mockLattice.EXPECT().ListTargetGroupsAsList(tt.ctx, gomock.Any()).
				Return(tt.targetGroups, nil).Times(1)

			mockLattice.EXPECT().ListTagsForResourceWithContext(tt.ctx, gomock.Any()).DoAndReturn(
				func(ctx aws.Context, input *vpclattice.ListTagsForResourceInput, opts ...request.Option) (*vpclattice.ListTagsForResourceOutput, error) {
					return &vpclattice.ListTagsForResourceOutput{Tags: tt.tgTags[aws.StringValue(input.ResourceArn)]}, nil
				}).Times(len(tt.targetGroups))

			got, err := lt.FindResourcesByTags(tt.ctx, ResourceTypeTargetGroup, tt.inputTags)

			assert.Nil(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
