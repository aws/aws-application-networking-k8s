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
type VpcAssociationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VpcAssociationPolicySpec `json:"spec"`
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
	// Both this flag and Gateway annotation "application-networking.k8s.aws/lattice-vpc-association" are reserved tentatively for backward compatibility.
	// Either one of them set to true or both of them undefined will result in the VpcServiceNetworkAssociation created.
	// +optional
	AssociateWithVpc *bool `json:"associateWithVpc,omitempty"`

	// TargetRef points to the kubernetes Gateway resource that will have this policy attached.
	//
	// This field is following the guidelines of Kubernetes Gateway API policy attachment.
	TargetRef *v1alpha2.PolicyTargetReference `json:"targetRef"`
}

func (p *VpcAssociationPolicy) GetTargetRef() *v1alpha2.PolicyTargetReference {
	return p.Spec.TargetRef
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
