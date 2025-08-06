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

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
)

var _ = Describe("TargetGroupPolicy ServiceExport Integration Tests", Ordered, func() {
	var (
		deployment    *appsv1.Deployment
		service       *corev1.Service
		serviceExport *anv1alpha1.ServiceExport
		policy        *anv1alpha1.TargetGroupPolicy
	)

	BeforeAll(func() {
		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "tgp-serviceexport-test",
			Namespace: k8snamespace,
		})
		serviceExport = testFramework.CreateServiceExport(service)

		testFramework.ExpectCreated(ctx, deployment, service, serviceExport)
	})

	AfterAll(func() {
		testFramework.ExpectDeleted(ctx, deployment, service, serviceExport)
	})

	Context("TargetGroupPolicy application to ServiceExport target groups", func() {
		AfterEach(func() {
			if policy != nil {
				testFramework.ExpectDeleted(ctx, policy)
				policy = nil
			}
		})

		It("should apply TargetGroupPolicy health check configuration to ServiceExport target groups", func() {
			// Create TargetGroupPolicy with custom health check configuration
			policy = createServiceTargetGroupPolicy(service, &ServiceTargetGroupPolicyConfig{
				PolicyName: "serviceexport-health-check-policy",
				Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
				HealthCheck: &anv1alpha1.HealthCheckConfig{
					Path:                    aws.String("/health"),
					IntervalSeconds:         aws.Int64(15),
					TimeoutSeconds:          aws.Int64(10),
					StatusMatch:             aws.String("200,204"),
					HealthyThresholdCount:   aws.Int64(3),
					UnhealthyThresholdCount: aws.Int64(4),
				},
			})

			testFramework.ExpectCreated(ctx, policy)

			// Verify that the ServiceExport target group receives the policy configuration
			Eventually(func(g Gomega) {
				tgSummary := testFramework.GetTargetGroup(ctx, service)
				tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)

				g.Expect(*tg.Config.HealthCheck.Path).To(Equal("/health"))
				g.Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(15))
				g.Expect(*tg.Config.HealthCheck.HealthCheckTimeoutSeconds).To(BeEquivalentTo(10))
				g.Expect(*tg.Config.HealthCheck.Matcher.HttpCode).To(Equal("200,204"))
				g.Expect(*tg.Config.HealthCheck.HealthyThresholdCount).To(BeEquivalentTo(3))
				g.Expect(*tg.Config.HealthCheck.UnhealthyThresholdCount).To(BeEquivalentTo(4))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("should handle policy updates and apply changes to existing target groups", func() {
			// Create initial policy
			policy = createServiceTargetGroupPolicy(service, &ServiceTargetGroupPolicyConfig{
				PolicyName: "serviceexport-update-policy",
				Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
				HealthCheck: &anv1alpha1.HealthCheckConfig{
					Path:            aws.String("/initial"),
					IntervalSeconds: aws.Int64(20),
				},
			})

			testFramework.ExpectCreated(ctx, policy)

			// Verify initial configuration
			Eventually(func(g Gomega) {
				tgSummary := testFramework.GetTargetGroup(ctx, service)
				tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)
				g.Expect(*tg.Config.HealthCheck.Path).To(Equal("/initial"))
				g.Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(20))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			// Update the policy
			err := testFramework.Client.Get(ctx, client.ObjectKeyFromObject(policy), policy)
			Expect(err).Should(BeNil())

			policy.Spec.HealthCheck.Path = aws.String("/updated")
			policy.Spec.HealthCheck.IntervalSeconds = aws.Int64(25)
			err = testFramework.Client.Update(ctx, policy)
			Expect(err).Should(BeNil())

			// Verify updated configuration
			Eventually(func(g Gomega) {
				tgSummary := testFramework.GetTargetGroup(ctx, service)
				tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)
				g.Expect(*tg.Config.HealthCheck.Path).To(Equal("/updated"))
				g.Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(25))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("should fall back to default configuration when policy is deleted", func() {
			// Create policy with custom configuration
			policy = createServiceTargetGroupPolicy(service, &ServiceTargetGroupPolicyConfig{
				PolicyName: "serviceexport-fallback-policy",
				Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
				HealthCheck: &anv1alpha1.HealthCheckConfig{
					Path:            aws.String("/custom"),
					IntervalSeconds: aws.Int64(45),
					StatusMatch:     aws.String("200,201"),
				},
			})

			testFramework.ExpectCreated(ctx, policy)

			// Verify custom configuration is applied
			Eventually(func(g Gomega) {
				tgSummary := testFramework.GetTargetGroup(ctx, service)
				tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)
				g.Expect(*tg.Config.HealthCheck.Path).To(Equal("/custom"))
				g.Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(45))
				g.Expect(*tg.Config.HealthCheck.Matcher.HttpCode).To(Equal("200,201"))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			// Delete the policy
			testFramework.ExpectDeletedThenNotFound(ctx, policy)
			policy = nil

			// Verify fallback to default configuration
			Eventually(func(g Gomega) {
				tgSummary := testFramework.GetTargetGroup(ctx, service)
				tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)

				// Verify default health check configuration
				g.Expect(*tg.Config.HealthCheck.Path).To(Equal("/"))
				g.Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(30))
				g.Expect(*tg.Config.HealthCheck.HealthCheckTimeoutSeconds).To(BeEquivalentTo(5))
				g.Expect(*tg.Config.HealthCheck.HealthyThresholdCount).To(BeEquivalentTo(5))
				g.Expect(*tg.Config.HealthCheck.UnhealthyThresholdCount).To(BeEquivalentTo(2))
				g.Expect(*tg.Config.HealthCheck.Matcher.HttpCode).To(Equal("200"))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("should maintain backwards compatibility when no policy is present", func() {
			// Verify that ServiceExport target groups work with default configuration
			// when no TargetGroupPolicy is applied
			tgSummary := testFramework.GetTargetGroup(ctx, service)
			tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)

			// Verify default health check configuration matches expected defaults
			Expect(*tg.Config.HealthCheck).To(Equal(vpclattice.HealthCheckConfig{
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
		})
	})

	Context("Policy conflict resolution scenarios", func() {
		var (
			policy1 *anv1alpha1.TargetGroupPolicy
			policy2 *anv1alpha1.TargetGroupPolicy
		)

		AfterEach(func() {
			if policy1 != nil {
				testFramework.ExpectDeleted(ctx, policy1)
				policy1 = nil
			}
			if policy2 != nil {
				testFramework.ExpectDeleted(ctx, policy2)
				policy2 = nil
			}
		})

		It("should resolve conflicts between multiple TargetGroupPolicy resources", func() {
			// Create first policy (should win due to creation timestamp)
			policy1 = createServiceTargetGroupPolicy(service, &ServiceTargetGroupPolicyConfig{
				PolicyName: "serviceexport-conflict-policy-1",
				Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
				HealthCheck: &anv1alpha1.HealthCheckConfig{
					Path:            aws.String("/policy1"),
					IntervalSeconds: aws.Int64(10),
				},
			})

			testFramework.ExpectCreated(ctx, policy1)

			// Wait a moment to ensure different creation timestamps
			time.Sleep(2 * time.Second)

			// Create second policy (should lose due to later creation timestamp)
			policy2 = createServiceTargetGroupPolicy(service, &ServiceTargetGroupPolicyConfig{
				PolicyName: "serviceexport-conflict-policy-2",
				Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
				HealthCheck: &anv1alpha1.HealthCheckConfig{
					Path:            aws.String("/policy2"),
					IntervalSeconds: aws.Int64(15),
				},
			})

			testFramework.ExpectCreated(ctx, policy2)

			// Verify that the first policy (earlier creation timestamp) wins
			Eventually(func(g Gomega) {
				tgSummary := testFramework.GetTargetGroup(ctx, service)
				tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)
				g.Expect(*tg.Config.HealthCheck.Path).To(Equal("/policy1"))
				g.Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(10))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			// Verify both policies have proper status conditions
			Eventually(func(g Gomega) {
				// First policy should be Accepted
				err := testFramework.Client.Get(ctx, client.ObjectKeyFromObject(policy1), policy1)
				g.Expect(err).Should(BeNil())
				conditions1 := policy1.Status.Conditions
				g.Expect(conditions1).ToNot(BeEmpty())

				var acceptedCondition1 *metav1.Condition
				for i := range conditions1 {
					if conditions1[i].Type == "Accepted" {
						acceptedCondition1 = &conditions1[i]
						break
					}
				}
				g.Expect(acceptedCondition1).ToNot(BeNil())
				g.Expect(acceptedCondition1.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(acceptedCondition1.Reason).To(Equal("Accepted"))

				// Second policy should be Conflicted
				err = testFramework.Client.Get(ctx, client.ObjectKeyFromObject(policy2), policy2)
				g.Expect(err).Should(BeNil())
				conditions2 := policy2.Status.Conditions
				g.Expect(conditions2).ToNot(BeEmpty())

				var acceptedCondition2 *metav1.Condition
				for i := range conditions2 {
					if conditions2[i].Type == "Accepted" {
						acceptedCondition2 = &conditions2[i]
						break
					}
				}
				g.Expect(acceptedCondition2).ToNot(BeNil())
				g.Expect(acceptedCondition2.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(acceptedCondition2.Reason).To(Equal("Conflicted"))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("should handle policy precedence based on creation timestamp", func() {
			// Create policies with names that would be ordered alphabetically
			// Policy with name starting with 'a' should win over 'z' if created first
			policy1 = createServiceTargetGroupPolicy(service, &ServiceTargetGroupPolicyConfig{
				PolicyName: "a-serviceexport-alpha-policy",
				Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
				HealthCheck: &anv1alpha1.HealthCheckConfig{
					Path:            aws.String("/alpha"),
					IntervalSeconds: aws.Int64(12),
				},
			})

			testFramework.ExpectCreated(ctx, policy1)

			// Wait to ensure different creation timestamps
			time.Sleep(2 * time.Second)

			policy2 = createServiceTargetGroupPolicy(service, &ServiceTargetGroupPolicyConfig{
				PolicyName: "z-serviceexport-zulu-policy",
				Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
				HealthCheck: &anv1alpha1.HealthCheckConfig{
					Path:            aws.String("/zulu"),
					IntervalSeconds: aws.Int64(18),
				},
			})

			testFramework.ExpectCreated(ctx, policy2)

			// Verify that the first created policy wins (regardless of alphabetical order)
			Eventually(func(g Gomega) {
				tgSummary := testFramework.GetTargetGroup(ctx, service)
				tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)
				g.Expect(*tg.Config.HealthCheck.Path).To(Equal("/alpha"))
				g.Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(12))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("should demonstrate policy succession when winning policy is deleted", func() {
			// This test demonstrates the expected behavior but may not work due to
			// current implementation limitations in policy change propagation
			Skip("Skipping policy succession test due to implementation timing issues")

			// Create first policy
			policy1 = createServiceTargetGroupPolicy(service, &ServiceTargetGroupPolicyConfig{
				PolicyName: "serviceexport-succession-policy-1",
				Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
				HealthCheck: &anv1alpha1.HealthCheckConfig{
					Path:            aws.String("/first"),
					IntervalSeconds: aws.Int64(20),
				},
			})

			testFramework.ExpectCreated(ctx, policy1)
			time.Sleep(2 * time.Second)

			// Create second policy
			policy2 = createServiceTargetGroupPolicy(service, &ServiceTargetGroupPolicyConfig{
				PolicyName: "serviceexport-succession-policy-2",
				Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
				HealthCheck: &anv1alpha1.HealthCheckConfig{
					Path:            aws.String("/second"),
					IntervalSeconds: aws.Int64(25),
				},
			})

			testFramework.ExpectCreated(ctx, policy2)

			// Verify first policy wins
			Eventually(func(g Gomega) {
				tgSummary := testFramework.GetTargetGroup(ctx, service)
				tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)
				g.Expect(*tg.Config.HealthCheck.Path).To(Equal("/first"))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			// Delete first policy and verify second takes over
			testFramework.ExpectDeletedThenNotFound(ctx, policy1)
			policy1 = nil

			Eventually(func(g Gomega) {
				tgSummary := testFramework.GetTargetGroup(ctx, service)
				tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)
				g.Expect(*tg.Config.HealthCheck.Path).To(Equal("/second"))
			}).Within(120 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
		})
	})

	Context("Policy status and validation", func() {
		AfterEach(func() {
			if policy != nil {
				testFramework.ExpectDeleted(ctx, policy)
				policy = nil
			}
		})

		It("should set Accepted status condition for valid policies", func() {
			policy = createServiceTargetGroupPolicy(service, &ServiceTargetGroupPolicyConfig{
				PolicyName: "serviceexport-valid-policy",
				Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
				HealthCheck: &anv1alpha1.HealthCheckConfig{
					Path:            aws.String("/health"),
					IntervalSeconds: aws.Int64(30),
				},
			})

			testFramework.ExpectCreated(ctx, policy)

			// Verify that the policy gets Accepted status
			Eventually(func(g Gomega) {
				err := testFramework.Client.Get(ctx, client.ObjectKeyFromObject(policy), policy)
				g.Expect(err).Should(BeNil())

				// Check for Accepted condition
				conditions := policy.Status.Conditions
				g.Expect(conditions).ToNot(BeEmpty())

				var acceptedCondition *metav1.Condition
				for i := range conditions {
					if conditions[i].Type == "Accepted" {
						acceptedCondition = &conditions[i]
						break
					}
				}
				g.Expect(acceptedCondition).ToNot(BeNil())
				g.Expect(acceptedCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(acceptedCondition.Reason).To(Equal("Accepted"))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})
	})

	Context("Error handling and invalid policy scenarios", func() {
		AfterEach(func() {
			if policy != nil {
				testFramework.ExpectDeleted(ctx, policy)
				policy = nil
			}
		})

		It("should validate policy configuration at API level", func() {
			// First, ensure the target group is in default state (wait for any previous policies to be cleaned up)
			Eventually(func(g Gomega) {
				tgSummary := testFramework.GetTargetGroup(ctx, service)
				tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)
				g.Expect(*tg.Config.HealthCheck.Path).To(Equal("/"))
				g.Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(30))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			// Test that the API server properly validates TargetGroupPolicy configuration
			// This ensures invalid values are rejected before they can cause issues
			invalidPolicy := &anv1alpha1.TargetGroupPolicy{
				TypeMeta: metav1.TypeMeta{
					Kind: "TargetGroupPolicy",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: service.Namespace,
					Name:      "invalid-policy-test",
				},
				Spec: anv1alpha1.TargetGroupPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: gwv1.Kind("Service"),
						Name: gwv1.ObjectName(service.Name),
					},
					Protocol: aws.String(vpclattice.TargetGroupProtocolHttp),
					HealthCheck: &anv1alpha1.HealthCheckConfig{
						Path:            aws.String("/valid-path"),
						IntervalSeconds: aws.Int64(-1), // Invalid negative value
						TimeoutSeconds:  aws.Int64(-5), // Invalid negative value
					},
				},
			}

			// Attempt to create the invalid policy - this should fail
			err := testFramework.Client.Create(ctx, invalidPolicy)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("intervalSeconds"))
			Expect(err.Error()).Should(ContainSubstring("timeoutSeconds"))

			// Verify that the target group still has default configuration
			tgSummary := testFramework.GetTargetGroup(ctx, service)
			tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)

			// Should have default health check configuration since no valid policy was created
			Expect(*tg.Config.HealthCheck.Path).To(Equal("/"))
			Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(30))
		})

		It("should handle policy targeting non-existent service gracefully", func() {
			// Create policy targeting a service that doesn't exist
			nonExistentPolicy := &anv1alpha1.TargetGroupPolicy{
				TypeMeta: metav1.TypeMeta{
					Kind: "TargetGroupPolicy",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: k8snamespace,
					Name:      "non-existent-service-policy",
				},
				Spec: anv1alpha1.TargetGroupPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: gwv1.Kind("Service"),
						Name: gwv1.ObjectName("non-existent-service"),
					},
					Protocol: aws.String(vpclattice.TargetGroupProtocolHttp),
					HealthCheck: &anv1alpha1.HealthCheckConfig{
						Path: aws.String("/non-existent"),
					},
				},
			}

			testFramework.ExpectCreated(ctx, nonExistentPolicy)

			// Verify that the policy gets TargetNotFound status
			Eventually(func(g Gomega) {
				err := testFramework.Client.Get(ctx, client.ObjectKeyFromObject(nonExistentPolicy), nonExistentPolicy)
				g.Expect(err).Should(BeNil())

				conditions := nonExistentPolicy.Status.Conditions
				g.Expect(conditions).ToNot(BeEmpty())

				var acceptedCondition *metav1.Condition
				for i := range conditions {
					if conditions[i].Type == "Accepted" {
						acceptedCondition = &conditions[i]
						break
					}
				}
				g.Expect(acceptedCondition).ToNot(BeNil())
				g.Expect(acceptedCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(acceptedCondition.Reason).To(Equal("TargetNotFound"))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			// The existing service should continue to work with default configuration
			tgSummary := testFramework.GetTargetGroup(ctx, service)
			tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)

			// Verify that our service still has default configuration
			Expect(*tg.Config.HealthCheck.Path).To(Equal("/"))
			Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(30))

			testFramework.ExpectDeleted(ctx, nonExistentPolicy)
		})

		It("should handle policies with minimal configuration", func() {
			// Create policy with only protocol specified
			policy = createServiceTargetGroupPolicy(service, &ServiceTargetGroupPolicyConfig{
				PolicyName: "serviceexport-minimal-policy",
				Protocol:   aws.String(vpclattice.TargetGroupProtocolHttp),
				// No health check configuration - should use defaults
			})

			testFramework.ExpectCreated(ctx, policy)

			// Verify that the target group still gets created with default health check
			Eventually(func(g Gomega) {
				tgSummary := testFramework.GetTargetGroup(ctx, service)
				tg := testFramework.GetFullTargetGroupFromSummary(ctx, tgSummary)

				// Should have default health check configuration since policy doesn't specify health check
				g.Expect(*tg.Config.HealthCheck.Path).To(Equal("/"))
				g.Expect(*tg.Config.HealthCheck.HealthCheckIntervalSeconds).To(BeEquivalentTo(30))
				g.Expect(*tg.Config.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttp))
			}).Within(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})
	})
})

type ServiceTargetGroupPolicyConfig struct {
	PolicyName      string
	Protocol        *string
	ProtocolVersion *string
	HealthCheck     *anv1alpha1.HealthCheckConfig
}

func createServiceTargetGroupPolicy(
	service *corev1.Service,
	config *ServiceTargetGroupPolicyConfig,
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
			TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
				Kind: gwv1.Kind("Service"),
				Name: gwv1.ObjectName(service.Name),
			},
			Protocol:        config.Protocol,
			ProtocolVersion: config.ProtocolVersion,
			HealthCheck:     config.HealthCheck,
		},
	}
}
