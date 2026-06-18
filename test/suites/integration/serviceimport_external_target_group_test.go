package integration

import (
	"errors"
	"log"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("ServiceImport invalid external target group does not takeover or delete customer service", Ordered, Label("service-import-external-tg"), func() {
	var (
		preCreatedSnId *string
		gateway        *gwv1.Gateway

		preCreatedServiceArn     *string
		preCreatedListenerArn    *string
		preCreatedTargetGroupArn *string
		preCreatedTargetGroupId  *string
		preCreatedAssociationArn *string
		preCreatedRuleId         *string

		deployment    *appsv1.Deployment
		service       *corev1.Service
		currentRoute  *gwv1.HTTPRoute
		currentImport *anv1alpha1.ServiceImport

		customerServiceName          = "external-svc-1"
		serviceNetworkAndGatewayName = "external-sn-1"
	)

	BeforeAll(func() {
		snResp, err := testFramework.LatticeClient.CreateServiceNetwork(ctx, &vpclattice.CreateServiceNetworkInput{
			Name: aws.String(serviceNetworkAndGatewayName),
		})
		Expect(err).To(BeNil())
		preCreatedSnId = snResp.Id

		gateway = &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceNetworkAndGatewayName,
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     "http",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: lo.ToPtr(gwv1.NamespacesFromSame),
							},
						},
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, gateway)

		Eventually(func(g Gomega) {
			gw := &gwv1.Gateway{}
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(gateway), gw)
			g.Expect(err).To(BeNil())
			programmed := false
			for _, cond := range gw.Status.Conditions {
				if cond.Type == string(gwv1.GatewayConditionProgrammed) &&
					cond.Status == metav1.ConditionTrue {
					programmed = true
					break
				}
			}
			g.Expect(programmed).To(BeTrue(), "gateway not Programmed")
		}).Should(Succeed())

		targetGroupResp, err := testFramework.LatticeClient.CreateTargetGroup(ctx, &vpclattice.CreateTargetGroupInput{
			Name: aws.String("external-tg-1"),
			Type: types.TargetGroupTypeIp,
			Config: &types.TargetGroupConfig{
				Protocol:      types.TargetGroupProtocolHttp,
				Port:          aws.Int32(80),
				VpcIdentifier: aws.String(testFramework.Cloud.Config().VpcId),
			},
		})
		Expect(err).To(BeNil())
		preCreatedTargetGroupArn = targetGroupResp.Arn
		preCreatedTargetGroupId = targetGroupResp.Id

		Eventually(func(g Gomega) {
			getTargetGroupResp, err := testFramework.LatticeClient.GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: preCreatedTargetGroupArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getTargetGroupResp.Status)).To(Equal(string(types.TargetGroupStatusActive)))
		}).Should(Succeed())

		serviceResp, err := testFramework.LatticeClient.CreateService(ctx, &vpclattice.CreateServiceInput{
			Name: aws.String(customerServiceName),
		})
		Expect(err).To(BeNil())
		preCreatedServiceArn = serviceResp.Arn

		Eventually(func(g Gomega) {
			getServiceResp, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getServiceResp.Status)).To(Equal(string(types.ServiceStatusActive)))
		}).Should(Succeed())

		listenerResp, err := testFramework.LatticeClient.CreateListener(ctx, &vpclattice.CreateListenerInput{
			ServiceIdentifier: preCreatedServiceArn,
			Name:              aws.String("external-listener-1"),
			Protocol:          types.ListenerProtocolHttp,
			Port:              aws.Int32(80),
			DefaultAction: &types.RuleActionMemberFixedResponse{
				Value: types.FixedResponseAction{
					StatusCode: aws.Int32(404),
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedListenerArn = listenerResp.Arn

		ruleResp, err := testFramework.LatticeClient.CreateRule(ctx, &vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn,
			Name:               aws.String("external-rule-1"),
			Priority:           aws.Int32(10),
			Match: &types.RuleMatchMemberHttpMatch{
				Value: types.HttpMatch{
					PathMatch: &types.PathMatch{
						Match:         &types.PathMatchTypeMemberPrefix{Value: "/"},
						CaseSensitive: aws.Bool(false),
					},
				},
			},
			Action: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{
						{TargetGroupIdentifier: preCreatedTargetGroupArn, Weight: aws.Int32(100)},
					},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleId = ruleResp.Id

		associationResp, err := testFramework.LatticeClient.CreateServiceNetworkServiceAssociation(ctx, &vpclattice.CreateServiceNetworkServiceAssociationInput{
			ServiceIdentifier:        preCreatedServiceArn,
			ServiceNetworkIdentifier: preCreatedSnId,
		})
		Expect(err).To(BeNil())
		preCreatedAssociationArn = associationResp.Arn

		Eventually(func(g Gomega) {
			getAssocResp, err := testFramework.LatticeClient.GetServiceNetworkServiceAssociation(ctx, &vpclattice.GetServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getAssocResp.Status)).To(Equal(string(types.ServiceNetworkServiceAssociationStatusActive)))
		}).Should(Succeed())

		deployment, service = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:       "eks-1",
			Namespace:  k8snamespace,
			Port:       80,
			TargetPort: 8080,
		})
		testFramework.ExpectCreated(ctx, deployment, service)
	})

	AfterEach(func() {
		if currentRoute != nil {
			_ = testFramework.Client.Delete(ctx, currentRoute)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), currentRoute)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentRoute = nil
		}
		if currentImport != nil {
			_ = testFramework.Client.Delete(ctx, currentImport)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentImport), currentImport)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentImport = nil
		}
	})

	It("Non existent ServiceImport", func() {
		parentNS := gwv1.Namespace(gateway.Namespace)
		currentRoute = &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "missing-import-route",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/service-name-override": customerServiceName,
				},
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name:      gwv1.ObjectName(gateway.Name),
						Namespace: &parentNS,
					}},
				},
				Rules: []gwv1.HTTPRouteRule{{
					BackendRefs: []gwv1.HTTPBackendRef{
						{BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName("missing-import"),
								Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
							},
							Weight: lo.ToPtr(int32(90)),
						}},
						{BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName(service.Name),
								Kind: lo.ToPtr(gwv1.Kind("Service")),
								Port: lo.ToPtr(gwv1.PortNumber(80)),
							},
							Weight: lo.ToPtr(int32(10)),
						}},
					},
				}},
			},
		}
		testFramework.ExpectCreated(ctx, currentRoute)

		Eventually(func(g Gomega) {
			got := &gwv1.HTTPRoute{}
			err := testFramework.Get(ctx, apitypes.NamespacedName{
				Name:      currentRoute.Name,
				Namespace: currentRoute.Namespace,
			}, got)
			g.Expect(err).To(BeNil())
			g.Expect(got.Status.Parents).ToNot(BeEmpty())

			parent := got.Status.Parents[0]
			conds := map[string]metav1.Condition{}
			for _, c := range parent.Conditions {
				conds[c.Type] = c
			}
			accepted, ok := conds[string(gwv1.RouteConditionAccepted)]
			g.Expect(ok).To(BeTrue())
			g.Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))

			resolved, ok := conds[string(gwv1.RouteConditionResolvedRefs)]
			g.Expect(ok).To(BeTrue())
			g.Expect(resolved.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(resolved.Reason).To(Equal(string(gwv1.RouteReasonBackendNotFound)))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeFalse(), "service untagged")
		}).Should(Succeed())

		Expect(testFramework.Client.Delete(ctx, currentRoute)).To(Succeed())
		Eventually(func(g Gomega) {
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), currentRoute)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "route finalized")
		}).Should(Succeed())
		currentRoute = nil

		Eventually(func(g Gomega) {
			getServiceResp, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(*getServiceResp.Arn).To(Equal(*preCreatedServiceArn))

			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeFalse(), "service untagged after delete")

			listenerResp, err := testFramework.LatticeClient.GetListener(ctx, &vpclattice.GetListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			g.Expect(err).To(BeNil(), "listener exists after delete")
			g.Expect(*listenerResp.Arn).To(Equal(*preCreatedListenerArn), "listener ARN preserved")
			g.Expect(*listenerResp.Port).To(BeEquivalentTo(80), "listener port unchanged")
			g.Expect(string(listenerResp.Protocol)).To(Equal(string(types.ListenerProtocolHttp)), "listener protocol unchanged")

			ruleResp, err := testFramework.LatticeClient.GetRule(ctx, &vpclattice.GetRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
				RuleIdentifier:     preCreatedRuleId,
			})
			g.Expect(err).To(BeNil())
			g.Expect(*ruleResp.Id).To(Equal(*preCreatedRuleId), "rule ID preserved")
			forward, ok := ruleResp.Action.(*types.RuleActionMemberForward)
			g.Expect(ok).To(BeTrue(), "rule action is Forward")
			g.Expect(forward.Value.TargetGroups).To(HaveLen(1))
			g.Expect(*forward.Value.TargetGroups[0].TargetGroupIdentifier).To(Equal(*preCreatedTargetGroupId))
			g.Expect(*forward.Value.TargetGroups[0].Weight).To(BeEquivalentTo(100))
		}).Should(Succeed())
	})

	It("Invalid target group arn on ServiceImport", func() {
		currentImport = &anv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-arn-import",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/target-group-arn": "not-an-arn",
				},
			},
			Spec: anv1alpha1.ServiceImportSpec{
				Type: anv1alpha1.ClusterSetIP,
				Ports: []anv1alpha1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentImport)

		parentNS := gwv1.Namespace(gateway.Namespace)
		currentRoute = &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-arn-route",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/service-name-override": customerServiceName,
				},
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name:      gwv1.ObjectName(gateway.Name),
						Namespace: &parentNS,
					}},
				},
				Rules: []gwv1.HTTPRouteRule{{
					BackendRefs: []gwv1.HTTPBackendRef{
						{BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName("invalid-arn-import"),
								Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
							},
							Weight: lo.ToPtr(int32(90)),
						}},
						{BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName(service.Name),
								Kind: lo.ToPtr(gwv1.Kind("Service")),
								Port: lo.ToPtr(gwv1.PortNumber(80)),
							},
							Weight: lo.ToPtr(int32(10)),
						}},
					},
				}},
			},
		}
		testFramework.ExpectCreated(ctx, currentRoute)

		Eventually(func(g Gomega) {
			rt := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), rt)).To(Succeed())
			g.Expect(rt.Status.Parents).ToNot(BeEmpty())
			conds := map[string]metav1.Condition{}
			for _, c := range rt.Status.Parents[0].Conditions {
				conds[c.Type] = c
			}
			accepted, ok := conds[string(gwv1.RouteConditionAccepted)]
			g.Expect(ok).To(BeTrue())
			g.Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))

			resolved, ok := conds[string(gwv1.RouteConditionResolvedRefs)]
			g.Expect(ok).To(BeTrue())
			g.Expect(resolved.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(resolved.Reason).To(Equal("InvalidExternalTargetGroup"))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeFalse(), "service untagged")
		}).Should(Succeed())

		Expect(testFramework.Client.Delete(ctx, currentRoute)).To(Succeed())
		Eventually(func(g Gomega) {
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), currentRoute)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "route finalized")
		}).Should(Succeed())
		currentRoute = nil

		Eventually(func(g Gomega) {
			getServiceResp, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(*getServiceResp.Arn).To(Equal(*preCreatedServiceArn))

			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeFalse(), "service untagged after delete")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			listenerResp, err := testFramework.LatticeClient.GetListener(ctx, &vpclattice.GetListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			g.Expect(err).To(BeNil(), "listener exists after delete")
			g.Expect(*listenerResp.Arn).To(Equal(*preCreatedListenerArn), "listener ARN preserved")
			g.Expect(*listenerResp.Port).To(BeEquivalentTo(80), "listener port unchanged")
			g.Expect(string(listenerResp.Protocol)).To(Equal(string(types.ListenerProtocolHttp)), "listener protocol unchanged")

			ruleResp, err := testFramework.LatticeClient.GetRule(ctx, &vpclattice.GetRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
				RuleIdentifier:     preCreatedRuleId,
			})
			g.Expect(err).To(BeNil())
			g.Expect(*ruleResp.Id).To(Equal(*preCreatedRuleId), "rule ID preserved")
			forward, ok := ruleResp.Action.(*types.RuleActionMemberForward)
			g.Expect(ok).To(BeTrue(), "rule action is Forward")
			g.Expect(forward.Value.TargetGroups).To(HaveLen(1))
			g.Expect(*forward.Value.TargetGroups[0].TargetGroupIdentifier).To(Equal(*preCreatedTargetGroupId))
			g.Expect(*forward.Value.TargetGroups[0].Weight).To(BeEquivalentTo(100))
		}).Should(Succeed())
	})

	It("Non existent target group with valid arn", func() {
		currentImport = &anv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "not-found-arn-import",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/target-group-arn": "arn:aws:vpc-lattice:us-east-2:685175429445:targetgroup/tg-deadbeefdeadbeef0",
				},
			},
			Spec: anv1alpha1.ServiceImportSpec{
				Type: anv1alpha1.ClusterSetIP,
				Ports: []anv1alpha1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentImport)

		parentNS := gwv1.Namespace(gateway.Namespace)
		currentRoute = &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "not-found-arn-route",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/service-name-override": customerServiceName,
				},
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name:      gwv1.ObjectName(gateway.Name),
						Namespace: &parentNS,
					}},
				},
				Rules: []gwv1.HTTPRouteRule{{
					BackendRefs: []gwv1.HTTPBackendRef{
						{BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName("not-found-arn-import"),
								Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
							},
							Weight: lo.ToPtr(int32(90)),
						}},
						{BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName(service.Name),
								Kind: lo.ToPtr(gwv1.Kind("Service")),
								Port: lo.ToPtr(gwv1.PortNumber(80)),
							},
							Weight: lo.ToPtr(int32(10)),
						}},
					},
				}},
			},
		}
		testFramework.ExpectCreated(ctx, currentRoute)

		Eventually(func(g Gomega) {
			rt := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), rt)).To(Succeed())
			g.Expect(rt.Status.Parents).ToNot(BeEmpty())
			conds := map[string]metav1.Condition{}
			for _, c := range rt.Status.Parents[0].Conditions {
				conds[c.Type] = c
			}
			accepted, ok := conds[string(gwv1.RouteConditionAccepted)]
			g.Expect(ok).To(BeTrue())
			g.Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))

			resolved, ok := conds[string(gwv1.RouteConditionResolvedRefs)]
			g.Expect(ok).To(BeTrue())
			g.Expect(resolved.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(resolved.Reason).To(Equal("ExternalTargetGroupNotFound"))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeFalse(), "service untagged")
		}).Should(Succeed())

		Expect(testFramework.Client.Delete(ctx, currentRoute)).To(Succeed())
		Eventually(func(g Gomega) {
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), currentRoute)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "route finalized")
		}).Should(Succeed())
		currentRoute = nil

		Eventually(func(g Gomega) {
			getServiceResp, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(*getServiceResp.Arn).To(Equal(*preCreatedServiceArn))

			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeFalse(), "service untagged after delete")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			listenerResp, err := testFramework.LatticeClient.GetListener(ctx, &vpclattice.GetListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			g.Expect(err).To(BeNil(), "listener exists after delete")
			g.Expect(*listenerResp.Arn).To(Equal(*preCreatedListenerArn), "listener ARN preserved")
			g.Expect(*listenerResp.Port).To(BeEquivalentTo(80), "listener port unchanged")
			g.Expect(string(listenerResp.Protocol)).To(Equal(string(types.ListenerProtocolHttp)), "listener protocol unchanged")
			ruleResp, err := testFramework.LatticeClient.GetRule(ctx, &vpclattice.GetRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
				RuleIdentifier:     preCreatedRuleId,
			})
			g.Expect(err).To(BeNil())
			g.Expect(*ruleResp.Id).To(Equal(*preCreatedRuleId), "rule ID preserved")
			forward, ok := ruleResp.Action.(*types.RuleActionMemberForward)
			g.Expect(ok).To(BeTrue(), "rule action is Forward")
			g.Expect(forward.Value.TargetGroups).To(HaveLen(1))
			g.Expect(*forward.Value.TargetGroups[0].TargetGroupIdentifier).To(Equal(*preCreatedTargetGroupId))
			g.Expect(*forward.Value.TargetGroups[0].Weight).To(BeEquivalentTo(100))
		}).Should(Succeed())
	})

	It("Conflicting annotations on ServiceImport", func() {
		currentImport = &anv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "conflicting-annotations-import",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/target-group-arn": "arn:aws:vpc-lattice:us-east-2:685175429445:targetgroup/tg-deadbeefdeadbeef0",
					"application-networking.k8s.aws/export-name":      "some-export-name",
				},
			},
			Spec: anv1alpha1.ServiceImportSpec{
				Type: anv1alpha1.ClusterSetIP,
				Ports: []anv1alpha1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentImport)

		parentNS := gwv1.Namespace(gateway.Namespace)
		currentRoute = &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "conflicting-annotations-route",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/service-name-override": customerServiceName,
				},
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name:      gwv1.ObjectName(gateway.Name),
						Namespace: &parentNS,
					}},
				},
				Rules: []gwv1.HTTPRouteRule{{
					BackendRefs: []gwv1.HTTPBackendRef{
						{BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName("conflicting-annotations-import"),
								Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
							},
							Weight: lo.ToPtr(int32(90)),
						}},
						{BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName(service.Name),
								Kind: lo.ToPtr(gwv1.Kind("Service")),
								Port: lo.ToPtr(gwv1.PortNumber(80)),
							},
							Weight: lo.ToPtr(int32(10)),
						}},
					},
				}},
			},
		}
		testFramework.ExpectCreated(ctx, currentRoute)

		Eventually(func(g Gomega) {
			rt := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), rt)).To(Succeed())
			g.Expect(rt.Status.Parents).ToNot(BeEmpty())
			conds := map[string]metav1.Condition{}
			for _, c := range rt.Status.Parents[0].Conditions {
				conds[c.Type] = c
			}
			accepted, ok := conds[string(gwv1.RouteConditionAccepted)]
			g.Expect(ok).To(BeTrue())
			g.Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))

			resolved, ok := conds[string(gwv1.RouteConditionResolvedRefs)]
			g.Expect(ok).To(BeTrue())
			g.Expect(resolved.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(resolved.Reason).To(Equal("ConflictingServiceImportAnnotations"))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeFalse(), "service untagged")
		}).Should(Succeed())

		Expect(testFramework.Client.Delete(ctx, currentRoute)).To(Succeed())
		Eventually(func(g Gomega) {
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), currentRoute)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "route finalized")
		}).Should(Succeed())
		currentRoute = nil

		Eventually(func(g Gomega) {
			getServiceResp, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(*getServiceResp.Arn).To(Equal(*preCreatedServiceArn))

			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeFalse(), "service untagged after delete")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			listenerResp, err := testFramework.LatticeClient.GetListener(ctx, &vpclattice.GetListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			g.Expect(err).To(BeNil(), "listener exists after delete")
			g.Expect(*listenerResp.Arn).To(Equal(*preCreatedListenerArn), "listener ARN preserved")
			g.Expect(*listenerResp.Port).To(BeEquivalentTo(80), "listener port unchanged")
			g.Expect(string(listenerResp.Protocol)).To(Equal(string(types.ListenerProtocolHttp)), "listener protocol unchanged")

			ruleResp, err := testFramework.LatticeClient.GetRule(ctx, &vpclattice.GetRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
				RuleIdentifier:     preCreatedRuleId,
			})
			g.Expect(err).To(BeNil())
			g.Expect(*ruleResp.Id).To(Equal(*preCreatedRuleId), "rule ID preserved")
			forward, ok := ruleResp.Action.(*types.RuleActionMemberForward)
			g.Expect(ok).To(BeTrue(), "rule action is Forward")
			g.Expect(forward.Value.TargetGroups).To(HaveLen(1))
			g.Expect(*forward.Value.TargetGroups[0].TargetGroupIdentifier).To(Equal(*preCreatedTargetGroupId))
			g.Expect(*forward.Value.TargetGroups[0].Weight).To(BeEquivalentTo(100))
		}).Should(Succeed())
	})

	AfterAll(func() {
		if preCreatedAssociationArn != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetworkServiceAssociation(ctx, &vpclattice.DeleteServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete association %s: %v", *preCreatedAssociationArn, err)
				}
			}
			Eventually(func(g Gomega) {
				_, getErr := testFramework.LatticeClient.GetServiceNetworkServiceAssociation(ctx, &vpclattice.GetServiceNetworkServiceAssociationInput{
					ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
				})
				g.Expect(getErr).To(HaveOccurred())
			}).Should(Succeed())
		}

		if preCreatedRuleId != nil {
			_, err := testFramework.LatticeClient.DeleteRule(ctx, &vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
				RuleIdentifier:     preCreatedRuleId,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete rule %s: %v", *preCreatedRuleId, err)
				}
			}
		}

		if preCreatedListenerArn != nil {
			_, err := testFramework.LatticeClient.DeleteListener(ctx, &vpclattice.DeleteListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete listener %s: %v", *preCreatedListenerArn, err)
				}
			}
		}

		if preCreatedServiceArn != nil {
			_, err := testFramework.LatticeClient.DeleteService(ctx, &vpclattice.DeleteServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete service %s: %v", *preCreatedServiceArn, err)
				}
			}
		}

		if preCreatedTargetGroupArn != nil {
			Eventually(func(g Gomega) {
				_, err := testFramework.LatticeClient.DeleteTargetGroup(ctx, &vpclattice.DeleteTargetGroupInput{
					TargetGroupIdentifier: preCreatedTargetGroupArn,
				})
				if err != nil {
					var respErr *smithyhttp.ResponseError
					if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
						return
					}
				}
				g.Expect(err).To(BeNil())
			}).Should(Succeed())
		}

		if deployment != nil && service != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, deployment, service)
		}

		if gateway != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, gateway)
		}

		if preCreatedSnId != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetwork(ctx, &vpclattice.DeleteServiceNetworkInput{
				ServiceNetworkIdentifier: preCreatedSnId,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete service network %s: %v", *preCreatedSnId, err)
				}
			}
		}
	})
})

var _ = Describe("External Service with single listener and single rule", Ordered, Label("service-import-external-tg"), func() {
	var (
		preCreatedSnId *string
		gateway        *gwv1.Gateway

		preCreatedServiceArn     *string
		preCreatedListenerArn    *string
		preCreatedTargetGroupArn *string
		preCreatedTargetGroupId  *string
		preCreatedAssociationArn *string
		preCreatedRuleId         *string

		deployment    *appsv1.Deployment
		service       *corev1.Service
		currentRoute  *gwv1.HTTPRoute
		currentImport *anv1alpha1.ServiceImport

		customerServiceName          = "external-svc-2"
		serviceNetworkAndGatewayName = "external-sn-2"
	)

	BeforeAll(func() {
		snResp, err := testFramework.LatticeClient.CreateServiceNetwork(ctx, &vpclattice.CreateServiceNetworkInput{
			Name: aws.String(serviceNetworkAndGatewayName),
		})
		Expect(err).To(BeNil())
		preCreatedSnId = snResp.Id

		gateway = &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceNetworkAndGatewayName,
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     "http",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: lo.ToPtr(gwv1.NamespacesFromSame),
							},
						},
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, gateway)

		Eventually(func(g Gomega) {
			gw := &gwv1.Gateway{}
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(gateway), gw)
			g.Expect(err).To(BeNil())
			programmed := false
			for _, cond := range gw.Status.Conditions {
				if cond.Type == string(gwv1.GatewayConditionProgrammed) &&
					cond.Status == metav1.ConditionTrue {
					programmed = true
					break
				}
			}
			g.Expect(programmed).To(BeTrue(), "gateway not Programmed")
		}).Should(Succeed())

		targetGroupResp, err := testFramework.LatticeClient.CreateTargetGroup(ctx, &vpclattice.CreateTargetGroupInput{
			Name: aws.String("external-tg-2"),
			Type: types.TargetGroupTypeIp,
			Config: &types.TargetGroupConfig{
				Protocol:      types.TargetGroupProtocolHttp,
				Port:          aws.Int32(80),
				VpcIdentifier: aws.String(testFramework.Cloud.Config().VpcId),
			},
		})
		Expect(err).To(BeNil())
		preCreatedTargetGroupArn = targetGroupResp.Arn
		preCreatedTargetGroupId = targetGroupResp.Id

		Eventually(func(g Gomega) {
			getTargetGroupResp, err := testFramework.LatticeClient.GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: preCreatedTargetGroupArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getTargetGroupResp.Status)).To(Equal(string(types.TargetGroupStatusActive)))
		}).Should(Succeed())

		serviceResp, err := testFramework.LatticeClient.CreateService(ctx, &vpclattice.CreateServiceInput{
			Name: aws.String(customerServiceName),
		})
		Expect(err).To(BeNil())
		preCreatedServiceArn = serviceResp.Arn

		Eventually(func(g Gomega) {
			getServiceResp, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getServiceResp.Status)).To(Equal(string(types.ServiceStatusActive)))
		}).Should(Succeed())

		listenerResp, err := testFramework.LatticeClient.CreateListener(ctx, &vpclattice.CreateListenerInput{
			ServiceIdentifier: preCreatedServiceArn,
			Name:              aws.String("external-listener-2"),
			Protocol:          types.ListenerProtocolHttp,
			Port:              aws.Int32(80),
			DefaultAction: &types.RuleActionMemberFixedResponse{
				Value: types.FixedResponseAction{
					StatusCode: aws.Int32(404),
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedListenerArn = listenerResp.Arn

		ruleResp, err := testFramework.LatticeClient.CreateRule(ctx, &vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn,
			Name:               aws.String("external-rule-2"),
			Priority:           aws.Int32(10),
			Match: &types.RuleMatchMemberHttpMatch{
				Value: types.HttpMatch{
					PathMatch: &types.PathMatch{
						Match:         &types.PathMatchTypeMemberPrefix{Value: "/"},
						CaseSensitive: aws.Bool(false),
					},
				},
			},
			Action: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{
						{TargetGroupIdentifier: preCreatedTargetGroupArn, Weight: aws.Int32(100)},
					},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleId = ruleResp.Id

		associationResp, err := testFramework.LatticeClient.CreateServiceNetworkServiceAssociation(ctx, &vpclattice.CreateServiceNetworkServiceAssociationInput{
			ServiceIdentifier:        preCreatedServiceArn,
			ServiceNetworkIdentifier: preCreatedSnId,
		})
		Expect(err).To(BeNil())
		preCreatedAssociationArn = associationResp.Arn

		Eventually(func(g Gomega) {
			getAssocResp, err := testFramework.LatticeClient.GetServiceNetworkServiceAssociation(ctx, &vpclattice.GetServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getAssocResp.Status)).To(Equal(string(types.ServiceNetworkServiceAssociationStatusActive)))
		}).Should(Succeed())

		deployment, service = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:       "eks-2",
			Namespace:  k8snamespace,
			Port:       80,
			TargetPort: 8080,
		})
		testFramework.ExpectCreated(ctx, deployment, service)
	})

	AfterEach(func() {
		if currentRoute != nil {
			_ = testFramework.Client.Delete(ctx, currentRoute)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), currentRoute)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentRoute = nil
		}
		if currentImport != nil {
			_ = testFramework.Client.Delete(ctx, currentImport)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentImport), currentImport)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentImport = nil
		}
	})

	AfterAll(func() {
		if preCreatedAssociationArn != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetworkServiceAssociation(ctx, &vpclattice.DeleteServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete association %s: %v", *preCreatedAssociationArn, err)
				}
			}
		}

		if preCreatedRuleId != nil {
			_, err := testFramework.LatticeClient.DeleteRule(ctx, &vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
				RuleIdentifier:     preCreatedRuleId,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete rule %s: %v", *preCreatedRuleId, err)
				}
			}
		}

		if preCreatedListenerArn != nil {
			_, err := testFramework.LatticeClient.DeleteListener(ctx, &vpclattice.DeleteListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete listener %s: %v", *preCreatedListenerArn, err)
				}
			}
		}

		if preCreatedServiceArn != nil {
			_, err := testFramework.LatticeClient.DeleteService(ctx, &vpclattice.DeleteServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete service %s: %v", *preCreatedServiceArn, err)
				}
			}
		}

		if preCreatedTargetGroupArn != nil {
			Eventually(func(g Gomega) {
				_, err := testFramework.LatticeClient.DeleteTargetGroup(ctx, &vpclattice.DeleteTargetGroupInput{
					TargetGroupIdentifier: preCreatedTargetGroupArn,
				})
				if err != nil {
					var respErr *smithyhttp.ResponseError
					if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
						return
					}
				}
				g.Expect(err).To(BeNil())
			}).Should(Succeed())
		}

		if deployment != nil && service != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, deployment, service)
		}

		if gateway != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, gateway)
		}

		if preCreatedSnId != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetwork(ctx, &vpclattice.DeleteServiceNetworkInput{
				ServiceNetworkIdentifier: preCreatedSnId,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete service network %s: %v", *preCreatedSnId, err)
				}
			}
		}
	})

	It("HTTPRoute takes over customer service, shifts traffic to 90/10, then to 0/100", func() {
		currentImport = &anv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "import-2",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/target-group-arn": *preCreatedTargetGroupArn,
				},
			},
			Spec: anv1alpha1.ServiceImportSpec{
				Type: anv1alpha1.ClusterSetIP,
				Ports: []anv1alpha1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentImport)

		parentNS := gwv1.Namespace(gateway.Namespace)
		currentRoute = &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route-2",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/service-name-override": customerServiceName,
				},
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name:      gwv1.ObjectName(gateway.Name),
						Namespace: &parentNS,
					}},
				},
				Rules: []gwv1.HTTPRouteRule{{
					BackendRefs: []gwv1.HTTPBackendRef{
						{BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName("import-2"),
								Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
							},
							Weight: lo.ToPtr(int32(90)),
						}},
						{BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName(service.Name),
								Kind: lo.ToPtr(gwv1.Kind("Service")),
								Port: lo.ToPtr(gwv1.PortNumber(80)),
							},
							Weight: lo.ToPtr(int32(10)),
						}},
					},
				}},
			},
		}
		testFramework.ExpectCreated(ctx, currentRoute)

		Eventually(func(g Gomega) {
			rt := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), rt)).To(Succeed())
			g.Expect(rt.Status.Parents).ToNot(BeEmpty())
			conds := map[string]metav1.Condition{}
			for _, c := range rt.Status.Parents[0].Conditions {
				conds[c.Type] = c
			}
			accepted, ok := conds[string(gwv1.RouteConditionAccepted)]
			g.Expect(ok).To(BeTrue())
			g.Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))

			resolved, ok := conds[string(gwv1.RouteConditionResolvedRefs)]
			g.Expect(ok).To(BeTrue())
			g.Expect(resolved.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(resolved.Reason).To(Equal(string(gwv1.RouteReasonResolvedRefs)))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeTrue(), "service ManagedBy stamped")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			listenerResp, err := testFramework.LatticeClient.GetListener(ctx, &vpclattice.GetListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			g.Expect(err).To(BeNil(), "listener exists after apply")
			g.Expect(*listenerResp.Arn).To(Equal(*preCreatedListenerArn), "listener ARN preserved")
			g.Expect(*listenerResp.Port).To(BeEquivalentTo(80), "listener port unchanged")
			g.Expect(string(listenerResp.Protocol)).To(Equal(string(types.ListenerProtocolHttp)), "listener protocol unchanged")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			rules, err := testFramework.LatticeClient.ListRulesAsList(ctx, &vpclattice.ListRulesInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			g.Expect(err).To(BeNil())

			var routeRule *types.RuleSummary
			for i := range rules {
				if rules[i].IsDefault != nil && *rules[i].IsDefault {
					continue
				}
				routeRule = &rules[i]
				break
			}
			g.Expect(routeRule).NotTo(BeNil(), "non-default rule after claim")

			ruleResp, err := testFramework.LatticeClient.GetRule(ctx, &vpclattice.GetRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
				RuleIdentifier:     routeRule.Id,
			})
			g.Expect(err).To(BeNil())

			forward, ok := ruleResp.Action.(*types.RuleActionMemberForward)
			g.Expect(ok).To(BeTrue())
			g.Expect(forward.Value.TargetGroups).To(HaveLen(2), "forward across external+EKS TG")

			var externalWeight, eksWeight int32 = -1, -1
			for _, wtg := range forward.Value.TargetGroups {
				if *wtg.TargetGroupIdentifier == *preCreatedTargetGroupId {
					externalWeight = *wtg.Weight
				} else {
					eksWeight = *wtg.Weight
				}
			}
			g.Expect(externalWeight).To(BeEquivalentTo(90), "external TG weight 90")
			g.Expect(eksWeight).To(BeEquivalentTo(10), "EKS TG weight 10")
			g.Expect(externalWeight+eksWeight).To(BeEquivalentTo(100), "weights sum to 100")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedTargetGroupArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeFalse(), "external TG untagged")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			latestRoute := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), latestRoute)).To(Succeed())
			latestRoute.Spec.Rules[0].BackendRefs = []gwv1.HTTPBackendRef{
				{BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: gwv1.ObjectName(service.Name),
						Kind: lo.ToPtr(gwv1.Kind("Service")),
						Port: lo.ToPtr(gwv1.PortNumber(80)),
					},
					Weight: lo.ToPtr(int32(100)),
				}},
			}
			g.Expect(testFramework.Update(ctx, latestRoute)).To(Succeed())
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			listenerResp, err := testFramework.LatticeClient.GetListener(ctx, &vpclattice.GetListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			g.Expect(err).To(BeNil(), "listener exists after apply")
			g.Expect(*listenerResp.Arn).To(Equal(*preCreatedListenerArn), "listener ARN preserved")
			g.Expect(*listenerResp.Port).To(BeEquivalentTo(80), "listener port unchanged")
			g.Expect(string(listenerResp.Protocol)).To(Equal(string(types.ListenerProtocolHttp)), "listener protocol unchanged")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			rules, err := testFramework.LatticeClient.ListRulesAsList(ctx, &vpclattice.ListRulesInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			g.Expect(err).To(BeNil())

			nonDefault := lo.Filter(rules, func(r types.RuleSummary, _ int) bool {
				return r.IsDefault == nil || !*r.IsDefault
			})
			g.Expect(nonDefault).To(HaveLen(1), "1 non-default rule after shift")

			ruleResp, err := testFramework.LatticeClient.GetRule(ctx, &vpclattice.GetRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
				RuleIdentifier:     nonDefault[0].Id,
			})
			g.Expect(err).To(BeNil())
			forward, ok := ruleResp.Action.(*types.RuleActionMemberForward)
			g.Expect(ok).To(BeTrue(), "rule action is Forward")
			g.Expect(forward.Value.TargetGroups).To(HaveLen(1), "forward has 1 TG after shift")
			g.Expect(*forward.Value.TargetGroups[0].TargetGroupIdentifier).ToNot(Equal(*preCreatedTargetGroupId), "surviving TG is not external")
			g.Expect(*forward.Value.TargetGroups[0].Weight).To(BeEquivalentTo(100), "EKS TG weight 100")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			tgResp, err := testFramework.LatticeClient.GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: preCreatedTargetGroupArn,
			})
			g.Expect(err).To(BeNil(), "external TG exists")
			g.Expect(string(tgResp.Status)).To(Equal(string(types.TargetGroupStatusActive)), "external TG Active")

			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedTargetGroupArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeFalse(), "external TG untagged")
		}).Should(Succeed())
	})
})

var _ = Describe("External Service with single listener and 2 rules", Ordered, Label("service-import-external-tg"), func() {
	var (
		preCreatedSnId *string
		gateway        *gwv1.Gateway

		preCreatedServiceArn      *string
		preCreatedListenerArn     *string
		preCreatedTargetGroupArnA *string
		preCreatedTargetGroupIdA  *string
		preCreatedTargetGroupArnB *string
		preCreatedTargetGroupIdB  *string
		preCreatedAssociationArn  *string
		preCreatedRuleIdV1        *string
		preCreatedRuleIdV2        *string

		deploymentV1   *appsv1.Deployment
		serviceV1      *corev1.Service
		deploymentV2   *appsv1.Deployment
		serviceV2      *corev1.Service
		currentRoute   *gwv1.HTTPRoute
		currentImportA *anv1alpha1.ServiceImport
		currentImportB *anv1alpha1.ServiceImport

		customerServiceName          = "external-svc-3"
		serviceNetworkAndGatewayName = "external-sn-3"
	)

	BeforeAll(func() {
		snResp, err := testFramework.LatticeClient.CreateServiceNetwork(ctx, &vpclattice.CreateServiceNetworkInput{
			Name: aws.String(serviceNetworkAndGatewayName),
		})
		Expect(err).To(BeNil())
		preCreatedSnId = snResp.Id

		gateway = &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceNetworkAndGatewayName,
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     "http",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: lo.ToPtr(gwv1.NamespacesFromSame),
							},
						},
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, gateway)

		Eventually(func(g Gomega) {
			gw := &gwv1.Gateway{}
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(gateway), gw)
			g.Expect(err).To(BeNil())
			programmed := false
			for _, cond := range gw.Status.Conditions {
				if cond.Type == string(gwv1.GatewayConditionProgrammed) &&
					cond.Status == metav1.ConditionTrue {
					programmed = true
					break
				}
			}
			g.Expect(programmed).To(BeTrue(), "gateway not Programmed")
		}).Should(Succeed())

		tgRespA, err := testFramework.LatticeClient.CreateTargetGroup(ctx, &vpclattice.CreateTargetGroupInput{
			Name: aws.String("external-tg-v1-3"),
			Type: types.TargetGroupTypeIp,
			Config: &types.TargetGroupConfig{
				Protocol:      types.TargetGroupProtocolHttp,
				Port:          aws.Int32(80),
				VpcIdentifier: aws.String(testFramework.Cloud.Config().VpcId),
			},
		})
		Expect(err).To(BeNil())
		preCreatedTargetGroupArnA = tgRespA.Arn
		preCreatedTargetGroupIdA = tgRespA.Id

		Eventually(func(g Gomega) {
			getResp, err := testFramework.LatticeClient.GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: preCreatedTargetGroupArnA,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getResp.Status)).To(Equal(string(types.TargetGroupStatusActive)))
		}).Should(Succeed())

		tgRespB, err := testFramework.LatticeClient.CreateTargetGroup(ctx, &vpclattice.CreateTargetGroupInput{
			Name: aws.String("external-tg-v2-3"),
			Type: types.TargetGroupTypeIp,
			Config: &types.TargetGroupConfig{
				Protocol:      types.TargetGroupProtocolHttp,
				Port:          aws.Int32(80),
				VpcIdentifier: aws.String(testFramework.Cloud.Config().VpcId),
			},
		})
		Expect(err).To(BeNil())
		preCreatedTargetGroupArnB = tgRespB.Arn
		preCreatedTargetGroupIdB = tgRespB.Id

		Eventually(func(g Gomega) {
			getResp, err := testFramework.LatticeClient.GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: preCreatedTargetGroupArnB,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getResp.Status)).To(Equal(string(types.TargetGroupStatusActive)))
		}).Should(Succeed())

		serviceResp, err := testFramework.LatticeClient.CreateService(ctx, &vpclattice.CreateServiceInput{
			Name: aws.String(customerServiceName),
		})
		Expect(err).To(BeNil())
		preCreatedServiceArn = serviceResp.Arn

		Eventually(func(g Gomega) {
			getServiceResp, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getServiceResp.Status)).To(Equal(string(types.ServiceStatusActive)))
		}).Should(Succeed())

		listenerResp, err := testFramework.LatticeClient.CreateListener(ctx, &vpclattice.CreateListenerInput{
			ServiceIdentifier: preCreatedServiceArn,
			Name:              aws.String("external-listener-3"),
			Protocol:          types.ListenerProtocolHttp,
			Port:              aws.Int32(80),
			DefaultAction: &types.RuleActionMemberFixedResponse{
				Value: types.FixedResponseAction{
					StatusCode: aws.Int32(404),
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedListenerArn = listenerResp.Arn

		ruleRespV1, err := testFramework.LatticeClient.CreateRule(ctx, &vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn,
			Name:               aws.String("external-rule-v1-3"),
			Priority:           aws.Int32(10),
			Match: &types.RuleMatchMemberHttpMatch{
				Value: types.HttpMatch{
					PathMatch: &types.PathMatch{
						Match:         &types.PathMatchTypeMemberPrefix{Value: "/v1"},
						CaseSensitive: aws.Bool(false),
					},
				},
			},
			Action: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{
						{TargetGroupIdentifier: preCreatedTargetGroupArnA, Weight: aws.Int32(100)},
					},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleIdV1 = ruleRespV1.Id

		ruleRespV2, err := testFramework.LatticeClient.CreateRule(ctx, &vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn,
			Name:               aws.String("external-rule-v2-3"),
			Priority:           aws.Int32(20),
			Match: &types.RuleMatchMemberHttpMatch{
				Value: types.HttpMatch{
					PathMatch: &types.PathMatch{
						Match:         &types.PathMatchTypeMemberPrefix{Value: "/v2"},
						CaseSensitive: aws.Bool(false),
					},
				},
			},
			Action: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{
						{TargetGroupIdentifier: preCreatedTargetGroupArnB, Weight: aws.Int32(100)},
					},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleIdV2 = ruleRespV2.Id

		associationResp, err := testFramework.LatticeClient.CreateServiceNetworkServiceAssociation(ctx, &vpclattice.CreateServiceNetworkServiceAssociationInput{
			ServiceIdentifier:        preCreatedServiceArn,
			ServiceNetworkIdentifier: preCreatedSnId,
		})
		Expect(err).To(BeNil())
		preCreatedAssociationArn = associationResp.Arn

		Eventually(func(g Gomega) {
			getAssocResp, err := testFramework.LatticeClient.GetServiceNetworkServiceAssociation(ctx, &vpclattice.GetServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getAssocResp.Status)).To(Equal(string(types.ServiceNetworkServiceAssociationStatusActive)))
		}).Should(Succeed())

		deploymentV1, serviceV1 = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:       "eks-v1-3",
			Namespace:  k8snamespace,
			Port:       80,
			TargetPort: 8080,
		})
		testFramework.ExpectCreated(ctx, deploymentV1, serviceV1)

		deploymentV2, serviceV2 = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:       "eks-v2-3",
			Namespace:  k8snamespace,
			Port:       80,
			TargetPort: 8080,
		})
		testFramework.ExpectCreated(ctx, deploymentV2, serviceV2)
	})

	AfterEach(func() {
		if currentRoute != nil {
			_ = testFramework.Client.Delete(ctx, currentRoute)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), currentRoute)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentRoute = nil
		}
		if currentImportA != nil {
			_ = testFramework.Client.Delete(ctx, currentImportA)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentImportA), currentImportA)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentImportA = nil
		}
		if currentImportB != nil {
			_ = testFramework.Client.Delete(ctx, currentImportB)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentImportB), currentImportB)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentImportB = nil
		}
	})

	AfterAll(func() {
		if preCreatedAssociationArn != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetworkServiceAssociation(ctx, &vpclattice.DeleteServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete association %s: %v", *preCreatedAssociationArn, err)
				}
			}
		}

		if preCreatedRuleIdV1 != nil {
			_, err := testFramework.LatticeClient.DeleteRule(ctx, &vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
				RuleIdentifier:     preCreatedRuleIdV1,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete rule v1 %s: %v", *preCreatedRuleIdV1, err)
				}
			}
		}

		if preCreatedRuleIdV2 != nil {
			_, err := testFramework.LatticeClient.DeleteRule(ctx, &vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
				RuleIdentifier:     preCreatedRuleIdV2,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete rule v2 %s: %v", *preCreatedRuleIdV2, err)
				}
			}
		}

		if preCreatedListenerArn != nil {
			_, err := testFramework.LatticeClient.DeleteListener(ctx, &vpclattice.DeleteListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete listener %s: %v", *preCreatedListenerArn, err)
				}
			}
		}

		if preCreatedServiceArn != nil {
			_, err := testFramework.LatticeClient.DeleteService(ctx, &vpclattice.DeleteServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete service %s: %v", *preCreatedServiceArn, err)
				}
			}
		}

		if preCreatedTargetGroupArnA != nil {
			Eventually(func(g Gomega) {
				_, err := testFramework.LatticeClient.DeleteTargetGroup(ctx, &vpclattice.DeleteTargetGroupInput{
					TargetGroupIdentifier: preCreatedTargetGroupArnA,
				})
				if err != nil {
					var respErr *smithyhttp.ResponseError
					if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
						return
					}
				}
				g.Expect(err).To(BeNil())
			}).Should(Succeed())
		}

		if preCreatedTargetGroupArnB != nil {
			Eventually(func(g Gomega) {
				_, err := testFramework.LatticeClient.DeleteTargetGroup(ctx, &vpclattice.DeleteTargetGroupInput{
					TargetGroupIdentifier: preCreatedTargetGroupArnB,
				})
				if err != nil {
					var respErr *smithyhttp.ResponseError
					if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
						return
					}
				}
				g.Expect(err).To(BeNil())
			}).Should(Succeed())
		}

		if deploymentV1 != nil && serviceV1 != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, deploymentV1, serviceV1)
		}
		if deploymentV2 != nil && serviceV2 != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, deploymentV2, serviceV2)
		}

		if gateway != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, gateway)
		}

		if preCreatedSnId != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetwork(ctx, &vpclattice.DeleteServiceNetworkInput{
				ServiceNetworkIdentifier: preCreatedSnId,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete service network %s: %v", *preCreatedSnId, err)
				}
			}
		}
	})

	It("HTTPRoute with path based rules takes over customer service and shifts traffic to 90/10 per path", func() {
		currentImportA = &anv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "import-v1-3",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/target-group-arn": *preCreatedTargetGroupArnA,
				},
			},
			Spec: anv1alpha1.ServiceImportSpec{
				Type: anv1alpha1.ClusterSetIP,
				Ports: []anv1alpha1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentImportA)

		currentImportB = &anv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "import-v2-3",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/target-group-arn": *preCreatedTargetGroupArnB,
				},
			},
			Spec: anv1alpha1.ServiceImportSpec{
				Type: anv1alpha1.ClusterSetIP,
				Ports: []anv1alpha1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentImportB)

		parentNS := gwv1.Namespace(gateway.Namespace)
		currentRoute = &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route-3",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/service-name-override": customerServiceName,
				},
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name:      gwv1.ObjectName(gateway.Name),
						Namespace: &parentNS,
					}},
				},
				Rules: []gwv1.HTTPRouteRule{
					{
						Matches: []gwv1.HTTPRouteMatch{{
							Path: &gwv1.HTTPPathMatch{
								Type:  lo.ToPtr(gwv1.PathMatchPathPrefix),
								Value: lo.ToPtr("/v1"),
							},
						}},
						BackendRefs: []gwv1.HTTPBackendRef{
							{BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName("import-v1-3"),
									Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
								},
								Weight: lo.ToPtr(int32(90)),
							}},
							{BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName(serviceV1.Name),
									Kind: lo.ToPtr(gwv1.Kind("Service")),
									Port: lo.ToPtr(gwv1.PortNumber(80)),
								},
								Weight: lo.ToPtr(int32(10)),
							}},
						},
					},
					{
						Matches: []gwv1.HTTPRouteMatch{{
							Path: &gwv1.HTTPPathMatch{
								Type:  lo.ToPtr(gwv1.PathMatchPathPrefix),
								Value: lo.ToPtr("/v2"),
							},
						}},
						BackendRefs: []gwv1.HTTPBackendRef{
							{BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName("import-v2-3"),
									Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
								},
								Weight: lo.ToPtr(int32(90)),
							}},
							{BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName(serviceV2.Name),
									Kind: lo.ToPtr(gwv1.Kind("Service")),
									Port: lo.ToPtr(gwv1.PortNumber(80)),
								},
								Weight: lo.ToPtr(int32(10)),
							}},
						},
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentRoute)

		Eventually(func(g Gomega) {
			rt := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), rt)).To(Succeed())
			g.Expect(rt.Status.Parents).ToNot(BeEmpty())
			conds := map[string]metav1.Condition{}
			for _, c := range rt.Status.Parents[0].Conditions {
				conds[c.Type] = c
			}
			accepted, ok := conds[string(gwv1.RouteConditionAccepted)]
			g.Expect(ok).To(BeTrue())
			g.Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))

			resolved, ok := conds[string(gwv1.RouteConditionResolvedRefs)]
			g.Expect(ok).To(BeTrue())
			g.Expect(resolved.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(resolved.Reason).To(Equal(string(gwv1.RouteReasonResolvedRefs)))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeTrue(), "service ManagedBy stamped")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			listenerResp, err := testFramework.LatticeClient.GetListener(ctx, &vpclattice.GetListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			g.Expect(err).To(BeNil(), "listener exists after apply")
			g.Expect(*listenerResp.Arn).To(Equal(*preCreatedListenerArn), "listener ARN preserved")
			g.Expect(*listenerResp.Port).To(BeEquivalentTo(80), "listener port unchanged")
			g.Expect(string(listenerResp.Protocol)).To(Equal(string(types.ListenerProtocolHttp)), "listener protocol unchanged")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			rules, err := testFramework.LatticeClient.ListRulesAsList(ctx, &vpclattice.ListRulesInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			g.Expect(err).To(BeNil())

			nonDefault := []types.RuleSummary{}
			for _, r := range rules {
				if r.IsDefault != nil && *r.IsDefault {
					continue
				}
				nonDefault = append(nonDefault, r)
			}
			g.Expect(nonDefault).To(HaveLen(2), "2 non-default rules")

			var foundA, foundB bool
			for _, r := range nonDefault {
				ruleResp, err := testFramework.LatticeClient.GetRule(ctx, &vpclattice.GetRuleInput{
					ServiceIdentifier:  preCreatedServiceArn,
					ListenerIdentifier: preCreatedListenerArn,
					RuleIdentifier:     r.Id,
				})
				g.Expect(err).To(BeNil())

				forward, ok := ruleResp.Action.(*types.RuleActionMemberForward)
				g.Expect(ok).To(BeTrue(), "rule action is Forward")
				g.Expect(forward.Value.TargetGroups).To(HaveLen(2), "forward across external+EKS TG")

				var externalId string
				var externalWeight, eksWeight int32 = -1, -1
				for _, wtg := range forward.Value.TargetGroups {
					if *wtg.TargetGroupIdentifier == *preCreatedTargetGroupIdA || *wtg.TargetGroupIdentifier == *preCreatedTargetGroupIdB {
						externalId = *wtg.TargetGroupIdentifier
						externalWeight = *wtg.Weight
					} else {
						eksWeight = *wtg.Weight
					}
				}
				g.Expect(externalWeight).To(BeEquivalentTo(90), "external TG weight 90")
				g.Expect(eksWeight).To(BeEquivalentTo(10), "EKS TG weight 10")

				if externalId == *preCreatedTargetGroupIdA {
					foundA = true
				} else if externalId == *preCreatedTargetGroupIdB {
					foundB = true
				}
			}
			g.Expect(foundA).To(BeTrue(), "rule forwards to TG A")
			g.Expect(foundB).To(BeTrue(), "rule forwards to TG B")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			for _, arn := range []*string{preCreatedTargetGroupArnA, preCreatedTargetGroupArnB} {
				tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
					ResourceArn: arn,
				})
				g.Expect(err).To(BeNil())
				_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
				g.Expect(hasManagedBy).To(BeFalse(), "external TG %s untagged", *arn)
			}
		}).Should(Succeed())
	})
})

var _ = Describe("External Service with multiple listeners", Ordered, Label("service-import-external-tg"), func() {
	var (
		preCreatedSnId *string
		gateway        *gwv1.Gateway

		preCreatedServiceArn      *string
		preCreatedListenerArn80   *string
		preCreatedListenerArn8080 *string
		preCreatedTargetGroupArnA *string
		preCreatedTargetGroupIdA  *string
		preCreatedTargetGroupArnB *string
		preCreatedTargetGroupIdB  *string
		preCreatedAssociationArn  *string
		preCreatedRuleId80V1      *string
		preCreatedRuleId80V2      *string
		preCreatedRuleId8080V1    *string
		preCreatedRuleId8080V2    *string

		deploymentV1   *appsv1.Deployment
		serviceV1      *corev1.Service
		deploymentV2   *appsv1.Deployment
		serviceV2      *corev1.Service
		currentRoute   *gwv1.HTTPRoute
		currentImportA *anv1alpha1.ServiceImport
		currentImportB *anv1alpha1.ServiceImport

		customerServiceName          = "external-svc-4"
		serviceNetworkAndGatewayName = "external-sn-4"
	)

	BeforeAll(func() {
		snResp, err := testFramework.LatticeClient.CreateServiceNetwork(ctx, &vpclattice.CreateServiceNetworkInput{
			Name: aws.String(serviceNetworkAndGatewayName),
		})
		Expect(err).To(BeNil())
		preCreatedSnId = snResp.Id

		gateway = &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceNetworkAndGatewayName,
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     "http-80",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: lo.ToPtr(gwv1.NamespacesFromSame),
							},
						},
					},
					{
						Name:     "http-8080",
						Protocol: gwv1.HTTPProtocolType,
						Port:     8080,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: lo.ToPtr(gwv1.NamespacesFromSame),
							},
						},
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, gateway)

		Eventually(func(g Gomega) {
			gw := &gwv1.Gateway{}
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(gateway), gw)
			g.Expect(err).To(BeNil())
			programmed := false
			for _, cond := range gw.Status.Conditions {
				if cond.Type == string(gwv1.GatewayConditionProgrammed) &&
					cond.Status == metav1.ConditionTrue {
					programmed = true
					break
				}
			}
			g.Expect(programmed).To(BeTrue(), "gateway not Programmed")
		}).Should(Succeed())

		tgRespA, err := testFramework.LatticeClient.CreateTargetGroup(ctx, &vpclattice.CreateTargetGroupInput{
			Name: aws.String("external-tg-v1-4"),
			Type: types.TargetGroupTypeIp,
			Config: &types.TargetGroupConfig{
				Protocol:      types.TargetGroupProtocolHttp,
				Port:          aws.Int32(80),
				VpcIdentifier: aws.String(testFramework.Cloud.Config().VpcId),
			},
		})
		Expect(err).To(BeNil())
		preCreatedTargetGroupArnA = tgRespA.Arn
		preCreatedTargetGroupIdA = tgRespA.Id

		Eventually(func(g Gomega) {
			getResp, err := testFramework.LatticeClient.GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: preCreatedTargetGroupArnA,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getResp.Status)).To(Equal(string(types.TargetGroupStatusActive)))
		}).Should(Succeed())

		tgRespB, err := testFramework.LatticeClient.CreateTargetGroup(ctx, &vpclattice.CreateTargetGroupInput{
			Name: aws.String("external-tg-v2-4"),
			Type: types.TargetGroupTypeIp,
			Config: &types.TargetGroupConfig{
				Protocol:      types.TargetGroupProtocolHttp,
				Port:          aws.Int32(80),
				VpcIdentifier: aws.String(testFramework.Cloud.Config().VpcId),
			},
		})
		Expect(err).To(BeNil())
		preCreatedTargetGroupArnB = tgRespB.Arn
		preCreatedTargetGroupIdB = tgRespB.Id

		Eventually(func(g Gomega) {
			getResp, err := testFramework.LatticeClient.GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: preCreatedTargetGroupArnB,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getResp.Status)).To(Equal(string(types.TargetGroupStatusActive)))
		}).Should(Succeed())

		serviceResp, err := testFramework.LatticeClient.CreateService(ctx, &vpclattice.CreateServiceInput{
			Name: aws.String(customerServiceName),
		})
		Expect(err).To(BeNil())
		preCreatedServiceArn = serviceResp.Arn

		Eventually(func(g Gomega) {
			getServiceResp, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getServiceResp.Status)).To(Equal(string(types.ServiceStatusActive)))
		}).Should(Succeed())

		listenerResp80, err := testFramework.LatticeClient.CreateListener(ctx, &vpclattice.CreateListenerInput{
			ServiceIdentifier: preCreatedServiceArn,
			Name:              aws.String("external-listener-80-4"),
			Protocol:          types.ListenerProtocolHttp,
			Port:              aws.Int32(80),
			DefaultAction: &types.RuleActionMemberFixedResponse{
				Value: types.FixedResponseAction{
					StatusCode: aws.Int32(404),
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedListenerArn80 = listenerResp80.Arn

		listenerResp8080, err := testFramework.LatticeClient.CreateListener(ctx, &vpclattice.CreateListenerInput{
			ServiceIdentifier: preCreatedServiceArn,
			Name:              aws.String("external-listener-8080-4"),
			Protocol:          types.ListenerProtocolHttp,
			Port:              aws.Int32(8080),
			DefaultAction: &types.RuleActionMemberFixedResponse{
				Value: types.FixedResponseAction{
					StatusCode: aws.Int32(404),
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedListenerArn8080 = listenerResp8080.Arn

		ruleResp80V1, err := testFramework.LatticeClient.CreateRule(ctx, &vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn80,
			Name:               aws.String("external-rule-80-v1-4"),
			Priority:           aws.Int32(10),
			Match: &types.RuleMatchMemberHttpMatch{
				Value: types.HttpMatch{
					PathMatch: &types.PathMatch{
						Match:         &types.PathMatchTypeMemberPrefix{Value: "/v1"},
						CaseSensitive: aws.Bool(false),
					},
				},
			},
			Action: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{
						{TargetGroupIdentifier: preCreatedTargetGroupArnA, Weight: aws.Int32(100)},
					},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleId80V1 = ruleResp80V1.Id

		ruleResp80V2, err := testFramework.LatticeClient.CreateRule(ctx, &vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn80,
			Name:               aws.String("external-rule-80-v2-4"),
			Priority:           aws.Int32(20),
			Match: &types.RuleMatchMemberHttpMatch{
				Value: types.HttpMatch{
					PathMatch: &types.PathMatch{
						Match:         &types.PathMatchTypeMemberPrefix{Value: "/v2"},
						CaseSensitive: aws.Bool(false),
					},
				},
			},
			Action: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{
						{TargetGroupIdentifier: preCreatedTargetGroupArnB, Weight: aws.Int32(100)},
					},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleId80V2 = ruleResp80V2.Id

		ruleResp8080V1, err := testFramework.LatticeClient.CreateRule(ctx, &vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn8080,
			Name:               aws.String("external-rule-8080-v1-4"),
			Priority:           aws.Int32(10),
			Match: &types.RuleMatchMemberHttpMatch{
				Value: types.HttpMatch{
					PathMatch: &types.PathMatch{
						Match:         &types.PathMatchTypeMemberPrefix{Value: "/v1"},
						CaseSensitive: aws.Bool(false),
					},
				},
			},
			Action: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{
						{TargetGroupIdentifier: preCreatedTargetGroupArnA, Weight: aws.Int32(100)},
					},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleId8080V1 = ruleResp8080V1.Id

		ruleResp8080V2, err := testFramework.LatticeClient.CreateRule(ctx, &vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn8080,
			Name:               aws.String("external-rule-8080-v2-4"),
			Priority:           aws.Int32(20),
			Match: &types.RuleMatchMemberHttpMatch{
				Value: types.HttpMatch{
					PathMatch: &types.PathMatch{
						Match:         &types.PathMatchTypeMemberPrefix{Value: "/v2"},
						CaseSensitive: aws.Bool(false),
					},
				},
			},
			Action: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{
						{TargetGroupIdentifier: preCreatedTargetGroupArnB, Weight: aws.Int32(100)},
					},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleId8080V2 = ruleResp8080V2.Id

		associationResp, err := testFramework.LatticeClient.CreateServiceNetworkServiceAssociation(ctx, &vpclattice.CreateServiceNetworkServiceAssociationInput{
			ServiceIdentifier:        preCreatedServiceArn,
			ServiceNetworkIdentifier: preCreatedSnId,
		})
		Expect(err).To(BeNil())
		preCreatedAssociationArn = associationResp.Arn

		Eventually(func(g Gomega) {
			getAssocResp, err := testFramework.LatticeClient.GetServiceNetworkServiceAssociation(ctx, &vpclattice.GetServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getAssocResp.Status)).To(Equal(string(types.ServiceNetworkServiceAssociationStatusActive)))
		}).Should(Succeed())

		deploymentV1, serviceV1 = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:       "eks-v1-4",
			Namespace:  k8snamespace,
			Port:       80,
			TargetPort: 8080,
		})
		testFramework.ExpectCreated(ctx, deploymentV1, serviceV1)

		deploymentV2, serviceV2 = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:       "eks-v2-4",
			Namespace:  k8snamespace,
			Port:       80,
			TargetPort: 8080,
		})
		testFramework.ExpectCreated(ctx, deploymentV2, serviceV2)
	})

	AfterEach(func() {
		if currentRoute != nil {
			_ = testFramework.Client.Delete(ctx, currentRoute)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), currentRoute)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentRoute = nil
		}
		if currentImportA != nil {
			_ = testFramework.Client.Delete(ctx, currentImportA)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentImportA), currentImportA)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentImportA = nil
		}
		if currentImportB != nil {
			_ = testFramework.Client.Delete(ctx, currentImportB)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentImportB), currentImportB)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentImportB = nil
		}
	})

	AfterAll(func() {
		if preCreatedAssociationArn != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetworkServiceAssociation(ctx, &vpclattice.DeleteServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete association %s: %v", *preCreatedAssociationArn, err)
				}
			}
		}

		if preCreatedRuleId80V1 != nil {
			_, err := testFramework.LatticeClient.DeleteRule(ctx, &vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn80,
				RuleIdentifier:     preCreatedRuleId80V1,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete rule 80 v1 %s: %v", *preCreatedRuleId80V1, err)
				}
			}
		}

		if preCreatedRuleId80V2 != nil {
			_, err := testFramework.LatticeClient.DeleteRule(ctx, &vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn80,
				RuleIdentifier:     preCreatedRuleId80V2,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete rule 80 v2 %s: %v", *preCreatedRuleId80V2, err)
				}
			}
		}

		if preCreatedRuleId8080V1 != nil {
			_, err := testFramework.LatticeClient.DeleteRule(ctx, &vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn8080,
				RuleIdentifier:     preCreatedRuleId8080V1,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete rule 8080 v1 %s: %v", *preCreatedRuleId8080V1, err)
				}
			}
		}

		if preCreatedRuleId8080V2 != nil {
			_, err := testFramework.LatticeClient.DeleteRule(ctx, &vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn8080,
				RuleIdentifier:     preCreatedRuleId8080V2,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete rule 8080 v2 %s: %v", *preCreatedRuleId8080V2, err)
				}
			}
		}

		if preCreatedListenerArn80 != nil {
			_, err := testFramework.LatticeClient.DeleteListener(ctx, &vpclattice.DeleteListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn80,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete listener 80 %s: %v", *preCreatedListenerArn80, err)
				}
			}
		}

		if preCreatedListenerArn8080 != nil {
			_, err := testFramework.LatticeClient.DeleteListener(ctx, &vpclattice.DeleteListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn8080,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete listener 8080 %s: %v", *preCreatedListenerArn8080, err)
				}
			}
		}

		if preCreatedServiceArn != nil {
			_, err := testFramework.LatticeClient.DeleteService(ctx, &vpclattice.DeleteServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete service %s: %v", *preCreatedServiceArn, err)
				}
			}
		}

		if preCreatedTargetGroupArnA != nil {
			Eventually(func(g Gomega) {
				_, err := testFramework.LatticeClient.DeleteTargetGroup(ctx, &vpclattice.DeleteTargetGroupInput{
					TargetGroupIdentifier: preCreatedTargetGroupArnA,
				})
				if err != nil {
					var respErr *smithyhttp.ResponseError
					if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
						return
					}
				}
				g.Expect(err).To(BeNil())
			}).Should(Succeed())
		}

		if preCreatedTargetGroupArnB != nil {
			Eventually(func(g Gomega) {
				_, err := testFramework.LatticeClient.DeleteTargetGroup(ctx, &vpclattice.DeleteTargetGroupInput{
					TargetGroupIdentifier: preCreatedTargetGroupArnB,
				})
				if err != nil {
					var respErr *smithyhttp.ResponseError
					if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
						return
					}
				}
				g.Expect(err).To(BeNil())
			}).Should(Succeed())
		}

		if deploymentV1 != nil && serviceV1 != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, deploymentV1, serviceV1)
		}
		if deploymentV2 != nil && serviceV2 != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, deploymentV2, serviceV2)
		}

		if gateway != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, gateway)
		}

		if preCreatedSnId != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetwork(ctx, &vpclattice.DeleteServiceNetworkInput{
				ServiceNetworkIdentifier: preCreatedSnId,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete service network %s: %v", *preCreatedSnId, err)
				}
			}
		}
	})

	It("HttpRoute with no sectionName replicates path segmented weighted rules across both listeners and shifts traffic to 90/10", func() {
		currentImportA = &anv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "import-v1-4",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/target-group-arn": *preCreatedTargetGroupArnA,
				},
			},
			Spec: anv1alpha1.ServiceImportSpec{
				Type: anv1alpha1.ClusterSetIP,
				Ports: []anv1alpha1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentImportA)

		currentImportB = &anv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "import-v2-4",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/target-group-arn": *preCreatedTargetGroupArnB,
				},
			},
			Spec: anv1alpha1.ServiceImportSpec{
				Type: anv1alpha1.ClusterSetIP,
				Ports: []anv1alpha1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentImportB)

		parentNS := gwv1.Namespace(gateway.Namespace)
		currentRoute = &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route-4",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/service-name-override": customerServiceName,
				},
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name:      gwv1.ObjectName(gateway.Name),
						Namespace: &parentNS,
					}},
				},
				Rules: []gwv1.HTTPRouteRule{
					{ // /v1
						Matches: []gwv1.HTTPRouteMatch{{
							Path: &gwv1.HTTPPathMatch{
								Type:  lo.ToPtr(gwv1.PathMatchPathPrefix),
								Value: lo.ToPtr("/v1"),
							},
						}},
						BackendRefs: []gwv1.HTTPBackendRef{
							{BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName("import-v1-4"),
									Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
								},
								Weight: lo.ToPtr(int32(90)),
							}},
							{BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName(serviceV1.Name),
									Kind: lo.ToPtr(gwv1.Kind("Service")),
									Port: lo.ToPtr(gwv1.PortNumber(80)),
								},
								Weight: lo.ToPtr(int32(10)),
							}},
						},
					},
					{ // /v2
						Matches: []gwv1.HTTPRouteMatch{{
							Path: &gwv1.HTTPPathMatch{
								Type:  lo.ToPtr(gwv1.PathMatchPathPrefix),
								Value: lo.ToPtr("/v2"),
							},
						}},
						BackendRefs: []gwv1.HTTPBackendRef{
							{BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName("import-v2-4"),
									Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
								},
								Weight: lo.ToPtr(int32(90)),
							}},
							{BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName(serviceV2.Name),
									Kind: lo.ToPtr(gwv1.Kind("Service")),
									Port: lo.ToPtr(gwv1.PortNumber(80)),
								},
								Weight: lo.ToPtr(int32(10)),
							}},
						},
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentRoute)

		Eventually(func(g Gomega) {
			rt := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), rt)).To(Succeed())
			g.Expect(rt.Status.Parents).ToNot(BeEmpty())
			conds := map[string]metav1.Condition{}
			for _, c := range rt.Status.Parents[0].Conditions {
				conds[c.Type] = c
			}
			accepted, ok := conds[string(gwv1.RouteConditionAccepted)]
			g.Expect(ok).To(BeTrue())
			g.Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))

			resolved, ok := conds[string(gwv1.RouteConditionResolvedRefs)]
			g.Expect(ok).To(BeTrue())
			g.Expect(resolved.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(resolved.Reason).To(Equal(string(gwv1.RouteReasonResolvedRefs)))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeTrue(), "service ManagedBy stamped")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			listeners, err := testFramework.LatticeClient.ListListenersAsList(ctx, &vpclattice.ListListenersInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			var l80, l8080 *types.ListenerSummary
			for i := range listeners {
				switch *listeners[i].Name {
				case "external-listener-80-4":
					l80 = &listeners[i]
				case "external-listener-8080-4":
					l8080 = &listeners[i]
				}
			}
			g.Expect(l80).ToNot(BeNil(), "listener on port %d must exist", 80)
			g.Expect(l8080).ToNot(BeNil(), "listener on port %d must exist", 8080)

			for _, l := range []*types.ListenerSummary{l80, l8080} {
				var expectedListenerArn *string
				var expectedPort int32
				switch *l.Name {
				case "external-listener-80-4":
					expectedListenerArn = preCreatedListenerArn80
					expectedPort = 80
				case "external-listener-8080-4":
					expectedListenerArn = preCreatedListenerArn8080
					expectedPort = 8080
				}
				g.Expect(*l.Arn).To(Equal(*expectedListenerArn), "listener %s ARN preserved", *l.Name)
				g.Expect(*l.Port).To(BeEquivalentTo(expectedPort), "listener %s port unchanged", *l.Name)
				g.Expect(string(l.Protocol)).To(Equal(string(types.ListenerProtocolHttp)), "listener %s protocol unchanged", *l.Name)

				rules, err := testFramework.LatticeClient.ListRulesAsList(ctx, &vpclattice.ListRulesInput{
					ServiceIdentifier:  preCreatedServiceArn,
					ListenerIdentifier: l.Arn,
				})
				g.Expect(err).To(BeNil())

				nonDefault := []types.RuleSummary{}
				for _, r := range rules {
					if r.IsDefault != nil && *r.IsDefault {
						continue
					}
					nonDefault = append(nonDefault, r)
				}
				g.Expect(nonDefault).To(HaveLen(2), "listener %s: 2 non-default rules", *l.Name)

				var foundA, foundB bool
				for _, r := range nonDefault {
					ruleResp, err := testFramework.LatticeClient.GetRule(ctx, &vpclattice.GetRuleInput{
						ServiceIdentifier:  preCreatedServiceArn,
						ListenerIdentifier: l.Arn,
						RuleIdentifier:     r.Id,
					})
					g.Expect(err).To(BeNil())
					forward, ok := ruleResp.Action.(*types.RuleActionMemberForward)
					g.Expect(ok).To(BeTrue(), "rule action is Forward")
					g.Expect(forward.Value.TargetGroups).To(HaveLen(2), "forward across external+EKS TG")

					var externalId string
					var externalWeight, eksWeight int32 = -1, -1
					for _, wtg := range forward.Value.TargetGroups {
						id := *wtg.TargetGroupIdentifier
						if id == *preCreatedTargetGroupIdA || id == *preCreatedTargetGroupIdB {
							externalId = id
							externalWeight = *wtg.Weight
						} else {
							eksWeight = *wtg.Weight
						}
					}
					g.Expect(externalWeight).To(BeEquivalentTo(90), "external TG weight 90 on %s", *l.Name)
					g.Expect(eksWeight).To(BeEquivalentTo(10), "EKS TG weight 10 on %s", *l.Name)

					if externalId == *preCreatedTargetGroupIdA {
						foundA = true
					} else if externalId == *preCreatedTargetGroupIdB {
						foundB = true
					}
				}
				g.Expect(foundA).To(BeTrue(), "%s: rule forwards to TG A", *l.Name)
				g.Expect(foundB).To(BeTrue(), "%s: rule forwards to TG B", *l.Name)
			}
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			for _, arn := range []*string{preCreatedTargetGroupArnA, preCreatedTargetGroupArnB} {
				tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
					ResourceArn: arn,
				})
				g.Expect(err).To(BeNil())
				_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
				g.Expect(hasManagedBy).To(BeFalse(), "external TG %s untagged", *arn)
			}
		}).Should(Succeed())
	})
})

var _ = Describe("External Grpc Service with multiple listeners", Ordered, Label("service-import-external-tg"), func() {
	var (
		preCreatedSnId *string
		gateway        *gwv1.Gateway

		preCreatedServiceArn      *string
		preCreatedListenerArn80   *string
		preCreatedListenerArn8080 *string
		preCreatedTargetGroupArnA *string
		preCreatedTargetGroupIdA  *string
		preCreatedTargetGroupArnB *string
		preCreatedTargetGroupIdB  *string
		preCreatedAssociationArn  *string
		preCreatedRuleId80V1      *string
		preCreatedRuleId80V2      *string
		preCreatedRuleId8080V1    *string
		preCreatedRuleId8080V2    *string

		deploymentV1   *appsv1.Deployment
		serviceV1      *corev1.Service
		deploymentV2   *appsv1.Deployment
		serviceV2      *corev1.Service
		currentRoute   *gwv1.GRPCRoute
		currentImportA *anv1alpha1.ServiceImport
		currentImportB *anv1alpha1.ServiceImport

		customerServiceName          = "external-svc-5"
		serviceNetworkAndGatewayName = "external-sn-5"
	)

	BeforeAll(func() {
		snResp, err := testFramework.LatticeClient.CreateServiceNetwork(ctx, &vpclattice.CreateServiceNetworkInput{
			Name: aws.String(serviceNetworkAndGatewayName),
		})
		Expect(err).To(BeNil())
		preCreatedSnId = snResp.Id

		gateway = &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceNetworkAndGatewayName,
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     "https-80",
						Protocol: gwv1.HTTPSProtocolType,
						Port:     80,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: lo.ToPtr(gwv1.NamespacesFromSame),
							},
						},
					},
					{
						Name:     "https-8080",
						Protocol: gwv1.HTTPSProtocolType,
						Port:     8080,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: lo.ToPtr(gwv1.NamespacesFromSame),
							},
						},
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, gateway)

		Eventually(func(g Gomega) {
			gw := &gwv1.Gateway{}
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(gateway), gw)
			g.Expect(err).To(BeNil())
			programmed := false
			for _, cond := range gw.Status.Conditions {
				if cond.Type == string(gwv1.GatewayConditionProgrammed) &&
					cond.Status == metav1.ConditionTrue {
					programmed = true
					break
				}
			}
			g.Expect(programmed).To(BeTrue(), "gateway not Programmed")
		}).Should(Succeed())

		tgRespA, err := testFramework.LatticeClient.CreateTargetGroup(ctx, &vpclattice.CreateTargetGroupInput{
			Name: aws.String("external-tg-v1-5"),
			Type: types.TargetGroupTypeIp,
			Config: &types.TargetGroupConfig{
				Protocol:        types.TargetGroupProtocolHttps,
				ProtocolVersion: types.TargetGroupProtocolVersionHttp2,
				Port:            aws.Int32(80),
				VpcIdentifier:   aws.String(testFramework.Cloud.Config().VpcId),
				IpAddressType:   types.IpAddressTypeIpv4,
			},
		})
		Expect(err).To(BeNil())
		preCreatedTargetGroupArnA = tgRespA.Arn
		preCreatedTargetGroupIdA = tgRespA.Id

		Eventually(func(g Gomega) {
			getResp, err := testFramework.LatticeClient.GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: preCreatedTargetGroupArnA,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getResp.Status)).To(Equal(string(types.TargetGroupStatusActive)))
		}).Should(Succeed())

		tgRespB, err := testFramework.LatticeClient.CreateTargetGroup(ctx, &vpclattice.CreateTargetGroupInput{
			Name: aws.String("external-tg-v2-5"),
			Type: types.TargetGroupTypeIp,
			Config: &types.TargetGroupConfig{
				Protocol:        types.TargetGroupProtocolHttps,
				ProtocolVersion: types.TargetGroupProtocolVersionHttp2,
				Port:            aws.Int32(80),
				VpcIdentifier:   aws.String(testFramework.Cloud.Config().VpcId),
				IpAddressType:   types.IpAddressTypeIpv4,
			},
		})
		Expect(err).To(BeNil())
		preCreatedTargetGroupArnB = tgRespB.Arn
		preCreatedTargetGroupIdB = tgRespB.Id

		Eventually(func(g Gomega) {
			getResp, err := testFramework.LatticeClient.GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: preCreatedTargetGroupArnB,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getResp.Status)).To(Equal(string(types.TargetGroupStatusActive)))
		}).Should(Succeed())

		serviceResp, err := testFramework.LatticeClient.CreateService(ctx, &vpclattice.CreateServiceInput{
			Name: aws.String(customerServiceName),
		})
		Expect(err).To(BeNil())
		preCreatedServiceArn = serviceResp.Arn

		Eventually(func(g Gomega) {
			getServiceResp, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getServiceResp.Status)).To(Equal(string(types.ServiceStatusActive)))
		}).Should(Succeed())

		listenerResp80, err := testFramework.LatticeClient.CreateListener(ctx, &vpclattice.CreateListenerInput{
			ServiceIdentifier: preCreatedServiceArn,
			Name:              aws.String("external-listener-80-5"),
			Protocol:          types.ListenerProtocolHttps,
			Port:              aws.Int32(80),
			DefaultAction: &types.RuleActionMemberFixedResponse{
				Value: types.FixedResponseAction{
					StatusCode: aws.Int32(404),
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedListenerArn80 = listenerResp80.Arn

		listenerResp8080, err := testFramework.LatticeClient.CreateListener(ctx, &vpclattice.CreateListenerInput{
			ServiceIdentifier: preCreatedServiceArn,
			Name:              aws.String("external-listener-8080-5"),
			Protocol:          types.ListenerProtocolHttps,
			Port:              aws.Int32(8080),
			DefaultAction: &types.RuleActionMemberFixedResponse{
				Value: types.FixedResponseAction{
					StatusCode: aws.Int32(404),
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedListenerArn8080 = listenerResp8080.Arn

		ruleResp80V1, err := testFramework.LatticeClient.CreateRule(ctx, &vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn80,
			Name:               aws.String("external-rule-80-v1-5"),
			Priority:           aws.Int32(10),
			Match: &types.RuleMatchMemberHttpMatch{
				Value: types.HttpMatch{
					Method: lo.ToPtr("POST"),
					PathMatch: &types.PathMatch{
						CaseSensitive: aws.Bool(true),
						Match: &types.PathMatchTypeMemberExact{
							Value: "/echo.EchoService/V1",
						},
					},
				},
			},
			Action: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{
						{TargetGroupIdentifier: preCreatedTargetGroupArnA, Weight: aws.Int32(100)},
					},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleId80V1 = ruleResp80V1.Id

		ruleResp80V2, err := testFramework.LatticeClient.CreateRule(ctx, &vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn80,
			Name:               aws.String("external-rule-80-v2-5"),
			Priority:           aws.Int32(20),
			Match: &types.RuleMatchMemberHttpMatch{
				Value: types.HttpMatch{
					Method: lo.ToPtr("POST"),
					PathMatch: &types.PathMatch{
						CaseSensitive: aws.Bool(true),
						Match: &types.PathMatchTypeMemberExact{
							Value: "/echo.EchoService/V2",
						},
					},
				},
			},
			Action: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{
						{TargetGroupIdentifier: preCreatedTargetGroupArnB, Weight: aws.Int32(100)},
					},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleId80V2 = ruleResp80V2.Id

		ruleResp8080V1, err := testFramework.LatticeClient.CreateRule(ctx, &vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn8080,
			Name:               aws.String("external-rule-8080-v1-5"),
			Priority:           aws.Int32(10),
			Match: &types.RuleMatchMemberHttpMatch{
				Value: types.HttpMatch{
					Method: lo.ToPtr("POST"),
					PathMatch: &types.PathMatch{
						CaseSensitive: aws.Bool(true),
						Match: &types.PathMatchTypeMemberExact{
							Value: "/echo.EchoService/V1",
						},
					},
				},
			},
			Action: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{
						{TargetGroupIdentifier: preCreatedTargetGroupArnA, Weight: aws.Int32(100)},
					},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleId8080V1 = ruleResp8080V1.Id

		ruleResp8080V2, err := testFramework.LatticeClient.CreateRule(ctx, &vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn8080,
			Name:               aws.String("external-rule-8080-v2-5"),
			Priority:           aws.Int32(20),
			Match: &types.RuleMatchMemberHttpMatch{
				Value: types.HttpMatch{
					Method: lo.ToPtr("POST"),
					PathMatch: &types.PathMatch{
						CaseSensitive: aws.Bool(true),
						Match: &types.PathMatchTypeMemberExact{
							Value: "/echo.EchoService/V2",
						},
					},
				},
			},
			Action: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{
						{TargetGroupIdentifier: preCreatedTargetGroupArnB, Weight: aws.Int32(100)},
					},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleId8080V2 = ruleResp8080V2.Id

		associationResp, err := testFramework.LatticeClient.CreateServiceNetworkServiceAssociation(ctx, &vpclattice.CreateServiceNetworkServiceAssociationInput{
			ServiceIdentifier:        preCreatedServiceArn,
			ServiceNetworkIdentifier: preCreatedSnId,
		})
		Expect(err).To(BeNil())
		preCreatedAssociationArn = associationResp.Arn

		Eventually(func(g Gomega) {
			getAssocResp, err := testFramework.LatticeClient.GetServiceNetworkServiceAssociation(ctx, &vpclattice.GetServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getAssocResp.Status)).To(Equal(string(types.ServiceNetworkServiceAssociationStatusActive)))
		}).Should(Succeed())

		deploymentV1, serviceV1 = testFramework.NewGrpcBin(test.GrpcAppOptions{
			AppName:   "eks-v1-5",
			Namespace: k8snamespace,
		})
		testFramework.ExpectCreated(ctx, deploymentV1, serviceV1)

		deploymentV2, serviceV2 = testFramework.NewGrpcHelloWorld(test.GrpcAppOptions{
			AppName:   "eks-v2-5",
			Namespace: k8snamespace,
		})
		testFramework.ExpectCreated(ctx, deploymentV2, serviceV2)
	})

	AfterEach(func() {
		if currentRoute != nil {
			_ = testFramework.Client.Delete(ctx, currentRoute)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), currentRoute)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentRoute = nil
		}
		if currentImportA != nil {
			_ = testFramework.Client.Delete(ctx, currentImportA)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentImportA), currentImportA)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentImportA = nil
		}
		if currentImportB != nil {
			_ = testFramework.Client.Delete(ctx, currentImportB)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentImportB), currentImportB)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentImportB = nil
		}
	})

	AfterAll(func() {
		if preCreatedAssociationArn != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetworkServiceAssociation(ctx, &vpclattice.DeleteServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete association %s: %v", *preCreatedAssociationArn, err)
				}
			}
		}

		if preCreatedRuleId80V1 != nil {
			_, err := testFramework.LatticeClient.DeleteRule(ctx, &vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn80,
				RuleIdentifier:     preCreatedRuleId80V1,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete rule 80 v1 %s: %v", *preCreatedRuleId80V1, err)
				}
			}
		}

		if preCreatedRuleId80V2 != nil {
			_, err := testFramework.LatticeClient.DeleteRule(ctx, &vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn80,
				RuleIdentifier:     preCreatedRuleId80V2,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete rule 80 v2 %s: %v", *preCreatedRuleId80V2, err)
				}
			}
		}

		if preCreatedRuleId8080V1 != nil {
			_, err := testFramework.LatticeClient.DeleteRule(ctx, &vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn8080,
				RuleIdentifier:     preCreatedRuleId8080V1,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete rule 8080 v1 %s: %v", *preCreatedRuleId8080V1, err)
				}
			}
		}

		if preCreatedRuleId8080V2 != nil {
			_, err := testFramework.LatticeClient.DeleteRule(ctx, &vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn8080,
				RuleIdentifier:     preCreatedRuleId8080V2,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete rule 8080 v2 %s: %v", *preCreatedRuleId8080V2, err)
				}
			}
		}

		if preCreatedListenerArn80 != nil {
			_, err := testFramework.LatticeClient.DeleteListener(ctx, &vpclattice.DeleteListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn80,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete listener 80 %s: %v", *preCreatedListenerArn80, err)
				}
			}
		}

		if preCreatedListenerArn8080 != nil {
			_, err := testFramework.LatticeClient.DeleteListener(ctx, &vpclattice.DeleteListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn8080,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete listener 8080 %s: %v", *preCreatedListenerArn8080, err)
				}
			}
		}

		if preCreatedServiceArn != nil {
			_, err := testFramework.LatticeClient.DeleteService(ctx, &vpclattice.DeleteServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete service %s: %v", *preCreatedServiceArn, err)
				}
			}
		}

		if preCreatedTargetGroupArnA != nil {
			Eventually(func(g Gomega) {
				_, err := testFramework.LatticeClient.DeleteTargetGroup(ctx, &vpclattice.DeleteTargetGroupInput{
					TargetGroupIdentifier: preCreatedTargetGroupArnA,
				})
				if err != nil {
					var respErr *smithyhttp.ResponseError
					if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
						return
					}
				}
				g.Expect(err).To(BeNil())
			}).Should(Succeed())
		}

		if preCreatedTargetGroupArnB != nil {
			Eventually(func(g Gomega) {
				_, err := testFramework.LatticeClient.DeleteTargetGroup(ctx, &vpclattice.DeleteTargetGroupInput{
					TargetGroupIdentifier: preCreatedTargetGroupArnB,
				})
				if err != nil {
					var respErr *smithyhttp.ResponseError
					if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
						return
					}
				}
				g.Expect(err).To(BeNil())
			}).Should(Succeed())
		}

		if deploymentV1 != nil && serviceV1 != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, deploymentV1, serviceV1)
		}
		if deploymentV2 != nil && serviceV2 != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, deploymentV2, serviceV2)
		}

		if gateway != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, gateway)
		}

		if preCreatedSnId != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetwork(ctx, &vpclattice.DeleteServiceNetworkInput{
				ServiceNetworkIdentifier: preCreatedSnId,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete service network %s: %v", *preCreatedSnId, err)
				}
			}
		}
	})

	It("GRPCRoute with no sectionName replicates method matched weighted rules across both listeners and shifts traffic to 90/10", func() {
		currentImportA = &anv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "import-v1-5",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/target-group-arn": *preCreatedTargetGroupArnA,
				},
			},
			Spec: anv1alpha1.ServiceImportSpec{
				Type: anv1alpha1.ClusterSetIP,
				Ports: []anv1alpha1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentImportA)

		currentImportB = &anv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "import-v2-5",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/target-group-arn": *preCreatedTargetGroupArnB,
				},
			},
			Spec: anv1alpha1.ServiceImportSpec{
				Type: anv1alpha1.ClusterSetIP,
				Ports: []anv1alpha1.ServicePort{
					{Port: 80, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentImportB)

		parentNS := gwv1.Namespace(gateway.Namespace)
		currentRoute = &gwv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route-5",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/service-name-override": customerServiceName,
				},
			},
			Spec: gwv1.GRPCRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name:      gwv1.ObjectName(gateway.Name),
						Namespace: &parentNS,
					}},
				},
				Rules: []gwv1.GRPCRouteRule{
					{
						Matches: []gwv1.GRPCRouteMatch{{
							Method: &gwv1.GRPCMethodMatch{
								Type:    lo.ToPtr(gwv1.GRPCMethodMatchExact),
								Service: lo.ToPtr("echo.EchoService"),
								Method:  lo.ToPtr("V1"),
							},
						}},
						BackendRefs: []gwv1.GRPCBackendRef{
							{BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName("import-v1-5"),
									Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
								},
								Weight: lo.ToPtr(int32(90)),
							}},
							{BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName(serviceV1.Name),
									Kind: lo.ToPtr(gwv1.Kind("Service")),
									Port: lo.ToPtr(gwv1.PortNumber(serviceV1.Spec.Ports[0].Port)),
								},
								Weight: lo.ToPtr(int32(10)),
							}},
						},
					},
					{
						Matches: []gwv1.GRPCRouteMatch{{
							Method: &gwv1.GRPCMethodMatch{
								Type:    lo.ToPtr(gwv1.GRPCMethodMatchExact),
								Service: lo.ToPtr("echo.EchoService"),
								Method:  lo.ToPtr("V2"),
							},
						}},
						BackendRefs: []gwv1.GRPCBackendRef{
							{BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName("import-v2-5"),
									Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
								},
								Weight: lo.ToPtr(int32(90)),
							}},
							{BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName(serviceV2.Name),
									Kind: lo.ToPtr(gwv1.Kind("Service")),
									Port: lo.ToPtr(gwv1.PortNumber(serviceV2.Spec.Ports[0].Port)),
								},
								Weight: lo.ToPtr(int32(10)),
							}},
						},
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentRoute)

		Eventually(func(g Gomega) {
			rt := &gwv1.GRPCRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), rt)).To(Succeed())
			g.Expect(rt.Status.Parents).ToNot(BeEmpty())
			conds := map[string]metav1.Condition{}
			for _, c := range rt.Status.Parents[0].Conditions {
				conds[c.Type] = c
			}
			accepted, ok := conds[string(gwv1.RouteConditionAccepted)]
			g.Expect(ok).To(BeTrue())
			g.Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))

			resolved, ok := conds[string(gwv1.RouteConditionResolvedRefs)]
			g.Expect(ok).To(BeTrue())
			g.Expect(resolved.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(resolved.Reason).To(Equal(string(gwv1.RouteReasonResolvedRefs)))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeTrue(), "service ManagedBy stamped")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			listeners, err := testFramework.LatticeClient.ListListenersAsList(ctx, &vpclattice.ListListenersInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			var l80, l8080 *types.ListenerSummary
			for i := range listeners {
				switch *listeners[i].Name {
				case "external-listener-80-5":
					l80 = &listeners[i]
				case "external-listener-8080-5":
					l8080 = &listeners[i]
				}
			}
			g.Expect(l80).ToNot(BeNil(), "listener on port %d must exist", 80)
			g.Expect(l8080).ToNot(BeNil(), "listener on port %d must exist", 8080)

			for _, l := range []*types.ListenerSummary{l80, l8080} {
				var expectedListenerArn *string
				var expectedPort int32
				switch *l.Name {
				case "external-listener-80-5":
					expectedListenerArn = preCreatedListenerArn80
					expectedPort = 80
				case "external-listener-8080-5":
					expectedListenerArn = preCreatedListenerArn8080
					expectedPort = 8080
				}
				g.Expect(*l.Arn).To(Equal(*expectedListenerArn), "listener %s ARN preserved", *l.Name)
				g.Expect(*l.Port).To(BeEquivalentTo(expectedPort), "listener %s port unchanged", *l.Name)
				g.Expect(string(l.Protocol)).To(Equal(string(types.ListenerProtocolHttps)), "listener %s protocol unchanged", *l.Name)

				rules, err := testFramework.LatticeClient.ListRulesAsList(ctx, &vpclattice.ListRulesInput{
					ServiceIdentifier:  preCreatedServiceArn,
					ListenerIdentifier: l.Arn,
				})
				g.Expect(err).To(BeNil())

				nonDefault := []types.RuleSummary{}
				for _, r := range rules {
					if r.IsDefault != nil && *r.IsDefault {
						continue
					}
					nonDefault = append(nonDefault, r)
				}
				g.Expect(nonDefault).To(HaveLen(2), "listener %s: 2 non-default rules", *l.Name)

				var foundA, foundB bool
				for _, r := range nonDefault {
					ruleResp, err := testFramework.LatticeClient.GetRule(ctx, &vpclattice.GetRuleInput{
						ServiceIdentifier:  preCreatedServiceArn,
						ListenerIdentifier: l.Arn,
						RuleIdentifier:     r.Id,
					})
					g.Expect(err).To(BeNil())

					httpMatch, ok := ruleResp.Match.(*types.RuleMatchMemberHttpMatch)
					g.Expect(ok).To(BeTrue(), "rule match is HttpMatch")
					g.Expect(httpMatch.Value.Method).ToNot(BeNil(), "httpMatch.Method set on %s", *l.Name)
					g.Expect(*httpMatch.Value.Method).To(Equal("POST"), "httpMatch.Method=POST on %s", *l.Name)
					g.Expect(httpMatch.Value.PathMatch).ToNot(BeNil(), "httpMatch.PathMatch set on %s", *l.Name)
					exactMatch, ok := httpMatch.Value.PathMatch.Match.(*types.PathMatchTypeMemberExact)
					g.Expect(ok).To(BeTrue(), "PathMatch is Exact on %s", *l.Name)

					forward, ok := ruleResp.Action.(*types.RuleActionMemberForward)
					g.Expect(ok).To(BeTrue(), "rule action is Forward")
					g.Expect(forward.Value.TargetGroups).To(HaveLen(2), "forward across external+EKS TG")

					var externalId string
					var externalWeight, eksWeight int32 = -1, -1
					for _, wtg := range forward.Value.TargetGroups {
						id := *wtg.TargetGroupIdentifier
						if id == *preCreatedTargetGroupIdA || id == *preCreatedTargetGroupIdB {
							externalId = id
							externalWeight = *wtg.Weight
						} else {
							eksWeight = *wtg.Weight
						}
					}
					g.Expect(externalWeight).To(BeEquivalentTo(90), "external TG weight 90 on %s", *l.Name)
					g.Expect(eksWeight).To(BeEquivalentTo(10), "EKS TG weight 10 on %s", *l.Name)

					if externalId == *preCreatedTargetGroupIdA {
						foundA = true
						g.Expect(exactMatch.Value).To(Equal("/echo.EchoService/V1"), "TG-A rule matches V1 path on %s", *l.Name)
					} else if externalId == *preCreatedTargetGroupIdB {
						foundB = true
						g.Expect(exactMatch.Value).To(Equal("/echo.EchoService/V2"), "TG-B rule matches V2 path on %s", *l.Name)
					}
				}
				g.Expect(foundA).To(BeTrue(), "%s: rule forwards to TG A (V1)", *l.Name)
				g.Expect(foundB).To(BeTrue(), "%s: rule forwards to TG B (V2)", *l.Name)
			}
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			for _, arn := range []*string{preCreatedTargetGroupArnA, preCreatedTargetGroupArnB} {
				tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
					ResourceArn: arn,
				})
				g.Expect(err).To(BeNil())
				_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
				g.Expect(hasManagedBy).To(BeFalse(), "external TG %s untagged", *arn)
			}
		}).Should(Succeed())
	})
})

var _ = Describe("External service with TLS listeners", Ordered, Label("service-import-external-tg"), func() {
	var (
		preCreatedSnId *string
		gateway        *gwv1.Gateway

		preCreatedServiceArn      *string
		preCreatedListenerArn443  *string
		preCreatedListenerArn8443 *string
		preCreatedTargetGroupArn  *string
		preCreatedTargetGroupId   *string
		preCreatedAssociationArn  *string

		deployment    *appsv1.Deployment
		tlsEksService *corev1.Service
		currentRoute  *gwv1.TLSRoute
		currentImport *anv1alpha1.ServiceImport

		customerServiceName          = "external-svc-6"
		customerServiceCustomDomain  = "tls-service.test.local"
		serviceNetworkAndGatewayName = "external-sn-6"
	)

	BeforeAll(func() {
		snResp, err := testFramework.LatticeClient.CreateServiceNetwork(ctx, &vpclattice.CreateServiceNetworkInput{
			Name: aws.String(serviceNetworkAndGatewayName),
		})
		Expect(err).To(BeNil())
		preCreatedSnId = snResp.Id

		gateway = &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceNetworkAndGatewayName,
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     "tls-443",
						Protocol: gwv1.TLSProtocolType,
						Port:     443,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: lo.ToPtr(gwv1.NamespacesFromSame),
							},
						},
						TLS: &gwv1.ListenerTLSConfig{
							Mode: lo.ToPtr(gwv1.TLSModePassthrough),
						},
					},
					{
						Name:     "tls-8443",
						Protocol: gwv1.TLSProtocolType,
						Port:     8443,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: lo.ToPtr(gwv1.NamespacesFromSame),
							},
						},
						TLS: &gwv1.ListenerTLSConfig{
							Mode: lo.ToPtr(gwv1.TLSModePassthrough),
						},
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, gateway)

		Eventually(func(g Gomega) {
			gw := &gwv1.Gateway{}
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(gateway), gw)
			g.Expect(err).To(BeNil())
			programmed := false
			for _, cond := range gw.Status.Conditions {
				if cond.Type == string(gwv1.GatewayConditionProgrammed) &&
					cond.Status == metav1.ConditionTrue {
					programmed = true
					break
				}
			}
			g.Expect(programmed).To(BeTrue(), "gateway not Programmed")
		}).Should(Succeed())

		targetGroupResp, err := testFramework.LatticeClient.CreateTargetGroup(ctx, &vpclattice.CreateTargetGroupInput{
			Name: aws.String("external-tg-6"),
			Type: types.TargetGroupTypeIp,
			Config: &types.TargetGroupConfig{
				Protocol:      types.TargetGroupProtocolTcp,
				Port:          aws.Int32(443),
				VpcIdentifier: aws.String(testFramework.Cloud.Config().VpcId),
				IpAddressType: types.IpAddressTypeIpv4,
			},
		})
		Expect(err).To(BeNil())
		preCreatedTargetGroupArn = targetGroupResp.Arn
		preCreatedTargetGroupId = targetGroupResp.Id

		Eventually(func(g Gomega) {
			getTargetGroupResp, err := testFramework.LatticeClient.GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: preCreatedTargetGroupArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getTargetGroupResp.Status)).To(Equal(string(types.TargetGroupStatusActive)))
		}).Should(Succeed())

		serviceResp, err := testFramework.LatticeClient.CreateService(ctx, &vpclattice.CreateServiceInput{
			Name:             aws.String(customerServiceName),
			CustomDomainName: aws.String(customerServiceCustomDomain),
		})
		Expect(err).To(BeNil())
		preCreatedServiceArn = serviceResp.Arn

		Eventually(func(g Gomega) {
			getServiceResp, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getServiceResp.Status)).To(Equal(string(types.ServiceStatusActive)))
		}).Should(Succeed())

		listenerResp443, err := testFramework.LatticeClient.CreateListener(ctx, &vpclattice.CreateListenerInput{
			ServiceIdentifier: preCreatedServiceArn,
			Name:              aws.String("external-listener-443-6"),
			Protocol:          types.ListenerProtocolTlsPassthrough,
			Port:              aws.Int32(443),
			DefaultAction: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{{
						TargetGroupIdentifier: preCreatedTargetGroupArn,
						Weight:                lo.ToPtr(int32(100)),
					}},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedListenerArn443 = listenerResp443.Arn

		listenerResp8443, err := testFramework.LatticeClient.CreateListener(ctx, &vpclattice.CreateListenerInput{
			ServiceIdentifier: preCreatedServiceArn,
			Name:              aws.String("external-listener-8443-6"),
			Protocol:          types.ListenerProtocolTlsPassthrough,
			Port:              aws.Int32(8443),
			DefaultAction: &types.RuleActionMemberForward{
				Value: types.ForwardAction{
					TargetGroups: []types.WeightedTargetGroup{{
						TargetGroupIdentifier: preCreatedTargetGroupArn,
						Weight:                lo.ToPtr(int32(100)),
					}},
				},
			},
		})
		Expect(err).To(BeNil())
		preCreatedListenerArn8443 = listenerResp8443.Arn

		associationResp, err := testFramework.LatticeClient.CreateServiceNetworkServiceAssociation(ctx, &vpclattice.CreateServiceNetworkServiceAssociationInput{
			ServiceIdentifier:        preCreatedServiceArn,
			ServiceNetworkIdentifier: preCreatedSnId,
		})
		Expect(err).To(BeNil())
		preCreatedAssociationArn = associationResp.Arn

		Eventually(func(g Gomega) {
			getAssocResp, err := testFramework.LatticeClient.GetServiceNetworkServiceAssociation(ctx, &vpclattice.GetServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(string(getAssocResp.Status)).To(Equal(string(types.ServiceNetworkServiceAssociationStatusActive)))
		}).Should(Succeed())

		deployment, tlsEksService = testFramework.NewHttpsApp(test.HTTPsAppOptions{
			Name:       "eks-6",
			Namespace:  k8snamespace,
			Port:       443,
			TargetPort: 443,
		})
		testFramework.ExpectCreated(ctx, deployment, tlsEksService)
	})

	AfterEach(func() {
		if currentRoute != nil {
			_ = testFramework.Client.Delete(ctx, currentRoute)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), currentRoute)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentRoute = nil
		}
		if currentImport != nil {
			_ = testFramework.Client.Delete(ctx, currentImport)
			Eventually(func(g Gomega) {
				err := testFramework.Get(ctx, client.ObjectKeyFromObject(currentImport), currentImport)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
			currentImport = nil
		}
	})

	AfterAll(func() {
		if preCreatedAssociationArn != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetworkServiceAssociation(ctx, &vpclattice.DeleteServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete association %s: %v", *preCreatedAssociationArn, err)
				}
			}
		}

		if preCreatedListenerArn443 != nil {
			_, err := testFramework.LatticeClient.DeleteListener(ctx, &vpclattice.DeleteListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn443,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete listener 443 %s: %v", *preCreatedListenerArn443, err)
				}
			}
		}

		if preCreatedListenerArn8443 != nil {
			_, err := testFramework.LatticeClient.DeleteListener(ctx, &vpclattice.DeleteListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn8443,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete listener 8443 %s: %v", *preCreatedListenerArn8443, err)
				}
			}
		}

		if preCreatedServiceArn != nil {
			_, err := testFramework.LatticeClient.DeleteService(ctx, &vpclattice.DeleteServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete service %s: %v", *preCreatedServiceArn, err)
				}
			}
		}

		if preCreatedTargetGroupArn != nil {
			Eventually(func(g Gomega) {
				_, err := testFramework.LatticeClient.DeleteTargetGroup(ctx, &vpclattice.DeleteTargetGroupInput{
					TargetGroupIdentifier: preCreatedTargetGroupArn,
				})
				if err != nil {
					var respErr *smithyhttp.ResponseError
					if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
						return
					}
				}
				g.Expect(err).To(BeNil())
			}).Should(Succeed())
		}

		if deployment != nil && tlsEksService != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, deployment, tlsEksService)
		}

		if gateway != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, gateway)
		}

		if preCreatedSnId != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetwork(ctx, &vpclattice.DeleteServiceNetworkInput{
				ServiceNetworkIdentifier: preCreatedSnId,
			})
			if err != nil {
				var respErr *smithyhttp.ResponseError
				if !errors.As(err, &respErr) || respErr.HTTPStatusCode() != 404 {
					log.Printf("Failed to delete service network %s: %v", *preCreatedSnId, err)
				}
			}
		}
	})

	It("TLSRoute with no sectionName replicates weighted forward across both listeners default action", func() {
		currentImport = &anv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "import-6",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/target-group-arn": *preCreatedTargetGroupArn,
				},
			},
			Spec: anv1alpha1.ServiceImportSpec{
				Type: anv1alpha1.ClusterSetIP,
				Ports: []anv1alpha1.ServicePort{
					{Port: 443, Protocol: corev1.ProtocolTCP},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentImport)

		parentNS := gwv1.Namespace(gateway.Namespace)
		currentRoute = &gwv1.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route-6",
				Namespace: k8snamespace,
				Labels: map[string]string{
					test.DiscoveryLabel: "true",
				},
				Annotations: map[string]string{
					"application-networking.k8s.aws/service-name-override": customerServiceName,
				},
			},
			Spec: gwv1.TLSRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name:      gwv1.ObjectName(gateway.Name),
						Namespace: &parentNS,
					}},
				},
				Hostnames: []gwv1.Hostname{gwv1.Hostname(customerServiceCustomDomain)},
				Rules: []gwv1.TLSRouteRule{
					{
						BackendRefs: []gwv1.BackendRef{
							{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName("import-6"),
									Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
								},
								Weight: lo.ToPtr(int32(90)),
							},
							{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: gwv1.ObjectName(tlsEksService.Name),
									Kind: lo.ToPtr(gwv1.Kind("Service")),
									Port: lo.ToPtr(gwv1.PortNumber(443)),
								},
								Weight: lo.ToPtr(int32(10)),
							},
						},
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, currentRoute)

		Eventually(func(g Gomega) {
			rt := &gwv1.TLSRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(currentRoute), rt)).To(Succeed())
			g.Expect(rt.Status.Parents).ToNot(BeEmpty())
			conds := map[string]metav1.Condition{}
			for _, c := range rt.Status.Parents[0].Conditions {
				conds[c.Type] = c
			}
			accepted, ok := conds[string(gwv1.RouteConditionAccepted)]
			g.Expect(ok).To(BeTrue())
			g.Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))

			resolved, ok := conds[string(gwv1.RouteConditionResolvedRefs)]
			g.Expect(ok).To(BeTrue())
			g.Expect(resolved.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(resolved.Reason).To(Equal(string(gwv1.RouteReasonResolvedRefs)))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeTrue(), "service ManagedBy stamped")
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			listeners, err := testFramework.LatticeClient.ListListenersAsList(ctx, &vpclattice.ListListenersInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			var l443, l8443 *types.ListenerSummary
			for i := range listeners {
				switch *listeners[i].Name {
				case "external-listener-443-6":
					l443 = &listeners[i]
				case "external-listener-8443-6":
					l8443 = &listeners[i]
				}
			}
			g.Expect(l443).ToNot(BeNil(), "listener on port %d must survive claim", 443)
			g.Expect(l8443).ToNot(BeNil(), "listener on port %d must survive claim", 8443)

			for _, l := range []*types.ListenerSummary{l443, l8443} {
				var expectedListenerArn *string
				var expectedPort int32
				switch *l.Name {
				case "external-listener-443-6":
					expectedListenerArn = preCreatedListenerArn443
					expectedPort = 443
				case "external-listener-8443-6":
					expectedListenerArn = preCreatedListenerArn8443
					expectedPort = 8443
				}
				g.Expect(*l.Arn).To(Equal(*expectedListenerArn), "listener %s ARN preserved", *l.Name)
				g.Expect(*l.Port).To(BeEquivalentTo(expectedPort), "listener %s port unchanged", *l.Name)
				g.Expect(string(l.Protocol)).To(Equal(string(types.ListenerProtocolTlsPassthrough)), "listener %s protocol unchanged", *l.Name)

				rules, err := testFramework.LatticeClient.ListRulesAsList(ctx, &vpclattice.ListRulesInput{
					ServiceIdentifier:  preCreatedServiceArn,
					ListenerIdentifier: l.Arn,
				})
				g.Expect(err).To(BeNil())
				nonDefault := []types.RuleSummary{}
				for _, r := range rules {
					if r.IsDefault != nil && *r.IsDefault {
						continue
					}
					nonDefault = append(nonDefault, r)
				}
				g.Expect(nonDefault).To(HaveLen(0), "listener %s: 0 non-default rules", *l.Name)

				listenerResp, err := testFramework.LatticeClient.GetListener(ctx, &vpclattice.GetListenerInput{
					ServiceIdentifier:  preCreatedServiceArn,
					ListenerIdentifier: l.Id,
				})
				g.Expect(err).To(BeNil())
				forward, ok := listenerResp.DefaultAction.(*types.RuleActionMemberForward)
				g.Expect(ok).To(BeTrue(), "listener %s default action is Forward", *l.Name)
				g.Expect(forward.Value.TargetGroups).To(HaveLen(2), "forward across external+EKS TG on %s", *l.Name)

				var externalWeight, eksWeight int32 = -1, -1
				for _, wtg := range forward.Value.TargetGroups {
					if *wtg.TargetGroupIdentifier == *preCreatedTargetGroupId {
						externalWeight = *wtg.Weight
					} else {
						eksWeight = *wtg.Weight
					}
				}
				g.Expect(externalWeight).To(BeEquivalentTo(90), "external TG weight 90 on %s", *l.Name)
				g.Expect(eksWeight).To(BeEquivalentTo(10), "EKS TG weight 10 on %s", *l.Name)
			}
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			tagsResp, err := testFramework.LatticeClient.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedTargetGroupArn,
			})
			g.Expect(err).To(BeNil())
			_, hasManagedBy := tagsResp.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(hasManagedBy).To(BeFalse(), "external TG %s untagged", *preCreatedTargetGroupArn)
		}).Should(Succeed())
	})
})
