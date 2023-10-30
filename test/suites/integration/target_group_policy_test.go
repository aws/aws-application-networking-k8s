package integration

import (
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var _ = Describe("Target Group Policy Tests", Ordered, func() {
	var (
		deployment *appsv1.Deployment
		service    *corev1.Service
		httpRoute  *gwv1beta1.HTTPRoute
		policy     *v1alpha1.TargetGroupPolicy
	)

	BeforeAll(func() {
		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "target-group-policy-test",
			Namespace: k8snamespace,
		})

		httpRoute = testFramework.NewHttpRoute(testGateway, service, service.Kind)

		testFramework.ExpectCreated(ctx, deployment, service, httpRoute)
	})

	It("Update Protocol creates new Target Group", func() {
		policy = testFramework.CreateTargetGroupPolicy(service, &test.TargetGroupPolicyConfig{
			PolicyName: "test-policy",
			Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
		})

		testFramework.ExpectCreated(ctx, policy)

		tg := testFramework.GetTargetGroup(ctx, service)

		Expect(*tg.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttp))

		testFramework.UpdateTargetGroupPolicy(policy, &test.TargetGroupPolicyConfig{
			Protocol: aws.String(vpclattice.TargetGroupProtocolHttps),
		})

		testFramework.ExpectUpdated(ctx, policy)

		httpsTG := testFramework.GetTargetGroupWithProtocol(ctx, service, "https", "http1")

		Expect(*httpsTG.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttps))
	})

	It("Delete Target Group Policy reset health check config for HTTP and HTTP1 Target Group", func() {
		policy = testFramework.CreateTargetGroupPolicy(service, &test.TargetGroupPolicyConfig{
			PolicyName:      "test-policy",
			Protocol:        aws.String(vpclattice.TargetGroupProtocolHttp),
			ProtocolVersion: aws.String(vpclattice.TargetGroupProtocolVersionHttp1),
			HealthCheck: &v1alpha1.HealthCheckConfig{
				IntervalSeconds: aws.Int64(7),
				StatusMatch:     aws.String("200,204"),
			},
		})

		testFramework.ExpectCreated(ctx, policy)

		// time.Sleep(10 * time.Second)

		Eventually(func(g Gomega) {
			tgSummary := testFramework.GetTargetGroup(ctx, service)

			tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)

			g.Expect(*tg.Config.HealthCheck.ProtocolVersion).To(Equal(vpclattice.TargetGroupProtocolVersionHttp1))
			g.Expect(*tg.Config.HealthCheck.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttp))
			g.Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(7))
			g.Expect(*tg.Config.HealthCheck.Matcher.HttpCode).To(Equal("200,204"))
		}).Within(10 * time.Second).WithPolling(1 * time.Second).Should(Succeed())

		testFramework.ExpectDeletedThenNotFound(ctx, policy)

		Eventually(func(g Gomega) {
			tgSummary := testFramework.GetTargetGroup(ctx, service)

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
		}).Within(10 * time.Second).WithPolling(1 * time.Second).Should(Succeed())
	})

	It("Delete Target Group Policy create HTTP and HTTP1 Target Group", func() {
		policy = testFramework.CreateTargetGroupPolicy(service, &test.TargetGroupPolicyConfig{
			PolicyName:      "test-policy",
			Protocol:        aws.String(vpclattice.TargetGroupProtocolHttps),
			ProtocolVersion: aws.String(vpclattice.TargetGroupProtocolVersionHttp2),
		})

		testFramework.ExpectCreated(ctx, policy)

		tg := testFramework.GetTargetGroupWithProtocol(ctx, service, "https", "http2")

		Expect(*tg.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttps))

		testFramework.ExpectDeleted(ctx, policy)

		httpTG := testFramework.GetTargetGroup(ctx, service)

		Expect(*httpTG.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttp))
	})

	AfterEach(func() {
		testFramework.ExpectDeleted(
			ctx,
			policy,
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
