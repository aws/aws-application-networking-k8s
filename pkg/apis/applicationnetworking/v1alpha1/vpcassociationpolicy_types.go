package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	VpcAssociationPolicyKind = "VpcAssociationPolicy"

	// VpcAssociationPolicyConditionProgrammed reports whether the VPC association is active and matches
	// the policy. Accepted keeps its Gateway API meaning: the targetRef is valid.
	VpcAssociationPolicyConditionProgrammed gwv1.PolicyConditionType = "Programmed"
	// Programmed=True: the association is active and matches, or AssociateWithVpc is false.
	VpcAssociationPolicyReasonProgrammed gwv1.PolicyConditionReason = "Programmed"
	// Programmed=Unknown: the association is being created, updated or deleted.
	VpcAssociationPolicyReasonPending gwv1.PolicyConditionReason = "Pending"
	// Programmed=False: the association has different private DNS settings and the controller did not create it.
	VpcAssociationPolicyReasonPrivateDnsMismatch gwv1.PolicyConditionReason = "PrivateDnsMismatch"
	// Programmed=False: the last create, update or delete failed.
	VpcAssociationPolicyReasonReconcileFailed gwv1.PolicyConditionReason = "ReconcileFailed"
	// Programmed=Unknown: the policy is not accepted, see the Accepted condition.
	VpcAssociationPolicyReasonNotAccepted gwv1.PolicyConditionReason = "NotAccepted"
)

// +genclient
// +kubebuilder:object:root=true

// +kubebuilder:resource:categories=gateway-api,shortName=vap
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="PrivateDNS",type=boolean,JSONPath=`.spec.privateDnsEnabled`
// +kubebuilder:printcolumn:name="Programmed",type=string,JSONPath=`.status.conditions[?(@.type=="Programmed")].status`
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

// PrivateDnsDomain is a domain name that VPC Lattice resolves privately in the cluster VPC.
// Wildcards such as *.example.com are allowed; the syntax is validated by VPC Lattice.
//
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=255
type PrivateDnsDomain string

// PrivateDnsPreference selects which custom domain names of the service network get private DNS
// in the associated VPC. Values map one-to-one to the VPC Lattice PrivateDnsPreference enum.
//
// +kubebuilder:validation:Enum=VERIFIED_DOMAINS_ONLY;ALL_DOMAINS;VERIFIED_DOMAINS_AND_SPECIFIED_DOMAINS;SPECIFIED_DOMAINS_ONLY
type PrivateDnsPreference string

const (
	// PrivateDnsPreferenceVerifiedDomainsOnly provisions private hosted zones only for custom domain names
	// verified by the resource owner. This is the VPC Lattice default and the value AWS recommends.
	PrivateDnsPreferenceVerifiedDomainsOnly PrivateDnsPreference = "VERIFIED_DOMAINS_ONLY"
	// PrivateDnsPreferenceAllDomains provisions private hosted zones for all custom domain names on the service network.
	PrivateDnsPreferenceAllDomains PrivateDnsPreference = "ALL_DOMAINS"
	// PrivateDnsPreferenceVerifiedDomainsAndSpecifiedDomains provisions private hosted zones for verified custom
	// domain names plus the names listed in privateDnsSpecifiedDomains.
	PrivateDnsPreferenceVerifiedDomainsAndSpecifiedDomains PrivateDnsPreference = "VERIFIED_DOMAINS_AND_SPECIFIED_DOMAINS"
	// PrivateDnsPreferenceSpecifiedDomainsOnly provisions private hosted zones only for the names listed in
	// privateDnsSpecifiedDomains.
	PrivateDnsPreferenceSpecifiedDomainsOnly PrivateDnsPreference = "SPECIFIED_DOMAINS_ONLY"
)

// VpcAssociationDnsOptions defines the private DNS options of the VpcServiceNetworkAssociation.
//
// For the detailed explanation and supported values, please check the VPC Lattice documentation https://docs.aws.amazon.com/vpc-lattice/latest/APIReference/API_DnsOptions.html
//
// +kubebuilder:validation:XValidation:rule="!has(self.privateDnsSpecifiedDomains) || (has(self.privateDnsPreference) && self.privateDnsPreference in ['VERIFIED_DOMAINS_AND_SPECIFIED_DOMAINS', 'SPECIFIED_DOMAINS_ONLY'])",message="privateDnsSpecifiedDomains can only be set when privateDnsPreference is VERIFIED_DOMAINS_AND_SPECIFIED_DOMAINS or SPECIFIED_DOMAINS_ONLY",fieldPath=".privateDnsSpecifiedDomains"
// +kubebuilder:validation:XValidation:rule="!(has(self.privateDnsPreference) && self.privateDnsPreference in ['VERIFIED_DOMAINS_AND_SPECIFIED_DOMAINS', 'SPECIFIED_DOMAINS_ONLY']) || has(self.privateDnsSpecifiedDomains)",message="privateDnsSpecifiedDomains is required when privateDnsPreference is VERIFIED_DOMAINS_AND_SPECIFIED_DOMAINS or SPECIFIED_DOMAINS_ONLY",fieldPath=".privateDnsSpecifiedDomains"
type VpcAssociationDnsOptions struct {
	// PrivateDnsPreference selects which custom domain names get private DNS in the cluster VPC.
	//
	// When unset, VPC Lattice applies its default, VERIFIED_DOMAINS_ONLY, which is also the value AWS
	// recommends, and the controller does not compare the preference of an existing association.
	//
	// +optional
	PrivateDnsPreference *PrivateDnsPreference `json:"privateDnsPreference,omitempty"`

	// PrivateDnsSpecifiedDomains lists custom domain names that get private DNS in addition to
	// (VERIFIED_DOMAINS_AND_SPECIFIED_DOMAINS) or instead of (SPECIFIED_DOMAINS_ONLY) the verified domains.
	// Wildcard names such as "*.example.com" are allowed.
	//
	// It is required for those two preferences and must not be set for the others.
	//
	// +optional
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	PrivateDnsSpecifiedDomains []PrivateDnsDomain `json:"privateDnsSpecifiedDomains,omitempty"`
}

// VpcAssociationPolicySpec defines the desired state of VpcAssociationPolicy.
//
// +kubebuilder:validation:XValidation:rule="!has(self.dnsOptions) || (has(self.privateDnsEnabled) && self.privateDnsEnabled)",message="dnsOptions can only be set when privateDnsEnabled is true",fieldPath=".dnsOptions"
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

	// PrivateDnsEnabled enables private DNS on the VpcServiceNetworkAssociation: VPC Lattice then
	// provisions private hosted zones in the cluster VPC for the custom domain names of resource
	// configurations associated with the service network, as selected by DnsOptions. The service
	// network owner must also enable private DNS on the resource association.
	//
	// When unset, the association is created with private DNS disabled (the VPC Lattice default)
	// and the setting is not compared on an existing association.
	//
	// If an existing association differs, the controller updates it in place. On a RAM-shared service
	// network it only does so for associations it created; for any other association it reports the
	// "Programmed" condition with reason "PrivateDnsMismatch" and leaves the association unchanged.
	//
	// For more details, please check the VPC Lattice documentation https://docs.aws.amazon.com/vpc-lattice/latest/ug/resource-configuration.html#custom-domain-name-resource-consumers
	//
	// +optional
	PrivateDnsEnabled *bool `json:"privateDnsEnabled,omitempty"`

	// DnsOptions selects which custom domain names get private DNS in the cluster VPC.
	// It can only be set when PrivateDnsEnabled is true. Removing it does not reset an existing
	// association to the VPC Lattice defaults: set the desired privateDnsPreference explicitly instead.
	//
	// +optional
	DnsOptions *VpcAssociationDnsOptions `json:"dnsOptions,omitempty"`

	// TargetRef points to the kubernetes Gateway resource that will have this policy attached.
	//
	// This field is following the guidelines of Kubernetes Gateway API policy attachment.
	TargetRef *gwv1alpha2.NamespacedPolicyTargetReference `json:"targetRef"`
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
	// * "Programmed"
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:default={{type: "Accepted", status: "Unknown", reason:"Pending", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"},{type: "Programmed", status: "Unknown", reason:"Pending", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"}}
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (p *VpcAssociationPolicy) GetTargetRef() *gwv1alpha2.NamespacedPolicyTargetReference {
	return p.Spec.TargetRef
}

func (p *VpcAssociationPolicy) GetStatusConditions() *[]metav1.Condition {
	return &p.Status.Conditions
}

func (pl *VpcAssociationPolicyList) GetItems() []*VpcAssociationPolicy {
	return toPtrSlice(pl.Items)
}
