package core

import (
	"context"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//go:generate mockgen -destination mutator_mocks.go -package core github.com/aws/aws-application-networking-k8s/pkg/webhook/core Mutator
type Mutator interface {
	// Prototype returns a prototype of Object for this admission request.
	Prototype(req admission.Request) (runtime.Object, error)

	// MutateCreate handles Object creation and returns the object after mutation and error if any.
	MutateCreate(ctx context.Context, obj runtime.Object) (runtime.Object, error)
	// MutateUpdate handles Object update and returns the object after mutation and error if any.
	MutateUpdate(ctx context.Context, obj runtime.Object, oldObj runtime.Object) (runtime.Object, error)
}

// MutatingWebhookForMutator creates a new mutating Webhook.
func MutatingWebhookForMutator(scheme *runtime.Scheme, mutator Mutator) *admission.Webhook {
	return &admission.Webhook{
		Handler: &mutatingHandler{
			mutator: mutator,
			decoder: admission.NewDecoder(scheme),
		},
	}
}
