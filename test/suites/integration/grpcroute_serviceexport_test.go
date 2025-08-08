package integration

import (
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"os"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
)

var _ = Describe("GRPCRoute Service Export/Import Test", Ordered, func() {
	var (
		grpcDeployment *appsv1.Deployment
		grpcSvc        *v1.Service
		grpcRoute      *gwv1.GRPCRoute
		serviceExport  *anv1alpha1.ServiceExport
		serviceImport  *anv1alpha1.ServiceImport
	)

	It("Create k8s resource", func() {
		// Create a gRPC service and deployment
		grpcDeployment, grpcSvc = testFramework.NewGrpcHelloWorld(test.GrpcAppOptions{AppName: "my-grpc-exportedports", Namespace: k8snamespace})
		testFramework.ExpectCreated(ctx, grpcDeployment, grpcSvc)

		// Create ServiceImport
		serviceImport = testFramework.CreateServiceImport(grpcSvc)
		testFramework.ExpectCreated(ctx, serviceImport)

		// Create ServiceExport with exportedPorts field
		serviceExport = &anv1alpha1.ServiceExport{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "application-networking.k8s.aws/v1alpha1",
				Kind:       "ServiceExport",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      grpcSvc.Name,
				Namespace: grpcSvc.Namespace,
				Annotations: map[string]string{
					"application-networking.k8s.aws/federation": "amazon-vpc-lattice",
				},
			},
			Spec: anv1alpha1.ServiceExportSpec{
				ExportedPorts: []anv1alpha1.ExportedPort{
					{
						Port:      grpcSvc.Spec.Ports[0].Port,
						RouteType: "GRPC",
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, serviceExport)

		// Create GRPCRoute
		grpcRoute = testFramework.NewGRPCRoute(k8snamespace, testGateway, []gwv1.GRPCRouteRule{
			{
				Matches: []gwv1.GRPCRouteMatch{
					{
						Method: &gwv1.GRPCMethodMatch{
							Service: lo.ToPtr("helloworld.Greeter"),
							Method:  lo.ToPtr("SayHello"),
							Type:    lo.ToPtr(gwv1.GRPCMethodMatchExact),
						},
					},
				},
				BackendRefs: []gwv1.GRPCBackendRef{
					{
						BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name:      gwv1.ObjectName(grpcSvc.Name),
								Namespace: lo.ToPtr(gwv1.Namespace(grpcSvc.Namespace)),
								Kind:      lo.ToPtr(gwv1.Kind("ServiceImport")),
								Port:      lo.ToPtr(gwv1.PortNumber(grpcSvc.Spec.Ports[0].Port)),
							},
						},
					},
				},
			},
		})
		testFramework.ExpectCreated(ctx, grpcRoute)
	})

	It("Verify lattice resource & traffic", func() {
		route, _ := core.NewRoute(grpcRoute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)

		// Get the target group and verify it's configured for gRPC
		tgSummary := testFramework.GetTargetGroupWithProtocol(ctx, grpcSvc, "http", "grpc")
		tg, err := testFramework.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{
			TargetGroupIdentifier: aws.String(*tgSummary.Id),
		})
		Expect(tg).To(Not(BeNil()))
		Expect(err).To(BeNil())
		Expect(*tgSummary.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))

		// Verify the target group is configured for gRPC
		Expect(*tgSummary.Protocol).To(Equal("HTTP"))
		Expect(*tg.Config.ProtocolVersion).To(Equal("GRPC"))

		// Verify targets are registered
		Eventually(func(g Gomega) {
			targets := testFramework.GetTargets(ctx, tgSummary, grpcDeployment)
			for _, target := range targets {
				g.Expect(*target.Port).To(BeEquivalentTo(grpcSvc.Spec.Ports[0].TargetPort.IntVal))
			}
		}).Should(Succeed())

		log.Println("Verifying traffic")
		grpcurlCmdOptions := test.RunGrpcurlCmdOptions{
			GrpcServerHostName:  *vpcLatticeService.DnsEntry.DomainName,
			GrpcServerPort:      "443",
			Service:             "helloworld.Greeter",
			Method:              "SayHello",
			ReqParamsJsonString: `{"name": "ExportedPorts"}`,
			UseTLS:              true,
		}
		Eventually(func(g Gomega) {
			stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
			g.Expect(err).To(BeNil())
			g.Expect(stderrStr).To(BeEmpty())
			g.Expect(stdoutStr).To(ContainSubstring("ExportedPorts"))
		}).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			grpcRoute,
			grpcDeployment,
			grpcSvc,
			serviceImport,
			serviceExport,
		)
	})
})
