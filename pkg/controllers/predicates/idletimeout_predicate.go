package predicates

import (
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
)

// IdleTimeoutSecondsAnnotationChangedPredicate triggers reconciliation on idle-timeout-seconds
// annotation changes. An annotation-only edit does not bump metadata.generation, so without this
// predicate the base RouteChangedPredicate would filter the event out and the new value would not
// apply until the next spec change.
var IdleTimeoutSecondsAnnotationChangedPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		oldAnnotations := e.ObjectOld.GetAnnotations()
		newAnnotations := e.ObjectNew.GetAnnotations()

		oldIdleTimeout := getIdleTimeoutSecondsAnnotation(oldAnnotations)
		newIdleTimeout := getIdleTimeoutSecondsAnnotation(newAnnotations)

		return oldIdleTimeout != newIdleTimeout
	},
	CreateFunc: func(e event.CreateEvent) bool {
		annotations := e.Object.GetAnnotations()
		return getIdleTimeoutSecondsAnnotation(annotations) != ""
	},
}

func getIdleTimeoutSecondsAnnotation(annotations map[string]string) string {
	if annotations == nil {
		return ""
	}
	// Trim to match GetIdleTimeoutSecondsWithValidation, so whitespace-only changes
	// (e.g. "120" -> "120 ") are not treated as a value change.
	return strings.TrimSpace(annotations[k8s.IdleTimeoutSecondsAnnotation])
}
