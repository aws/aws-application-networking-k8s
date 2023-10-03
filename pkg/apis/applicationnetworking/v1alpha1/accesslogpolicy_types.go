package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

const (
	AccessLogPolicyKind = "AccessLogPolicy"
)

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=gateway-api,shortName=tgp
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type AccessLogPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AccessLogPolicySpec `json:"spec"`
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
	// +optional
	DestinationArn *string `json:"protocol,omitempty"`

	// TargetRef points to the kubernetes Service or Gateway resource that will have this policy attached.
	//
	// This field is following the guidelines of Kubernetes Gateway API policy attachment.
	TargetRef *v1alpha2.PolicyTargetReference `json:"targetRef"`
}

func (p *AccessLogPolicy) GetTargetRef() *v1alpha2.PolicyTargetReference {
	return p.Spec.TargetRef
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
