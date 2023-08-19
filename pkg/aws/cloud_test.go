package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagManagedBy(t *testing.T) {
	cfg := CloudConfig{
		VpcId:     "vpc-id",
		AccountId: "acc-id",
	}
	c := NewDefaultCloud(nil, nil, cfg)

	tags := c.NewTagsWithManagedBy()
	assert.Equal(t, gatewayApiUniqTag(cfg.VpcId), *tags[TagManagedBy])

	isManaged := c.IsTagManagedBy(tags)
	assert.True(t, isManaged)
}
