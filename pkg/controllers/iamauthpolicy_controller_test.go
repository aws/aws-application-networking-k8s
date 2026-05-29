package controllers

import (
	"context"
	"testing"
	"time"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	policy "github.com/aws/aws-application-networking-k8s/pkg/k8s/policyhelper"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// Test_IAMAuthPolicy_PeriodicRequeue verifies that on a successful reconcile of
// an accepted IAMAuthPolicy, the controller flows through HandleReconcileError
// and schedules a periodic requeue at the configured drift-detection interval.
//
// This protects against future regressions where the reconcile flow stops
// going through HandleReconcileError, which would silently disable drift
// detection for IAMAuthPolicy.
func Test_IAMAuthPolicy_PeriodicRequeue(t *testing.T) {
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

	iap := &anv1alpha1.IAMAuthPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "test-namespace",
		},
		Spec: anv1alpha1.IAMAuthPolicySpec{
			Policy: `{"Version":"2012-10-17","Statement":[]}`,
			TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
				Group: gwv1.GroupName,
				Kind:  "Gateway",
				Name:  "test-gateway",
			},
		},
	}

	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(iap, gw).
		WithStatusSubresource(&anv1alpha1.IAMAuthPolicy{}).
		Build()

	mockLattice := mocks.NewMockLattice(c)
	mockCloud := pkg_aws.NewMockCloud(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	// Stub the manager's Lattice calls so Put() returns a successful status.
	// For a Gateway target, Put() routes through putSn and calls:
	//   FindServiceNetwork -> GetAuthPolicy -> PutAuthPolicy -> UpdateServiceNetwork
	// We stub GetAuthPolicy to return State=Inactive so the manager's
	// in-sync short-circuit does not fire and the rewrite path is exercised.
	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), "test-gateway").Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: vpclattice.ServiceNetworkSummary{
				Id:   aws.String("sn-1234"),
				Name: aws.String("test-gateway"),
				Arn:  aws.String("arn:aws:vpc-lattice:us-west-2:123456789012:servicenetwork/sn-1234"),
			},
		}, nil)
	mockLattice.EXPECT().GetAuthPolicyWithContext(gomock.Any(), gomock.Any()).Return(
		&vpclattice.GetAuthPolicyOutput{
			State: aws.String(vpclattice.AuthPolicyStateInactive),
		}, nil)
	mockLattice.EXPECT().PutAuthPolicyWithContext(gomock.Any(), gomock.Any()).Return(
		&vpclattice.PutAuthPolicyOutput{}, nil)
	mockLattice.EXPECT().UpdateServiceNetworkWithContext(gomock.Any(), gomock.Any()).Return(
		&vpclattice.UpdateServiceNetworkOutput{}, nil)

	mockEventRecorder := mock_client.NewMockEventRecorder(c)
	mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	r := &IAMAuthPolicyController{
		log:           gwlog.FallbackLogger,
		client:        k8sClient,
		pm:            deploy.NewIAMAuthPolicyManager(mockCloud),
		ph:            policy.NewIAMAuthPolicyHandler(gwlog.FallbackLogger, k8sClient),
		cloud:         mockCloud,
		eventRecorder: mockEventRecorder,
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

// Test_IAMAuthPolicy_LatticeResourceNotFound verifies that when the underlying
// VPC Lattice resource (Service or ServiceNetwork) is missing, reconcile sets
// the policy's Accepted condition to Invalid with a descriptive message rather
// than silently succeeding. It also verifies the resource-id annotation is
// preserved (not stomped to empty), so that recovery is possible once the
// upstream resource returns.
//
// This case is primarily reachable for Gateway-targeted policies, because
// service networks are not recreated by drift detection. Without this branch,
// the controller reports Accepted=True while not actually applying the policy.
func Test_IAMAuthPolicy_LatticeResourceNotFound(t *testing.T) {
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

	// Seed the policy with a previously-applied resource id so we can assert
	// it is preserved across the NotFound branch.
	const previouslyAppliedResId = "sn-previously-applied"
	iap := &anv1alpha1.IAMAuthPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "test-namespace",
			Annotations: map[string]string{
				IAMAuthPolicyAnnotationResId:   previouslyAppliedResId,
				IAMAuthPolicyAnnotationType:    "ServiceNetwork",
				IAMAuthPolicyAnnotationResName: "test-gateway",
			},
		},
		Spec: anv1alpha1.IAMAuthPolicySpec{
			Policy: `{"Version":"2012-10-17","Statement":[]}`,
			TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
				Group: gwv1.GroupName,
				Kind:  "Gateway",
				Name:  "test-gateway",
			},
		},
	}

	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(iap, gw).
		WithStatusSubresource(&anv1alpha1.IAMAuthPolicy{}).
		Build()

	mockLattice := mocks.NewMockLattice(c)
	mockCloud := pkg_aws.NewMockCloud(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	// Simulate the underlying ServiceNetwork being deleted out-of-band: the
	// manager's Find* call returns a NotFound. PutAuthPolicy and
	// UpdateServiceNetwork must not be called in this path.
	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), "test-gateway").Return(
		nil, mocks.NewNotFoundError("Service network", "test-gateway"))

	mockEventRecorder := mock_client.NewMockEventRecorder(c)
	mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	r := &IAMAuthPolicyController{
		log:           gwlog.FallbackLogger,
		client:        k8sClient,
		pm:            deploy.NewIAMAuthPolicyManager(mockCloud),
		ph:            policy.NewIAMAuthPolicyHandler(gwlog.FallbackLogger, k8sClient),
		cloud:         mockCloud,
		eventRecorder: mockEventRecorder,
	}

	_, err := r.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy", Namespace: "test-namespace"},
	})
	assert.NoError(t, err)

	// Reload and verify the policy's Accepted condition is False/Invalid with
	// a message that names the missing resource type and resource name.
	got := &anv1alpha1.IAMAuthPolicy{}
	assert.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(iap), got))

	cond := meta.FindStatusCondition(got.Status.Conditions, string(gwv1.PolicyConditionAccepted))
	assert.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, string(gwv1.PolicyReasonInvalid), cond.Reason)
	assert.Contains(t, cond.Message, "test-gateway")

	// The previously-applied resource-id annotation must not be stomped to
	// empty; otherwise the controller loses the handle to the resource it
	// previously managed.
	assert.Equal(t, previouslyAppliedResId, got.Annotations[IAMAuthPolicyAnnotationResId])
}
