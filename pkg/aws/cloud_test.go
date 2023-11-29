package aws

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"context"
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
)

func TestGetManagedByTag(t *testing.T) {

	t.Run("account, cluster name, and vpc", func(t *testing.T) {
		cfg := CloudConfig{
			AccountId:   "acc",
			VpcId:       "vpc",
			ClusterName: "cluster",
		}
		tag := getManagedByTag(cfg)
		assert.Equal(t, "acc/cluster/vpc", tag)
	})

}

func TestDefaultTags(t *testing.T) {
	cfg := CloudConfig{"acc", "vpc", "region", "cluster"}
	c := NewDefaultCloud(nil, cfg)
	tags := c.DefaultTags()
	tagWant := getManagedByTag(cfg)
	tagGot, exits := tags[TagManagedBy]
	assert.True(t, exits)
	assert.Equal(t, tagWant, *tagGot)
}

func TestIsArnManaged(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockLattice := services.NewMockLattice(c)
	cfg := CloudConfig{VpcId: "vpc-id", AccountId: "account-id"}
	cl := NewDefaultCloud(mockLattice, cfg)

	t.Run("arn sent", func(t *testing.T) {
		arn := "arn"
		mockLattice.EXPECT().ListTagsForResourceWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(
				func(_ context.Context, req *vpclattice.ListTagsForResourceInput, _ ...interface{}) (*vpclattice.ListTagsForResourceOutput, error) {
					assert.Equal(t, arn, *req.ResourceArn)
					return &vpclattice.ListTagsForResourceOutput{}, nil
				})
		cl.IsArnManaged(context.Background(), arn)
	})

	t.Run("is managed", func(t *testing.T) {
		arn := "arn"
		mockLattice.EXPECT().ListTagsForResourceWithContext(gomock.Any(), gomock.Any()).
			Return(&vpclattice.ListTagsForResourceOutput{
				Tags: cl.DefaultTags(),
			}, nil)
		managed, err := cl.IsArnManaged(context.Background(), arn)
		assert.Nil(t, err)
		assert.True(t, managed)
	})

	t.Run("not managed", func(t *testing.T) {
		mockLattice.EXPECT().ListTagsForResourceWithContext(gomock.Any(), gomock.Any()).
			Return(&vpclattice.ListTagsForResourceOutput{}, nil)
		managed, err := cl.IsArnManaged(context.Background(), "arn")
		assert.Nil(t, err)
		assert.False(t, managed)
	})

	t.Run("error", func(t *testing.T) {
		mockLattice.EXPECT().ListTagsForResourceWithContext(gomock.Any(), gomock.Any()).
			Return(nil, errors.New(":("))
		managed, err := cl.IsArnManaged(context.Background(), "arn")
		assert.Error(t, err)
		assert.False(t, managed)
	})
}

func Test_DefaultTagsMergedWith(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockLattice := services.NewMockLattice(c)
	cfg := CloudConfig{VpcId: "vpc-id", AccountId: "account-id"}
	cloud := NewDefaultCloud(mockLattice, cfg)

	t.Run("Given non-overlapping tags, returns default tags merged with new tags", func(t *testing.T) {
		input := services.Tags{
			"Key1": aws.String("Value1"),
			"Key2": aws.String("Value2"),
			"Key3": aws.String("Value3"),
		}
		expected := cloud.DefaultTags()
		expected["Key1"] = aws.String("Value1")
		expected["Key2"] = aws.String("Value2")
		expected["Key3"] = aws.String("Value3")
		actual := cloud.DefaultTagsMergedWith(input)
		assert.Equal(t, expected, actual)
	})

	t.Run("Given overlapping tags, returns default tags overwritten by new tags", func(t *testing.T) {
		input := services.Tags{}
		expected := cloud.DefaultTags()
		for k := range expected {
			input[k] = aws.String("TestValue")
			expected[k] = aws.String("TestValue")
		}
		actual := cloud.DefaultTagsMergedWith(input)
		assert.Equal(t, expected, actual)
	})

	t.Run("Given empty tags, returns only default tags", func(t *testing.T) {
		input := services.Tags{}
		expected := cloud.DefaultTags()
		actual := cloud.DefaultTagsMergedWith(input)
		assert.Equal(t, expected, actual)
	})

	t.Run("Given nil tags, returns only default tags", func(t *testing.T) {
		expected := cloud.DefaultTags()
		actual := cloud.DefaultTagsMergedWith(nil)
		assert.Equal(t, expected, actual)
	})
}

func Test_TryOwnFromTags(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockLattice := services.NewMockLattice(c)
	cfg := CloudConfig{VpcId: "vpc-id", AccountId: "account-id", ClusterName: "cluster"}
	cloud := NewDefaultCloud(mockLattice, cfg)

	tcs := []struct {
		name       string
		tags       services.Tags
		owned      bool
		tryAcquire bool
		isErr      bool
	}{
		{
			name:       "no ownership tag acquires ownership",
			tags:       services.Tags{},
			owned:      true,
			tryAcquire: true,
			isErr:      false,
		},
		{
			name:       "proper ownership tag considered valid",
			tags:       cloud.DefaultTags(),
			owned:      true,
			tryAcquire: false,
			isErr:      false,
		},
		{
			name: "improper ownership tag considered invalid",
			tags: services.Tags{
				TagManagedBy: aws.String("not/this/owner"),
			},
			owned:      false,
			tryAcquire: false,
			isErr:      false,
		},
	}

	for i, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			arn := fmt.Sprintf("arn-%d", i)

			tagResourceCallCount := 0
			if tc.tryAcquire {
				tagResourceCallCount = 1
			}
			mockLattice.EXPECT().TagResourceWithContext(gomock.Any(), &vpclattice.TagResourceInput{ResourceArn: aws.String(arn), Tags: cloud.DefaultTags()}).
				Return(&vpclattice.TagResourceOutput{}, nil).Times(tagResourceCallCount)

			res, err := cloud.TryOwnFromTags(context.Background(), arn, tc.tags)

			assert.Equal(t, tc.owned, res)
			if tc.isErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
