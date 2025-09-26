package runtime

import (
	"errors"

	ctrl "sigs.k8s.io/controller-runtime"
)

// HandleReconcileError will handle errors from reconcile handlers, which respects runtime errors.
func HandleReconcileError(err error) (ctrl.Result, error) {
	if err == nil {
		return ctrl.Result{}, nil
	}

	var requeueNeededAfter *RequeueNeededAfter
	if errors.As(err, &requeueNeededAfter) {
		return ctrl.Result{RequeueAfter: requeueNeededAfter.Duration()}, nil
	}

	return ctrl.Result{}, err
}
