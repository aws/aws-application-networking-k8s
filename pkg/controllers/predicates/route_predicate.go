package predicates

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
)

// RouteChangedPredicate implements a predicate function that triggers reconciliation
// when either the generation changes (spec changes) or when standalone annotations change.
// This ensures that annotation-based transitions are properly handled.
type RouteChangedPredicate struct {
	predicate.Funcs
}

// Update implements the predicate interface for update events
func (p RouteChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	// Always reconcile if generation changed (spec changes)
	if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
		return true
	}

	// Check if standalone annotation changed
	oldAnnotations := e.ObjectOld.GetAnnotations()
	newAnnotations := e.ObjectNew.GetAnnotations()

	oldStandalone := getStandaloneAnnotation(oldAnnotations)
	newStandalone := getStandaloneAnnotation(newAnnotations)

	// Reconcile if standalone annotation changed
	return oldStandalone != newStandalone
}

// Create implements the predicate interface for create events
func (p RouteChangedPredicate) Create(e event.CreateEvent) bool {
	// Always reconcile on create
	return true
}

// Delete implements the predicate interface for delete events
func (p RouteChangedPredicate) Delete(e event.DeleteEvent) bool {
	// Always reconcile on delete
	return true
}

// Generic implements the predicate interface for generic events
func (p RouteChangedPredicate) Generic(e event.GenericEvent) bool {
	// Always reconcile on generic events
	return true
}

// getStandaloneAnnotation extracts the standalone annotation value from annotations map
func getStandaloneAnnotation(annotations map[string]string) string {
	if annotations == nil {
		return ""
	}
	return annotations[k8s.StandaloneAnnotation]
}

// NewRouteChangedPredicate creates a new RouteChangedPredicate
func NewRouteChangedPredicate() RouteChangedPredicate {
	return RouteChangedPredicate{}
}
