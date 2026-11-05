package predicates

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
)

var DnsEndpointAnnotationChangedPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		return k8s.IsDnsEndpointDisabled(e.ObjectOld) != k8s.IsDnsEndpointDisabled(e.ObjectNew)
	},
	CreateFunc: func(e event.CreateEvent) bool {
		return k8s.IsDnsEndpointDisabled(e.Object)
	},
}
