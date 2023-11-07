package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

const (
	VpcAssociationPolicyKind = "VpcAssociationPolicy"
)

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=gateway-api,shortName=vap
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:subresource:status
type VpcAssociationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VpcAssociationPolicySpec `json:"spec"`

	Status VpcAssociationPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// VpcAssociationPolicyList contains a list of VpcAssociationPolicies.
type VpcAssociationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VpcAssociationPolicy `json:"items"`
}

// +kubebuilder:validation:MaxLength=32
// +kubebuilder:validation:MinLength=3
// +kubebuilder:validation:Pattern=`^sg-[0-9a-z]+$`
type SecurityGroupId string

// VpcAssociationPolicySpec defines the desired state of VpcAssociationPolicy.
type VpcAssociationPolicySpec struct {

	// SecurityGroupIds defines the security groups enforced on the VpcServiceNetworkAssociation.
	// Security groups does not take effect if AssociateWithVpc is set to false.
	//
	// For more details, please check the VPC Lattice documentation https://docs.aws.amazon.com/vpc-lattice/latest/ug/security-groups.html
	//
	// +optional
	// +kubebuilder:validation:MinItems=1
	SecurityGroupIds []SecurityGroupId `json:"securityGroupIds,omitempty"`

	// AssociateWithVpc indicates whether the VpcServiceNetworkAssociation should be created for the current VPC of k8s cluster.
	//
	// This value will be considered true by default.
	// +optional
	AssociateWithVpc *bool `json:"associateWithVpc,omitempty"`

	// TargetRef points to the kubernetes Gateway resource that will have this policy attached.
	//
	// This field is following the guidelines of Kubernetes Gateway API policy attachment.
	TargetRef *v1alpha2.PolicyTargetReference `json:"targetRef"`
}

// VpcAssociationPolicyStatus defines the observed state of VpcAssociationPolicy.
type VpcAssociationPolicyStatus struct {
	// Conditions describe the current conditions of the VpcAssociationPolicy.
	//
	// Implementations should prefer to express Policy conditions
	// using the `PolicyConditionType` and `PolicyConditionReason`
	// constants so that operators and tools can converge on a common
	// vocabulary to describe VpcAssociationPolicy state.
	//
	// Known condition types are:
	//
	// * "Accepted"
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:default={{type: "Accepted", status: "Unknown", reason:"Pending", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"}}
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (p *VpcAssociationPolicy) GetTargetRef() *v1alpha2.PolicyTargetReference {
	return p.Spec.TargetRef
}

func (p *VpcAssociationPolicy) GetStatusConditions() []metav1.Condition {
	return p.Status.Conditions
}

func (p *VpcAssociationPolicy) SetStatusConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}

func (p *VpcAssociationPolicy) GetNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(p)
}

func (pl *VpcAssociationPolicyList) GetItems() []core.Policy {
	items := make([]core.Policy, len(pl.Items))
	for i, item := range pl.Items {
		items[i] = &item
	}
	return items
}
