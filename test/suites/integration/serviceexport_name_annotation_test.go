package integration

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
)

var _ = Describe("ServiceExport service-name Annotation Test", Ordered, func() {
	var (
		deployment    *appsv1.Deployment
		service       *v1.Service
		serviceExport *anv1alpha1.ServiceExport
		serviceImport *anv1alpha1.ServiceImport
		httpRoute     *gwv1.HTTPRoute
	)

	It("Create resources with service-name and export-name annotations", func() {
		deployment, service = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "svcname-annotation",
			Namespace: k8snamespace,
		})
		testFramework.ExpectCreated(ctx, deployment, service)

		// ServiceExport with a different name than the Service, using service-name annotation
		serviceExport = test.New(&anv1alpha1.ServiceExport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svcname-annotation-export",
				Namespace: k8snamespace,
				Annotations: map[string]string{
					"application-networking.k8s.aws/federation":   "amazon-vpc-lattice",
					"application-networking.k8s.aws/service-name": service.Name,
				},
			},
			Spec: anv1alpha1.ServiceExportSpec{
				ExportedPorts: []anv1alpha1.ExportedPort{
					{
						Port:      service.Spec.Ports[0].Port,
						RouteType: "HTTP",
					},
				},
			},
		})
		testFramework.ExpectCreated(ctx, serviceExport)

		// ServiceImport with export-name annotation pointing to the ServiceExport
		serviceImport = test.New(&anv1alpha1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svcname-annotation-import",
				Namespace: k8snamespace,
				Annotations: map[string]string{
					"application-networking.k8s.aws/export-name": serviceExport.Name,
				},
			},
			Spec: anv1alpha1.ServiceImportSpec{
				Type: anv1alpha1.ClusterSetIP,
				Ports: []anv1alpha1.ServicePort{
					{
						Port:     service.Spec.Ports[0].Port,
						Protocol: v1.ProtocolTCP,
					},
				},
			},
		})
		testFramework.ExpectCreated(ctx, serviceImport)

		// HTTPRoute referencing the ServiceImport by its name (not the Service name)
		parentNS := gwv1.Namespace(testGateway.Namespace)
		httpRoute = test.New(&gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svcname-annotation-route",
				Namespace: k8snamespace,
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name:      gwv1.ObjectName(testGateway.Name),
						Namespace: &parentNS,
					}},
				},
				Rules: []gwv1.HTTPRouteRule{{
					BackendRefs: []gwv1.HTTPBackendRef{{
						BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName(serviceImport.Name),
								Kind: lo.ToPtr(gwv1.Kind("ServiceImport")),
							},
						},
					}},
				}},
			},
		})
		testFramework.ExpectCreated(ctx, httpRoute)
	})

	It("Verify target group is created with export name tag and targets are registered", func() {
		// The TG should be tagged with the ServiceExport name, not the Service name
		tgSpec := model.TargetGroupSpec{
			TargetGroupTagFields: model.TargetGroupTagFields{
				K8SServiceName:      serviceExport.Name,
				K8SServiceNamespace: k8snamespace,
			},
			Protocol:        vpclattice.TargetGroupProtocolHttp,
			ProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp1,
		}

		var tgSummary *vpclattice.TargetGroupSummary
		Eventually(func(g Gomega) {
			tg, err := testFramework.FindTargetGroupFromSpec(ctx, tgSpec)
			g.Expect(err).To(BeNil())
			g.Expect(tg).ToNot(BeNil())
			g.Expect(*tg.Status).To(Equal(vpclattice.TargetGroupStatusActive))
			tgSummary = tg
		}).Should(Succeed())

		// Verify targets from the actual Service are registered
		Eventually(func(g Gomega) {
			targets := testFramework.GetTargets(ctx, tgSummary, deployment)
			g.Expect(len(targets)).To(Equal(int(*deployment.Spec.Replicas)))
		}).WithTimeout(3 * time.Minute).WithOffset(1).Should(Succeed())
	})

	It("Verify HTTPRoute resolves and traffic works", func() {
		route := core.NewHTTPRoute(*httpRoute)
		testFramework.GetVpcLatticeService(ctx, route)

		log.Println("Verifying traffic")
		dnsName := testFramework.GetVpcLatticeServiceDns(httpRoute.Name, httpRoute.Namespace)
		pods := testFramework.GetPodsByDeploymentName(deployment.Name, deployment.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))

		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("curl %s", dnsName)
			stdout, _, err := testFramework.PodExec(pods[0], cmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("handler pod"))
		}).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			httpRoute,
			serviceImport,
			serviceExport,
			deployment,
			service,
		)
	})
})
