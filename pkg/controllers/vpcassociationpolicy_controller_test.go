package controllers

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	vpclatticetypes "github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	policy "github.com/aws/aws-application-networking-k8s/pkg/k8s/policyhelper"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	vapTestNamespace = "test-namespace"
	vapTestGateway   = "test-gateway"
	vapTestPolicy    = "test-policy"
	vapTestSnvaArn   = "arn:aws:vpc-lattice:us-west-2:123456789012:servicenetworkvpcassociation/snva-1234"
)

type vapTestEnv struct {
	ctx       context.Context
	client    client.Client
	manager   *deploy.MockServiceNetworkManager
	recorder  *record.FakeRecorder
	r         *vpcAssociationPolicyReconciler
	req       reconcile.Request
	finalizer *k8s.MockFinalizerManager
}

func newVapTestGateway() *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: vapTestGateway, Namespace: vapTestNamespace},
		Spec:       gwv1.GatewaySpec{GatewayClassName: "amazon-vpc-lattice"},
	}
}

func newVapTestPolicy(mutate func(spec *anv1alpha1.VpcAssociationPolicySpec)) *anv1alpha1.VpcAssociationPolicy {
	vap := &anv1alpha1.VpcAssociationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: vapTestPolicy, Namespace: vapTestNamespace, Generation: 3},
		Spec: anv1alpha1.VpcAssociationPolicySpec{
			TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
				Group: gwv1.GroupName,
				Kind:  "Gateway",
				Name:  vapTestGateway,
			},
		},
	}
	if mutate != nil {
		mutate(&vap.Spec)
	}
	return vap
}

// newVapTestEnv builds a reconciler over a fake client. The manager is a strict mock: tests declare
// the calls they expect. Finalizer operations are no-ops.
func newVapTestEnv(t *testing.T, objs ...client.Object) *vapTestEnv {
	originalInterval := config.ReconcileDefaultResyncInterval
	config.ReconcileDefaultResyncInterval = 0
	t.Cleanup(func() { config.ReconcileDefaultResyncInterval = originalInterval })

	c := gomock.NewController(t)
	t.Cleanup(c.Finish)

	k8sScheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(k8sScheme)
	_ = gwv1.Install(k8sScheme)
	_ = anv1alpha1.Install(k8sScheme)

	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(objs...).
		WithStatusSubresource(&anv1alpha1.VpcAssociationPolicy{}).
		Build()

	mockManager := deploy.NewMockServiceNetworkManager(c)
	mockFinalizer := k8s.NewMockFinalizerManager(c)
	mockFinalizer.EXPECT().AddFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockFinalizer.EXPECT().RemoveFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	recorder := record.NewFakeRecorder(16)

	r := &vpcAssociationPolicyReconciler{
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		finalizerManager: mockFinalizer,
		manager:          mockManager,
		eventRecorder:    recorder,
		ph:               policy.NewVpcAssociationPolicyHandler(gwlog.FallbackLogger, k8sClient),
	}
	return &vapTestEnv{
		ctx:       context.TODO(),
		client:    k8sClient,
		manager:   mockManager,
		recorder:  recorder,
		r:         r,
		req:       reconcile.Request{NamespacedName: types.NamespacedName{Name: vapTestPolicy, Namespace: vapTestNamespace}},
		finalizer: mockFinalizer,
	}
}

func (e *vapTestEnv) reconcile(t *testing.T) (reconcile.Result, error) {
	t.Helper()
	return e.r.Reconcile(e.ctx, e.req)
}

func (e *vapTestEnv) getPolicy(t *testing.T) *anv1alpha1.VpcAssociationPolicy {
	t.Helper()
	vap := &anv1alpha1.VpcAssociationPolicy{}
	assert.NoError(t, e.client.Get(e.ctx, e.req.NamespacedName, vap))
	return vap
}

func (e *vapTestEnv) programmed(t *testing.T) *metav1.Condition {
	t.Helper()
	return meta.FindStatusCondition(e.getPolicy(t).Status.Conditions, string(anv1alpha1.VpcAssociationPolicyConditionProgrammed))
}

func (e *vapTestEnv) drainEvents() []string {
	var events []string
	for {
		select {
		case ev := <-e.recorder.Events:
			events = append(events, ev)
		default:
			return events
		}
	}
}

// Test_VpcAssociationPolicy_PeriodicRequeue verifies that on a successful
// reconcile of an accepted VpcAssociationPolicy, the controller flows through
// HandleReconcileError and schedules a periodic requeue at the configured
// drift-detection interval.
//
// This protects against future regressions where the reconcile flow stops
// going through HandleReconcileError, which would silently disable drift
// detection for VpcAssociationPolicy.
func Test_VpcAssociationPolicy_PeriodicRequeue(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(nil), newVapTestGateway())
	interval := 5 * time.Minute
	config.ReconcileDefaultResyncInterval = interval

	e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).
		Return(deploy.VpcAssociationResult{Arn: vapTestSnvaArn}, nil)

	result, err := e.reconcile(t)
	assert.NoError(t, err)
	// RequeueAfter should be between interval and interval + 20% jitter,
	// matching pkg/runtime/reconcile_test.go::Test_NilError_WithReconcileInterval.
	assert.GreaterOrEqual(t, result.RequeueAfter, interval)
	assert.LessOrEqual(t, result.RequeueAfter, time.Duration(float64(interval)*1.2))
}

func Test_VpcAssociationPolicy_PassesPrivateDnsConfigToManager(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(func(spec *anv1alpha1.VpcAssociationPolicySpec) {
		spec.SecurityGroupIds = []anv1alpha1.SecurityGroupId{"sg-123456789"}
		spec.PrivateDnsEnabled = aws.Bool(true)
		spec.DnsOptions = &anv1alpha1.VpcAssociationDnsOptions{
			PrivateDnsPreference:       ptrTo(anv1alpha1.PrivateDnsPreferenceSpecifiedDomainsOnly),
			PrivateDnsSpecifiedDomains: []anv1alpha1.PrivateDnsDomain{"*.internal.example.com"},
		}
	}), newVapTestGateway())

	var got deploy.VpcAssociationRequest
	e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).DoAndReturn(
		func(ctx context.Context, snName string, req deploy.VpcAssociationRequest) (deploy.VpcAssociationResult, error) {
			got = req
			return deploy.VpcAssociationResult{Arn: vapTestSnvaArn}, nil
		})

	_, err := e.reconcile(t)
	assert.NoError(t, err)
	assert.Equal(t, []string{"sg-123456789"}, got.SecurityGroupIds)
	assert.Equal(t, deploy.VpcAssociationDnsConfig{
		PrivateDnsEnabled:          aws.Bool(true),
		PrivateDnsPreference:       "SPECIFIED_DOMAINS_ONLY",
		PrivateDnsSpecifiedDomains: []string{"*.internal.example.com"},
	}, got.Dns)
}

func Test_VpcAssociationPolicy_UnsetPrivateDns_PassesNilToManager(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(nil), newVapTestGateway())

	var got deploy.VpcAssociationRequest
	e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).DoAndReturn(
		func(ctx context.Context, snName string, req deploy.VpcAssociationRequest) (deploy.VpcAssociationResult, error) {
			got = req
			return deploy.VpcAssociationResult{Arn: vapTestSnvaArn}, nil
		})

	_, err := e.reconcile(t)
	assert.NoError(t, err)
	assert.Equal(t, deploy.VpcAssociationDnsConfig{}, got.Dns)
}

func Test_VpcAssociationPolicy_PreferenceOnlyDnsOptions_PassesNilDomains(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(func(spec *anv1alpha1.VpcAssociationPolicySpec) {
		spec.PrivateDnsEnabled = aws.Bool(true)
		spec.DnsOptions = &anv1alpha1.VpcAssociationDnsOptions{PrivateDnsPreference: ptrTo(anv1alpha1.PrivateDnsPreferenceAllDomains)}
	}), newVapTestGateway())

	var got deploy.VpcAssociationRequest
	e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).DoAndReturn(
		func(ctx context.Context, snName string, req deploy.VpcAssociationRequest) (deploy.VpcAssociationResult, error) {
			got = req
			return deploy.VpcAssociationResult{Arn: vapTestSnvaArn}, nil
		})

	_, err := e.reconcile(t)
	assert.NoError(t, err)
	assert.Equal(t, "ALL_DOMAINS", got.Dns.PrivateDnsPreference)
	assert.Nil(t, got.Dns.PrivateDnsSpecifiedDomains, "an absent domain list must stay nil so it is not serialized as an empty array")
}

func Test_VpcAssociationPolicy_NoMismatch_SetsProgrammedTrueAndAnnotation(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(nil), newVapTestGateway())
	e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).
		Return(deploy.VpcAssociationResult{Arn: vapTestSnvaArn}, nil)

	result, err := e.reconcile(t)
	assert.NoError(t, err)
	assert.Equal(t, time.Duration(0), result.RequeueAfter)

	vap := e.getPolicy(t)
	assert.Equal(t, vapTestSnvaArn, vap.Annotations["application-networking.k8s.aws/resourceArn"])
	accepted := meta.FindStatusCondition(vap.Status.Conditions, string(policy.ConditionTypeAccepted))
	if assert.NotNil(t, accepted) {
		assert.Equal(t, metav1.ConditionTrue, accepted.Status)
	}
	programmed := e.programmed(t)
	if assert.NotNil(t, programmed) {
		assert.Equal(t, metav1.ConditionTrue, programmed.Status)
		assert.Equal(t, string(anv1alpha1.VpcAssociationPolicyReasonProgrammed), programmed.Reason)
		assert.Equal(t, vap.Generation, programmed.ObservedGeneration)
		assert.Contains(t, programmed.Message, vapTestSnvaArn)
	}
	assert.Empty(t, e.drainEvents())
}

func Test_VpcAssociationPolicy_PrivateDnsMismatch_SetsProgrammedFalse_EmitsWarningOnce(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(func(spec *anv1alpha1.VpcAssociationPolicySpec) {
		spec.PrivateDnsEnabled = aws.Bool(true)
	}), newVapTestGateway())
	e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).
		Return(deploy.VpcAssociationResult{
			Arn:         vapTestSnvaArn,
			RamShared:   true,
			DnsMismatch: &deploy.VpcAssociationDnsMismatch{Field: "privateDnsEnabled", Policy: "true", Actual: "false"},
		}, nil).Times(2)

	for i := 0; i < 2; i++ {
		result, err := e.reconcile(t)
		assert.NoError(t, err, "a mismatch is reported through status, not as a reconcile error")
		assert.Equal(t, time.Duration(0), result.RequeueAfter)
	}

	vap := e.getPolicy(t)
	assert.Equal(t, vapTestSnvaArn, vap.Annotations["application-networking.k8s.aws/resourceArn"], "the association ARN is still recorded")
	programmed := e.programmed(t)
	if assert.NotNil(t, programmed) {
		assert.Equal(t, metav1.ConditionFalse, programmed.Status)
		assert.Equal(t, string(anv1alpha1.VpcAssociationPolicyReasonPrivateDnsMismatch), programmed.Reason)
		assert.Contains(t, programmed.Message, "privateDnsEnabled (policy: true, association: false)")
		assert.Contains(t, programmed.Message, "out of band")
	}
	events := e.drainEvents()
	if assert.Len(t, events, 1, "the warning is emitted only when the condition changes") {
		assert.True(t, strings.HasPrefix(events[0], "Warning "+k8s.VpcAssociationPolicyEventReasonPrivateDnsMismatch), events[0])
	}
}

func Test_VpcAssociationPolicy_MismatchResolved_FlipsProgrammedBackToTrue(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(func(spec *anv1alpha1.VpcAssociationPolicySpec) {
		spec.PrivateDnsEnabled = aws.Bool(true)
	}), newVapTestGateway())
	gomock.InOrder(
		e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).
			Return(deploy.VpcAssociationResult{
				Arn:         vapTestSnvaArn,
				RamShared:   true,
				DnsMismatch: &deploy.VpcAssociationDnsMismatch{Field: "privateDnsEnabled", Policy: "true", Actual: "false"},
			}, nil),
		e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).
			Return(deploy.VpcAssociationResult{Arn: vapTestSnvaArn + "-new"}, nil),
	)

	_, err := e.reconcile(t)
	assert.NoError(t, err)
	_, err = e.reconcile(t)
	assert.NoError(t, err)

	programmed := e.programmed(t)
	if assert.NotNil(t, programmed) {
		assert.Equal(t, metav1.ConditionTrue, programmed.Status)
		assert.Contains(t, programmed.Message, vapTestSnvaArn+"-new")
	}
	assert.Len(t, e.drainEvents(), 1)
}

func Test_VpcAssociationPolicy_RetryError_SetsProgrammedPending_NoAnnotation(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(nil), newVapTestGateway())
	e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).
		Return(deploy.VpcAssociationResult{Arn: vapTestSnvaArn},
			fmt.Errorf("%w, vpc association status in CREATE_IN_PROGRESS", lattice_runtime.NewRetryError()))

	result, err := e.reconcile(t)
	assert.NoError(t, err)
	assert.Equal(t, 10*time.Second, result.RequeueAfter)

	vap := e.getPolicy(t)
	assert.NotContains(t, vap.Annotations, "application-networking.k8s.aws/resourceArn")
	programmed := e.programmed(t)
	if assert.NotNil(t, programmed) {
		assert.Equal(t, metav1.ConditionUnknown, programmed.Status)
		assert.Equal(t, string(anv1alpha1.VpcAssociationPolicyReasonPending), programmed.Reason)
		assert.Contains(t, programmed.Message, "become active")
	}
	assert.Empty(t, e.drainEvents())
}

func Test_VpcAssociationPolicy_ValidationError_SetsReconcileFailed_DeterministicMessage(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(nil), newVapTestGateway())
	for _, requestId := range []string{"req-1", "req-2"} {
		e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).
			Return(deploy.VpcAssociationResult{}, fmt.Errorf("operation error VPC Lattice: CreateServiceNetworkVpcAssociation, RequestID: %s, %w",
				requestId, &vpclatticetypes.ValidationException{Message: aws.String("privateDnsSpecifiedDomains must not be empty")}))
	}

	for i := 0; i < 2; i++ {
		result, err := e.reconcile(t)
		assert.Error(t, err, "real failures keep the exponential backoff behaviour")
		assert.Equal(t, time.Duration(0), result.RequeueAfter)
	}

	programmed := e.programmed(t)
	if assert.NotNil(t, programmed) {
		assert.Equal(t, metav1.ConditionFalse, programmed.Status)
		assert.Equal(t, string(anv1alpha1.VpcAssociationPolicyReasonReconcileFailed), programmed.Reason)
		assert.Equal(t, "ValidationException: privateDnsSpecifiedDomains must not be empty", programmed.Message)
	}
	events := e.drainEvents()
	if assert.Len(t, events, 1, "identical failures across retries emit one event") {
		assert.True(t, strings.HasPrefix(events[0], "Warning "+k8s.FailedReconcileEvent), events[0])
	}
}

func Test_VpcAssociationPolicy_ServiceNetworkNotFound_SetsProgrammedPending(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(nil), newVapTestGateway())
	e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).
		Return(deploy.VpcAssociationResult{}, services.NewNotFoundError("ServiceNetwork", vapTestGateway))

	_, err := e.reconcile(t)
	assert.Error(t, err)

	programmed := e.programmed(t)
	if assert.NotNil(t, programmed) {
		assert.Equal(t, metav1.ConditionUnknown, programmed.Status)
		assert.Equal(t, string(anv1alpha1.VpcAssociationPolicyReasonPending), programmed.Reason)
	}
	assert.Empty(t, e.drainEvents())
}

func Test_VpcAssociationPolicy_AssociateWithVpcFalse_SetsProgrammedTrueAndAccepted(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(func(spec *anv1alpha1.VpcAssociationPolicySpec) {
		spec.AssociateWithVpc = aws.Bool(false)
		spec.PrivateDnsEnabled = aws.Bool(true)
	}), newVapTestGateway())
	e.manager.EXPECT().DeleteVpcAssociation(gomock.Any(), vapTestGateway).Return(nil)

	_, err := e.reconcile(t)
	assert.NoError(t, err)

	vap := e.getPolicy(t)
	accepted := meta.FindStatusCondition(vap.Status.Conditions, string(policy.ConditionTypeAccepted))
	if assert.NotNil(t, accepted) {
		assert.Equal(t, metav1.ConditionTrue, accepted.Status)
	}
	programmed := e.programmed(t)
	if assert.NotNil(t, programmed) {
		assert.Equal(t, metav1.ConditionTrue, programmed.Status)
		assert.Equal(t, string(anv1alpha1.VpcAssociationPolicyReasonProgrammed), programmed.Reason)
		assert.Contains(t, programmed.Message, "No VPC association is managed")
	}
}

func Test_VpcAssociationPolicy_AssociateWithVpcFalse_DeleteInProgress_SetsPending(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(func(spec *anv1alpha1.VpcAssociationPolicySpec) {
		spec.AssociateWithVpc = aws.Bool(false)
	}), newVapTestGateway())
	e.manager.EXPECT().DeleteVpcAssociation(gomock.Any(), vapTestGateway).Return(lattice_runtime.NewRetryError())

	result, err := e.reconcile(t)
	assert.NoError(t, err)
	assert.Equal(t, 10*time.Second, result.RequeueAfter)

	programmed := e.programmed(t)
	if assert.NotNil(t, programmed) {
		assert.Equal(t, metav1.ConditionUnknown, programmed.Status)
		assert.Equal(t, string(anv1alpha1.VpcAssociationPolicyReasonPending), programmed.Reason)
		assert.Contains(t, programmed.Message, "deleted")
	}
}

func Test_VpcAssociationPolicy_NotAccepted_SetsProgrammedUnknown(t *testing.T) {
	// No Gateway object: the policy is not accepted (TargetNotFound) and the manager must not be called.
	e := newVapTestEnv(t, newVapTestPolicy(nil))

	_, err := e.reconcile(t)
	assert.NoError(t, err)

	vap := e.getPolicy(t)
	accepted := meta.FindStatusCondition(vap.Status.Conditions, string(policy.ConditionTypeAccepted))
	if assert.NotNil(t, accepted) {
		assert.Equal(t, metav1.ConditionFalse, accepted.Status)
		assert.Equal(t, string(policy.ReasonTargetNotFound), accepted.Reason)
	}
	programmed := e.programmed(t)
	if assert.NotNil(t, programmed) {
		assert.Equal(t, metav1.ConditionUnknown, programmed.Status)
		assert.Equal(t, string(anv1alpha1.VpcAssociationPolicyReasonNotAccepted), programmed.Reason)
		assert.Contains(t, programmed.Message, string(policy.ReasonTargetNotFound))
	}
}

func Test_dnsConfigFromSpec(t *testing.T) {
	tests := []struct {
		name string
		spec anv1alpha1.VpcAssociationPolicySpec
		want deploy.VpcAssociationDnsConfig
	}{
		{name: "unset", spec: anv1alpha1.VpcAssociationPolicySpec{}, want: deploy.VpcAssociationDnsConfig{}},
		{name: "explicitly disabled", spec: anv1alpha1.VpcAssociationPolicySpec{PrivateDnsEnabled: aws.Bool(false)},
			want: deploy.VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(false)}},
		{name: "enabled without options", spec: anv1alpha1.VpcAssociationPolicySpec{PrivateDnsEnabled: aws.Bool(true)},
			want: deploy.VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(true)}},
		{name: "enabled with empty options", spec: anv1alpha1.VpcAssociationPolicySpec{PrivateDnsEnabled: aws.Bool(true), DnsOptions: &anv1alpha1.VpcAssociationDnsOptions{}},
			want: deploy.VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(true)}},
		{name: "enabled with preference and domains", spec: anv1alpha1.VpcAssociationPolicySpec{
			PrivateDnsEnabled: aws.Bool(true),
			DnsOptions: &anv1alpha1.VpcAssociationDnsOptions{
				PrivateDnsPreference:       ptrTo(anv1alpha1.PrivateDnsPreferenceVerifiedDomainsAndSpecifiedDomains),
				PrivateDnsSpecifiedDomains: []anv1alpha1.PrivateDnsDomain{"a.example.com", "*.b.example.com"},
			}},
			want: deploy.VpcAssociationDnsConfig{
				PrivateDnsEnabled:          aws.Bool(true),
				PrivateDnsPreference:       "VERIFIED_DOMAINS_AND_SPECIFIED_DOMAINS",
				PrivateDnsSpecifiedDomains: []string{"a.example.com", "*.b.example.com"},
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, dnsConfigFromSpec(tt.spec))
		})
	}
}

func Test_deterministicErrorMessage(t *testing.T) {
	wrapped := fmt.Errorf("operation error VPC Lattice: CreateServiceNetworkVpcAssociation, https response error StatusCode: 400, RequestID: abc, %w",
		&vpclatticetypes.ValidationException{Message: aws.String("bad input")})
	assert.Equal(t, "ValidationException: bad input", deterministicErrorMessage(wrapped))
	assert.Equal(t, "plain", deterministicErrorMessage(fmt.Errorf("plain")))
}

func Test_isTransientError(t *testing.T) {
	assert.True(t, isTransientError(services.NewNotFoundError("ServiceNetwork", "x")))
	assert.True(t, isTransientError(fmt.Errorf("wrapped: %w", &vpclatticetypes.ThrottlingException{Message: aws.String("slow down")})))
	assert.True(t, isTransientError(&vpclatticetypes.InternalServerException{Message: aws.String("oops")}))
	assert.False(t, isTransientError(&vpclatticetypes.ValidationException{Message: aws.String("bad")}))
	assert.False(t, isTransientError(services.NewConflictError("snva", "x", "not owned")))
}

func ptrTo[T any](v T) *T {
	return &v
}

func Test_VpcAssociationPolicy_RamSharedNoMismatch_MessageSaysSecurityGroupsUnmanaged(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(func(spec *anv1alpha1.VpcAssociationPolicySpec) {
		spec.SecurityGroupIds = []anv1alpha1.SecurityGroupId{"sg-123456789"}
	}), newVapTestGateway())
	e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).
		Return(deploy.VpcAssociationResult{Arn: vapTestSnvaArn, RamShared: true}, nil)

	_, err := e.reconcile(t)
	assert.NoError(t, err)
	programmed := e.programmed(t)
	if assert.NotNil(t, programmed) {
		assert.Equal(t, metav1.ConditionTrue, programmed.Status)
		assert.Contains(t, programmed.Message, "RAM-shared")
		assert.NotContains(t, programmed.Message, "matches the policy")
	}
}

func Test_VpcAssociationPolicy_TransientError_KeepsProgrammedTrue(t *testing.T) {
	e := newVapTestEnv(t, newVapTestPolicy(nil), newVapTestGateway())
	gomock.InOrder(
		e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).
			Return(deploy.VpcAssociationResult{Arn: vapTestSnvaArn}, nil),
		e.manager.EXPECT().UpsertVpcAssociation(gomock.Any(), vapTestGateway, gomock.Any()).
			Return(deploy.VpcAssociationResult{}, fmt.Errorf("wrapped: %w", &vpclatticetypes.ThrottlingException{Message: aws.String("slow down")})),
	)

	_, err := e.reconcile(t)
	assert.NoError(t, err)
	before := e.programmed(t)
	_, err = e.reconcile(t)
	assert.Error(t, err, "the throttling error still drives the backoff")

	after := e.programmed(t)
	if assert.NotNil(t, before) && assert.NotNil(t, after) {
		assert.Equal(t, metav1.ConditionTrue, after.Status, "a healthy association is not flipped to Pending by a transient error")
		assert.Equal(t, before.LastTransitionTime, after.LastTransitionTime)
	}
	assert.Empty(t, e.drainEvents())
}

func Test_VpcAssociationPolicy_AssociateWithVpcFalse_NotAccepted_DoesNotDelete(t *testing.T) {
	// No Gateway object: the policy is not accepted, so it must not delete an association.
	e := newVapTestEnv(t, newVapTestPolicy(func(spec *anv1alpha1.VpcAssociationPolicySpec) {
		spec.AssociateWithVpc = aws.Bool(false)
	}))
	e.manager.EXPECT().DeleteVpcAssociation(gomock.Any(), gomock.Any()).Times(0)

	_, err := e.reconcile(t)
	assert.NoError(t, err)

	programmed := e.programmed(t)
	if assert.NotNil(t, programmed) {
		assert.Equal(t, metav1.ConditionUnknown, programmed.Status)
		assert.Equal(t, string(anv1alpha1.VpcAssociationPolicyReasonNotAccepted), programmed.Reason)
	}
}

func Test_VpcAssociationPolicy_Deleting_DeletesAssociationWithoutStatusWrites(t *testing.T) {
	now := metav1.Now()
	vap := newVapTestPolicy(nil)
	vap.DeletionTimestamp = &now
	vap.Finalizers = []string{finalizer}
	e := newVapTestEnv(t, vap, newVapTestGateway())
	gomock.InOrder(
		e.manager.EXPECT().DeleteVpcAssociation(gomock.Any(), vapTestGateway).Return(lattice_runtime.NewRetryError()),
		e.manager.EXPECT().DeleteVpcAssociation(gomock.Any(), vapTestGateway).Return(nil),
	)

	result, err := e.reconcile(t)
	assert.NoError(t, err)
	assert.Equal(t, 10*time.Second, result.RequeueAfter, "the in-progress deletion is polled")
	assert.Nil(t, e.programmed(t), "a policy being deleted gets no status writes")

	_, err = e.reconcile(t)
	assert.NoError(t, err)
	assert.Nil(t, e.programmed(t))
	assert.Empty(t, e.drainEvents())
}
