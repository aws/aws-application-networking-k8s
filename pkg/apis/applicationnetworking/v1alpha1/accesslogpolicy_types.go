package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

const (
	AccessLogPolicyKind                = "AccessLogPolicy"
	AccessLogSubscriptionAnnotationKey = "VpcLatticeAccessLogSubscription"
)

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=gateway-api,shortName=alp
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:subresource:status
type AccessLogPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AccessLogPolicySpec `json:"spec"`

	// Status defines the current state of AccessLogPolicy.
	//
	// +kubebuilder:default={conditions: {{type: "Accepted", status: "Unknown", reason:"NotReconciled", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"}}}
	Status AccessLogPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// AccessLogPolicyList contains a list of AccessLogPolicies.
type AccessLogPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccessLogPolicy `json:"items"`
}

// AccessLogPolicySpec defines the desired state of AccessLogPolicy.
type AccessLogPolicySpec struct {
	// The Amazon Resource Name (ARN) of the destination that will store access logs.
	// Supported values are S3 Bucket, CloudWatch Log Group, and Firehose Delivery Stream ARNs.
	//
	// Changes to this value results in replacement of the VPC Lattice Access Log Subscription.
	// +kubebuilder:validation:Pattern=`^arn(:[a-z0-9]+([.-][a-z0-9]+)*){2}(:([a-z0-9]+([.-][a-z0-9]+)*)?){2}:([^/].*)?`
	DestinationArn *string `json:"destinationArn"`

	// TargetRef points to the Kubernetes Gateway, HTTPRoute, or GRPCRoute resource that will have this policy attached.
	//
	// This field is following the guidelines of Kubernetes Gateway API policy attachment.
	TargetRef *v1alpha2.PolicyTargetReference `json:"targetRef"`
}

// AccessLogPolicyStatus defines the observed state of AccessLogPolicy.
type AccessLogPolicyStatus struct {
	// Conditions describe the current conditions of the AccessLogPolicy.
	//
	// Implementations should prefer to express Policy conditions
	// using the `PolicyConditionType` and `PolicyConditionReason`
	// constants so that operators and tools can converge on a common
	// vocabulary to describe AccessLogPolicy state.
	//
	// Known condition types are:
	//
	// * "Accepted"
	// * "Ready"
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:default={{type: "Accepted", status: "Unknown", reason:"Pending", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"},{type: "Programmed", status: "Unknown", reason:"Pending", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"}}
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (p *AccessLogPolicy) GetTargetRef() *v1alpha2.PolicyTargetReference {
	return p.Spec.TargetRef
}

func (p *AccessLogPolicy) GetStatusConditions() []metav1.Condition {
	return p.Status.Conditions
}

func (p *AccessLogPolicy) SetStatusConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}

func (p *AccessLogPolicy) GetNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(p)
}

func (pl *AccessLogPolicyList) GetItems() []core.Policy {
	items := make([]core.Policy, len(pl.Items))
	for i, item := range pl.Items {
		items[i] = &item
	}
	return items
}
