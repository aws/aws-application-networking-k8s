package aws

import (
	"errors"
	"testing"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
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

	lat := services.NewMockLattice(c)
	cfg := CloudConfig{VpcId: "vpc-id", AccountId: "account-id"}
	cl := NewDefaultCloud(lat, cfg)

	t.Run("arn sent", func(t *testing.T) {
		arn := "arn"
		lat.EXPECT().ListTagsForResource(gomock.Any()).
			DoAndReturn(
				func(req *vpclattice.ListTagsForResourceInput) (*vpclattice.ListTagsForResourceOutput, error) {
					assert.Equal(t, arn, *req.ResourceArn)
					return &vpclattice.ListTagsForResourceOutput{}, nil
				})
		cl.IsArnManaged(arn)
	})

	t.Run("is managed", func(t *testing.T) {
		arn := "arn"
		lat.EXPECT().ListTagsForResource(gomock.Any()).
			Return(&vpclattice.ListTagsForResourceOutput{
				Tags: cl.DefaultTags(),
			}, nil)
		managed, err := cl.IsArnManaged(arn)
		assert.Nil(t, err)
		assert.True(t, managed)
	})

	t.Run("not managed", func(t *testing.T) {
		lat.EXPECT().ListTagsForResource(gomock.Any()).
			Return(&vpclattice.ListTagsForResourceOutput{}, nil)
		managed, err := cl.IsArnManaged("arn")
		assert.Nil(t, err)
		assert.False(t, managed)
	})

	t.Run("error", func(t *testing.T) {
		lat.EXPECT().ListTagsForResource(gomock.Any()).
			Return(nil, errors.New(":("))
		_, err := cl.IsArnManaged("arn")
		assert.NotNil(t, err)
	})
}
