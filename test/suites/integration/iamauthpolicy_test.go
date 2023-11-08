package integration

import (
	"context"
	"time"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	apimachineryv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("IAM Auth Policy", Ordered, func() {

	const Policy = `
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": "*",
      "Action": "vpc-lattice-svcs:Invoke",
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "vpc-lattice-svcs:RequestHeader/header1": "value1"
        }
      }
    }
  ]
}`

	var ctx = context.Background()

	newPolicyWithGroup := func(name, trGroup, trKind, trName string) anv1alpha1.IAMAuthPolicy {
		p := anv1alpha1.IAMAuthPolicy{
			Spec: anv1alpha1.IAMAuthPolicySpec{
				Policy: Policy,
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group: gwv1beta1.Group(trGroup),
					Kind:  gwv1beta1.Kind(trKind),
					Name:  gwv1beta1.ObjectName(trName),
				},
			},
		}
		p.Name = name
		p.Namespace = k8snamespace
		testFramework.Client.Create(ctx, &p)
		return p
	}

	newPolicy := func(name, trKind, trName string) anv1alpha1.IAMAuthPolicy {
		return newPolicyWithGroup(name, gwv1beta1.GroupName, trKind, trName)
	}

	testPolicyStatus := func(policy anv1alpha1.IAMAuthPolicy, wantStatus gwv1alpha2.PolicyConditionReason) {
		Eventually(func(g Gomega) string {
			p := &anv1alpha1.IAMAuthPolicy{}
			testFramework.Client.Get(ctx, client.ObjectKeyFromObject(&policy), p)
			return GetPolicyStatusReason(p)
		}).WithTimeout(10 * time.Second).WithPolling(time.Second).
			Should(Equal(string(wantStatus)))
	}

	var (
		httpDep   *appsv1.Deployment
		httpSvc   *corev1.Service
		httpRoute *gwv1beta1.HTTPRoute
	)

	BeforeAll(func() {
		httpDep, httpSvc = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "http-app",
			Namespace: k8snamespace,
		})
		httpRoute = testFramework.NewHttpRoute(testGateway, httpSvc, "Service")
		testFramework.ExpectCreated(ctx, httpDep, httpSvc, httpRoute)
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx, httpDep, httpSvc, httpRoute)
	})

	It("GroupName Error", func() {
		policy := newPolicyWithGroup("group-name-err", "wrong.group", "Gateway", "gw")
		testPolicyStatus(policy, gwv1alpha2.PolicyReasonInvalid)
		testFramework.Delete(ctx, &policy)
	})

	It("Kind Error", func() {
		policy := newPolicy("kind-err", "WrongKind", "gw")
		testPolicyStatus(policy, gwv1alpha2.PolicyReasonInvalid)
		testFramework.Delete(ctx, &policy)
	})

	It("TargetRef Not Found", func() {
		policy := newPolicy("not-found", "Gateway", "not-found")
		testPolicyStatus(policy, gwv1alpha2.PolicyReasonTargetNotFound)
		testFramework.Delete(ctx, &policy)
	})

	It("TargetRef Conflict", func() {
		policy1 := newPolicy("conflict-1", "Gateway", "test-gateway")
		policy2 := newPolicy("conflict-2", "Gateway", "test-gateway")
		// at least second policy should be in conflicted state
		testPolicyStatus(policy2, gwv1alpha2.PolicyReasonConflicted)
		testFramework.ExpectDeletedThenNotFound(ctx, &policy1, &policy2)
	})

	It("Accepted - Gateway", func() {
		policy := newPolicy("gw", "Gateway", "test-gateway")
		testPolicyStatus(policy, gwv1alpha2.PolicyReasonAccepted)
		testFramework.ExpectDeletedThenNotFound(ctx, &policy)
	})

	It("Accepted - HTTPRoute", func() {
		policy := newPolicy("http", "HTTPRoute", "http-app")
		testPolicyStatus(policy, gwv1alpha2.PolicyReasonAccepted)
		testFramework.ExpectDeletedThenNotFound(ctx, &policy)
	})
})

type StatusConditionsReader interface {
	GetStatusConditions() []apimachineryv1.Condition
}

func GetPolicyStatusReason(obj StatusConditionsReader) string {
	cnd := meta.FindStatusCondition(obj.GetStatusConditions(), string(gwv1alpha2.PolicyConditionAccepted))
	if cnd != nil {
		return cnd.Reason
	}
	return ""
}
