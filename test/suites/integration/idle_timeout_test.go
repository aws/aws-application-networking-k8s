package integration

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Idle Timeout", Ordered, func() {
	var (
		deployment *appsv1.Deployment
		svc        *v1.Service
		httpRoute  *gwv1.HTTPRoute
	)

	expectIdleTimeout := func(want int32) {
		route := core.NewHTTPRoute(*httpRoute)
		Eventually(func(g Gomega) {
			summary := testFramework.GetVpcLatticeService(ctx, route)
			out, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: summary.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(out.IdleTimeoutSeconds).ToNot(BeNil())
			g.Expect(*out.IdleTimeoutSeconds).To(Equal(want))
		}).WithTimeout(3 * time.Minute).WithPolling(15 * time.Second).Should(Succeed())
	}

	It("sets idleTimeoutSeconds on the Lattice service from the annotation", func() {
		deployment, svc = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "idle-timeout-test",
			Namespace: k8snamespace,
		})
		httpRoute = &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "idle-timeout-route",
				Namespace: k8snamespace,
				Annotations: map[string]string{
					"application-networking.k8s.aws/idle-timeout-seconds": "120",
				},
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{Name: gwv1.ObjectName(testGateway.Name)}},
				},
				Rules: []gwv1.HTTPRouteRule{
					{
						BackendRefs: []gwv1.HTTPBackendRef{
							{BackendRef: gwv1.BackendRef{BackendObjectReference: gwv1.BackendObjectReference{
								Name: gwv1.ObjectName(svc.Name),
								Port: (*gwv1.PortNumber)(&svc.Spec.Ports[0].Port),
							}}},
						},
					},
				},
			},
		}

		testFramework.ExpectCreated(ctx, deployment, svc, httpRoute)
		expectIdleTimeout(120)
	})

	It("updates idleTimeoutSeconds when the annotation changes", func() {
		// Re-fetch to avoid a stale resourceVersion conflict on Update.
		Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), httpRoute)).To(Succeed())
		httpRoute.Annotations["application-networking.k8s.aws/idle-timeout-seconds"] = "300"
		testFramework.ExpectUpdated(ctx, httpRoute)

		expectIdleTimeout(300)
	})

	It("leaves the last value when the annotation is removed", func() {
		Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), httpRoute)).To(Succeed())
		delete(httpRoute.Annotations, "application-networking.k8s.aws/idle-timeout-seconds")
		testFramework.ExpectUpdated(ctx, httpRoute)

		// Manage-only-when-set: removing the annotation does not revert; the service keeps 300.
		Consistently(func(g Gomega) {
			summary := testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(*httpRoute))
			out, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: summary.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(out.IdleTimeoutSeconds).ToNot(BeNil())
			g.Expect(*out.IdleTimeoutSeconds).To(Equal(int32(300)))
		}).WithTimeout(30 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx, httpRoute, deployment, svc)
	})
})
