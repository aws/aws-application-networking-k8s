package lattice

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func Test_SynthesizeTargets(t *testing.T) {

	targetList := []model.Target{
		{
			TargetIP: "10.10.1.1",
			Port:     8675,
		},
		{
			TargetIP: "10.10.1.1",
			Port:     309,
		},
		{
			TargetIP: "10.10.1.2",
			Port:     8675,
		},
		{
			TargetIP: "10.10.1.2",
			Port:     309,
		},
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockTargetsManager := NewMockTargetsManager(c)

	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})

	modelTg := model.TargetGroup{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "tg-stack-id"),
		Status: &model.TargetGroupStatus{
			Name: "tg-name",
			Arn:  "tg-arn",
			Id:   "tg-id",
		},
	}
	assert.NoError(t, stack.AddResource(&modelTg))

	targetsSpec := model.TargetsSpec{
		StackTargetGroupId: modelTg.ID(),
		TargetList:         targetList,
	}
	model.NewTargets(stack, targetsSpec)

	mockTargetsManager.EXPECT().Update(ctx, gomock.Any(), gomock.Any()).Return(nil)

	synthesizer := NewTargetsSynthesizer(gwlog.FallbackLogger, nil, mockTargetsManager, stack)
	err := synthesizer.Synthesize(ctx)
	assert.Nil(t, err)
}

func Test_PostSynthesize_Conditions(t *testing.T) {

	newPod := func(namespace, name string, hasGate bool, ready bool) *corev1.Pod {
		var readinessGates []corev1.PodReadinessGate
		if hasGate {
			readinessGates = append(readinessGates, corev1.PodReadinessGate{
				ConditionType: LatticeReadinessGateConditionType,
			})
		}
		condition := corev1.PodCondition{
			Type:   LatticeReadinessGateConditionType,
			Status: corev1.ConditionFalse,
			Reason: ReadinessReasonUnhealthy,
		}
		if ready {
			condition = corev1.PodCondition{
				Type:   LatticeReadinessGateConditionType,
				Status: corev1.ConditionTrue,
				Reason: ReadinessReasonHealthy,
			}
		}

		return &corev1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind: "Pod",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
			},
			Spec: corev1.PodSpec{
				ReadinessGates: readinessGates,
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{condition},
			},
		}
	}
	newLatticeTarget := func(ip string, port int64, status string) *vpclattice.TargetSummary {
		return &vpclattice.TargetSummary{
			Id:     aws.String(ip),
			Port:   aws.Int64(port),
			Status: aws.String(status),
		}
	}

	target := model.Target{
		TargetIP:  "10.10.1.1",
		Port:      8675,
		Ready:     false,
		TargetRef: types.NamespacedName{Namespace: "ns", Name: "pod1"},
	}

	tests := []struct {
		name           string
		model          model.Target
		lattice        *vpclattice.TargetSummary
		pod            *corev1.Pod
		expectedStatus corev1.ConditionStatus
		expectedReason string
		requeue        bool
	}{
		{
			name:           "Healthy targets make pod ready",
			model:          target,
			lattice:        newLatticeTarget("10.10.1.1", 8675, vpclattice.TargetStatusHealthy),
			pod:            newPod("ns", "pod1", true, false),
			expectedStatus: corev1.ConditionTrue,
			expectedReason: ReadinessReasonHealthy,
			requeue:        false,
		},
		{
			name:           "Unavailable targets make pod ready",
			model:          target,
			lattice:        newLatticeTarget("10.10.1.1", 8675, vpclattice.TargetStatusUnavailable),
			pod:            newPod("ns", "pod1", true, false),
			expectedStatus: corev1.ConditionTrue,
			expectedReason: ReadinessReasonHealthCheckUnavailable,
			requeue:        false,
		},
		{
			name:           "Initial targets do not make pod ready",
			model:          target,
			lattice:        newLatticeTarget("10.10.1.1", 8675, vpclattice.TargetStatusInitial),
			pod:            newPod("ns", "pod1", true, false),
			expectedStatus: corev1.ConditionFalse,
			expectedReason: ReadinessReasonInitial,
			requeue:        true,
		},
		{
			name:           "Unhealthy targets do not make pod ready",
			model:          target,
			lattice:        newLatticeTarget("10.10.1.1", 8675, vpclattice.TargetStatusUnhealthy),
			pod:            newPod("ns", "pod1", true, false),
			expectedStatus: corev1.ConditionFalse,
			expectedReason: ReadinessReasonUnhealthy,
			requeue:        true,
		},
		{
			name:           "Draining(unhealthy) targets do not make pod ready",
			model:          target,
			lattice:        newLatticeTarget("10.10.1.1", 8675, vpclattice.TargetStatusDraining),
			pod:            newPod("ns", "pod1", true, false),
			expectedStatus: corev1.ConditionFalse,
			expectedReason: ReadinessReasonUnhealthy,
			requeue:        true,
		},
		{
			name:           "Requeues if target not found",
			model:          target,
			lattice:        newLatticeTarget("dummy", 8675, vpclattice.TargetStatusHealthy),
			pod:            newPod("ns", "pod1", true, false),
			expectedStatus: corev1.ConditionFalse,
			expectedReason: ReadinessReasonTargetNotFound,
			requeue:        true,
		},
		{
			name:           "Pod without gate does not change condition",
			model:          target,
			lattice:        newLatticeTarget("10.10.1.1", 8675, vpclattice.TargetStatusHealthy),
			pod:            newPod("ns", "pod1", false, false),
			expectedStatus: corev1.ConditionFalse,
			expectedReason: ReadinessReasonUnhealthy,
			requeue:        false,
		},
		{
			name:           "Ready pods keep condition (even if target is unhealthy)",
			model:          target,
			lattice:        newLatticeTarget("10.10.1.1", 8675, vpclattice.TargetStatusUnhealthy),
			pod:            newPod("ns", "pod1", true, true),
			expectedStatus: corev1.ConditionTrue,
			expectedReason: ReadinessReasonHealthy,
			requeue:        false,
		},
		{
			name:           "Unused pods keep condition",
			model:          target,
			lattice:        newLatticeTarget("10.10.1.1", 8675, vpclattice.TargetStatusUnused),
			pod:            newPod("ns", "pod1", true, false),
			expectedStatus: corev1.ConditionFalse,
			expectedReason: ReadinessReasonUnused,
			requeue:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			mockTargetsManager := NewMockTargetsManager(c)

			stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})

			modelTg := model.TargetGroup{
				ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "tg-stack-id"),
				Status: &model.TargetGroupStatus{
					Name: "tg-name",
					Arn:  "tg-arn",
					Id:   "tg-id",
				},
			}
			assert.NoError(t, stack.AddResource(&modelTg))

			targetsSpec := model.TargetsSpec{
				StackTargetGroupId: modelTg.ID(),
				TargetList:         []model.Target{tt.model},
			}
			model.NewTargets(stack, targetsSpec)

			mockTargetsManager.EXPECT().List(ctx, gomock.Any()).Return([]*vpclattice.TargetSummary{tt.lattice}, nil)

			k8sClient := testclient.NewClientBuilder().Build()
			assert.NoError(t, k8sClient.Create(ctx, tt.pod))

			synthesizer := NewTargetsSynthesizer(gwlog.FallbackLogger, k8sClient, mockTargetsManager, stack)
			err := synthesizer.PostSynthesize(ctx)

			if tt.requeue {
				assert.ErrorAs(t, err, &RetryErr)
			} else {
				assert.Nil(t, err)
			}

			pod := &corev1.Pod{}
			k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns", Name: "pod1"}, pod)
			cond := utils.FindPodStatusCondition(pod.Status.Conditions, LatticeReadinessGateConditionType)
			assert.NotNil(t, cond)

			assert.Equal(t, tt.expectedReason, cond.Reason)
			assert.Equal(t, tt.expectedStatus, cond.Status)
		})
	}
}
