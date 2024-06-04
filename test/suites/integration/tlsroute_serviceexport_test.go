package integration

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
)

var _ = Describe("TLSRoute Service Export/Import Test", Ordered, func() {
	var (
		deployment1   *appsv1.Deployment
		service1      *v1.Service
		tlsRoute      *v1alpha2.TLSRoute
		serviceExport *anv1alpha1.ServiceExport
		serviceImport *anv1alpha1.ServiceImport
		policy        *anv1alpha1.TargetGroupPolicy
	)

	It("Create k8s resource", func() {
		deployment1, service1 = testFramework.NewHttpsApp(test.HTTPsAppOptions{Name: "my-https-1", Namespace: k8snamespace})
		policy = createTCPTargetGroupPolicy(service1)
		testFramework.ExpectCreated(ctx, policy)
		serviceImport = testFramework.CreateServiceImport(service1)
		testFramework.ExpectCreated(ctx, serviceImport)
		serviceExport = testFramework.CreateServiceExport(service1)
		testFramework.ExpectCreated(ctx, serviceExport)

		tlsRoute = testFramework.NewTLSRoute(k8snamespace, testGateway, []v1alpha2.TLSRouteRule{
			{
				BackendRefs: []gwv1.BackendRef{
					{
						BackendObjectReference: v1beta1.BackendObjectReference{
							Name:      v1alpha2.ObjectName(service1.Name),
							Namespace: lo.ToPtr(v1beta1.Namespace(service1.Namespace)),
							Kind:      lo.ToPtr(v1beta1.Kind("ServiceImport")),
							Port:      lo.ToPtr(v1beta1.PortNumber(443)),
						},
					},
				},
			},
		})

		// Create Kubernetes API Objects
		testFramework.ExpectCreated(ctx,
			tlsRoute,
			service1,
			deployment1,
		)
	})

	It("Verify lattice resource ", func() {
		route, _ := core.NewRoute(tlsRoute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
		fmt.Printf("vpcLatticeService: %v \n", vpcLatticeService)

		tgSummary := testFramework.GetTCPTargetGroup(ctx, service1)
		tg, err := testFramework.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{
			TargetGroupIdentifier: aws.String(*tgSummary.Id),
		})
		Expect(tg).To(Not(BeNil()))
		Expect(err).To(BeNil())
		Expect(*tgSummary.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
		Expect(*tgSummary.Protocol).To(Equal("TCP"))
		Expect(tg.Config.HealthCheck).To(Not(BeNil()))
		Expect(*tg.Config.HealthCheck.Enabled).To(BeTrue())
		Expect(*tg.Config.HealthCheck.Protocol).To(Equal("HTTPS"))
		Expect(*tg.Config.HealthCheck.ProtocolVersion).To(Equal("HTTP1"))
		Expect(*tg.Config.HealthCheck.Port).To(BeEquivalentTo(443))
		Expect(*tgSummary.Port).To(BeEquivalentTo(80))
		Eventually(func(g Gomega) {
			targets := testFramework.GetTargets(ctx, tgSummary, deployment1)
			g.Expect(*tgSummary.Port).To(BeEquivalentTo(80))
			for _, target := range targets {
				g.Expect(*target.Port).To(BeEquivalentTo(service1.Spec.Ports[0].TargetPort.IntVal))
				g.Expect(*target.Status).To(Equal(vpclattice.TargetStatusHealthy))
			}
		})
	})

	It("Verify traffic", func() {
		customDns := tlsRoute.Spec.Hostnames[0]
		log.Println("Verifying traffic")
		latticeGeneratedDnsName := testFramework.GetVpcLatticeServiceTLSDns(tlsRoute.Name, tlsRoute.Namespace)
		dnsIP, _ := net.LookupIP(latticeGeneratedDnsName)
		testFramework.Get(ctx, types.NamespacedName{Name: deployment1.Name, Namespace: deployment1.Namespace}, deployment1)
		pods := testFramework.GetPodsByDeploymentName(deployment1.Name, deployment1.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))
		pod := pods[0]
		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("curl -k https:/%s:444 --resolve %s:444:%s", customDns, customDns, dnsIP[0])
			log.Printf("Executing command [%s] \n", cmd)
			stdout, _, err := testFramework.PodExec(pod, cmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("my-https-1 handler pod"))
		}).WithTimeout(30 * time.Second).WithOffset(1).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			tlsRoute,
			deployment1,
			service1,
			serviceImport,
			serviceExport,
			policy,
		)
	})
})

func createTCPTargetGroupPolicy(
	service *corev1.Service,
) *anv1alpha1.TargetGroupPolicy {
	healthCheckProtocol := anv1alpha1.HealthCheckProtocol("HTTPS")
	healthCheckProtocolVersion := anv1alpha1.HealthCheckProtocolVersion("HTTP1")
	return &anv1alpha1.TargetGroupPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind: "TargetGroupPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: service.Namespace,
			Name:      "tcp-policy",
		},
		Spec: anv1alpha1.TargetGroupPolicySpec{
			TargetRef: &v1alpha2.PolicyTargetReference{
				Group: "application-networking.k8s.aws",
				Kind:  gwv1.Kind("ServiceExport"),
				Name:  gwv1.ObjectName(service.Name),
			},
			Protocol:        aws.String("TCP"),
			ProtocolVersion: aws.String("HTTP1"),
			HealthCheck: &anv1alpha1.HealthCheckConfig{
				Enabled:         aws.Bool(true),
				Protocol:        &healthCheckProtocol,
				ProtocolVersion: &healthCheckProtocolVersion,
				Port:            aws.Int64(443),
			},
		},
	}
}
