package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

const (
	TargetGroupPolicyKind = "TargetGroupPolicy"
)

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=gateway-api,shortName=tgp
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type TargetGroupPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec TargetGroupPolicySpec `json:"spec"`

	Status TargetGroupPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// TargetGroupPolicyList contains a list of TargetGroupPolicies.
type TargetGroupPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TargetGroupPolicy `json:"items"`
}

// TargetGroupPolicySpec defines the desired state of TargetGroupPolicy.
type TargetGroupPolicySpec struct {
	// The protocol to use for routing traffic to the targets. Supported values are HTTP (default) and HTTPS.
	//
	// Changes to this value results in a replacement of VPC Lattice target group.
	// +optional
	Protocol *string `json:"protocol,omitempty"`

	// The protocol version to use. Supported values are HTTP1 (default) and HTTP2. When a policy is behind GRPCRoute,
	// this field value will be ignored as GRPC is only supported through HTTP/2.
	//
	// Changes to this value results in a replacement of VPC Lattice target group.
	// +optional
	ProtocolVersion *string `json:"protocolVersion,omitempty"`

	// TargetRef points to the kubernetes Service resource that will have this policy attached.
	//
	// This field is following the guidelines of Kubernetes Gateway API policy attachment.
	TargetRef *v1alpha2.PolicyTargetReference `json:"targetRef"`

	// The health check configuration.
	//
	// Changes to this value will update VPC Lattice resource in place.
	// +optional
	HealthCheck *HealthCheckConfig `json:"healthCheck,omitempty"`
}

// HealthCheckConfig defines health check configuration for given VPC Lattice target group.
// For the detailed explanation and supported values, please refer to VPC Lattice documentationon health checks.
type HealthCheckConfig struct {
	// Indicates whether health checking is enabled.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// The approximate amount of time, in seconds, between health checks of an individual target.
	// +optional
	// +kubebuilder:validation:Minimum=5
	// +kubebuilder:validation:Maximum=300
	IntervalSeconds *int64 `json:"intervalSeconds,omitempty"`

	// The amount of time, in seconds, to wait before reporting a target as unhealthy.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=120
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`

	// The number of consecutive successful health checks required before considering an unhealthy target healthy.
	// +optional
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=10
	HealthyThresholdCount *int64 `json:"healthyThresholdCount,omitempty"`

	// The number of consecutive failed health checks required before considering a target unhealthy.
	// +optional
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=10
	UnhealthyThresholdCount *int64 `json:"unhealthyThresholdCount,omitempty"`

	// A regular expression to match HTTP status codes when checking for successful response from a target.
	// +optional
	StatusMatch *string `json:"statusMatch,omitempty"`

	// The destination for health checks on the targets.
	// +optional
	Path *string `json:"path,omitempty"`

	// The port used when performing health checks on targets. If not specified, health check defaults to the
	// port that a target receives traffic on.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port *int64 `json:"port,omitempty"`

	// The protocol used when performing health checks on targets.
	// +optional
	Protocol *HealthCheckProtocol `json:"protocol,omitempty"`

	// The protocol version used when performing health checks on targets. Defaults to HTTP/1.
	// +optional
	ProtocolVersion *HealthCheckProtocolVersion `json:"protocolVersion,omitempty"`
}

// TargetGroupPolicyStatus defines the observed state of AccessLogPolicy.
type TargetGroupPolicyStatus struct {
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

// +kubebuilder:validation:Enum=HTTP;HTTPS
type HealthCheckProtocol string

const (
	HealthCheckProtocolHTTP  HealthCheckProtocol = "HTTP"
	HealthCheckProtocolHTTPS HealthCheckProtocol = "HTTPS"
)

// +kubebuilder:validation:Enum=HTTP1;HTTP2
type HealthCheckProtocolVersion string

const (
	HealthCheckProtocolVersionHTTP1 HealthCheckProtocolVersion = "HTTP1"
	HealthCheckProtocolVersionHTTP2 HealthCheckProtocolVersion = "HTTP2"
)

func (p *TargetGroupPolicy) GetTargetRef() *v1alpha2.PolicyTargetReference {
	return p.Spec.TargetRef
}

func (p *TargetGroupPolicy) GetNamespacedName() types.NamespacedName {
	return k8s.NamespacedName(p)
}

func (p *TargetGroupPolicy) GetStatusConditions() []metav1.Condition {
	return p.Status.Conditions
}

func (p *TargetGroupPolicy) SetStatusConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}

func (pl *TargetGroupPolicyList) GetItems() []core.Policy {
	items := make([]core.Policy, len(pl.Items))
	for i, item := range pl.Items {
		items[i] = &item
	}
	return items
}
