package integration

import (
	"context"
	"time"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/controllers"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	apimachineryv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var _ = Describe("IAM Auth Policy", Ordered, func() {

	const (
		AllowAllInvoke = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"vpc-lattice-svcs:Invoke","Resource":"*"}]}`
		NoPolicy       = ""
		SvcName        = "iam-auth-http"
	)

	var (
		log     = testFramework.Log.Named("iam-auth-policy")
		ctx     = context.Background()
		lattice = testFramework.LatticeClient
	)

	newPolicyWithGroup := func(name, trGroup, trKind, trName string) *anv1alpha1.IAMAuthPolicy {
		p := &anv1alpha1.IAMAuthPolicy{
			Spec: anv1alpha1.IAMAuthPolicySpec{
				Policy: AllowAllInvoke,
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group: gwv1.Group(trGroup),
					Kind:  gwv1.Kind(trKind),
					Name:  gwv1.ObjectName(trName),
				},
			},
		}
		p.Name = name
		p.Namespace = k8snamespace
		testFramework.Create(ctx, p)
		return p
	}

	newPolicy := func(name, trKind, trName string) *anv1alpha1.IAMAuthPolicy {
		return newPolicyWithGroup(name, gwv1.GroupName, trKind, trName)
	}

	type K8sResults struct {
		statusReason      gwv1alpha2.PolicyConditionReason
		annotationResType string
		annotationResId   string
	}

	testK8sPolicy := func(policy *anv1alpha1.IAMAuthPolicy, wantResults K8sResults) {
		Eventually(func(g Gomega) (K8sResults, error) {
			p := &anv1alpha1.IAMAuthPolicy{}
			err := testFramework.Client.Get(ctx, client.ObjectKeyFromObject(policy), p)
			if err != nil {
				return K8sResults{}, err
			}
			return K8sResults{
				statusReason:      GetPolicyStatusReason(p),
				annotationResType: p.Annotations[controllers.IAMAuthPolicyAnnotationType],
				annotationResId:   p.Annotations[controllers.IAMAuthPolicyAnnotationResId],
			}, nil
		}).WithTimeout(30 * time.Second).WithPolling(time.Second).
			Should(Equal(wantResults))
	}

	testLatticePolicy := func(resId, policy string) {
		out, _ := lattice.GetAuthPolicy(&vpclattice.GetAuthPolicyInput{
			ResourceIdentifier: &resId,
		})
		Expect(aws.StringValue(out.Policy)).To(Equal(policy))
	}

	testLatticeSnPolicy := func(snId, authType, jsonPolicy string) {
		sn, _ := lattice.GetServiceNetwork(&vpclattice.GetServiceNetworkInput{
			ServiceNetworkIdentifier: &snId,
		})
		Expect(aws.StringValue(sn.AuthType)).To(Equal(authType))
		testLatticePolicy(snId, jsonPolicy)
	}

	testLatticeSvcPolicy := func(svcId, authType, jsonPolicy string) {
		svc, _ := lattice.GetService(&vpclattice.GetServiceInput{
			ServiceIdentifier: &svcId,
		})
		Expect(*svc.AuthType).To(Equal(authType))
		testLatticePolicy(svcId, jsonPolicy)
	}

	var (
		httpDep   *appsv1.Deployment
		httpSvc   *corev1.Service
		httpRoute *gwv1.HTTPRoute
	)

	BeforeAll(func() {
		httpDep, httpSvc = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      SvcName,
			Namespace: k8snamespace,
		})
		httpRoute = testFramework.NewHttpRoute(testGateway, httpSvc, "Service")
		testFramework.ExpectCreated(ctx, httpDep, httpSvc, httpRoute)
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx, httpDep, httpSvc, httpRoute)
		testFramework.ExpectDeleteAllToSucceed(ctx, &anv1alpha1.IAMAuthPolicy{}, k8snamespace)
	})

	It("GroupName Error", func() {
		policy := newPolicyWithGroup("group-name-err", "wrong.group", "Gateway", "gw")
		testK8sPolicy(policy, K8sResults{statusReason: gwv1alpha2.PolicyReasonInvalid})
		testFramework.Delete(ctx, policy)
	})

	It("Kind Error", func() {
		policy := newPolicy("kind-err", "WrongKind", "gw")
		testK8sPolicy(policy, K8sResults{statusReason: gwv1alpha2.PolicyReasonInvalid})
		testFramework.Delete(ctx, policy)
	})

	It("TargetRef Not Found", func() {
		policy := newPolicy("not-found", "Gateway", "not-found")
		testK8sPolicy(policy, K8sResults{statusReason: gwv1alpha2.PolicyReasonTargetNotFound})
		testFramework.Delete(ctx, policy)
	})

	It("TargetRef Conflict", func() {
		policy1 := newPolicy("conflict-1", "Gateway", "test-gateway")
		policy2 := newPolicy("conflict-2", "Gateway", "test-gateway")
		// at least second policy should be in conflicted state
		testK8sPolicy(policy2, K8sResults{statusReason: gwv1alpha2.PolicyReasonConflicted})
		testFramework.ExpectDeletedThenNotFound(ctx, policy1, policy2)
	})

	It("accepted, applied, and removed from Gateway", func() {
		policy := newPolicy("gw", "Gateway", "test-gateway")
		sn, _ := lattice.FindServiceNetwork(context.TODO(), "test-gateway", "")
		snId := *sn.SvcNetwork.Id

		// accepted
		wantResults := K8sResults{
			statusReason:      gwv1alpha2.PolicyReasonAccepted,
			annotationResType: model.ServiceNetworkType,
			annotationResId:   snId,
		}
		testK8sPolicy(policy, wantResults)
		log.Infof("policy accepted: %+v", wantResults)

		// applied
		testLatticeSnPolicy(snId, vpclattice.AuthTypeAwsIam, policy.Spec.Policy)
		log.Infof("policy applied for SN=%s", snId)

		// removed
		testFramework.ExpectDeletedThenNotFound(ctx, policy)
		testLatticeSnPolicy(snId, vpclattice.AuthTypeNone, NoPolicy)
		log.Infof("policy removed from SN=%s", snId)
	})

	It("accepted, applied, and removed from HTTPRoute", func() {
		policy := newPolicy("http", "HTTPRoute", SvcName)
		svc := testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(gwv1beta1.HTTPRoute(*httpRoute)))
		svcId := *svc.Id

		// accepted
		wantResults := K8sResults{
			statusReason:      gwv1alpha2.PolicyReasonAccepted,
			annotationResType: model.ServiceType,
			annotationResId:   svcId,
		}
		testK8sPolicy(policy, wantResults)
		log.Infof("policy accepted: %+v", wantResults)

		// applied
		testLatticeSvcPolicy(svcId, vpclattice.AuthTypeAwsIam, policy.Spec.Policy)
		log.Infof("policy applied for Svc=%s", svcId)

		// removed
		testFramework.ExpectDeletedThenNotFound(ctx, policy)
		testLatticeSvcPolicy(svcId, vpclattice.AuthTypeNone, NoPolicy)
		log.Infof("policy removed from Svc=%s", svcId)
	})
})

type StatusConditionsReader interface {
	GetStatusConditions() *[]apimachineryv1.Condition
}

func GetPolicyStatusReason(obj StatusConditionsReader) gwv1alpha2.PolicyConditionReason {
	cnd := meta.FindStatusCondition(*obj.GetStatusConditions(), string(gwv1alpha2.PolicyConditionAccepted))
	if cnd != nil {
		return gwv1alpha2.PolicyConditionReason(cnd.Reason)
	}
	return ""
}
