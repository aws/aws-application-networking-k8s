package lattice

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

const AccessLogPolicyTagKey = aws.TagBase + "AccessLogPolicy"

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
	SourceType        SourceType
	SourceName        string
	DestinationArn    string
	ALPNamespacedName types.NamespacedName
	EventType         core.EventType
}

type AccessLogSubscriptionStatus struct {
	Arn string `json:"arn"`
}

func NewAccessLogSubscription(
	stack core.Stack,
	spec AccessLogSubscriptionSpec,
	status *AccessLogSubscriptionStatus,
) *AccessLogSubscription {
	id := fmt.Sprintf("%s-%s-%s", spec.SourceType, spec.SourceName, spec.DestinationArn)
	return &AccessLogSubscription{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::VPCServiceNetwork::AccessLogSubscription", id),
		Spec:         spec,
		Status:       status,
	}
}
