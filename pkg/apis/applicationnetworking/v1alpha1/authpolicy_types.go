package v1alpha1

import (
	apimachineryv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

const (
	IAMAuthPolicyKind = "IAMAuthPolicy"
)

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=gateway-api,shortName=iap
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:subresource:status
type IAMAuthPolicy struct {
	apimachineryv1.TypeMeta   `json:",inline"`
	apimachineryv1.ObjectMeta `json:"metadata,omitempty"`

	Spec IAMAuthPolicySpec `json:"spec"`

	// Status defines the current state of IAMAuthPolicy.
	//
	// +kubebuilder:default={conditions: {{type: "Accepted", status: "Unknown", reason:"NotReconciled", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"}}}
	Status IAMAuthPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// IAMAuthPolicyList contains a list of IAMAuthPolicies.
type IAMAuthPolicyList struct {
	apimachineryv1.TypeMeta `json:",inline"`
	apimachineryv1.ListMeta `json:"metadata,omitempty"`
	Items                   []IAMAuthPolicy `json:"items"`
}

// IAMAuthPolicySpec defines the desired state of IAMAuthPolicy.
// When the controller handles IAMAuthPolicy creation, if the targetRef k8s and VPC Lattice resource exists, the controller will change the auth_type of that VPC Lattice resource to AWS_IAM and attach this policy.
// When the controller handles IAMAuthPolicy deletion, if the targetRef k8s and VPC Lattice resource exists, the controller will change the auth_type of that VPC Lattice resource to NONE and detach this policy.
type IAMAuthPolicySpec struct {

	// IAM auth policy content. It is a JSON string that uses the same syntax as AWS IAM policies. Please check the VPC Lattice documentation to get [the common elements in an auth policy](https://docs.aws.amazon.com/vpc-lattice/latest/ug/auth-policies.html#auth-policies-common-elements)
	Policy string `json:"policy"`

	// TargetRef points to the Kubernetes Gateway, HTTPRoute, or GRPCRoute resource that will have this policy attached.
	//
	// This field is following the guidelines of Kubernetes Gateway API policy attachment.
	TargetRef *v1alpha2.PolicyTargetReference `json:"targetRef"`
}

// IAMAuthPolicyStatus defines the observed state of IAMAuthPolicy.
type IAMAuthPolicyStatus struct {
	// Conditions describe the current conditions of the IAMAuthPolicy.
	//
	// Implementations should prefer to express Policy conditions
	// using the `PolicyConditionType` and `PolicyConditionReason`
	// constants so that operators and tools can converge on a common
	// vocabulary to describe IAMAuthPolicy state.
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
	Conditions []apimachineryv1.Condition `json:"conditions,omitempty"`
}

func (p *IAMAuthPolicy) GetTargetRef() *v1alpha2.PolicyTargetReference {
	return p.Spec.TargetRef
}

func (p *IAMAuthPolicy) GetStatusConditions() []apimachineryv1.Condition {
	return p.Status.Conditions
}

func (p *IAMAuthPolicy) SetStatusConditions(conditions []apimachineryv1.Condition) {
	p.Status.Conditions = conditions
}

func (p *IAMAuthPolicy) GetNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(p)
}

func (pl *IAMAuthPolicyList) GetItems() []core.Policy {
	items := make([]core.Policy, len(pl.Items))
	for i, item := range pl.Items {
		items[i] = &item
	}
	return items
}
