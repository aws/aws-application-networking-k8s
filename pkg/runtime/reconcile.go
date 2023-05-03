package runtime

import (
	"errors"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

// HandleReconcileError will handle errors from reconcile handlers, which respects runtime errors.
func HandleReconcileError(err error) (ctrl.Result, error) {
	if err == nil {
		return ctrl.Result{}, nil
	}

	fmt.Printf("HandleReconcileError handle Error %v \n", err)

	retryErr := NewRetryError()
	if errors.As(err, &retryErr) {
		fmt.Printf(">>>>>> Retrying Reconcile after 20 seconds ...\n")
		return ctrl.Result{RequeueAfter: time.Second * 20}, nil
	}

	var requeueNeededAfter *RequeueNeededAfter
	if errors.As(err, &requeueNeededAfter) {
		fmt.Print("requeue after", "duration", requeueNeededAfter.Duration(), "reason", requeueNeededAfter.Reason())
		return ctrl.Result{RequeueAfter: requeueNeededAfter.Duration()}, nil
	}

	var requeueNeeded *RequeueNeeded
	if errors.As(err, &requeueNeeded) {
		fmt.Print("requeue", "reason", requeueNeeded.Reason())
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{RequeueAfter: time.Minute * 10}, err
}
