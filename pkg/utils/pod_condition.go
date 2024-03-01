package utils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

func PodHasReadinessGate(pod *corev1.Pod, conditionType corev1.PodConditionType) bool {
	if pod == nil {
		return false
	}
	for _, gate := range pod.Spec.ReadinessGates {
		if gate.ConditionType == conditionType {
			return true
		}
	}
	return false
}

// Copied from: k8s.io/apimachinery/pkg/apis/meta
func FindPodStatusCondition(conditions []corev1.PodCondition, conditionType corev1.PodConditionType) *corev1.PodCondition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// Copied from: k8s.io/apimachinery/pkg/apis/meta
func SetPodStatusCondition(conditions *[]corev1.PodCondition, newCondition corev1.PodCondition) {
	if conditions == nil {
		return
	}
	existingCondition := FindPodStatusCondition(*conditions, newCondition.Type)
	if existingCondition == nil {
		if newCondition.LastTransitionTime.IsZero() {
			newCondition.LastTransitionTime = metav1.NewTime(time.Now())
		}
		*conditions = append(*conditions, newCondition)
		return
	}

	if existingCondition.Status != newCondition.Status {
		existingCondition.Status = newCondition.Status
		if !newCondition.LastTransitionTime.IsZero() {
			existingCondition.LastTransitionTime = newCondition.LastTransitionTime
		} else {
			existingCondition.LastTransitionTime = metav1.NewTime(time.Now())
		}
	}

	existingCondition.Reason = newCondition.Reason
	existingCondition.Message = newCondition.Message
}
