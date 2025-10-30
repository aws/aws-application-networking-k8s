package integration

import (
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Service Takeover Test", Ordered, func() {
	var (
		preCreatedServiceArn     *string
		preCreatedListenerArn    *string
		preCreatedRuleArn        *string
		preCreatedAssociationArn *string
		preCreatedTargetGroupArn *string

		deployment1 *appsv1.Deployment
		service1    *v1.Service
		deployment2 *appsv1.Deployment
		service2    *v1.Service
		httpRoute   *gwv1.HTTPRoute

		originalManagedBy = "685175429445/blue-controller/vpc-0e19af3ab36ee2915"
		serviceName       = "inventory-e2e-test"
	)

	It("Create lattice resources simulating HttpRoute created by blue controller", func() {
		serviceResp, err := testFramework.LatticeClient.CreateService(&vpclattice.CreateServiceInput{
			Name: aws.String(serviceName),
			Tags: map[string]*string{
				"application-networking.k8s.aws/ManagedBy":      aws.String(originalManagedBy),
				"application-networking.k8s.aws/RouteName":      aws.String("inventory"),
				"application-networking.k8s.aws/RouteNamespace": aws.String(k8snamespace),
				"application-networking.k8s.aws/RouteType":      aws.String("http"),
				"application-networking.k8s.aws/ClusterName":    aws.String("blue-cluster"),
			},
		})
		Expect(err).To(BeNil())
		preCreatedServiceArn = serviceResp.Arn

		Eventually(func(g Gomega) {
			getServiceResp, err := testFramework.LatticeClient.GetService(&vpclattice.GetServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(*getServiceResp.Status).To(Equal("ACTIVE"))
		}).Should(Succeed())

		listenerResp, err := testFramework.LatticeClient.CreateListener(&vpclattice.CreateListenerInput{
			ServiceIdentifier: preCreatedServiceArn,
			Name:              aws.String("inventory-listener"),
			Protocol:          aws.String("HTTP"),
			Port:              aws.Int64(80),
			DefaultAction: &vpclattice.RuleAction{
				FixedResponse: &vpclattice.FixedResponseAction{
					StatusCode: aws.Int64(404),
				},
			},
			Tags: map[string]*string{
				"application-networking.k8s.aws/ManagedBy": aws.String(originalManagedBy),
			},
		})
		Expect(err).To(BeNil())
		preCreatedListenerArn = listenerResp.Arn

		targetGroupResp, err := testFramework.LatticeClient.CreateTargetGroup(&vpclattice.CreateTargetGroupInput{
			Name: aws.String("inventory-takeover-tg"),
			Type: aws.String("IP"),
			Config: &vpclattice.TargetGroupConfig{
				Protocol:      aws.String("HTTP"),
				Port:          aws.Int64(80),
				VpcIdentifier: aws.String(testFramework.Cloud.Config().VpcId),
			},
			Tags: map[string]*string{
				"application-networking.k8s.aws/ManagedBy": aws.String(originalManagedBy),
			},
		})
		Expect(err).To(BeNil())
		preCreatedTargetGroupArn = targetGroupResp.Arn

		Eventually(func(g Gomega) {
			getTargetGroupResp, err := testFramework.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: preCreatedTargetGroupArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(*getTargetGroupResp.Status).To(Equal("ACTIVE"))
		}).Should(Succeed())

		_, err = testFramework.LatticeClient.RegisterTargets(&vpclattice.RegisterTargetsInput{
			TargetGroupIdentifier: preCreatedTargetGroupArn,
			Targets: []*vpclattice.Target{
				{
					Id:   aws.String("192.168.1.100"),
					Port: aws.Int64(8090),
				},
			},
		})
		Expect(err).To(BeNil())

		ruleResp, err := testFramework.LatticeClient.CreateRule(&vpclattice.CreateRuleInput{
			ServiceIdentifier:  preCreatedServiceArn,
			ListenerIdentifier: preCreatedListenerArn,
			Name:               aws.String("inventory-rule"),
			Priority:           aws.Int64(1),
			Match: &vpclattice.RuleMatch{
				HttpMatch: &vpclattice.HttpMatch{
					PathMatch: &vpclattice.PathMatch{
						Match: &vpclattice.PathMatchType{
							Prefix: aws.String("/"),
						},
						CaseSensitive: aws.Bool(true),
					},
				},
			},
			Action: &vpclattice.RuleAction{
				Forward: &vpclattice.ForwardAction{
					TargetGroups: []*vpclattice.WeightedTargetGroup{
						{
							TargetGroupIdentifier: preCreatedTargetGroupArn,
							Weight:                aws.Int64(100),
						},
					},
				},
			},
			Tags: map[string]*string{
				"application-networking.k8s.aws/ManagedBy": aws.String(originalManagedBy),
			},
		})
		Expect(err).To(BeNil())
		preCreatedRuleArn = ruleResp.Arn

		associationResp, err := testFramework.LatticeClient.CreateServiceNetworkServiceAssociation(&vpclattice.CreateServiceNetworkServiceAssociationInput{
			ServiceIdentifier:        preCreatedServiceArn,
			ServiceNetworkIdentifier: testServiceNetwork.Id,
			Tags: map[string]*string{
				"application-networking.k8s.aws/ManagedBy": aws.String(originalManagedBy),
			},
		})
		Expect(err).To(BeNil())
		preCreatedAssociationArn = associationResp.Arn

		Eventually(func(g Gomega) {
			getAssocResp, err := testFramework.LatticeClient.GetServiceNetworkServiceAssociation(&vpclattice.GetServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(*getAssocResp.Status).To(Equal("ACTIVE"))
		}).Should(Succeed())
	})

	It("Creating HTTPRoute without takeover annotation should fail", func() {
		deployment1, service1 = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:       "backend-1",
			Namespace:  k8snamespace,
			Port:       80,
			TargetPort: 8090,
		})
		deployment2, service2 = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:       "backend-2",
			Namespace:  k8snamespace,
			Port:       80,
			TargetPort: 8090,
		})
		testFramework.ExpectCreated(ctx, deployment1, service1, deployment2, service2)

		httpRoute = &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "inventory",
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
						BackendRefs: []gwv1.HTTPBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: gwv1.ObjectName(service1.Name),
										Kind: lo.ToPtr(gwv1.Kind("Service")),
										Port: lo.ToPtr(gwv1.PortNumber(80)),
									},
									Weight: lo.ToPtr(int32(50)),
								},
							},
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: gwv1.ObjectName(service2.Name),
										Kind: lo.ToPtr(gwv1.Kind("Service")),
										Port: lo.ToPtr(gwv1.PortNumber(80)),
									},
									Weight: lo.ToPtr(int32(50)),
								},
							},
						},
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, httpRoute)

		Eventually(func(g Gomega) {
			events := &corev1.EventList{}
			err := testFramework.List(ctx, events, client.InNamespace(k8snamespace))
			g.Expect(err).To(BeNil())

			found := false
			for _, event := range events.Items {
				if event.InvolvedObject.Name == httpRoute.Name &&
					event.Reason == "FailedDeployModel" {
					if strings.Contains(event.Message, "Found existing resource not owned by controller") {
						found = true
						break
					}
				}
			}
			g.Expect(found).To(BeTrue())
		}).Should(Succeed())
	})

	It("Adding takeover annotation to HttpRoute should allow HttpRoute to takeover the service", func() {
		Eventually(func(g Gomega) {
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), httpRoute)
			g.Expect(err).To(BeNil())

			httpRoute.Annotations = map[string]string{
				"application-networking.k8s.aws/allow-takeover-from": originalManagedBy,
			}
			err = testFramework.Update(ctx, httpRoute)
			g.Expect(err).To(BeNil())
		}).Should(Succeed())
	})

	It("Verify takeover completed successfully", func() {
		currentManagedBy := fmt.Sprintf("%s/%s/%s",
			testFramework.Cloud.Config().AccountId,
			testFramework.Cloud.Config().ClusterName,
			testFramework.Cloud.Config().VpcId)

		Eventually(func(g Gomega) {
			getRuleResp, err := testFramework.LatticeClient.GetRule(&vpclattice.GetRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
				RuleIdentifier:     preCreatedRuleArn,
			})
			g.Expect(err).To(BeNil())

			// Verify service ManagedBy tag updated
			serviceTags, err := testFramework.LatticeClient.ListTagsForResource(&vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedServiceArn,
			})
			g.Expect(err).To(BeNil())
			managedByTag := serviceTags.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(*managedByTag).To(Equal(currentManagedBy))

			// Verify rule now has 2 target groups
			g.Expect(getRuleResp.Action.Forward).ToNot(BeNil())
			g.Expect(len(getRuleResp.Action.Forward.TargetGroups)).To(Equal(2))
			g.Expect(*getRuleResp.Action.Forward.TargetGroups[0].Weight).To(Equal(int64(50)))
			g.Expect(*getRuleResp.Action.Forward.TargetGroups[1].Weight).To(Equal(int64(50)))

			// Verify original target group is no longer referenced in the rule
			for _, tg := range getRuleResp.Action.Forward.TargetGroups {
				g.Expect(*tg.TargetGroupIdentifier).ToNot(Equal(*preCreatedTargetGroupArn))
			}

			// Verify rule ManagedBy tag updated
			ruleTags, err := testFramework.LatticeClient.ListTagsForResource(&vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedRuleArn,
			})
			g.Expect(err).To(BeNil())
			ruleManagedByTag := ruleTags.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(*ruleManagedByTag).To(Equal(currentManagedBy))

			// Verify listener ManagedBy tag updated
			listenerTags, err := testFramework.LatticeClient.ListTagsForResource(&vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedListenerArn,
			})
			g.Expect(err).To(BeNil())
			listenerManagedByTag := listenerTags.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(*listenerManagedByTag).To(Equal(currentManagedBy))

			// Verify service network service association ManagedBy tag updated
			assocTags, err := testFramework.LatticeClient.ListTagsForResource(&vpclattice.ListTagsForResourceInput{
				ResourceArn: preCreatedAssociationArn,
			})
			g.Expect(err).To(BeNil())
			assocManagedByTag := assocTags.Tags["application-networking.k8s.aws/ManagedBy"]
			g.Expect(*assocManagedByTag).To(Equal(currentManagedBy))

		}).Should(Succeed())
	})

	AfterAll(func() {
		if httpRoute != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, httpRoute)
		}
		if deployment1 != nil && service1 != nil && deployment2 != nil && service2 != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, deployment1, service1, deployment2, service2)
		}
		if preCreatedRuleArn != nil {
			_, err := testFramework.LatticeClient.DeleteRule(&vpclattice.DeleteRuleInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
				RuleIdentifier:     preCreatedRuleArn,
			})
			if err != nil {
				if reqErr, ok := err.(awserr.RequestFailure); !ok || reqErr.StatusCode() != 404 {
					log.Printf("Failed to delete rule %s: %v", *preCreatedRuleArn, err)
				}
			}
		}

		if preCreatedListenerArn != nil {
			_, err := testFramework.LatticeClient.DeleteListener(&vpclattice.DeleteListenerInput{
				ServiceIdentifier:  preCreatedServiceArn,
				ListenerIdentifier: preCreatedListenerArn,
			})
			if err != nil {
				if reqErr, ok := err.(awserr.RequestFailure); !ok || reqErr.StatusCode() != 404 {
					log.Printf("Failed to delete listener %s: %v", *preCreatedListenerArn, err)
				}
			}
		}

		if preCreatedAssociationArn != nil {
			_, err := testFramework.LatticeClient.DeleteServiceNetworkServiceAssociation(&vpclattice.DeleteServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
			})
			if err != nil {
				if reqErr, ok := err.(awserr.RequestFailure); !ok || reqErr.StatusCode() != 404 {
					log.Printf("Failed to delete association %s: %v", *preCreatedAssociationArn, err)
				}
			} else {
				Eventually(func(g Gomega) {
					_, err := testFramework.LatticeClient.GetServiceNetworkServiceAssociation(&vpclattice.GetServiceNetworkServiceAssociationInput{
						ServiceNetworkServiceAssociationIdentifier: preCreatedAssociationArn,
					})
					g.Expect(err).To(HaveOccurred())
				}).Should(Succeed())
			}
		}

		if preCreatedServiceArn != nil {
			_, err := testFramework.LatticeClient.DeleteService(&vpclattice.DeleteServiceInput{
				ServiceIdentifier: preCreatedServiceArn,
			})
			if err != nil {
				if reqErr, ok := err.(awserr.RequestFailure); !ok || reqErr.StatusCode() != 404 {
					log.Printf("Failed to delete service %s: %v", *preCreatedServiceArn, err)
				}
			}
		}

		if preCreatedTargetGroupArn != nil {
			_, err := testFramework.LatticeClient.DeregisterTargets(&vpclattice.DeregisterTargetsInput{
				TargetGroupIdentifier: preCreatedTargetGroupArn,
				Targets: []*vpclattice.Target{
					{
						Id:   aws.String("192.168.1.100"),
						Port: aws.Int64(8090),
					},
				},
			})
			if err != nil {
				if reqErr, ok := err.(awserr.RequestFailure); !ok || reqErr.StatusCode() != 404 {
					log.Printf("Failed to deregister targets from %s: %v", *preCreatedTargetGroupArn, err)
				}
			}

			_, err = testFramework.LatticeClient.DeleteTargetGroup(&vpclattice.DeleteTargetGroupInput{
				TargetGroupIdentifier: preCreatedTargetGroupArn,
			})
			if err != nil {
				if reqErr, ok := err.(awserr.RequestFailure); !ok || reqErr.StatusCode() != 404 {
					log.Printf("Failed to delete target group %s: %v", *preCreatedTargetGroupArn, err)
				}
			}
		}
	})
})
