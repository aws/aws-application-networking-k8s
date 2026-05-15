package controllers

import (
	"context"
	"testing"
	"time"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	policy "github.com/aws/aws-application-networking-k8s/pkg/k8s/policyhelper"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// Test_VpcAssociationPolicy_PeriodicRequeue verifies that on a successful
// reconcile of an accepted VpcAssociationPolicy, the controller flows through
// HandleReconcileError and schedules a periodic requeue at the configured
// drift-detection interval.
//
// This protects against future regressions where the reconcile flow stops
// going through HandleReconcileError, which would silently disable drift
// detection for VpcAssociationPolicy.
func Test_VpcAssociationPolicy_PeriodicRequeue(t *testing.T) {
	originalInterval := config.ReconcileDefaultResyncInterval
	defer func() { config.ReconcileDefaultResyncInterval = originalInterval }()
	interval := 5 * time.Minute
	config.ReconcileDefaultResyncInterval = interval

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)
	gwv1.Install(k8sScheme)
	anv1alpha1.Install(k8sScheme)

	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gateway",
			Namespace: "test-namespace",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "amazon-vpc-lattice",
		},
	}

	vap := &anv1alpha1.VpcAssociationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "test-namespace",
		},
		Spec: anv1alpha1.VpcAssociationPolicySpec{
			TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
				Group: gwv1.GroupName,
				Kind:  "Gateway",
				Name:  "test-gateway",
			},
		},
	}

	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(vap, gw).
		WithStatusSubresource(&anv1alpha1.VpcAssociationPolicy{}).
		Build()

	// Stub the manager so UpsertVpcAssociation returns a successful ARN.
	mockSNManager := deploy.NewMockServiceNetworkManager(c)
	mockSNManager.EXPECT().UpsertVpcAssociation(gomock.Any(), "test-gateway", gomock.Any(), gomock.Any()).Return(
		"arn:aws:vpc-lattice:us-west-2:123456789012:servicenetworkvpcassociation/snva-1234", nil)

	mockFinalizer := k8s.NewMockFinalizerManager(c)
	mockFinalizer.EXPECT().AddFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	r := &vpcAssociationPolicyReconciler{
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		finalizerManager: mockFinalizer,
		manager:          mockSNManager,
		ph:               policy.NewVpcAssociationPolicyHandler(gwlog.FallbackLogger, k8sClient),
	}

	result, err := r.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy", Namespace: "test-namespace"},
	})
	assert.NoError(t, err)
	// RequeueAfter should be between interval and interval + 20% jitter,
	// matching pkg/runtime/reconcile_test.go::Test_NilError_WithReconcileInterval.
	assert.GreaterOrEqual(t, result.RequeueAfter, interval)
	assert.LessOrEqual(t, result.RequeueAfter, time.Duration(float64(interval)*1.2))
}
