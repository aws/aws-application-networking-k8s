package runtime

import (
	"errors"
	"math/rand"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	ctrl "sigs.k8s.io/controller-runtime"
)

// HandleReconcileError will handle errors from reconcile handlers, which respects runtime errors.
func HandleReconcileError(err error) (ctrl.Result, error) {
	if err == nil {
		if config.ReconcileDefaultResyncInterval > 0 {
			// Add 0-20% jitter to prevent thundering herd at startup.
			// Jitter scales with interval: short intervals get small jitter
			// (preserving fast recovery), long intervals get proportionally
			// larger jitter (acceptable since the user opted for slow recovery).
			jitter := time.Duration(rand.Int63n(int64(config.ReconcileDefaultResyncInterval) / 5))
			return ctrl.Result{RequeueAfter: config.ReconcileDefaultResyncInterval + jitter}, nil
		}
		return ctrl.Result{}, nil
	}

	var requeueNeededAfter *RequeueNeededAfter
	if errors.As(err, &requeueNeededAfter) {
		return ctrl.Result{RequeueAfter: requeueNeededAfter.Duration()}, nil
	}

	return ctrl.Result{}, err
}
