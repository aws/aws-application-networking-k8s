package test

import (
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type TargetGroupPolicyConfig struct {
	PolicyName      string
	Protocol        *string
	ProtocolVersion *string
	HealthCheck     *anv1alpha1.HealthCheckConfig
}

func (env *Framework) CreateTargetGroupPolicy(
	service *corev1.Service,
	config *TargetGroupPolicyConfig,
) *anv1alpha1.TargetGroupPolicy {
	return &anv1alpha1.TargetGroupPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind: "TargetGroupPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: service.Namespace,
			Name:      config.PolicyName,
		},
		Spec: anv1alpha1.TargetGroupPolicySpec{
			TargetRef: &gwv1alpha2.PolicyTargetReference{
				Kind: gwv1beta1.Kind("Service"),
				Name: gwv1beta1.ObjectName(service.Name),
			},
			Protocol:        config.Protocol,
			ProtocolVersion: config.ProtocolVersion,
			HealthCheck:     config.HealthCheck,
		},
	}
}

func (env *Framework) UpdateTargetGroupPolicy(
	policy *anv1alpha1.TargetGroupPolicy,
	config *TargetGroupPolicyConfig,
) *anv1alpha1.TargetGroupPolicy {
	if config.Protocol != nil {
		policy.Spec.Protocol = config.Protocol
	}

	if config.ProtocolVersion != nil {
		policy.Spec.ProtocolVersion = config.ProtocolVersion
	}

	if config.HealthCheck != nil {
		policy.Spec.HealthCheck = config.HealthCheck
	}

	return policy
}
