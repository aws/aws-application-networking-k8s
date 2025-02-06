package integration

import (
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
)

var _ = Describe("GRPCRoute test", Ordered, func() {

	var (
		grpcBinDeployment        *appsv1.Deployment
		grpcBinService           *v1.Service
		grpcHelloWorldDeployment *appsv1.Deployment
		grpcHelloWorldService    *v1.Service
		grpcRoute                *gwv1.GRPCRoute
		latticeService           *vpclattice.ServiceSummary
	)

	BeforeAll(func() {
		grpcBinDeployment, grpcBinService = testFramework.NewGrpcBin(test.GrpcAppOptions{AppName: "my-grpcbin-1", Namespace: k8snamespace})
		grpcHelloWorldDeployment, grpcHelloWorldService = testFramework.NewGrpcHelloWorld(test.GrpcAppOptions{AppName: "my-grpc-hello-world-1", Namespace: k8snamespace})
		testFramework.ExpectCreated(ctx, grpcBinDeployment, grpcBinService, grpcHelloWorldDeployment, grpcHelloWorldService)
	})

	When("Create a grpcRoute that have one rule with no matches BackendRef to grpcBinService", func() {
		It("Expect create grpcRoute successfully", func() {
			grpcRoute = testFramework.NewGRPCRoute(k8snamespace, testGateway, []gwv1.GRPCRouteRule{
				{
					BackendRefs: []gwv1.GRPCBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      gwv1.ObjectName(grpcBinService.Name),
									Namespace: lo.ToPtr(gwv1.Namespace(grpcBinService.Namespace)),
									Kind:      lo.ToPtr(gwv1.Kind("Service")),
									Port:      lo.ToPtr(gwv1.PortNumber(19000)),
								},
							},
						},
					},
				},
			})
			testFramework.ExpectCreated(ctx, grpcRoute)
		})
		It("Expect one lattice targetGroup with GRPC ProtocolVersion is created", func() {
			var tgSummary = testFramework.GetTargetGroupWithProtocol(ctx, grpcBinService, "http", "grpc")
			tg, err := testFramework.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{TargetGroupIdentifier: tgSummary.Id})
			Expect(err).To(BeNil())
			Expect(*tg.Config.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttp))
			Expect(*tg.Config.ProtocolVersion).To(Equal(vpclattice.TargetGroupProtocolVersionGrpc))
		})
		It("Expected one lattice service listener rule is created with Prefix match `/` and HttpMethodMach POST", func() {
			route, _ := core.NewRoute(grpcRoute)
			latticeService = testFramework.GetVpcLatticeService(ctx, route)
			Eventually(func(g Gomega) {
				rules, err := testFramework.GetLatticeServiceHttpsListenerNonDefaultRules(ctx, latticeService)
				g.Expect(err).To(BeNil())
				g.Expect(len(rules)).To(Equal(1))
				g.Expect(*rules[0].Match.HttpMatch.Method).To(Equal("POST"))
				g.Expect(*rules[0].Match.HttpMatch.PathMatch.Match.Prefix).To(Equal("/"))
			}).Within(1 * time.Minute).Should(Succeed())
		})

		Context("Traffic test: client pod (grpcurl-runner) can send request to all services/methods of grpcBinService", func() {
			It("Can send grpc request to grpcbin.GRPCBin/DummyUnary method in grpcBin server", func() {
				grpcurlCmdOptions := test.RunGrpcurlCmdOptions{
					GrpcServerHostName:  *latticeService.DnsEntry.DomainName,
					GrpcServerPort:      "443",
					Service:             "grpcbin.GRPCBin",
					Method:              "DummyUnary",
					ReqParamsJsonString: `{ "f_string": "myTestString", "f_int32": 42, "f_bytes": "SGVsbG8gV29ybGQ="}`,
					UseTLS:              true,
				}
				Eventually(func(g Gomega) {
					stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
					g.Expect(err).To(BeNil())
					g.Expect(stderrStr).To(BeEmpty())
					g.Expect(stdoutStr).To(ContainSubstring(`"fString": "myTestString"`))
					g.Expect(stdoutStr).To(ContainSubstring(`"fInt32": 42`))
				}).Should(Succeed())
			})
			It("Can send grpc request to addsvc.Add/Sum method in grpcBin server", func() {
				grpcurlCmdOptions := test.RunGrpcurlCmdOptions{
					GrpcServerHostName:  *latticeService.DnsEntry.DomainName,
					GrpcServerPort:      "443",
					Service:             "addsvc.Add",
					Method:              "Sum",
					ReqParamsJsonString: `{"a": 5, "b": 6}`,
					UseTLS:              true,
				}
				Eventually(func(g Gomega) {
					stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
					g.Expect(err).To(BeNil())
					g.Expect(stderrStr).To(BeEmpty())
					g.Expect(stdoutStr).To(ContainSubstring("\"v\": " + strconv.Itoa(5+6)))
				}).Should(Succeed())
			})
			It("Can send grpc request to addsvc.Add/Sum method in grpcBin server", func() {
				grpcurlCmdOptions := test.RunGrpcurlCmdOptions{
					GrpcServerHostName:  *latticeService.DnsEntry.DomainName,
					GrpcServerPort:      "443",
					Service:             "addsvc.Add",
					Method:              "Concat",
					ReqParamsJsonString: `{ "a": "Str1", "b": "Str2"}`,
					UseTLS:              true,
				}
				//Happy path: Verify client is able to invoke addsvc.Add/Concat method
				Eventually(func(g Gomega) {
					stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
					g.Expect(err).To(BeNil())
					g.Expect(stderrStr).To(BeEmpty())
					g.Expect(stdoutStr).To(ContainSubstring(`"v": "Str1Str2"`))
				}).Should(Succeed())
			})
		})
	})

	When("Update existing GRPCRoute to have a new rule with gRPCMethod, gRPCService and header matching", func() {
		It("Expect update GRPCRoute successfully", func() {
			Expect(grpcRoute).To(Not(BeNil()))
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(grpcRoute), grpcRoute)
			Expect(err).To(BeNil())
			grpcRoute.Spec.Rules = []gwv1.GRPCRouteRule{
				{
					Matches: []gwv1.GRPCRouteMatch{
						{
							Headers: []gwv1.GRPCHeaderMatch{
								{
									Name:  "test-key1",
									Value: "test-value1",
								},
								{
									Name:  "test-key2",
									Value: "test-value2",
								},
								{
									Name:  "test-key3",
									Value: "test-value3",
								},
							},
							Method: &gwv1.GRPCMethodMatch{
								Type:    lo.ToPtr(gwv1.GRPCMethodMatchExact),
								Service: lo.ToPtr("grpcbin.GRPCBin"),
								Method:  lo.ToPtr("HeadersUnary"),
							},
						},
					},
					BackendRefs: []gwv1.GRPCBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      gwv1.ObjectName(grpcBinService.Name),
									Namespace: lo.ToPtr(gwv1.Namespace(grpcBinService.Namespace)),
									Kind:      lo.ToPtr(gwv1.Kind("Service")),
									Port:      lo.ToPtr(gwv1.PortNumber(19000)),
								},
							},
						},
					},
				},
			}
			err = testFramework.Update(ctx, grpcRoute)
			Expect(err).To(BeNil())
		})

		It("Expect LatticeService's https listener have that new rule with Exact PathMatch and HeaderMatches", func() {
			Eventually(func(g Gomega) {
				rules, err := testFramework.GetLatticeServiceHttpsListenerNonDefaultRules(ctx, latticeService)
				g.Expect(err).To(BeNil())
				g.Expect(len(rules)).To(Equal(1))
				g.Expect(*rules[0].Match.HttpMatch.Method).To(Equal("POST"))
				g.Expect(rules[0].Match.HttpMatch.PathMatch.Match.Exact).To(Not(BeNil()))
				g.Expect(*rules[0].Match.HttpMatch.PathMatch.Match.Exact).To(Equal("/grpcbin.GRPCBin/HeadersUnary"))
				headerMatches := rules[0].Match.HttpMatch.HeaderMatches
				g.Expect(len(headerMatches)).To(Equal(3))
				g.Expect(*headerMatches[0].Name).To(Equal("test-key1"))
				g.Expect(*headerMatches[0].Match.Exact).To(Equal("test-value1"))
				g.Expect(*headerMatches[1].Name).To(Equal("test-key2"))
				g.Expect(*headerMatches[1].Match.Exact).To(Equal("test-value2"))
				g.Expect(*headerMatches[2].Name).To(Equal("test-key3"))
				g.Expect(*headerMatches[2].Match.Exact).To(Equal("test-value3"))
			}).Should(Succeed())
		})

		Context("Traffic test: client pod (grpcurl-runner) can only send request to matched grpcMethod with matched headers", func() {
			It("Can send grpc request to grpcbin.GRPCBin/HeadersUnary method with matched headers", func() {
				grpcurlCmdOptions := test.RunGrpcurlCmdOptions{
					GrpcServerHostName: *latticeService.DnsEntry.DomainName,
					GrpcServerPort:     "443",
					Service:            "grpcbin.GRPCBin",
					Method:             "HeadersUnary",
					Headers: [][2]string{
						{"test-key1", "test-value1"},
						{"test-key2", "test-value2"},
						{"test-key3", "test-value3"},
					},
					UseTLS: true,
				}
				Eventually(func(g Gomega) {
					stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
					g.Expect(err).To(BeNil())
					g.Expect(stderrStr).To(BeEmpty())
					g.Expect(stdoutStr).To(ContainSubstring(`:authority`))
					g.Expect(stdoutStr).To(ContainSubstring(*latticeService.DnsEntry.DomainName))
					g.Expect(stdoutStr).To(ContainSubstring(`test-key1`))
					g.Expect(stdoutStr).To(ContainSubstring(`test-value1`))
				}).Should(Succeed())
			})

			It("Can NOT send grpc request to grpcbin.GRPCBin/HeadersUnary method that has invalid headers matching", func() {
				grpcurlCmdOptions := test.RunGrpcurlCmdOptions{
					GrpcServerHostName: *latticeService.DnsEntry.DomainName,
					GrpcServerPort:     "443",
					Service:            "grpcbin.GRPCBin",
					Method:             "HeadersUnary",
					Headers: [][2]string{
						{"test-key1", "test-value1"},
						{"test-key2", "test-value2"},
						{"test-key3", "invalid-value"},
					},
					UseTLS: true,
				}
				Eventually(func(g Gomega) {
					_, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
					g.Expect(err).To(Not(BeNil()))
					g.Expect(stderrStr).To(ContainSubstring("Not Found"))
				}).Should(Succeed())
			})

			It("Can NOT send grpc request to other method grpcbin.GRPCBin/DummyUnary that don't have matched rule", func() {
				grpcurlCmdOptions := test.RunGrpcurlCmdOptions{
					GrpcServerHostName:  *latticeService.DnsEntry.DomainName,
					GrpcServerPort:      "443",
					Service:             "grpcbin.GRPCBin",
					Method:              "DummyUnary",
					ReqParamsJsonString: `{ "f_string": "myTestString", "f_int32": 42, "f_bytes": "SGVsbG8gV29ybGQ="}`,
					UseTLS:              true,
				}
				Eventually(func(g Gomega) {
					_, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
					g.Expect(err).To(Not(BeNil()))
					g.Expect(stderrStr).To(ContainSubstring("Not Found"))
				}).Should(Succeed())
			})
		})
	})

	When("Update existing GRPCRoute to have 2 rules: "+
		"the rule1 only has grpcService `addsvc.Add` matching  and no grpcMethod matching, "+
		"the rule2 backendRefs to another grpcHelloWorldService without matching.", func() {

		It("Expect update GRPCRoute successfully", func() {
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(grpcRoute), grpcRoute)
			Expect(err).To(BeNil())
			grpcRoute.Spec.Rules = []gwv1.GRPCRouteRule{
				{
					Matches: []gwv1.GRPCRouteMatch{
						{
							Method: &gwv1.GRPCMethodMatch{
								Type:    lo.ToPtr(gwv1.GRPCMethodMatchExact),
								Service: lo.ToPtr("addsvc.Add"),
							},
						},
					},
					BackendRefs: []gwv1.GRPCBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      gwv1.ObjectName(grpcBinService.Name),
									Namespace: lo.ToPtr(gwv1.Namespace(grpcBinService.Namespace)),
									Kind:      lo.ToPtr(gwv1.Kind("Service")),
									Port:      lo.ToPtr(gwv1.PortNumber(19000)),
								},
							},
						},
					},
				},
				{
					BackendRefs: []gwv1.GRPCBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      gwv1.ObjectName(grpcHelloWorldService.Name),
									Namespace: lo.ToPtr(gwv1.Namespace(grpcHelloWorldService.Namespace)),
									Kind:      lo.ToPtr(gwv1.Kind("Service")),
									Port:      lo.ToPtr(gwv1.PortNumber(10051)),
								},
							},
						},
					},
				},
			}
			err = testFramework.Update(ctx, grpcRoute)
			Expect(err).To(BeNil())
		})

		It("Expect 2 lattice listener rules created", func() {
			var grpcBinTG = testFramework.GetTargetGroupWithProtocol(ctx, grpcBinService, "http", "grpc")
			var grpcHelloWorldTG = testFramework.GetTargetGroupWithProtocol(ctx, grpcHelloWorldService, "http", "grpc")
			Eventually(func(g Gomega) {
				rules, err := testFramework.GetLatticeServiceHttpsListenerNonDefaultRules(ctx, latticeService)
				g.Expect(err).To(BeNil())
				g.Expect(len(rules)).To(Equal(2))
				g.Expect(*rules[0].Match.HttpMatch.Method).To(Equal("POST"))
				g.Expect(rules[0].Match.HttpMatch.PathMatch.Match.Prefix).To(Not(BeNil()))
				for _, rule := range rules {
					if *rule.Priority == 1 {
						g.Expect(*rule.Match.HttpMatch.PathMatch.Match.Prefix).To(Equal("/addsvc.Add/"))
						g.Expect(*rule.Action.Forward.TargetGroups[0].TargetGroupIdentifier).To(Equal(*grpcBinTG.Id))
					} else if *rule.Priority == 2 {
						g.Expect(*rule.Match.HttpMatch.PathMatch.Match.Prefix).To(Equal("/"))
						g.Expect(*rule.Action.Forward.TargetGroups[0].TargetGroupIdentifier).To(Equal(*grpcHelloWorldTG.Id))
					}
				}
			}).Should(Succeed())
		})

		It("Can send request to addsvc.Add/Concat method from grpcBin server", func() {
			grpcurlCmdOptions := test.RunGrpcurlCmdOptions{
				GrpcServerHostName:  *latticeService.DnsEntry.DomainName,
				GrpcServerPort:      "443",
				Service:             "addsvc.Add",
				Method:              "Concat",
				ReqParamsJsonString: `{"a": "Str1", "b": "Str2"}`,
				UseTLS:              true,
			}
			Eventually(func(g Gomega) {
				stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
				g.Expect(err).To(BeNil())
				g.Expect(stderrStr).To(BeEmpty())
				g.Expect(stdoutStr).To(ContainSubstring(`"v": "Str1Str2"`))
			}).Should(Succeed())
		})

		It("Can send request to addsvc.Add/Sum method from grpcBin server", func() {
			grpcurlCmdOptions := test.RunGrpcurlCmdOptions{
				GrpcServerHostName:  *latticeService.DnsEntry.DomainName,
				GrpcServerPort:      "443",
				Service:             "addsvc.Add",
				Method:              "Sum",
				ReqParamsJsonString: `{"a": 5, "b": 6}`,
				UseTLS:              true,
			}
			Eventually(func(g Gomega) {
				stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
				g.Expect(err).To(BeNil())
				g.Expect(stderrStr).To(BeEmpty())
				g.Expect(stdoutStr).To(ContainSubstring("\"v\": " + strconv.Itoa(5+6)))
			}).Should(Succeed())
		})

		It("Can send request to grpcHelloWorld.Greeter/SayHello method from grpcHelloWorld server", func() {
			grpcurlCmdOptions := test.RunGrpcurlCmdOptions{
				GrpcServerHostName:  *latticeService.DnsEntry.DomainName,
				GrpcServerPort:      "443",
				Service:             "helloworld.Greeter",
				Method:              "SayHello",
				ReqParamsJsonString: `{"name": "myTestName"}`,
				UseTLS:              true,
			}
			Eventually(func(g Gomega) {
				stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
				g.Expect(err).To(BeNil())
				g.Expect(stderrStr).To(BeEmpty())
				g.Expect(stdoutStr).To(ContainSubstring("\"message\": \"Hello myTestName\""))
			}).Should(Succeed())
		})

	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			grpcRoute,
			grpcBinService,
			grpcBinDeployment,
			grpcHelloWorldService,
			grpcHelloWorldDeployment,
		)
	})
})
