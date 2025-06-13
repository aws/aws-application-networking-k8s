package integration

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var _ = Describe("Pod IP Update test", Ordered, func() {
	var (
		httpsDeployment1 *appsv1.Deployment
		httpsSvc1        *v1.Service
		tlsRoute         *v1alpha2.TLSRoute
		initialPodIPs    []string
		initialTargets   []*vpclattice.TargetSummary
	)

	It("Set up k8s resource for TLS passthrough", func() {
		httpsDeployment1, httpsSvc1 = testFramework.NewHttpsApp(test.HTTPsAppOptions{Name: "tls-passthrough-test", Namespace: k8snamespace})
		tlsRoute = testFramework.NewTLSRoute(k8snamespace, testGateway, []v1alpha2.TLSRouteRule{
			{
				BackendRefs: []gwv1.BackendRef{
					{
						BackendObjectReference: gwv1.BackendObjectReference{
							Name:      v1alpha2.ObjectName(httpsSvc1.Name),
							Namespace: lo.ToPtr(gwv1.Namespace(httpsSvc1.Namespace)),
							Kind:      lo.ToPtr(gwv1.Kind("Service")),
							Port:      lo.ToPtr(gwv1.PortNumber(443)),
						},
					},
				},
			},
		})
		// Create Kubernetes API Objects
		testFramework.ExpectCreated(ctx,
			tlsRoute,
			httpsSvc1,
			httpsDeployment1,
		)
	})

	It("Verify initial Lattice resource and capture pod IPs", func() {
		route, _ := core.NewRoute(tlsRoute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
		fmt.Printf("vpcLatticeService: %v \n", vpcLatticeService)

		tgSummary := testFramework.GetTCPTargetGroup(ctx, httpsSvc1)
		tg, err := testFramework.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{
			TargetGroupIdentifier: aws.String(*tgSummary.Id),
		})
		Expect(err).To(BeNil())
		Expect(tg).NotTo(BeNil())
		Expect(*tgSummary.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
		Expect(*tgSummary.Protocol).To(Equal("TCP"))

		// Capture initial targets and pod IPs
		initialTargets = testFramework.GetTargets(ctx, tgSummary, httpsDeployment1)
		Expect(len(initialTargets)).To(BeNumerically(">", 0))

		// Get initial pod IPs
		pods := testFramework.GetPodsByDeploymentName(httpsDeployment1.Name, httpsDeployment1.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))
		for _, pod := range pods {
			initialPodIPs = append(initialPodIPs, pod.Status.PodIP)
		}

		fmt.Printf("Initial pod IPs: %v\n", initialPodIPs)
		fmt.Printf("Initial target count: %d\n", len(initialTargets))
		for _, target := range initialTargets {
			fmt.Printf("Initial target: %s:%d\n", *target.Id, *target.Port)
		}
	})

	It("Scale deployment to zero and back to one", func() {
		// Scale down to zero
		testFramework.Get(ctx, types.NamespacedName{Name: httpsDeployment1.Name, Namespace: httpsDeployment1.Namespace}, httpsDeployment1)
		replicas := int32(0)
		httpsDeployment1.Spec.Replicas = &replicas
		testFramework.ExpectUpdated(ctx, httpsDeployment1)

		// Wait for pods to be terminated
		Eventually(func(g Gomega) {
			pods := testFramework.GetPodsByDeploymentName(httpsDeployment1.Name, httpsDeployment1.Namespace)
			g.Expect(len(pods)).To(BeEquivalentTo(0))
		}).WithTimeout(2 * time.Minute).WithOffset(1).Should(Succeed())

		fmt.Println("Deployment scaled down to zero")

		// Scale back up to one
		testFramework.Get(ctx, types.NamespacedName{Name: httpsDeployment1.Name, Namespace: httpsDeployment1.Namespace}, httpsDeployment1)
		replicas = int32(1)
		httpsDeployment1.Spec.Replicas = &replicas
		testFramework.ExpectUpdated(ctx, httpsDeployment1)

		// Wait for new pod to be ready
		Eventually(func(g Gomega) {
			pods := testFramework.GetPodsByDeploymentName(httpsDeployment1.Name, httpsDeployment1.Namespace)
			g.Expect(len(pods)).To(BeEquivalentTo(1))
			g.Expect(pods[0].Status.Phase).To(Equal(v1.PodRunning))
		}).WithTimeout(3 * time.Minute).WithOffset(1).Should(Succeed())

		fmt.Println("Deployment scaled back up to one")
	})

	It("Verify new pod IPs are different and targets are updated", func() {
		// Get new pod IPs
		var newPodIPs []string
		pods := testFramework.GetPodsByDeploymentName(httpsDeployment1.Name, httpsDeployment1.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))
		for _, pod := range pods {
			newPodIPs = append(newPodIPs, pod.Status.PodIP)
		}

		fmt.Printf("New pod IPs: %v\n", newPodIPs)

		// Verify pod IPs have changed (this is the core issue we're testing)
		Expect(newPodIPs).NotTo(Equal(initialPodIPs))

		// Get updated targets from VPC Lattice
		tgSummary := testFramework.GetTCPTargetGroup(ctx, httpsSvc1)

		// Wait for targets to be updated in VPC Lattice
		Eventually(func(g Gomega) {
			newTargets := testFramework.GetTargets(ctx, tgSummary, httpsDeployment1)
			fmt.Printf("Current target count: %d\n", len(newTargets))
			for _, target := range newTargets {
				fmt.Printf("Current target: %s:%d (status: %s)\n", *target.Id, *target.Port, *target.Status)
			}

			// Verify that targets reflect the new pod IPs
			targetIPs := make([]string, 0)
			for _, target := range newTargets {
				// Only consider healthy or initial targets, not draining ones
				if *target.Status != vpclattice.TargetStatusDraining {
					targetIPs = append(targetIPs, *target.Id)
				}
			}

			// The key assertion: new pod IPs should be registered as targets
			for _, newPodIP := range newPodIPs {
				g.Expect(targetIPs).To(ContainElement(newPodIP),
					fmt.Sprintf("New pod IP %s should be registered as a target", newPodIP))
			}

			// Old pod IPs should not be in active targets (they should be draining or removed)
			for _, oldPodIP := range initialPodIPs {
				g.Expect(targetIPs).NotTo(ContainElement(oldPodIP),
					fmt.Sprintf("Old pod IP %s should not be in active targets", oldPodIP))
			}
		}).WithTimeout(5 * time.Minute).WithOffset(1).Should(Succeed())

		fmt.Println("Target registration successfully updated with new pod IPs")
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			tlsRoute,
			httpsDeployment1,
			httpsSvc1,
		)
	})
})
