package integration

import (
	"strconv"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
)

var _ = Describe("GRPCRoute test", Ordered, func() {

	var (
		grpcBinDeployment        *appsv1.Deployment
		grpcBinService           *v1.Service
		grpcHelloWorldDeployment *appsv1.Deployment
		grpcHelloWorldService    *v1.Service
		grpcRoute                *v1alpha2.GRPCRoute
		latticeService           *vpclattice.ServiceSummary
	)

	BeforeAll(func() {
		grpcBinDeployment, grpcBinService = testFramework.NewGrpcBin(test.GrpcAppOptions{AppName: "my-grpcbin-1", Namespace: k8snamespace})
		grpcHelloWorldDeployment, grpcHelloWorldService = testFramework.NewGrpcHelloWorld(test.GrpcAppOptions{AppName: "my-grpc-hello-world-1", Namespace: k8snamespace})
		testFramework.ExpectCreated(ctx, grpcBinDeployment, grpcBinService, grpcHelloWorldDeployment, grpcHelloWorldService)
	})

	It("GRPCRoute rules with no matches, client pod could invoke all services/methods of grpcBinService", func() {
		grpcRoute = testFramework.NewGRPCRoute(k8snamespace, testGateway, []v1alpha2.GRPCRouteRule{
			{
				BackendRefs: []v1alpha2.GRPCBackendRef{
					{
						BackendRef: v1alpha2.BackendRef{
							BackendObjectReference: v1beta1.BackendObjectReference{
								Name:      v1alpha2.ObjectName(grpcBinService.Name),
								Namespace: lo.ToPtr(v1beta1.Namespace(grpcBinService.Namespace)),
								Kind:      lo.ToPtr(v1beta1.Kind("Service")),
								Port:      lo.ToPtr(v1beta1.PortNumber(19000)),
							},
						},
					},
				},
			},
		})

		testFramework.ExpectCreated(ctx, grpcRoute)

		var tgSummary = testFramework.GetTargetGroupWithProtocol(ctx, grpcBinService, "http", "grpc")
		tg, err := testFramework.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{TargetGroupIdentifier: tgSummary.Id})
		Expect(err).To(BeNil())
		Expect(*tg.Config.Protocol).To(Equal(vpclattice.TargetGroupProtocolHttp))
		Expect(*tg.Config.ProtocolVersion).To(Equal(vpclattice.TargetGroupProtocolVersionGrpc))
		route, _ := core.NewRoute(grpcRoute)
		latticeService = testFramework.GetVpcLatticeService(ctx, route)
		Eventually(func(g Gomega) {
			rules, err := testFramework.GetLatticeServiceHttpsListenerNonDefaultRules(ctx, latticeService)
			g.Expect(err).To(BeNil())
			g.Expect(len(rules)).To(Equal(1))
			g.Expect(*rules[0].Match.HttpMatch.Method).To(Equal("POST"))
			g.Expect(*rules[0].Match.HttpMatch.PathMatch.Match.Prefix).To(Equal("/"))
		}).Should(Succeed())

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

		grpcurlCmdOptions = test.RunGrpcurlCmdOptions{
			GrpcServerHostName:  *latticeService.DnsEntry.DomainName,
			GrpcServerPort:      "443",
			Service:             "addsvc.Add",
			Method:              "Sum",
			ReqParamsJsonString: `{"a": 5, "b": 6}`,
			UseTLS:              true,
		}
		//Happy path: Verify client is able to invoke addsvc.Add/Sum method
		Eventually(func(g Gomega) {
			stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
			g.Expect(err).To(BeNil())
			g.Expect(stderrStr).To(BeEmpty())
			g.Expect(stdoutStr).To(ContainSubstring("\"v\": " + strconv.Itoa(5+6)))
		}).Should(Succeed())

		grpcurlCmdOptions = test.RunGrpcurlCmdOptions{
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

	It("Update existing GRPCRoute to have only one new rule with gRPCMethod, gRPCService and header match, "+
		"client pod can only invoke the matched gRPCMethod with correct headers", func() {
		Expect(grpcRoute).To(Not(BeNil()))
		err := testFramework.Get(ctx, client.ObjectKeyFromObject(grpcRoute), grpcRoute)
		Expect(err).To(BeNil())
		grpcRoute.Spec.Rules = []v1alpha2.GRPCRouteRule{
			{
				Matches: []v1alpha2.GRPCRouteMatch{
					{
						Headers: []v1alpha2.GRPCHeaderMatch{
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
						Method: &v1alpha2.GRPCMethodMatch{
							Type:    lo.ToPtr(v1alpha2.GRPCMethodMatchExact),
							Service: lo.ToPtr("grpcbin.GRPCBin"),
							Method:  lo.ToPtr("HeadersUnary"),
						},
					},
				},
				BackendRefs: []v1alpha2.GRPCBackendRef{
					{
						BackendRef: v1alpha2.BackendRef{
							BackendObjectReference: v1beta1.BackendObjectReference{
								Name:      v1alpha2.ObjectName(grpcBinService.Name),
								Namespace: lo.ToPtr(v1beta1.Namespace(grpcBinService.Namespace)),
								Kind:      lo.ToPtr(v1beta1.Kind("Service")),
								Port:      lo.ToPtr(v1beta1.PortNumber(19000)),
							},
						},
					},
				},
			},
		}

		err = testFramework.Update(ctx, grpcRoute)
		Expect(err).To(BeNil())
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
		//Happy path: Verify client is able to invoke grpcbin.GRPCBin/HeadersUnary method with correct headers matching
		Eventually(func(g Gomega) {
			stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
			g.Expect(err).To(BeNil())
			g.Expect(stderrStr).To(BeEmpty())
			g.Expect(stdoutStr).To(ContainSubstring(`:authority`))
			g.Expect(stdoutStr).To(ContainSubstring(*latticeService.DnsEntry.DomainName))
			g.Expect(stdoutStr).To(ContainSubstring(`test-key1`))
			g.Expect(stdoutStr).To(ContainSubstring(`test-value1`))
		}).Should(Succeed())

		grpcurlCmdOptions = test.RunGrpcurlCmdOptions{
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
		//Unhappy path: Verify client is NOT able to invoke grpcbin.GRPCBin/HeadersUnary method that has invalid headers matching
		Eventually(func(g Gomega) {
			_, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
			g.Expect(err).To(Not(BeNil()))
			g.Expect(stderrStr).To(ContainSubstring("Not Found"))
		}).Should(Succeed())

		grpcurlCmdOptions = test.RunGrpcurlCmdOptions{
			GrpcServerHostName:  *latticeService.DnsEntry.DomainName,
			GrpcServerPort:      "443",
			Service:             "grpcbin.GRPCBin",
			Method:              "DummyUnary",
			ReqParamsJsonString: `{ "f_string": "myTestString", "f_int32": 42, "f_bytes": "SGVsbG8gV29ybGQ="}`,
			UseTLS:              true,
		}

		//Unhappy path: Verify client is NOT able to invoke grpcbin.GRPCBin/DummyUnary method that has no rule matches
		Eventually(func(g Gomega) {
			_, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
			g.Expect(err).To(Not(BeNil()))
			g.Expect(stderrStr).To(ContainSubstring("Not Found"))
		}).Should(Succeed())
	})

	It("Update existing GRPCRoute to have 2 rules: rule1 only has grpcService matching and no grpcMethod matching, "+
		"client pod can invokes ALL grpcMethods of addsvc.Add service from grpcBinService. "+
		"The rule2 backendRefs to another grpcHelloWorldService without matching."+
		"Expect client pod can send traffic to these 2 different k8sServices(targetGroups)", func() {
		err := testFramework.Get(ctx, client.ObjectKeyFromObject(grpcRoute), grpcRoute)
		Expect(err).To(BeNil())
		grpcRoute.Spec.Rules = []v1alpha2.GRPCRouteRule{
			{
				Matches: []v1alpha2.GRPCRouteMatch{
					{
						Method: &v1alpha2.GRPCMethodMatch{
							Type:    lo.ToPtr(v1alpha2.GRPCMethodMatchExact),
							Service: lo.ToPtr("addsvc.Add"),
						},
					},
				},
				BackendRefs: []v1alpha2.GRPCBackendRef{
					{
						BackendRef: v1alpha2.BackendRef{
							BackendObjectReference: v1beta1.BackendObjectReference{
								Name:      v1alpha2.ObjectName(grpcBinService.Name),
								Namespace: lo.ToPtr(v1beta1.Namespace(grpcBinService.Namespace)),
								Kind:      lo.ToPtr(v1beta1.Kind("Service")),
								Port:      lo.ToPtr(v1beta1.PortNumber(19000)),
							},
						},
					},
				},
			},
			{
				BackendRefs: []v1alpha2.GRPCBackendRef{
					{
						BackendRef: v1alpha2.BackendRef{
							BackendObjectReference: v1beta1.BackendObjectReference{
								Name:      v1alpha2.ObjectName(grpcHelloWorldService.Name),
								Namespace: lo.ToPtr(v1beta1.Namespace(grpcHelloWorldService.Namespace)),
								Kind:      lo.ToPtr(v1beta1.Kind("Service")),
								Port:      lo.ToPtr(v1beta1.PortNumber(10051)),
							},
						},
					},
				},
			},
		}

		err = testFramework.Update(ctx, grpcRoute)
		Expect(err).To(BeNil())
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

		grpcurlCmdOptions := test.RunGrpcurlCmdOptions{
			GrpcServerHostName:  *latticeService.DnsEntry.DomainName,
			GrpcServerPort:      "443",
			Service:             "addsvc.Add",
			Method:              "Concat",
			ReqParamsJsonString: `{"a": "Str1", "b": "Str2"}`,
			UseTLS:              true,
		}
		//Happy path: Verify client is able to invoke methods from `addsvc.Add` grpcService
		Eventually(func(g Gomega) {
			stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
			g.Expect(err).To(BeNil())
			g.Expect(stderrStr).To(BeEmpty())
			g.Expect(stdoutStr).To(ContainSubstring(`"v": "Str1Str2"`))
		}).Should(Succeed())

		grpcurlCmdOptions = test.RunGrpcurlCmdOptions{
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

		grpcurlCmdOptions = test.RunGrpcurlCmdOptions{
			GrpcServerHostName:  *latticeService.DnsEntry.DomainName,
			GrpcServerPort:      "443",
			Service:             "helloworld.Greeter",
			Method:              "SayHello",
			ReqParamsJsonString: `{"name": "myTestName"}`,
			UseTLS:              true,
		}
		//Happy path: Verify client is able to invoke methods of helloworld.Greeter that from another grpc-helloworld-server(targetGroup)
		Eventually(func(g Gomega) {
			stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
			g.Expect(err).To(BeNil())
			g.Expect(stderrStr).To(BeEmpty())
			g.Expect(stdoutStr).To(ContainSubstring("\"message\": \"Hello myTestName\""))
		}).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeleted(ctx, grpcRoute)
		testFramework.SleepForRouteDeletion()
		testFramework.ExpectDeletedThenNotFound(ctx,
			grpcBinService,
			grpcBinDeployment,
			grpcHelloWorldService,
			grpcHelloWorldDeployment,
			grpcRoute,
		)
	})
})
