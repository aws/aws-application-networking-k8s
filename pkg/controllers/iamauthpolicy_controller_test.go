package controllers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	policy "github.com/aws/aws-application-networking-k8s/pkg/k8s/policyhelper"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	latticetypes "github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
			SvcNetwork: latticetypes.ServiceNetworkSummary{
				Id:   aws.String("sn-1234"),
				Name: aws.String("test-gateway"),
				Arn:  aws.String("arn:aws:vpc-lattice:us-west-2:123456789012:servicenetwork/sn-1234"),
			},
		}, nil)
	mockLattice.EXPECT().GetAuthPolicy(gomock.Any(), gomock.Any()).Return(
		&vpclattice.GetAuthPolicyOutput{
			State: latticetypes.AuthPolicyStateInactive,
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

func Test_IAMAuthPolicy_ForeignOwnedService_Conflicted(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)
	gwv1.Install(k8sScheme)
	anv1alpha1.Install(k8sScheme)

	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-route",
			Namespace: "test-namespace",
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{Name: "test-gateway"},
				},
			},
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
				Kind:  "HTTPRoute",
				Name:  "test-route",
			},
		},
	}

	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(iap, route).
		WithStatusSubresource(&anv1alpha1.IAMAuthPolicy{}).
		Build()

	mockLattice := mocks.NewMockLattice(c)
	mockCloud := pkg_aws.NewMockCloud(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	svcArn := "arn:aws:vpc-lattice:us-west-2:999999999999:service/svc-foreign"
	mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(
		&latticetypes.ServiceSummary{
			Id:  aws.String("svc-foreign"),
			Arn: aws.String(svcArn),
		}, nil)
	// Write APIs must not be called once ownership is rejected.
	mockCloud.EXPECT().IsArnManaged(gomock.Any(), svcArn).Return(false, nil)

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

	got := &anv1alpha1.IAMAuthPolicy{}
	assert.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(iap), got))

	cond := meta.FindStatusCondition(got.Status.Conditions, string(gwv1.PolicyConditionAccepted))
	assert.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, string(gwv1.PolicyReasonConflicted), cond.Reason)
}

const (
	snA = "sn-0000000000000000a"
	snB = "sn-0000000000000000b"
)

func Test_IAMAuthPolicy_ServiceNetworkCleanup(t *testing.T) {
	const (
		ns            = "tenant"
		latticeGwc    = "amazon-vpc-lattice"
		victimSnId    = "sn-00000000000victim"
		victimSnName  = "payments"
		defaultSnId   = "sn-0000000000default"
		defaultSnName = "default-sn"
	)

	gateway := func(namespace, name, class string) *gwv1.Gateway {
		return &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       gwv1.GatewaySpec{GatewayClassName: gwv1.ObjectName(class)},
		}
	}
	terminating := func(gw *gwv1.Gateway) *gwv1.Gateway {
		gw.DeletionTimestamp = &metav1.Time{Time: time.Now()}
		gw.Finalizers = []string{"gateway.k8s.aws/resources"}
		return gw
	}
	snIdFor := map[string]string{"gw-a": snA, "gw-b": snB}
	snNames := map[string]string{snA: "gw-a", snB: "gw-b", victimSnId: victimSnName, defaultSnId: defaultSnName}

	tests := []struct {
		name          string
		override      bool
		defaultSn     string // DEFAULT_SERVICE_NETWORK in override mode
		deleting      bool
		targetGw      string
		annotations   map[string]string // defaults to a true record for gw-a
		gateways      []*gwv1.Gateway
		getErr        error  // returned by GetServiceNetwork
		gwcErr        error  // returned when reading a GatewayClass
		applyTo       string // gateway whose SN gets AWS_IAM applied
		cleanedSn     string // SN expected to be set to NONE
		wantSkipEvent bool
		wantErr       bool
		wantResId     string // resource-id annotation after upsert
	}{
		{
			name:      "delete with gateway present cleans up",
			deleting:  true,
			targetGw:  "gw-a",
			gateways:  []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
			cleanedSn: snA,
		},
		{
			name:      "delete while the gateway is terminating cleans up",
			deleting:  true,
			targetGw:  "gw-a",
			gateways:  []*gwv1.Gateway{terminating(gateway(ns, "gw-a", latticeGwc))},
			cleanedSn: snA,
		},
		{
			name:      "retarget cleans up the previous gateway's SN",
			targetGw:  "gw-b",
			gateways:  []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc), gateway(ns, "gw-b", latticeGwc)},
			applyTo:   "gw-b",
			cleanedSn: snA,
			wantResId: snB,
		},
		{
			name:          "delete after gateway deleted skips cleanup",
			deleting:      true,
			targetGw:      "gw-a",
			wantSkipEvent: true,
		},
		{
			name:          "retarget after old gateway deleted skips cleanup",
			targetGw:      "gw-b",
			gateways:      []*gwv1.Gateway{gateway(ns, "gw-b", latticeGwc)},
			applyTo:       "gw-b",
			wantSkipEvent: true,
			wantResId:     snB,
		},
		{
			name:      "delete accepts a gateway named after the SN ID",
			deleting:  true,
			targetGw:  snA,
			gateways:  []*gwv1.Gateway{gateway(ns, snA, latticeGwc)},
			cleanedSn: snA,
		},
		{
			name:      "steady state does not clean up the SN it just applied",
			targetGw:  "gw-a",
			gateways:  []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
			applyTo:   "gw-a",
			wantResId: snA,
		},
		{
			name:     "delete with a record lacking the name annotation cleans up",
			deleting: true,
			targetGw: "gw-a",
			annotations: map[string]string{
				IAMAuthPolicyAnnotationResId: snA,
				IAMAuthPolicyAnnotationType:  model.ServiceNetworkType,
			},
			gateways:  []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
			cleanedSn: snA,
		},
		{
			name:        "delete without a recorded resource removes the finalizer",
			deleting:    true,
			targetGw:    "gw-a",
			annotations: map[string]string{},
			gateways:    []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
		},
		{
			name:        "delete when the recorded SN no longer exists removes the finalizer",
			deleting:    true,
			targetGw:    "gw-a",
			annotations: snAnnotations("sn-0000000000000gone", "gw-a"),
			gateways:    []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
		},
		{
			name:     "lattice error, including access denied, keeps the finalizer for retry",
			deleting: true,
			targetGw: "gw-a",
			gateways: []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
			getErr:   &latticetypes.AccessDeniedException{},
			wantErr:  true,
		},
		{
			name:          "recorded value that is not an SN ID is skipped without a Lattice call",
			deleting:      true,
			targetGw:      "gw-a",
			annotations:   snAnnotations("arn:aws:vpc-lattice:us-west-2:111111111111:servicenetwork/"+victimSnId, "gw-a"),
			gateways:      []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
			wantSkipEvent: true,
		},
		{
			name:     "gatewayclass read error keeps the finalizer for retry",
			deleting: true,
			targetGw: "gw-a",
			gateways: []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
			gwcErr:   errors.New("cache read failed"),
			wantErr:  true,
		},
		{
			name:        "override mode delete cleans up the default SN",
			override:    true,
			deleting:    true,
			targetGw:    "gw-a",
			annotations: snAnnotations(defaultSnId, "gw-a"),
			gateways:    []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
			cleanedSn:   defaultSnId,
		},
		{
			name:        "override mode matches a default SN configured by ID",
			override:    true,
			defaultSn:   defaultSnId,
			deleting:    true,
			targetGw:    "gw-a",
			annotations: snAnnotations(defaultSnId, "gw-a"),
			gateways:    []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
			cleanedSn:   defaultSnId,
		},
		{
			name:          "override mode needs a gateway in the policy's namespace",
			override:      true,
			deleting:      true,
			targetGw:      "gw-a",
			annotations:   snAnnotations(defaultSnId, "gw-a"),
			gateways:      []*gwv1.Gateway{gateway("other", "gw-a", latticeGwc), gateway(ns, "gw-x", "other-class")},
			wantSkipEvent: true,
		},
		{
			name:          "override mode ignores a forged non-default SN",
			override:      true,
			deleting:      true,
			targetGw:      "gw-a",
			annotations:   snAnnotations(victimSnId, "gw-a"),
			gateways:      []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
			wantSkipEvent: true,
		},
		{
			name:          "forged record on delete is ignored",
			deleting:      true,
			targetGw:      "gw-a",
			annotations:   snAnnotations(victimSnId, "gw-a"),
			gateways:      []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
			wantSkipEvent: true,
		},
		{
			name:        "forged name with a true ID only cleans up the true SN",
			deleting:    true,
			targetGw:    "gw-a",
			annotations: snAnnotations(snA, "victim"),
			gateways:    []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
			cleanedSn:   snA,
		},
		{
			name:          "forged record on retarget is ignored",
			targetGw:      "gw-a",
			annotations:   snAnnotations(victimSnId, "gw-a"),
			gateways:      []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc)},
			applyTo:       "gw-a",
			wantSkipEvent: true,
			wantResId:     snA,
		},
		{
			name:          "forged record with targetRef to a missing gateway is ignored on delete",
			deleting:      true,
			targetGw:      "missing",
			annotations:   snAnnotations(victimSnId, victimSnName),
			wantSkipEvent: true,
		},
		{
			name:          "forged record for another namespace's gateway is ignored",
			deleting:      true,
			targetGw:      "gw-a",
			annotations:   snAnnotations(victimSnId, victimSnName),
			gateways:      []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc), gateway("other", victimSnName, latticeGwc)},
			wantSkipEvent: true,
		},
		{
			name:          "forged record for a gateway of another class is ignored",
			deleting:      true,
			targetGw:      "gw-a",
			annotations:   snAnnotations(victimSnId, victimSnName),
			gateways:      []*gwv1.Gateway{gateway(ns, "gw-a", latticeGwc), gateway(ns, victimSnName, "other-class")},
			wantSkipEvent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origOverride, origDefault := config.ServiceNetworkOverrideMode, config.DefaultServiceNetwork
			defer func() { config.ServiceNetworkOverrideMode, config.DefaultServiceNetwork = origOverride, origDefault }()
			config.ServiceNetworkOverrideMode = tt.override
			if tt.override {
				config.DefaultServiceNetwork = defaultSnName
				if tt.defaultSn != "" {
					config.DefaultServiceNetwork = tt.defaultSn
				}
			}

			ctx := context.TODO()
			c := gomock.NewController(t)
			scheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(scheme)
			gwv1.Install(scheme)
			anv1alpha1.Install(scheme)

			annotations := tt.annotations
			if annotations == nil {
				annotations = snAnnotations(snA, "gw-a")
			}
			iap := &anv1alpha1.IAMAuthPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "policy",
					Namespace:   ns,
					Annotations: annotations,
					Finalizers:  []string{IAMAuthPolicyFinalizer},
				},
				Spec: anv1alpha1.IAMAuthPolicySpec{
					Policy: `{"Version":"2012-10-17","Statement":[]}`,
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Group: gwv1.GroupName,
						Kind:  "Gateway",
						Name:  gwv1.ObjectName(tt.targetGw),
					},
				},
			}
			if tt.deleting {
				iap.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			}
			objs := []client.Object{iap,
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{Name: latticeGwc},
					Spec:       gwv1.GatewayClassSpec{ControllerName: gwv1.GatewayController(config.LatticeGatewayControllerName)},
				},
				&gwv1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{Name: "other-class"},
					Spec:       gwv1.GatewayClassSpec{ControllerName: "example.com/other"},
				},
			}
			for _, gw := range tt.gateways {
				objs = append(objs, gw)
			}
			k8sClient := testclient.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
				WithStatusSubresource(&anv1alpha1.IAMAuthPolicy{}).
				WithInterceptorFuncs(interceptor.Funcs{Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*gwv1.GatewayClass); ok && tt.gwcErr != nil {
						return tt.gwcErr
					}
					return cl.Get(ctx, key, obj, opts...)
				}}).Build()

			// Strict mocks: any unexpected write, e.g. on the victim SN, fails the test.
			mockLattice := mocks.NewMockLattice(c)
			mockCloud := pkg_aws.NewMockCloud(c)
			mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()
			recordedId, recorded := annotations[IAMAuthPolicyAnnotationResId]
			if recorded && recordedId != snIdFor[tt.applyTo] && snIdPattern.MatchString(recordedId) {
				mockLattice.EXPECT().GetServiceNetwork(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, in *vpclattice.GetServiceNetworkInput, _ ...func(*vpclattice.Options)) (*vpclattice.GetServiceNetworkOutput, error) {
						id := aws.ToString(in.ServiceNetworkIdentifier)
						name, ok := snNames[id]
						switch {
						case tt.getErr != nil:
							return nil, tt.getErr
						case !ok:
							return nil, &latticetypes.ResourceNotFoundException{}
						}
						return &vpclattice.GetServiceNetworkOutput{Id: aws.String(id), Name: aws.String(name)}, nil
					})
			}
			if tt.applyTo != "" {
				snId := snIdFor[tt.applyTo]
				mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), tt.applyTo).Return(&mocks.ServiceNetworkInfo{
					SvcNetwork: latticetypes.ServiceNetworkSummary{Id: aws.String(snId), Name: aws.String(tt.applyTo)},
				}, nil)
				mockLattice.EXPECT().GetAuthPolicy(gomock.Any(), gomock.Any()).Return(&vpclattice.GetAuthPolicyOutput{}, nil)
				mockLattice.EXPECT().PutAuthPolicy(gomock.Any(), gomock.Any()).Return(&vpclattice.PutAuthPolicyOutput{}, nil)
				mockLattice.EXPECT().UpdateServiceNetwork(gomock.Any(), snUpdate(snId, latticetypes.AuthTypeAwsIam)).
					Return(&vpclattice.UpdateServiceNetworkOutput{}, nil)
			}
			if tt.cleanedSn != "" {
				mockLattice.EXPECT().UpdateServiceNetwork(gomock.Any(), snUpdate(tt.cleanedSn, latticetypes.AuthTypeNone)).
					Return(&vpclattice.UpdateServiceNetworkOutput{}, nil)
				mockLattice.EXPECT().DeleteAuthPolicy(gomock.Any(), gomock.Cond(func(in *vpclattice.DeleteAuthPolicyInput) bool {
					return aws.ToString(in.ResourceIdentifier) == tt.cleanedSn
				})).Return(&vpclattice.DeleteAuthPolicyOutput{}, nil)
			}

			recorder := record.NewFakeRecorder(20)
			r := &IAMAuthPolicyController{
				log:           gwlog.FallbackLogger,
				client:        k8sClient,
				pm:            deploy.NewIAMAuthPolicyManager(mockCloud),
				ph:            policy.NewIAMAuthPolicyHandler(gwlog.FallbackLogger, k8sClient),
				cloud:         mockCloud,
				eventRecorder: recorder,
			}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "policy", Namespace: ns}})

			skipEvent := corev1.EventTypeWarning + " " + k8s.IAMAuthPolicyEventReasonSkippedCleanup + " "
			skipped := false
			for len(recorder.Events) > 0 {
				if e := <-recorder.Events; strings.HasPrefix(e, skipEvent) {
					assert.Contains(t, e, recordedId)
					if snIdPattern.MatchString(recordedId) {
						assert.Contains(t, e, snNames[recordedId])
					}
					skipped = true
				}
			}
			assert.Equal(t, tt.wantSkipEvent, skipped, "SkippedCleanup warning")

			got := &anv1alpha1.IAMAuthPolicy{}
			getErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(iap), got)
			switch {
			case tt.wantErr:
				assert.Error(t, err)
				assert.NoError(t, getErr)
				assert.Contains(t, got.Finalizers, IAMAuthPolicyFinalizer)
			case tt.deleting:
				assert.NoError(t, err)
				assert.True(t, apierrors.IsNotFound(getErr), "policy should be gone, got %v", getErr)
			default:
				assert.NoError(t, err)
				assert.NoError(t, getErr)
				assert.Equal(t, tt.wantResId, got.Annotations[IAMAuthPolicyAnnotationResId])
				assert.Equal(t, tt.applyTo, got.Annotations[IAMAuthPolicyAnnotationResName])
			}
		})
	}
}

func snAnnotations(id, name string) map[string]string {
	return map[string]string{
		IAMAuthPolicyAnnotationResId:   id,
		IAMAuthPolicyAnnotationType:    model.ServiceNetworkType,
		IAMAuthPolicyAnnotationResName: name,
	}
}

func snUpdate(id string, authType latticetypes.AuthType) gomock.Matcher {
	return gomock.Cond(func(in *vpclattice.UpdateServiceNetworkInput) bool {
		return aws.ToString(in.ServiceNetworkIdentifier) == id && in.AuthType == authType
	})
}

// The Gateway must outlive its policy so the policy can still remove its auth.
func Test_IAMAuthPolicy_NamespaceDeletionCleansUpBeforeGatewayGoes(t *testing.T) {
	ctx := context.TODO()
	c := gomock.NewController(t)
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	gwv1.Install(scheme)
	gwv1alpha2.Install(scheme)
	anv1alpha1.Install(scheme)

	now := &metav1.Time{Time: time.Now()}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw-a", Namespace: "tenant", Finalizers: []string{gatewayFinalizer}, DeletionTimestamp: now},
		Spec:       gwv1.GatewaySpec{GatewayClassName: "amazon-vpc-lattice"},
	}
	iap := &anv1alpha1.IAMAuthPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "policy", Namespace: "tenant", Annotations: snAnnotations(snA, "gw-a"),
			Finalizers: []string{IAMAuthPolicyFinalizer}, DeletionTimestamp: now,
		},
		Spec: anv1alpha1.IAMAuthPolicySpec{
			TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{Group: gwv1.GroupName, Kind: "Gateway", Name: "gw-a"},
		},
	}
	gwc := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "amazon-vpc-lattice"},
		Spec:       gwv1.GatewayClassSpec{ControllerName: gwv1.GatewayController(config.LatticeGatewayControllerName)},
	}
	k8sClient := testclient.NewClientBuilder().WithScheme(scheme).WithObjects(gw, iap, gwc).
		WithStatusSubresource(&gwv1.Gateway{}, &anv1alpha1.IAMAuthPolicy{}).Build()

	mockLattice := mocks.NewMockLattice(c)
	mockCloud := pkg_aws.NewMockCloud(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()
	mockLattice.EXPECT().GetServiceNetwork(gomock.Any(), gomock.Any()).Return(
		&vpclattice.GetServiceNetworkOutput{Id: aws.String(snA), Name: aws.String("gw-a")}, nil)
	mockLattice.EXPECT().UpdateServiceNetwork(gomock.Any(), snUpdate(snA, latticetypes.AuthTypeNone)).
		Return(&vpclattice.UpdateServiceNetworkOutput{}, nil)
	mockLattice.EXPECT().DeleteAuthPolicy(gomock.Any(), gomock.Any()).Return(&vpclattice.DeleteAuthPolicyOutput{}, nil)

	gwReconciler := &gatewayReconciler{log: gwlog.FallbackLogger, client: k8sClient,
		finalizerManager: k8s.NewDefaultFinalizerManager(k8sClient)}
	iapReconciler := &IAMAuthPolicyController{
		log:           gwlog.FallbackLogger,
		client:        k8sClient,
		pm:            deploy.NewIAMAuthPolicyManager(mockCloud),
		ph:            policy.NewIAMAuthPolicyHandler(gwlog.FallbackLogger, k8sClient),
		cloud:         mockCloud,
		eventRecorder: record.NewFakeRecorder(20),
	}

	assert.ErrorContains(t, gwReconciler.reconcileDelete(ctx, gw.DeepCopy()), "IAMAuthPolicy")

	_, err := iapReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(iap)})
	assert.NoError(t, err)

	assert.NoError(t, gwReconciler.reconcileDelete(ctx, gw.DeepCopy()))
	assert.True(t, apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), &gwv1.Gateway{})))
}
