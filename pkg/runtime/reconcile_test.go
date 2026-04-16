package runtime

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/stretchr/testify/assert"
	ctrl "sigs.k8s.io/controller-runtime"
)

func Test_NilError(t *testing.T) {
	result, err := HandleReconcileError(nil)

	assert.Equal(t, ctrl.Result{}, result)
	assert.NoError(t, err)
}

func Test_LatticeRetryError(t *testing.T) {
	retryErr := NewRetryError()
	result, err := HandleReconcileError(retryErr)

	assert.Equal(t, ctrl.Result{RequeueAfter: time.Second * 10}, result)
	assert.NoError(t, err)
}

func Test_RequeueNeededAfter(t *testing.T) {
	requeueErr := NewRequeueNeededAfter("test reason", time.Minute*5)
	result, err := HandleReconcileError(requeueErr)

	assert.Equal(t, ctrl.Result{RequeueAfter: time.Minute * 5}, result)
	assert.NoError(t, err)
}

func Test_GenericError(t *testing.T) {
	genericErr := errors.New("generic error")
	result, err := HandleReconcileError(genericErr)

	assert.Equal(t, ctrl.Result{}, result)
	assert.Error(t, err)
	assert.Equal(t, "generic error", err.Error())
}

func Test_NilError_WithReconcileInterval(t *testing.T) {
	originalInterval := config.ReconcileDefaultResyncSeconds
	defer func() { config.ReconcileDefaultResyncSeconds = originalInterval }()

	config.ReconcileDefaultResyncSeconds = 5 * time.Minute

	result, err := HandleReconcileError(nil)
	assert.Equal(t, ctrl.Result{RequeueAfter: 5 * time.Minute}, result)
	assert.NoError(t, err)
}

func Test_NilError_WithZeroReconcileInterval(t *testing.T) {
	originalInterval := config.ReconcileDefaultResyncSeconds
	defer func() { config.ReconcileDefaultResyncSeconds = originalInterval }()

	config.ReconcileDefaultResyncSeconds = 0

	result, err := HandleReconcileError(nil)
	assert.Equal(t, ctrl.Result{}, result)
	assert.NoError(t, err)
}
