package webhook

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PodReadinessGateConditionType = "application-networking.k8s.aws/pod-readiness-gate"
)

func NewPodReadinessGateInjector(k8sClient client.Client, log gwlog.Logger) *PodReadinessGateInjector {
	return &PodReadinessGateInjector{
		k8sClient: k8sClient,
		log:       log,
	}
}

type PodReadinessGateInjector struct {
	k8sClient client.Client
	log       gwlog.Logger
}

func (m *PodReadinessGateInjector) Mutate(ctx context.Context, pod *corev1.Pod) error {
	pct := corev1.PodConditionType(PodReadinessGateConditionType)
	m.log.Debugf(ctx, "Webhook invoked for pod %s/%s", pod.Name, pod.Namespace)

	found := false
	for _, rg := range pod.Spec.ReadinessGates {
		if rg.ConditionType == pct {
			found = true
			break
		}
	}
	if !found {
		pod.Spec.ReadinessGates = append(pod.Spec.ReadinessGates, corev1.PodReadinessGate{
			ConditionType: pct,
		})
	}
	return nil
}
