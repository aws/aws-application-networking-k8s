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
	EventType      core.EventType
}

type AccessLogSubscriptionStatus struct {
	Arn string `json:"arn"`
}

func NewAccessLogSubscription(
	stack core.Stack,
	sourceType SourceType,
	sourceName string,
	destinationArn string,
	eventType core.EventType,
	status *AccessLogSubscriptionStatus,
) *AccessLogSubscription {
	id := fmt.Sprintf("%s-%s-%s", sourceType, sourceName, destinationArn)
	return &AccessLogSubscription{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::VPCServiceNetwork::AccessLogSubscription", id),
		Spec: AccessLogSubscriptionSpec{
			SourceType:     sourceType,
			SourceName:     sourceName,
			DestinationArn: destinationArn,
			EventType:      eventType,
		},
		Status: status,
	}
}
