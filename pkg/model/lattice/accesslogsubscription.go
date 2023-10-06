package lattice

import (
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

type SourceType string

const (
	ServiceNetworkSourceType SourceType = "ServiceNetwork"
	ServiceSourceType        SourceType = "Service"
)

type AccessLogSubscription struct {
	core.ResourceMeta `json:"-"`
	Spec              AccessLogSubscriptionSpec    `json:"spec"`
	Status            *AccessLogSubscriptionStatus `json:"status,omitempty"`
}

type AccessLogSubscriptionSpec struct {
	SourceType     SourceType
	SourceName     string
	DestinationArn string
	IsDeleted      bool
}

type AccessLogSubscriptionStatus struct {
	Arn string `json:"arn"`
	Id  string `json:"id"`
}

func NewAccessLogSubscription(
	stack core.Stack,
	sourceType SourceType,
	sourceName string,
	destinationArn string,
	isDeleted bool,
) *AccessLogSubscription {
	id := fmt.Sprintf("%s-%s-%s", sourceType, sourceName, destinationArn)
	als := &AccessLogSubscription{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::VPCServiceNetwork::AccessLogSubscription", id),
		Spec: AccessLogSubscriptionSpec{
			SourceType:     sourceType,
			SourceName:     sourceName,
			DestinationArn: destinationArn,
			IsDeleted:      isDeleted,
		},
		Status: nil,
	}

	stack.AddResource(als)

	return als
}
