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
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	latticetypes "github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
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
	//   FindServiceNetwork -> PutAuthPolicyWithContext -> UpdateServiceNetworkWithContext
	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), "test-gateway").Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: latticetypes.ServiceNetworkSummary{
				Id:   aws.String("sn-1234"),
				Name: aws.String("test-gateway"),
				Arn:  aws.String("arn:aws:vpc-lattice:us-west-2:123456789012:servicenetwork/sn-1234"),
			},
		}, nil)
	mockLattice.EXPECT().PutAuthPolicy(gomock.Any(), gomock.Any()).Return(
		&vpclattice.PutAuthPolicyOutput{}, nil)
	mockLattice.EXPECT().UpdateServiceNetwork(gomock.Any(), gomock.Any()).Return(
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
