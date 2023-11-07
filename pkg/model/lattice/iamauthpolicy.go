package lattice

import (
	"fmt"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
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

func NewIAMAuthPolicy(k8sPolicy *anv1alpha1.IAMAuthPolicy) IAMAuthPolicy {
	kind := k8sPolicy.Spec.TargetRef.Kind
	policy := k8sPolicy.Spec.Policy
	switch kind {
	case "Gateway":
		return IAMAuthPolicy{
			Type:   ServiceNetworkType,
			Name:   string(k8sPolicy.Spec.TargetRef.Name),
			Policy: policy,
		}
	case "HTTPRoute", "GRPCRoute":
		return IAMAuthPolicy{
			Type:   ServiceType,
			Name:   utils.LatticeServiceName(string(k8sPolicy.Spec.TargetRef.Name), k8sPolicy.Namespace),
			Policy: policy,
		}
	default:
		panic(fmt.Sprintf("unexpected targetRef, Kind=%s", kind))
	}
}
