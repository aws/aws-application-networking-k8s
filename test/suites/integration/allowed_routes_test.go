package integration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
)

var _ = Describe("AllowedRoutes Test", Ordered, func() {
	var (
		diffNS = "diff-namespace"

		deployment    *appsv1.Deployment
		service       *corev1.Service
		diffNamespace *corev1.Namespace
		httpRoute     *gwv1.HTTPRoute
		tlsRoute      *gwv1alpha2.TLSRoute

		originalGatewaySpec gwv1.GatewaySpec
	)

	BeforeAll(func() {
		// Backup common testGateway spec
		originalGatewaySpec = *testGateway.Spec.DeepCopy()

		diffNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: diffNS},
		}
		testFramework.ExpectCreated(ctx, diffNamespace)

		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "inventory-ver1",
			Namespace: diffNS,
		})
		testFramework.ExpectCreated(ctx, deployment, service)
	})

	Context("Listeners with default policy to allow routes from same namespace", func() {
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				currentGateway := &gwv1.Gateway{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(testGateway), currentGateway)).To(Succeed())

				currentGateway.Spec.Listeners = []gwv1.Listener{
					{
						Name:     "http",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80,
					},
					{
						Name:     "https",
						Protocol: gwv1.HTTPSProtocolType,
						Port:     443,
					},
				}
				g.Expect(testFramework.Update(ctx, currentGateway)).To(Succeed())
			}).Should(Succeed())
		})

		It("HTTPRoute from different namespace should be rejected", func() {
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")
			testFramework.ExpectCreated(ctx, httpRoute)

			Eventually(func(g Gomega) {
				updatedRoute := &gwv1.HTTPRoute{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), updatedRoute)).To(Succeed())
				g.Expect(updatedRoute.Status.Parents).To(HaveLen(1))

				parent := updatedRoute.Status.Parents[0]
				acceptedCondition := findCondition(parent.Conditions, "Accepted")
				g.Expect(acceptedCondition).ToNot(BeNil())
				g.Expect(acceptedCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(acceptedCondition.Reason).To(Equal("NotAllowedByListeners"))
				g.Expect(acceptedCondition.Message).To(ContainSubstring("No matching listeners allow this route"))
			}).Should(Succeed())

			Consistently(func(g Gomega) {
				route := core.NewHTTPRoute(*httpRoute)
				_, err := testFramework.LatticeClient.FindService(ctx, utils.LatticeServiceName(route.Name(), route.Namespace()))
				g.Expect(err).To(HaveOccurred())
			}, "30s", "5s").Should(Succeed())
		})
	})

	Context("Listeners with one allowing routes from default same and other from all namespaces", func() {
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				currentGateway := &gwv1.Gateway{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(testGateway), currentGateway)).To(Succeed())

				currentGateway.Spec.Listeners = []gwv1.Listener{
					{
						Name:     "http",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80,
					},
					{
						Name:     "https",
						Protocol: gwv1.HTTPSProtocolType,
						Port:     443,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: &[]gwv1.FromNamespaces{gwv1.NamespacesFromAll}[0],
							},
						},
					},
				}
				g.Expect(testFramework.Update(ctx, currentGateway)).To(Succeed())
			}).Should(Succeed())
		})

		It("HTTPRoute from different namespace should be accepted by HTTPS listener allowing routes from all namespaces", func() {
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")
			testFramework.ExpectCreated(ctx, httpRoute)

			Eventually(func(g Gomega) {
				updatedRoute := &gwv1.HTTPRoute{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), updatedRoute)).To(Succeed())
				g.Expect(updatedRoute.Status.Parents).To(HaveLen(1))

				parent := updatedRoute.Status.Parents[0]
				acceptedCondition := findCondition(parent.Conditions, "Accepted")
				g.Expect(acceptedCondition).ToNot(BeNil())
				g.Expect(acceptedCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(acceptedCondition.Reason).To(Equal("Accepted"))

				route := core.NewHTTPRoute(*updatedRoute)
				vpcLatticeService, err := testFramework.LatticeClient.FindService(ctx, utils.LatticeServiceName(route.Name(), route.Namespace()))
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(vpcLatticeService).ToNot(BeNil())

				listListenersResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx, &vpclattice.ListListenersInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(listListenersResp.Items).To(HaveLen(1))

				listener := listListenersResp.Items[0]
				g.Expect(*listener.Port).To(Equal(int64(443)))
				g.Expect(*listener.Protocol).To(Equal("HTTPS"))
			}).Should(Succeed())
		})
	})

	Context("Listeners with namespace selector allowing routes from specific labeled namespaces", func() {
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				ns := &corev1.Namespace{}
				g.Expect(testFramework.Get(ctx, client.ObjectKey{Name: diffNS}, ns)).To(Succeed())
				if ns.Labels == nil {
					ns.Labels = make(map[string]string)
				}
				ns.Labels["env"] = "prod"
				g.Expect(testFramework.Update(ctx, ns)).To(Succeed())
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				currentGateway := &gwv1.Gateway{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(testGateway), currentGateway)).To(Succeed())

				currentGateway.Spec.Listeners = []gwv1.Listener{
					{
						Name:     "http",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: &[]gwv1.FromNamespaces{gwv1.NamespacesFromSelector}[0],
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"env": "prod"},
								},
							},
						},
					},
					{
						Name:     "https",
						Protocol: gwv1.HTTPSProtocolType,
						Port:     443,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: &[]gwv1.FromNamespaces{gwv1.NamespacesFromSelector}[0],
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"env": "dev"},
								},
							},
						},
					},
				}
				g.Expect(testFramework.Update(ctx, currentGateway)).To(Succeed())
			}).Should(Succeed())
		})

		It("HTTPRoute from prod labeled namespace should be accepted by HTTP listener with matching selector", func() {
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")
			testFramework.ExpectCreated(ctx, httpRoute)

			Eventually(func(g Gomega) {
				updatedRoute := &gwv1.HTTPRoute{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), updatedRoute)).To(Succeed())
				g.Expect(updatedRoute.Status.Parents).To(HaveLen(1))

				parent := updatedRoute.Status.Parents[0]
				acceptedCondition := findCondition(parent.Conditions, "Accepted")
				g.Expect(acceptedCondition).ToNot(BeNil())
				g.Expect(acceptedCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(acceptedCondition.Reason).To(Equal("Accepted"))

				route := core.NewHTTPRoute(*updatedRoute)
				vpcLatticeService, err := testFramework.LatticeClient.FindService(ctx, utils.LatticeServiceName(route.Name(), route.Namespace()))
				g.Expect(err).ToNot(HaveOccurred())

				listListenersResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx, &vpclattice.ListListenersInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(listListenersResp.Items).To(HaveLen(1))

				listener := listListenersResp.Items[0]
				g.Expect(*listener.Port).To(Equal(int64(80)))
				g.Expect(*listener.Protocol).To(Equal("HTTP"))
			}).Should(Succeed())
		})
	})

	Context("Both listeners allowing routes from all namespaces", func() {
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				currentGateway := &gwv1.Gateway{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(testGateway), currentGateway)).To(Succeed())

				currentGateway.Spec.Listeners = []gwv1.Listener{
					{
						Name:     "http",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: &[]gwv1.FromNamespaces{gwv1.NamespacesFromAll}[0],
							},
						},
					},
					{
						Name:     "http1",
						Protocol: gwv1.HTTPProtocolType,
						Port:     90,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: &[]gwv1.FromNamespaces{gwv1.NamespacesFromAll}[0],
							},
						},
					},
				}
				g.Expect(testFramework.Update(ctx, currentGateway)).To(Succeed())
			}).Should(Succeed())
		})

		It("HTTPRoute with multiple parentRefs should be accepted by both listeners", func() {
			httpRoute = &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t5-httproute",
					Namespace: diffNS,
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        gwv1.ObjectName(testGateway.Name),
								Namespace:   &[]gwv1.Namespace{gwv1.Namespace(testGateway.Namespace)}[0],
								SectionName: &[]gwv1.SectionName{"http"}[0],
							},
							{
								Name:        gwv1.ObjectName(testGateway.Name),
								Namespace:   &[]gwv1.Namespace{gwv1.Namespace(testGateway.Namespace)}[0],
								SectionName: &[]gwv1.SectionName{"http1"}[0],
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name:      gwv1.ObjectName(service.Name),
											Namespace: &[]gwv1.Namespace{gwv1.Namespace(service.Namespace)}[0],
											Kind:      &[]gwv1.Kind{"Service"}[0],
											Port:      &[]gwv1.PortNumber{gwv1.PortNumber(service.Spec.Ports[0].Port)}[0],
										},
									},
								},
							},
						},
					},
				},
			}
			testFramework.ExpectCreated(ctx, httpRoute)

			Eventually(func(g Gomega) {
				updatedRoute := &gwv1.HTTPRoute{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), updatedRoute)).To(Succeed())
				g.Expect(updatedRoute.Status.Parents).To(HaveLen(2))

				for _, parent := range updatedRoute.Status.Parents {
					acceptedCondition := findCondition(parent.Conditions, "Accepted")
					g.Expect(acceptedCondition).ToNot(BeNil())
					g.Expect(acceptedCondition.Status).To(Equal(metav1.ConditionTrue))
					g.Expect(acceptedCondition.Reason).To(Equal("Accepted"))
				}

				route := core.NewHTTPRoute(*updatedRoute)
				vpcLatticeService, err := testFramework.LatticeClient.FindService(ctx, utils.LatticeServiceName(route.Name(), route.Namespace()))
				g.Expect(err).ToNot(HaveOccurred())

				listListenersResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx, &vpclattice.ListListenersInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(listListenersResp.Items).To(HaveLen(2))
				ports := []int64{}
				for _, listener := range listListenersResp.Items {
					ports = append(ports, *listener.Port)
				}
				g.Expect(ports).To(ContainElement(int64(80)))
				g.Expect(ports).To(ContainElement(int64(90)))
			}).Should(Succeed())
		})
	})

	Context("Listeners with mixed namespace policies allowing routes from all and same namespace respectively", func() {
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				currentGateway := &gwv1.Gateway{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(testGateway), currentGateway)).To(Succeed())

				currentGateway.Spec.Listeners = []gwv1.Listener{
					{
						Name:     "http",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: &[]gwv1.FromNamespaces{gwv1.NamespacesFromAll}[0],
							},
						},
					},
					{
						Name:     "http1",
						Protocol: gwv1.HTTPProtocolType,
						Port:     90,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Namespaces: &gwv1.RouteNamespaces{
								From: &[]gwv1.FromNamespaces{gwv1.NamespacesFromSame}[0],
							},
						},
					},
				}
				g.Expect(testFramework.Update(ctx, currentGateway)).To(Succeed())
			}).Should(Succeed())
		})

		It("HTTPRoute with multiple parentRefs should have one accepted parentRef by all namespace listener and one rejected by same namespace listener", func() {
			httpRoute = &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "t6-httproute",
					Namespace: diffNS,
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        gwv1.ObjectName(testGateway.Name),
								Namespace:   &[]gwv1.Namespace{gwv1.Namespace(testGateway.Namespace)}[0],
								SectionName: &[]gwv1.SectionName{"http"}[0],
								Port:        &[]gwv1.PortNumber{80}[0],
							},
							{
								Name:        gwv1.ObjectName(testGateway.Name),
								Namespace:   &[]gwv1.Namespace{gwv1.Namespace(testGateway.Namespace)}[0],
								SectionName: &[]gwv1.SectionName{"http1"}[0],
								Port:        &[]gwv1.PortNumber{90}[0],
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name:      gwv1.ObjectName(service.Name),
											Namespace: &[]gwv1.Namespace{gwv1.Namespace(service.Namespace)}[0],
											Kind:      &[]gwv1.Kind{"Service"}[0],
											Port:      &[]gwv1.PortNumber{gwv1.PortNumber(service.Spec.Ports[0].Port)}[0],
										},
									},
								},
							},
						},
					},
				},
			}
			testFramework.ExpectCreated(ctx, httpRoute)

			Eventually(func(g Gomega) {
				updatedRoute := &gwv1.HTTPRoute{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), updatedRoute)).To(Succeed())
				g.Expect(updatedRoute.Status.Parents).To(HaveLen(2))

				var httpParent, http1Parent *gwv1.RouteParentStatus
				for i, parent := range updatedRoute.Status.Parents {
					if parent.ParentRef.SectionName != nil && *parent.ParentRef.SectionName == "http" {
						httpParent = &updatedRoute.Status.Parents[i]
					} else if parent.ParentRef.SectionName != nil && *parent.ParentRef.SectionName == "http1" {
						http1Parent = &updatedRoute.Status.Parents[i]
					}
				}
				g.Expect(httpParent).ToNot(BeNil())
				httpAcceptedCondition := findCondition(httpParent.Conditions, "Accepted")
				g.Expect(httpAcceptedCondition).ToNot(BeNil())
				g.Expect(httpAcceptedCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(httpAcceptedCondition.Reason).To(Equal("Accepted"))

				g.Expect(http1Parent).ToNot(BeNil())
				http1AcceptedCondition := findCondition(http1Parent.Conditions, "Accepted")
				g.Expect(http1AcceptedCondition).ToNot(BeNil())
				g.Expect(http1AcceptedCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(http1AcceptedCondition.Reason).To(Equal("NotAllowedByListeners"))

				route := core.NewHTTPRoute(*updatedRoute)
				vpcLatticeService, err := testFramework.LatticeClient.FindService(ctx, utils.LatticeServiceName(route.Name(), route.Namespace()))
				g.Expect(err).ToNot(HaveOccurred())

				listListenersResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx, &vpclattice.ListListenersInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(listListenersResp.Items).To(HaveLen(1))

				listener := listListenersResp.Items[0]
				g.Expect(*listener.Port).To(Equal(int64(80)))
				g.Expect(*listener.Protocol).To(Equal("HTTP"))
			}).Should(Succeed())
		})
	})

	Context("Listeners with default protocol based kind policy", func() {
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				currentGateway := &gwv1.Gateway{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(testGateway), currentGateway)).To(Succeed())

				currentGateway.Spec.Listeners = []gwv1.Listener{
					{
						Name:     "http",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80},
					{
						Name:     "https",
						Protocol: gwv1.HTTPSProtocolType,
						Port:     443,
					},
					{
						Name:     "tls",
						Protocol: gwv1.TLSProtocolType,
						Port:     444,
					},
				}
				g.Expect(testFramework.Update(ctx, currentGateway)).To(Succeed())
			}).Should(Succeed())
		})

		It("HTTPRoute should be accepted by compatible HTTP and HTTPS listeners but filtered out from TLS listener", func() {
			httpRoute = &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "k1-httproute",
					Namespace: testGateway.Namespace,
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName(testGateway.Name),
								Namespace: &[]gwv1.Namespace{gwv1.Namespace(testGateway.Namespace)}[0],
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name:      gwv1.ObjectName(service.Name),
											Namespace: &[]gwv1.Namespace{gwv1.Namespace(service.Namespace)}[0],
											Kind:      &[]gwv1.Kind{"Service"}[0],
											Port:      &[]gwv1.PortNumber{gwv1.PortNumber(service.Spec.Ports[0].Port)}[0],
										},
									},
								},
							},
						},
					},
				},
			}
			testFramework.ExpectCreated(ctx, httpRoute)

			Eventually(func(g Gomega) {
				updatedRoute := &gwv1.HTTPRoute{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), updatedRoute)).To(Succeed())
				g.Expect(updatedRoute.Status.Parents).To(HaveLen(1))
				parent := updatedRoute.Status.Parents[0]
				acceptedCondition := findCondition(parent.Conditions, "Accepted")
				g.Expect(acceptedCondition).ToNot(BeNil())
				g.Expect(acceptedCondition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(acceptedCondition.Reason).To(Equal("Accepted"))

				route := core.NewHTTPRoute(*updatedRoute)
				vpcLatticeService, err := testFramework.LatticeClient.FindService(ctx, utils.LatticeServiceName(route.Name(), route.Namespace()))
				g.Expect(err).ToNot(HaveOccurred())

				listListenersResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx, &vpclattice.ListListenersInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(listListenersResp.Items).To(HaveLen(2))

				ports := []int64{}
				protocols := []string{}
				for _, listener := range listListenersResp.Items {
					ports = append(ports, *listener.Port)
					protocols = append(protocols, *listener.Protocol)
				}
				g.Expect(ports).To(ContainElement(int64(80)))
				g.Expect(ports).To(ContainElement(int64(443)))
				g.Expect(ports).ToNot(ContainElement(int64(444)))
				g.Expect(protocols).To(ContainElement("HTTP"))
				g.Expect(protocols).To(ContainElement("HTTPS"))
			}).Should(Succeed())
		})
	})

	Context("HTTPS listener configured to allow only GRPCRoute", func() {
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				currentGateway := &gwv1.Gateway{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(testGateway), currentGateway)).To(Succeed())

				currentGateway.Spec.Listeners = []gwv1.Listener{
					{
						Name:     "https",
						Protocol: gwv1.HTTPSProtocolType,
						Port:     443,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Kinds: []gwv1.RouteGroupKind{
								{
									Kind: "GRPCRoute",
								},
							},
						},
					},
				}
				g.Expect(testFramework.Update(ctx, currentGateway)).To(Succeed())
			}).Should(Succeed())
		})

		It("HTTPRoute should be rejected by HTTPS listener configured to allow only GRPCRoute", func() {
			httpRoute = &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "k2-httproute",
					Namespace: testGateway.Namespace,
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName(testGateway.Name),
								Namespace: &[]gwv1.Namespace{gwv1.Namespace(testGateway.Namespace)}[0],
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name:      gwv1.ObjectName(service.Name),
											Namespace: &[]gwv1.Namespace{gwv1.Namespace(service.Namespace)}[0],
											Kind:      &[]gwv1.Kind{"Service"}[0],
											Port:      &[]gwv1.PortNumber{gwv1.PortNumber(service.Spec.Ports[0].Port)}[0],
										},
									},
								},
							},
						},
					},
				},
			}
			testFramework.ExpectCreated(ctx, httpRoute)

			Eventually(func(g Gomega) {
				updatedRoute := &gwv1.HTTPRoute{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), updatedRoute)).To(Succeed())
				g.Expect(updatedRoute.Status.Parents).To(HaveLen(1))

				parent := updatedRoute.Status.Parents[0]
				acceptedCondition := findCondition(parent.Conditions, "Accepted")
				g.Expect(acceptedCondition).ToNot(BeNil())
				g.Expect(acceptedCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(acceptedCondition.Reason).To(Equal("NotAllowedByListeners"))
				g.Expect(acceptedCondition.Message).To(ContainSubstring("No matching listeners allow this route"))
			}).Should(Succeed())

			Consistently(func(g Gomega) {
				route := core.NewHTTPRoute(*httpRoute)
				_, err := testFramework.LatticeClient.FindService(ctx, utils.LatticeServiceName(route.Name(), route.Namespace()))
				g.Expect(err).To(HaveOccurred())
			}, "30s", "5s").Should(Succeed())
		})
	})

	Context("HTTP and HTTPS listeners with default kind policies incompatible with TLSRoute", func() {
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				currentGateway := &gwv1.Gateway{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(testGateway), currentGateway)).To(Succeed())

				currentGateway.Spec.Listeners = []gwv1.Listener{
					{
						Name:     "http",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80,
					},
					{
						Name:     "https",
						Protocol: gwv1.HTTPSProtocolType,
						Port:     443,
					},
				}
				g.Expect(testFramework.Update(ctx, currentGateway)).To(Succeed())
			}).Should(Succeed())
		})

		It("TLSRoute should be rejected by HTTP and HTTPS listeners due to protocol incompatibility", func() {
			tlsRoute = &gwv1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "k4-tlsroute",
					Namespace: testGateway.Namespace,
				},
				Spec: gwv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName(testGateway.Name),
								Namespace: &[]gwv1.Namespace{gwv1.Namespace(testGateway.Namespace)}[0],
							},
						},
					},
					Hostnames: []gwv1alpha2.Hostname{"test.example.com"},
					Rules: []gwv1alpha2.TLSRouteRule{
						{
							BackendRefs: []gwv1alpha2.BackendRef{
								{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name:      gwv1.ObjectName(service.Name),
										Namespace: &[]gwv1.Namespace{gwv1.Namespace(service.Namespace)}[0],
										Kind:      &[]gwv1.Kind{"Service"}[0],
										Port:      &[]gwv1.PortNumber{gwv1.PortNumber(service.Spec.Ports[0].Port)}[0],
									},
								},
							},
						},
					},
				},
			}
			testFramework.ExpectCreated(ctx, tlsRoute)

			Eventually(func(g Gomega) {
				updatedRoute := &gwv1alpha2.TLSRoute{}
				g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(tlsRoute), updatedRoute)).To(Succeed())
				g.Expect(updatedRoute.Status.Parents).To(HaveLen(1)) // Single parentRef

				parent := updatedRoute.Status.Parents[0]
				acceptedCondition := findCondition(parent.Conditions, "Accepted")
				g.Expect(acceptedCondition).ToNot(BeNil())
				g.Expect(acceptedCondition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(acceptedCondition.Reason).To(Equal("NotAllowedByListeners"))
				g.Expect(acceptedCondition.Message).To(ContainSubstring("No matching listeners allow this route"))
			}).Should(Succeed())

			Consistently(func(g Gomega) {
				route := core.NewTLSRoute(*tlsRoute)
				_, err := testFramework.LatticeClient.FindService(ctx, utils.LatticeServiceName(route.Name(), route.Namespace()))
				g.Expect(err).To(HaveOccurred())
			}, "30s", "5s").Should(Succeed())
		})
	})

	AfterEach(func() {
		if httpRoute != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, httpRoute)
			httpRoute = nil
		}
		if tlsRoute != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, tlsRoute)
			tlsRoute = nil
		}

		Eventually(func(g Gomega) {
			currentGateway := &gwv1.Gateway{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(testGateway), currentGateway)).To(Succeed())
			currentGateway.Spec = *originalGatewaySpec.DeepCopy()
			g.Expect(testFramework.Update(ctx, currentGateway)).To(Succeed())
		}).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx, deployment, service, diffNamespace)
	})
})

func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}
