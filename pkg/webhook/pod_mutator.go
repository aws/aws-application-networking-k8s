package webhook

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/webhook/core"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	apiPathMutatePod = "/mutate-pod"
)

func NewPodMutator(scheme *runtime.Scheme, podReadinessGateInjector *PodReadinessGateInjector) *podMutator {
	return &podMutator{
		podReadinessGateInjector: podReadinessGateInjector,
		scheme:                   scheme,
	}
}

var _ core.Mutator = &podMutator{}

type podMutator struct {
	podReadinessGateInjector *PodReadinessGateInjector
	scheme                   *runtime.Scheme
}

func (m *podMutator) Prototype(_ admission.Request) (runtime.Object, error) {
	return &corev1.Pod{}, nil
}

func (m *podMutator) MutateCreate(ctx context.Context, obj runtime.Object) (runtime.Object, error) {
	pod := obj.(*corev1.Pod)
	if err := m.podReadinessGateInjector.Mutate(ctx, pod); err != nil {
		return pod, err
	}
	return pod, nil
}

func (m *podMutator) MutateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error) {
	return obj, nil
}

func (m *podMutator) SetupWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(apiPathMutatePod, core.MutatingWebhookForMutator(m.scheme, m))
}
