package lattice

import (
	"fmt"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
)

type IAMAuthPolicy struct {
	Type       string
	Name       string
	ResourceId string
	Policy     string
}

type IAMAuthPolicyStatus struct {
	ResourceId string
}

func NewIAMAuthPolicy(k8sPolicy *anv1alpha1.IAMAuthPolicy, name string) IAMAuthPolicy {
	kind := k8sPolicy.Spec.TargetRef.Kind
	policy := k8sPolicy.Spec.Policy
	switch kind {
	case "Gateway":
		return IAMAuthPolicy{
			Type:   ServiceNetworkType,
			Name:   name,
			Policy: policy,
		}
	case "HTTPRoute", "GRPCRoute":
		return IAMAuthPolicy{
			Type:   ServiceType,
			Name:   name,
			Policy: policy,
		}
	default:
		panic(fmt.Sprintf("unexpected targetRef, Kind=%s", kind))
	}
}
