package predicates

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
)

var AdditionalTagsAnnotationChangedPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		oldAnnotations := e.ObjectOld.GetAnnotations()
		newAnnotations := e.ObjectNew.GetAnnotations()

		oldAdditionalTags := getAdditionalTagsAnnotation(oldAnnotations)
		newAdditionalTags := getAdditionalTagsAnnotation(newAnnotations)

		return oldAdditionalTags != newAdditionalTags
	},
	CreateFunc: func(e event.CreateEvent) bool {
		annotations := e.Object.GetAnnotations()
		return getAdditionalTagsAnnotation(annotations) != ""
	},
}

func getAdditionalTagsAnnotation(annotations map[string]string) string {
	if annotations == nil {
		return ""
	}
	return annotations[k8s.TagsAnnotationKey]
}
