package webhook

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func TestReadinessGateInjectionNew(t *testing.T) {
	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)

	k8sClient := testclient.
		NewClientBuilder().
		WithScheme(k8sScheme).
		Build()

	injector := NewPodReadinessGateInjector(k8sClient, gwlog.FallbackLogger)
	m := NewPodMutator(k8sScheme, injector)

	pod := &corev1.Pod{}

	ret, err := m.MutateCreate(context.TODO(), pod)
	newPod := ret.(*corev1.Pod)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(newPod.Spec.ReadinessGates))
	ct := newPod.Spec.ReadinessGates[0].ConditionType
	assert.Equal(t, PodReadinessGateConditionType, string(ct))
}

func TestReadinessGateAlreadyExists(t *testing.T) {
	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)

	k8sClient := testclient.
		NewClientBuilder().
		WithScheme(k8sScheme).
		Build()

	injector := NewPodReadinessGateInjector(k8sClient, gwlog.FallbackLogger)
	m := NewPodMutator(k8sScheme, injector)

	pod := &corev1.Pod{}
	prg := corev1.PodReadinessGate{ConditionType: PodReadinessGateConditionType}
	pod.Spec.ReadinessGates = append(pod.Spec.ReadinessGates, prg)

	ret, err := m.MutateCreate(context.TODO(), pod)
	newPod := ret.(*corev1.Pod)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(newPod.Spec.ReadinessGates))
	ct := newPod.Spec.ReadinessGates[0].ConditionType
	assert.Equal(t, PodReadinessGateConditionType, string(ct))
}

func TestUpdateDoesNothing(t *testing.T) {
	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)

	k8sClient := testclient.
		NewClientBuilder().
		WithScheme(k8sScheme).
		Build()

	injector := NewPodReadinessGateInjector(k8sClient, gwlog.FallbackLogger)
	m := NewPodMutator(k8sScheme, injector)

	p1 := &corev1.Pod{}
	p1.Spec.Hostname = "foo"
	p2 := &corev1.Pod{}
	p2.Spec.Hostname = "bar"

	ret, err := m.MutateUpdate(context.TODO(), p1, p2)
	newPod := ret.(*corev1.Pod)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(newPod.Spec.ReadinessGates))
}
