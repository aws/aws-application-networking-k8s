package integration

import (
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Service Name Override", Ordered, func() {
	It("HttpRoute creates VPC Lattice service with custom name when override specified", func() {
		httpDeployment, httpSvc := testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "service-name-override-test",
			Namespace: k8snamespace,
		})

		httpRoute := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-name-override-route",
				Namespace: k8snamespace,
				Annotations: map[string]string{
					"application-networking.k8s.aws/service-name-override": "custom-service-name",
				},
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name: gwv1.ObjectName(testGateway.Name),
						},
					},
				},
				Rules: []gwv1.HTTPRouteRule{
					{
						BackendRefs: []gwv1.HTTPBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: gwv1.ObjectName(httpSvc.Name),
										Port: (*gwv1.PortNumber)(&httpSvc.Spec.Ports[0].Port),
									},
								},
							},
						},
					},
				},
			},
		}

		testFramework.ExpectCreated(ctx, httpDeployment, httpSvc, httpRoute)
		Eventually(func(g Gomega) {
			route := core.NewHTTPRoute(*httpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)

			g.Expect(*vpcLatticeService.Name).To(Equal("custom-service-name"))
			g.Expect(*vpcLatticeService.Status).To(Equal(vpclattice.ServiceStatusActive))
			g.Expect(*vpcLatticeService.DnsEntry.DomainName).To(ContainSubstring("custom-service-name"))

			targetGroup := testFramework.GetTargetGroup(ctx, httpSvc)
			g.Expect(*targetGroup.Protocol).To(Equal("HTTP"))

			listeners, err := testFramework.LatticeClient.ListListeners(&vpclattice.ListListenersInput{
				ServiceIdentifier: vpcLatticeService.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(listeners.Items)).To(BeNumerically(">", 0))

			for _, listener := range listeners.Items {
				rules, err := testFramework.LatticeClient.ListRules(&vpclattice.ListRulesInput{
					ServiceIdentifier:  vpcLatticeService.Id,
					ListenerIdentifier: listener.Id,
				})
				g.Expect(err).To(BeNil())
				g.Expect(len(rules.Items)).To(BeNumerically(">", 0))

				foundForwardRule := false
				for _, rule := range rules.Items {
					if rule.IsDefault != nil && *rule.IsDefault {
						continue
					}

					ruleDetail, err := testFramework.LatticeClient.GetRule(&vpclattice.GetRuleInput{
						ServiceIdentifier:  vpcLatticeService.Id,
						ListenerIdentifier: listener.Id,
						RuleIdentifier:     rule.Id,
					})
					g.Expect(err).To(BeNil())

					if ruleDetail.Action != nil && ruleDetail.Action.Forward != nil {
						for _, targetGroupWeight := range ruleDetail.Action.Forward.TargetGroups {
							if *targetGroupWeight.TargetGroupIdentifier == *targetGroup.Id {
								foundForwardRule = true
								break
							}
						}
					}
				}
				g.Expect(foundForwardRule).To(BeTrue(), "Expected to find a rule that forwards to target group %s", *targetGroup.Id)
			}

			associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociations(&vpclattice.ListServiceNetworkServiceAssociationsInput{
				ServiceIdentifier: vpcLatticeService.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(associations.Items)).To(BeNumerically(">", 0))

			for _, association := range associations.Items {
				g.Expect(*association.Status).To(Equal(vpclattice.ServiceNetworkServiceAssociationStatusActive))
			}

			targets := testFramework.GetTargets(ctx, targetGroup, httpDeployment)
			g.Expect(targets).ToNot(BeEmpty())
		}).Should(Succeed())

		testFramework.ExpectDeletedThenNotFound(ctx, httpRoute, httpDeployment, httpSvc)
	})

	It("rejects HTTPRoute with invalid service name override and creates no VPC Lattice resources", func() {
		invalidDep, invalidSvc := testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "invalid-test",
			Namespace: k8snamespace,
		})

		invalidRoute := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-route",
				Namespace: k8snamespace,
				Annotations: map[string]string{
					"application-networking.k8s.aws/service-name-override": "INVALID-UPPERCASE",
				},
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name: gwv1.ObjectName(testGateway.Name),
						},
					},
				},
				Rules: []gwv1.HTTPRouteRule{
					{
						BackendRefs: []gwv1.HTTPBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: gwv1.ObjectName(invalidSvc.Name),
										Port: (*gwv1.PortNumber)(&invalidSvc.Spec.Ports[0].Port),
									},
								},
							},
						},
					},
				},
			},
		}

		testFramework.ExpectCreated(ctx, invalidDep, invalidSvc, invalidRoute)

		Eventually(func(g Gomega) {
			_, err := testFramework.LatticeClient.FindService(ctx, "INVALID-UPPERCASE")
			g.Expect(services.IsNotFoundError(err)).To(BeTrue())

			targetGroups, err := testFramework.LatticeClient.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{})
			g.Expect(err).To(BeNil())
			for _, tg := range targetGroups {
				g.Expect(*tg.Name).ToNot(ContainSubstring("invalid-test"))
			}

			events := &v1.EventList{}
			err = testFramework.List(ctx, events, client.InNamespace(k8snamespace))
			g.Expect(err).ToNot(HaveOccurred())

			foundValidationError := false
			for _, event := range events.Items {
				if event.InvolvedObject.Name == invalidRoute.Name &&
					event.Reason == "FailedBuildModel" &&
					strings.Contains(event.Message, "invalid service name override") {
					foundValidationError = true
					break
				}
			}
			g.Expect(foundValidationError).To(BeTrue(), "Expected FailedBuildModel event with validation error")
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		testFramework.ExpectDeletedThenNotFound(ctx, invalidRoute, invalidDep, invalidSvc)
	})
})
