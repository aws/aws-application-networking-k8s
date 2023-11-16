package k8s

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//go:generate mockgen -destination finalizer_mock.go -package k8s github.com/aws/aws-application-networking-k8s/pkg/k8s FinalizerManager

type FinalizerManager interface {
	AddFinalizers(ctx context.Context, object client.Object, finalizers ...string) error
	RemoveFinalizers(ctx context.Context, object client.Object, finalizers ...string) error
}

func NewDefaultFinalizerManager(k8sClient client.Client) FinalizerManager {
	return &defaultFinalizerManager{
		k8sClient: k8sClient,
	}
}

type defaultFinalizerManager struct {
	k8sClient client.Client
}

func (m *defaultFinalizerManager) AddFinalizers(ctx context.Context, obj client.Object, finalizers ...string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := m.k8sClient.Get(ctx, NamespacedName(obj), obj); err != nil {
			return err
		}

		oldObj := obj.DeepCopyObject().(client.Object)
		needsUpdate := false
		for _, finalizer := range finalizers {
			if !HasFinalizer(obj, finalizer) {
				controllerutil.AddFinalizer(obj, finalizer)
				needsUpdate = true
			}
		}
		if !needsUpdate {
			return nil
		}
		return m.k8sClient.Patch(ctx, obj, client.MergeFromWithOptions(oldObj, client.MergeFromWithOptimisticLock{}))
	})
}

func (m *defaultFinalizerManager) RemoveFinalizers(ctx context.Context, obj client.Object, finalizers ...string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := m.k8sClient.Get(ctx, NamespacedName(obj), obj); err != nil {
			return err
		}

		oldObj := obj.DeepCopyObject().(client.Object)
		needsUpdate := false
		for _, finalizer := range finalizers {
			if HasFinalizer(obj, finalizer) {
				controllerutil.RemoveFinalizer(obj, finalizer)
				needsUpdate = true
			}
		}
		if !needsUpdate {
			return nil
		}
		return m.k8sClient.Patch(ctx, obj, client.MergeFromWithOptions(oldObj, client.MergeFromWithOptimisticLock{}))
	})
}

// HasFinalizer tests whether k8s object has specified finalizer
func HasFinalizer(obj metav1.Object, finalizer string) bool {
	f := obj.GetFinalizers()
	for _, e := range f {
		if e == finalizer {
			return true
		}
	}
	return false
}
