package controllers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func updateCondition(conditions []metav1.Condition, newCond metav1.Condition) []metav1.Condition {
	newConditions := make([]metav1.Condition, 0)

	found := false
	for _, cond := range conditions {
		if cond.Type == newCond.Type {
			// Update existing condition. Time is kept only if status is unchanged.
			newCond.LastTransitionTime = cond.LastTransitionTime
			if cond.Status != newCond.Status {
				newCond.LastTransitionTime = metav1.Now()
			}
			newConditions = append(newConditions, newCond)
			found = true
		} else {
			newConditions = append(newConditions, cond)
		}
	}

	if !found {
		// Add new condition instead.
		newCond.LastTransitionTime = metav1.Now()
		newConditions = append(newConditions, newCond)
	}

	return newConditions
}
