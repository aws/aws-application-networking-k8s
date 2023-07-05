package controllers

import (
	"github.com/golang/glog"
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

	glog.V(6).Infof("Conditions update before: %v after: %v found: %d", conditions, newConditions, found)
	if len(conditions) > 0 {
		glog.V(6).Infof("time %d", conditions[0].LastTransitionTime.UnixNano())
		glog.V(6).Infof("time %d", newConditions[0].LastTransitionTime.UnixNano())
	}

	return newConditions
}
