package integration

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/controllers"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"

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
				TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
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
		httpDep                                 *appsv1.Deployment
		httpSvc                                 *corev1.Service
		httpRoute                               *gwv1.HTTPRoute
		httpDepWithServiceNameOverride          *appsv1.Deployment
		httpSvcWithServiceNameOverride          *corev1.Service
		httpRouteWithServiceNameOverride        *gwv1.HTTPRoute
		httpDepWithInvalidServiceNameOverride   *appsv1.Deployment
		httpSvcWithInvalidServiceNameOverride   *corev1.Service
		httpRouteWithInvalidServiceNameOverride *gwv1.HTTPRoute
	)

	BeforeAll(func() {
		httpDep, httpSvc = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      SvcName,
			Namespace: k8snamespace,
		})
		httpRoute = testFramework.NewHttpRoute(testGateway, httpSvc, "Service")
		testFramework.ExpectCreated(ctx, httpDep, httpSvc, httpRoute)

		httpDepWithServiceNameOverride, httpSvcWithServiceNameOverride = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "iam-auth-override",
			Namespace: k8snamespace,
		})
		httpRouteWithServiceNameOverride = testFramework.NewHttpRoute(testGateway, httpSvcWithServiceNameOverride, "Service")
		httpRouteWithServiceNameOverride.Annotations = map[string]string{
			"application-networking.k8s.aws/service-name-override": "my-awesome-service",
		}
		testFramework.ExpectCreated(ctx, httpDepWithServiceNameOverride, httpSvcWithServiceNameOverride, httpRouteWithServiceNameOverride)

		httpDepWithInvalidServiceNameOverride, httpSvcWithInvalidServiceNameOverride = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "iam-auth-invalid",
			Namespace: k8snamespace,
		})
		httpRouteWithInvalidServiceNameOverride = testFramework.NewHttpRoute(testGateway, httpSvcWithInvalidServiceNameOverride, "Service")
		httpRouteWithInvalidServiceNameOverride.Annotations = map[string]string{
			"application-networking.k8s.aws/service-name-override": "svc-my-awesome-service",
		}
		testFramework.ExpectCreated(ctx, httpDepWithInvalidServiceNameOverride, httpSvcWithInvalidServiceNameOverride, httpRouteWithInvalidServiceNameOverride)
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			httpDep, httpSvc, httpRoute,
			httpDepWithServiceNameOverride, httpSvcWithServiceNameOverride, httpRouteWithServiceNameOverride,
			httpDepWithInvalidServiceNameOverride, httpSvcWithInvalidServiceNameOverride, httpRouteWithInvalidServiceNameOverride)
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
		sn, _ := lattice.FindServiceNetwork(context.TODO(), "test-gateway")
		snId := *sn.SvcNetwork.Id

		// accepted
		wantResults := K8sResults{
			statusReason:      gwv1alpha2.PolicyReasonAccepted,
			annotationResType: model.ServiceNetworkType,
			annotationResId:   snId,
		}
		testK8sPolicy(policy, wantResults)
		log.Infof(ctx, "policy accepted: %+v", wantResults)

		// applied
		testLatticeSnPolicy(snId, vpclattice.AuthTypeAwsIam, policy.Spec.Policy)
		log.Infof(ctx, "policy applied for SN=%s", snId)

		// removed
		testFramework.ExpectDeletedThenNotFound(ctx, policy)
		testLatticeSnPolicy(snId, vpclattice.AuthTypeNone, NoPolicy)
		log.Infof(ctx, "policy removed from SN=%s", snId)
	})

	It("accepted, applied, and removed from HTTPRoute", func() {
		policy := newPolicy("http", "HTTPRoute", SvcName)
		svc := testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(gwv1.HTTPRoute(*httpRoute)))
		svcId := *svc.Id

		// accepted
		wantResults := K8sResults{
			statusReason:      gwv1alpha2.PolicyReasonAccepted,
			annotationResType: model.ServiceType,
			annotationResId:   svcId,
		}
		testK8sPolicy(policy, wantResults)
		log.Infof(ctx, "policy accepted: %+v", wantResults)

		// applied
		testLatticeSvcPolicy(svcId, vpclattice.AuthTypeAwsIam, policy.Spec.Policy)
		log.Infof(ctx, "policy applied for Svc=%s", svcId)

		// removed
		testFramework.ExpectDeletedThenNotFound(ctx, policy)
		testLatticeSvcPolicy(svcId, vpclattice.AuthTypeNone, NoPolicy)
		log.Infof(ctx, "policy removed from Svc=%s", svcId)
	})

	It("removes IAM AuthPolicy from old HTTPRoute when targetRef changes to new HTTPRoute", func() {
		// Create second HTTPRoute for target change test
		secondHttpDep, secondHttpSvc := testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      SvcName + "-2",
			Namespace: k8snamespace,
		})
		secondHttpRoute := testFramework.NewHttpRoute(testGateway, secondHttpSvc, "Service")
		secondHttpRoute.Name = SvcName + "-2"
		testFramework.ExpectCreated(ctx, secondHttpSvc, secondHttpRoute)

		// Get VPC Lattice service IDs for both routes
		firstSvc := testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(gwv1.HTTPRoute(*httpRoute)))
		secondSvc := testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(gwv1.HTTPRoute(*secondHttpRoute)))
		firstSvcId := *firstSvc.Id
		secondSvcId := *secondSvc.Id

		// Create IAMAuthPolicy targeting first route
		policy := newPolicy("target-change", "HTTPRoute", SvcName)

		// Verify policy applied to first service
		wantResults := K8sResults{
			statusReason:      gwv1alpha2.PolicyReasonAccepted,
			annotationResType: model.ServiceType,
			annotationResId:   firstSvcId,
		}
		testK8sPolicy(policy, wantResults)
		testLatticeSvcPolicy(firstSvcId, vpclattice.AuthTypeAwsIam, policy.Spec.Policy)
		log.Infof(ctx, "policy initially applied for first Svc=%s", firstSvcId)

		// Change targetRef to second route
		err := testFramework.Client.Get(ctx, client.ObjectKeyFromObject(policy), policy)
		Expect(err).ToNot(HaveOccurred())
		policy.Spec.TargetRef.Name = gwv1.ObjectName(SvcName + "-2")
		testFramework.Update(ctx, policy)

		wantResults = K8sResults{
			statusReason:      gwv1alpha2.PolicyReasonAccepted,
			annotationResType: model.ServiceType,
			annotationResId:   secondSvcId,
		}
		testK8sPolicy(policy, wantResults)

		// Verify new service has auth policy
		testLatticeSvcPolicy(secondSvcId, vpclattice.AuthTypeAwsIam, policy.Spec.Policy)
		log.Infof(ctx, "policy moved to second Svc=%s", secondSvcId)

		// Verify auth policy removed from  old service
		testLatticeSvcPolicy(firstSvcId, vpclattice.AuthTypeNone, NoPolicy)
		log.Infof(ctx, "old policy cleaned up from first Svc=%s", firstSvcId)

		testFramework.ExpectDeletedThenNotFound(ctx, policy)
		testLatticeSvcPolicy(secondSvcId, vpclattice.AuthTypeNone, NoPolicy)
		testFramework.ExpectDeletedThenNotFound(ctx, secondHttpDep, secondHttpSvc, secondHttpRoute)
	})

	It("accepted, applied, and removed from HTTPRoute with service name override", func() {
		policy := newPolicy("http-override", "HTTPRoute", httpRouteWithServiceNameOverride.Name)
		svc := testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(*httpRouteWithServiceNameOverride))
		svcId := *svc.Id

		Expect(*svc.Name).To(Equal("my-awesome-service"))

		wantResults := K8sResults{
			statusReason:      gwv1alpha2.PolicyReasonAccepted,
			annotationResType: model.ServiceType,
			annotationResId:   svcId,
		}
		testK8sPolicy(policy, wantResults)

		testLatticeSvcPolicy(svcId, vpclattice.AuthTypeAwsIam, policy.Spec.Policy)

		testFramework.ExpectDeletedThenNotFound(ctx, policy)
		testLatticeSvcPolicy(svcId, vpclattice.AuthTypeNone, NoPolicy)
	})

	It("supports targetRef HTTPRoute change from invalid to valid service name override", func() {
		policy := newPolicy("recovery-test", "HTTPRoute", httpRouteWithInvalidServiceNameOverride.Name)

		testK8sPolicy(policy, K8sResults{statusReason: gwv1alpha2.PolicyReasonInvalid})

		err := testFramework.Client.Get(ctx, client.ObjectKeyFromObject(policy), policy)
		Expect(err).ToNot(HaveOccurred())
		policy.Spec.TargetRef.Name = gwv1.ObjectName(httpRouteWithServiceNameOverride.Name)
		testFramework.Update(ctx, policy)

		svc := testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(*httpRouteWithServiceNameOverride))
		svcId := *svc.Id
		Expect(*svc.Name).To(Equal("my-awesome-service"))

		wantResults := K8sResults{
			statusReason:      gwv1alpha2.PolicyReasonAccepted,
			annotationResType: model.ServiceType,
			annotationResId:   svcId,
		}
		testK8sPolicy(policy, wantResults)
		testLatticeSvcPolicy(svcId, vpclattice.AuthTypeAwsIam, policy.Spec.Policy)

		testFramework.ExpectDeletedThenNotFound(ctx, policy)
		testLatticeSvcPolicy(svcId, vpclattice.AuthTypeNone, NoPolicy)
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
