package integration

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
)

var _ = Describe("Target Group Policy Tests", Ordered, func() {
	var (
		deployment *appsv1.Deployment
		service    *corev1.Service
		httpRoute  *gwv1.HTTPRoute
		policy     *anv1alpha1.TargetGroupPolicy
	)

	BeforeAll(func() {
		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "target-group-policy-test",
			Namespace: k8snamespace,
		})

		httpRoute = testFramework.NewHttpRoute(testGateway, service, service.Kind)

		testFramework.ExpectCreated(ctx, deployment, service, httpRoute)
	})

	It("Update Protocol replaces the Target Group with new one", func() {
		policy = createTargetGroupPolicy(service, &TargetGroupPolicyConfig{
			PolicyName: "test-policy",
			Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
		})

		testFramework.ExpectCreated(ctx, policy)

		tg := testFramework.GetHttpTargetGroup(ctx, service)
		Expect(*tg.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttp))

		err := testFramework.Client.Get(ctx, client.ObjectKeyFromObject(policy), policy)
		Expect(err).Should(BeNil())

		policy.Spec.Protocol = aws.String(vpclattice.TargetGroupProtocolHttps)
		err = testFramework.Client.Update(ctx, policy)
		Expect(err).Should(BeNil())

		testFramework.VerifyTargetGroupNotFound(tg)

		httpsTG := testFramework.GetTargetGroupWithProtocol(ctx, service, "https", "http1")

		Expect(*httpsTG.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttps))
	})

	It("Delete Target Group Policy reset health check config for HTTP and HTTP1 Target Group", func() {
		policy = createTargetGroupPolicy(service, &TargetGroupPolicyConfig{
			PolicyName:      "test-policy",
			Protocol:        aws.String(vpclattice.TargetGroupProtocolHttp),
			ProtocolVersion: aws.String(vpclattice.TargetGroupProtocolVersionHttp1),
			HealthCheck: &anv1alpha1.HealthCheckConfig{
				IntervalSeconds: aws.Int64(7),
				StatusMatch:     aws.String("200,204"),
			},
		})

		testFramework.ExpectCreated(ctx, policy)

		Eventually(func(g Gomega) {
			tgSummary := testFramework.GetHttpTargetGroup(ctx, service)
			tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)
			g.Expect(*tg.Config.HealthCheck.ProtocolVersion).To(Equal(vpclattice.TargetGroupProtocolVersionHttp1))
			g.Expect(*tg.Config.HealthCheck.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttp))
			g.Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(7))
			g.Expect(*tg.Config.HealthCheck.Matcher.HttpCode).To(Equal("200,204"))
		}).Within(60 * time.Second).WithPolling(1 * time.Second).Should(Succeed())

		testFramework.ExpectDeletedThenNotFound(ctx, policy)

		Eventually(func(g Gomega) {
			tgSummary := testFramework.GetHttpTargetGroup(ctx, service)

			tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)

			g.Expect(*tg.Config.HealthCheck).To(Equal(vpclattice.HealthCheckConfig{
				Enabled:                    aws.Bool(true),
				Path:                       aws.String("/"),
				HealthCheckIntervalSeconds: aws.Int64(30),
				HealthCheckTimeoutSeconds:  aws.Int64(5),
				HealthyThresholdCount:      aws.Int64(5),
				UnhealthyThresholdCount:    aws.Int64(2),
				Protocol:                   aws.String(vpclattice.TargetGroupProtocolHttp),
				ProtocolVersion:            aws.String(vpclattice.TargetGroupProtocolVersionHttp1),
				Port:                       nil,
				Matcher: &vpclattice.Matcher{
					HttpCode: aws.String("200"),
				},
			}))
		}).Within(60 * time.Second).WithPolling(1 * time.Second).Should(Succeed())
	})

	It("Delete Target Group Policy create HTTP and HTTP1 Target Group", func() {
		policy = createTargetGroupPolicy(service, &TargetGroupPolicyConfig{
			PolicyName:      "test-policy",
			Protocol:        aws.String(vpclattice.TargetGroupProtocolHttps),
			ProtocolVersion: aws.String(vpclattice.TargetGroupProtocolVersionHttp2),
		})

		testFramework.ExpectCreated(ctx, policy)

		tg := testFramework.GetTargetGroupWithProtocol(ctx, service, "https", "http2")

		Expect(*tg.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttps))

		testFramework.ExpectDeleted(ctx, policy)

		httpTG := testFramework.GetHttpTargetGroup(ctx, service)

		Expect(*httpTG.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttp))
	})

	AfterEach(func() {
		testFramework.ExpectDeleted(
			ctx,
			&anv1alpha1.TargetGroupPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      policy.Name,
					Namespace: policy.Namespace,
				},
			},
		)
	})

	AfterAll(func() {
		testFramework.ExpectDeleted(
			ctx,
			deployment,
			service,
			httpRoute,
		)
	})
})

type TargetGroupPolicyConfig struct {
	PolicyName      string
	Protocol        *string
	ProtocolVersion *string
	HealthCheck     *anv1alpha1.HealthCheckConfig
}

func createTargetGroupPolicy(
	service *corev1.Service,
	config *TargetGroupPolicyConfig,
) *anv1alpha1.TargetGroupPolicy {
	return &anv1alpha1.TargetGroupPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind: "TargetGroupPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: service.Namespace,
			Name:      config.PolicyName,
		},
		Spec: anv1alpha1.TargetGroupPolicySpec{
			TargetRef: &gwv1alpha2.PolicyTargetReference{
				Kind: gwv1beta1.Kind("Service"),
				Name: gwv1beta1.ObjectName(service.Name),
			},
			Protocol:        config.Protocol,
			ProtocolVersion: config.ProtocolVersion,
			HealthCheck:     config.HealthCheck,
		},
	}
}
