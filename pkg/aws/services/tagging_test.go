package services

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	gomock "go.uber.org/mock/gomock"
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
				"Key1": "Value1",
			},
			sourceTags: Tags{
				"Key1": "Value1",
			},
			testName: "exact match with one tag",
		},
		{
			contains: true,
			findTags: Tags{
				"Key1": "Value1",
				"Key2": "Value2",
			},
			sourceTags: Tags{
				"Key1": "Value1",
				"Key2": "Value2",
			},
			testName: "exact match with multiple tags",
		},
		{
			contains: true,
			findTags: Tags{
				"Key1": "Value1",
			},
			sourceTags: Tags{
				"Key1": "Value1",
				"Key2": "Value2",
			},
			testName: "match one tag from source's multiple tags",
		},
		{
			contains: true,
			findTags: Tags{
				"Key1": "Value1",
				"Key2": "Value2",
			},
			sourceTags: Tags{
				"Key3": "Value3",
				"Key2": "Value2",
				"Key1": "Value1",
			},
			testName: "match subset of tags from source's multiple tags",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": "Value1",
			},
			sourceTags: Tags{
				"Key2": "Value2",
			},
			testName: "no match for tag with different key and value",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": "Value1",
			},
			sourceTags: Tags{
				"Key1": "Value2",
			},
			testName: "no match for tag with same key, different value",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": "Value1",
			},
			sourceTags: Tags{
				"Key1": "value1",
			},
			testName: "no match for tag with same key, different value case",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": "Value1",
			},
			sourceTags: Tags{
				"key1": "Value1",
			},
			testName: "no match for tag with different key case",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": "Value",
			},
			sourceTags: Tags{},
			testName:   "no match for tag when source has no tags",
		},
		{
			contains: false,
			findTags: Tags{},
			sourceTags: Tags{
				"Key1": "Value1",
				"Key2": "Value2",
				"Key3": "Value3",
			},
			testName: "no match when searching for empty tags",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": "",
			},
			sourceTags: Tags{
				"Key1": "Value1",
			},
			testName: "no match for tag with empty check value",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": "",
			},
			sourceTags: Tags{},
			testName:   "no match for tag with empty check value when source has no tags",
		},
		{
			contains: false,
			findTags: Tags{
				"Key1": "Value1",
			},
			sourceTags: Tags{
				"Key1": "",
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
		"Key1": "Value1",
	}

	targetGroups := []types.TargetGroupSummary{
		{
			Arn:  aws.String("tg-arn-1"),
			Id:   aws.String("tg-id-1"),
			Name: aws.String("tg-name-1"),
		},
	}

	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).
		Return(targetGroups, nil).Times(1)

	mockLattice.EXPECT().ListTagsForResource(ctx, gomock.Any()).
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
		targetGroups []types.TargetGroupSummary
		tgTags       map[string]Tags
		want         []string
	}{
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeService,
			inputTags: Tags{
				"Key1": "Value1",
			},
			testName:     "no results for ListTargetGroups returns empty list",
			targetGroups: []types.TargetGroupSummary{},
			tgTags:       map[string]Tags{},
			want:         []string{},
		},
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeTargetGroup,
			inputTags: Tags{
				"Key1": "Value1",
			},
			testName: "no resources found if target groups do not have tags",
			targetGroups: []types.TargetGroupSummary{
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
				"Key1": "Value1",
			},
			testName: "one item with tags is found",
			targetGroups: []types.TargetGroupSummary{
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
					"Key1": "Value1",
				},
				"tg-arn-2": {
					"Key2": "Value2",
				},
			},
			want: []string{"tg-arn-1"},
		},
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeTargetGroup,
			inputTags: Tags{
				"Key1": "Value1",
			},
			testName: "multiple items with tags are found",
			targetGroups: []types.TargetGroupSummary{
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
					"Key1": "Value1",
				},
				"tg-arn-2": {
					"Key1": "Value1",
				},
				"tg-arn-3": {
					"Key2": "Value2",
				},
			},
			want: []string{"tg-arn-1", "tg-arn-2"},
		},
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeTargetGroup,
			inputTags: Tags{
				"Key1": "Value1",
			},
			testName: "no resources found if no target groups have matching tags",
			targetGroups: []types.TargetGroupSummary{
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
					"Key2": "Value2",
				},
				"tg-arn-2": {
					"Key2": "Value2",
				},
			},
			want: []string{},
		},
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeTargetGroup,
			inputTags: Tags{
				"Key1": "Value1",
			},
			testName: "no resources found with tags case mismatch",
			targetGroups: []types.TargetGroupSummary{
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
					"Key1": "VALUE1",
				},
				"tg-arn-2": {
					"Key1": "value1",
				},
				"tg-arn-3": {
					"key1": "Value1",
				},
			},
			want: []string{},
		},
		{
			ctx:          context.TODO(),
			resourceType: ResourceTypeTargetGroup,
			inputTags: Tags{
				"Key1": "Value1",
				"Key2": "Value2",
			},
			testName: "no resources found when only a subset of tags match",
			targetGroups: []types.TargetGroupSummary{
				{
					Arn:  aws.String("tg-arn-1"),
					Id:   aws.String("tg-id-1"),
					Name: aws.String("tg-name-1"),
				},
			},
			tgTags: map[string]Tags{
				"tg-arn-1": {
					"Key1": "Value1",
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

			mockLattice.EXPECT().ListTagsForResource(tt.ctx, gomock.Any()).DoAndReturn(
				func(ctx context.Context, input *vpclattice.ListTagsForResourceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.ListTagsForResourceOutput, error) {
					return &vpclattice.ListTagsForResourceOutput{Tags: tt.tgTags[aws.ToString(input.ResourceArn)]}, nil
				}).Times(len(tt.targetGroups))

			got, err := lt.FindResourcesByTags(tt.ctx, ResourceTypeTargetGroup, tt.inputTags)

			assert.Nil(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLatticeTagging_UpdateTags(t *testing.T) {
	ctx := context.TODO()
	tests := []struct {
		name               string
		resourceArn        string
		existingTags       Tags
		additionalTags     Tags
		awsManagedTags     Tags
		expectedTagCalls   int
		expectedUntagCalls int
		expectError        bool
		description        string
	}{
		{
			name:        "nil additional tags removes all existing additional tags",
			resourceArn: "arn:aws:vpc-lattice:us-west-2:123456789:service/svc-123",
			existingTags: Tags{
				"Environment": "Dev",
				"Project":     "MyApp",
				"application-networking.k8s.aws/ManagedBy": "123456789/cluster/vpc-123",
			},
			additionalTags:     nil,
			awsManagedTags:     nil,
			expectedTagCalls:   0,
			expectedUntagCalls: 1,
			expectError:        false,
			description:        "should remove all additional tags when additionalTags is nil",
		},
		{
			name:        "add new additional tags when no existing additional tags",
			resourceArn: "arn:aws:vpc-lattice:us-west-2:123456789:service/svc-123",
			existingTags: Tags{
				"application-networking.k8s.aws/ManagedBy": "123456789/cluster/vpc-123",
			},
			additionalTags: Tags{
				"Environment": "Dev",
				"Project":     "MyApp",
			},
			awsManagedTags:     nil,
			expectedTagCalls:   1,
			expectedUntagCalls: 0,
			expectError:        false,
			description:        "should add new additional tags when no existing additional tags",
		},
		{
			name:        "update AWS managed tags only",
			resourceArn: "arn:aws:vpc-lattice:us-west-2:123456789:service/svc-123",
			existingTags: Tags{
				"Environment": "Dev",
				"application-networking.k8s.aws/ManagedBy": "old-cluster/old-vpc",
			},
			additionalTags: Tags{
				"Environment": "Dev",
			},
			awsManagedTags: Tags{
				"application-networking.k8s.aws/ManagedBy": "new-cluster/new-vpc",
			},
			expectedTagCalls:   1,
			expectedUntagCalls: 0,
			expectError:        false,
			description:        "should update AWS managed tags when provided",
		},
		{
			name:        "update both additional and AWS managed tags",
			resourceArn: "arn:aws:vpc-lattice:us-west-2:123456789:service/svc-123",
			existingTags: Tags{
				"Environment": "Dev",
				"Project":     "OldApp",
				"application-networking.k8s.aws/ManagedBy": "old-cluster/old-vpc",
			},
			additionalTags: Tags{
				"Environment": "Prod",
				"Project":     "NewApp",
			},
			awsManagedTags: Tags{
				"application-networking.k8s.aws/ManagedBy": "new-cluster/new-vpc",
			},
			expectedTagCalls:   1,
			expectedUntagCalls: 0,
			expectError:        false,
			description:        "should update both additional and AWS managed tags",
		},
		{
			name:        "no changes needed with AWS managed tags",
			resourceArn: "arn:aws:vpc-lattice:us-west-2:123456789:service/svc-123",
			existingTags: Tags{
				"Environment": "Dev",
				"Project":     "MyApp",
				"application-networking.k8s.aws/ManagedBy": "123456789/cluster/vpc-123",
			},
			additionalTags: Tags{
				"Environment": "Dev",
				"Project":     "MyApp",
			},
			awsManagedTags: Tags{
				"application-networking.k8s.aws/ManagedBy": "123456789/cluster/vpc-123",
			},
			expectedTagCalls:   0,
			expectedUntagCalls: 0,
			expectError:        false,
			description:        "should not make API calls when no changes needed",
		},
		{
			name:         "filters out AWS managed tags from additional tags",
			resourceArn:  "arn:aws:vpc-lattice:us-west-2:123456789:service/svc-123",
			existingTags: Tags{},
			additionalTags: Tags{
				"application-networking.k8s.aws/ManagedBy": "test-override",
				"application-networking.k8s.aws/RouteType": "http",
				"Environment": "Dev",
			},
			awsManagedTags: Tags{
				"application-networking.k8s.aws/ManagedBy": "correct-value",
			},
			expectedTagCalls:   1,
			expectedUntagCalls: 0,
			expectError:        false,
			description:        "should filter out AWS managed tags from additional tags but include them from awsManagedTags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			mockLattice := NewMockLattice(c)

			lt := &latticeTagging{
				Lattice: mockLattice,
			}

			mockLattice.EXPECT().ListTagsForResource(ctx, gomock.Any()).
				Return(&vpclattice.ListTagsForResourceOutput{Tags: tt.existingTags}, nil).Times(1)

			if tt.expectedUntagCalls > 0 {
				mockLattice.EXPECT().UntagResource(ctx, gomock.Any()).
					Return(nil, nil).Times(tt.expectedUntagCalls)
			}

			if tt.expectedTagCalls > 0 {
				mockLattice.EXPECT().TagResource(ctx, gomock.Any()).
					Return(nil, nil).Times(tt.expectedTagCalls)
			}

			err := lt.UpdateTags(ctx, tt.resourceArn, tt.additionalTags, tt.awsManagedTags)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}
