package integration

import (
	"log"
	"os"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("HTTPRoute rule priorities", func() {
	var (
		deployment1           *appsv1.Deployment
		deployment2           *appsv1.Deployment
		deployment3           *appsv1.Deployment
		service1              *v1.Service
		service2              *v1.Service
		service3              *v1.Service
		rulePriorityHttpRoute *gwv1.HTTPRoute
	)

	It("HTTPRoute should support manual rule priorities through annotations", func() {
		// Create three different apps to test priority routing
		deployment1, service1 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "priority-test-v1", Namespace: k8snamespace})
		deployment2, service2 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "priority-test-v2", Namespace: k8snamespace})
		deployment3, service3 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "priority-test-v3", Namespace: k8snamespace})

		// Create HTTPRoute with rules having different priorities
		rulePriorityHttpRoute = &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "priority-test-route",
				Namespace: k8snamespace,
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name:        gwv1.ObjectName(testGateway.Name),
							SectionName: lo.ToPtr(gwv1.SectionName("http")),
						},
					},
				},
				Rules: []gwv1.HTTPRouteRule{
					{
						// High priority rule (1) - should be evaluated first
						Matches: []gwv1.HTTPRouteMatch{
							{
								Path: &gwv1.HTTPPathMatch{
									Type:  lo.ToPtr(gwv1.PathMatchPathPrefix),
									Value: lo.ToPtr("/api"),
								},
							},
						},
						BackendRefs: []gwv1.HTTPBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: gwv1.ObjectName(service1.Name),
										Port: lo.ToPtr(gwv1.PortNumber(80)),
									},
								},
							},
						},
						Filters: []gwv1.HTTPRouteFilter{},
					},
					{
						// Low priority rule (100) - should be evaluated last
						Matches: []gwv1.HTTPRouteMatch{
							{
								Path: &gwv1.HTTPPathMatch{
									Type:  lo.ToPtr(gwv1.PathMatchPathPrefix),
									Value: lo.ToPtr("/api/v2"),
								},
							},
						},
						BackendRefs: []gwv1.HTTPBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: gwv1.ObjectName(service2.Name),
										Port: lo.ToPtr(gwv1.PortNumber(80)),
									},
								},
							},
						},
						Filters: []gwv1.HTTPRouteFilter{},
					},
					{
						// Medium priority rule (50) - should be evaluated second
						Matches: []gwv1.HTTPRouteMatch{
							{
								Path: &gwv1.HTTPPathMatch{
									Type:  lo.ToPtr(gwv1.PathMatchPathPrefix),
									Value: lo.ToPtr("/api/special"),
								},
							},
						},
						BackendRefs: []gwv1.HTTPBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: gwv1.ObjectName(service3.Name),
										Port: lo.ToPtr(gwv1.PortNumber(80)),
									},
								},
							},
						},
						Filters: []gwv1.HTTPRouteFilter{},
					},
				},
			},
		}

		// Add priority annotations to the HTTPRoute
		if rulePriorityHttpRoute.Annotations == nil {
			rulePriorityHttpRoute.Annotations = make(map[string]string)
		}
		rulePriorityHttpRoute.Annotations["application-networking.k8s.aws/rule-0-priority"] = "1"   // High priority
		rulePriorityHttpRoute.Annotations["application-networking.k8s.aws/rule-1-priority"] = "100" // Low priority
		rulePriorityHttpRoute.Annotations["application-networking.k8s.aws/rule-2-priority"] = "50"  // Medium priority

		// Create Kubernetes API Objects
		testFramework.ExpectCreated(ctx,
			rulePriorityHttpRoute,
			service1,
			deployment1,
			service2,
			deployment2,
			service3,
			deployment3,
		)

		route, _ := core.NewRoute(rulePriorityHttpRoute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)

		// Verify target groups
		targetGroupV1 := testFramework.GetTargetGroup(ctx, service1)
		Expect(*targetGroupV1.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
		Expect(*targetGroupV1.Protocol).To(Equal("HTTP"))
		targetsV1 := testFramework.GetTargets(ctx, targetGroupV1, deployment1)
		Expect(*targetGroupV1.Port).To(BeEquivalentTo(80))
		for _, target := range targetsV1 {
			Expect(*target.Port).To(BeEquivalentTo(service1.Spec.Ports[0].TargetPort.IntVal))
			Expect(*target.Status).To(Or(
				Equal(vpclattice.TargetStatusInitial),
				Equal(vpclattice.TargetStatusHealthy),
			))
		}

		targetGroupV2 := testFramework.GetTargetGroup(ctx, service2)
		Expect(*targetGroupV2.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
		Expect(*targetGroupV2.Protocol).To(Equal("HTTP"))
		targetsV2 := testFramework.GetTargets(ctx, targetGroupV2, deployment2)
		Expect(*targetGroupV2.Port).To(BeEquivalentTo(80))
		for _, target := range targetsV2 {
			Expect(*target.Port).To(BeEquivalentTo(service2.Spec.Ports[0].TargetPort.IntVal))
			Expect(*target.Status).To(Or(
				Equal(vpclattice.TargetStatusInitial),
				Equal(vpclattice.TargetStatusHealthy),
			))
		}

		targetGroupV3 := testFramework.GetTargetGroup(ctx, service3)
		Expect(*targetGroupV3.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
		Expect(*targetGroupV3.Protocol).To(Equal("HTTP"))
		targetsV3 := testFramework.GetTargets(ctx, targetGroupV3, deployment3)
		Expect(*targetGroupV3.Port).To(BeEquivalentTo(80))
		for _, target := range targetsV3 {
			Expect(*target.Port).To(BeEquivalentTo(service3.Spec.Ports[0].TargetPort.IntVal))
			Expect(*target.Status).To(Or(
				Equal(vpclattice.TargetStatusInitial),
				Equal(vpclattice.TargetStatusHealthy),
			))
		}

		log.Println("Verifying VPC lattice service listeners and rules")
		Eventually(func(g Gomega) {
			listListenerResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx, &vpclattice.ListListenersInput{
				ServiceIdentifier: vpcLatticeService.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(listListenerResp.Items)).To(BeEquivalentTo(1))
			listener := listListenerResp.Items[0]
			g.Expect(*listener.Port).To(BeEquivalentTo(testGateway.Spec.Listeners[0].Port))
			listenerId := listener.Id

			listRulesResp, err := testFramework.LatticeClient.ListRulesWithContext(ctx, &vpclattice.ListRulesInput{
				ListenerIdentifier: listenerId,
				ServiceIdentifier:  vpcLatticeService.Id,
			})
			nonDefaultRules := lo.Filter(listRulesResp.Items, func(rule *vpclattice.RuleSummary, _ int) bool {
				return rule.IsDefault == nil || *rule.IsDefault == false
			})
			ruleIds := lo.Map(nonDefaultRules, func(rule *vpclattice.RuleSummary, _ int) *string {
				return rule.Id
			})

			g.Expect(len(ruleIds)).To(Equal(3))

			// Verify rules are created with correct priorities
			rules := make([]*vpclattice.GetRuleOutput, len(ruleIds))
			for i, ruleId := range ruleIds {
				rule, err := testFramework.LatticeClient.GetRuleWithContext(ctx, &vpclattice.GetRuleInput{
					ServiceIdentifier:  vpcLatticeService.Id,
					ListenerIdentifier: listenerId,
					RuleIdentifier:     ruleId,
				})
				g.Expect(err).To(BeNil())
				rules[i] = rule
			}

			// Verify rule priorities are set correctly
			// Rule priorities in VPC Lattice should match our annotations
			for _, rule := range rules {
				switch *rule.Match.HttpMatch.PathMatch.Match.Prefix {
				case "/api/v2":
					g.Expect(*rule.Priority).To(BeEquivalentTo(100))
				case "/api/special":
					g.Expect(*rule.Priority).To(BeEquivalentTo(50))
				case "/api":
					g.Expect(*rule.Priority).To(BeEquivalentTo(1))
				}
			}
		}).WithOffset(1).Should(Succeed())
	})

	AfterEach(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			rulePriorityHttpRoute,
			deployment1,
			deployment2,
			deployment3,
			service1,
			service2,
			service3,
		)
	})
})
