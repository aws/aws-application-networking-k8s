package predicates

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
)

var AllowTakeoverFromAnnotationChangedPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		oldAnnotations := e.ObjectOld.GetAnnotations()
		newAnnotations := e.ObjectNew.GetAnnotations()

		oldAllowTakeoverFromAnnotation := getAllowTakeoverFromAnnotation(oldAnnotations)
		newAllowTakeoverFromAnnotation := getAllowTakeoverFromAnnotation(newAnnotations)

		return oldAllowTakeoverFromAnnotation != newAllowTakeoverFromAnnotation
	},
	CreateFunc: func(e event.CreateEvent) bool {
		annotations := e.Object.GetAnnotations()
		return getAllowTakeoverFromAnnotation(annotations) != ""
	},
}

func getAllowTakeoverFromAnnotation(annotations map[string]string) string {
	if annotations == nil {
		return ""
	}
	return annotations[k8s.AllowTakeoverFromAnnotation]
}
