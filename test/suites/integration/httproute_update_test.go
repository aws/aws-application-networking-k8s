package integration

import (
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"time"

	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("HTTPRoute Update", func() {

	var (
		route1      *gwv1.HTTPRoute
		route2      *gwv1.HTTPRoute
		deployment1 *appsv1.Deployment
		service1    *corev1.Service
		tg1         *vpclattice.TargetGroupSummary
		tg2         *vpclattice.TargetGroupSummary
		err         error
	)

	Context("BackendRefs to the same service use different target groups", func() {
		It("Target groups for the same service are different for different routes", func() {
			deployment1, service1 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "update-test-v1", Namespace: k8snamespace})
			route1 = testFramework.NewPathMatchHttpRoute(testGateway, []client.Object{service1}, "http",
				"route-one", k8snamespace)
			route2 = testFramework.NewPathMatchHttpRoute(testGateway, []client.Object{service1}, "http",
				"route-two", k8snamespace)

			r1TgSpec := model.TargetGroupSpec{
				Protocol:        vpclattice.TargetGroupProtocolHttp,
				ProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp1,
				TargetGroupTagFields: model.TargetGroupTagFields{
					K8SServiceName:      service1.Name,
					K8SServiceNamespace: service1.Namespace,
					K8SRouteName:        route1.Name,
					K8SRouteNamespace:   route1.Namespace,
				},
			}
			r2TgSpec := model.TargetGroupSpec{
				Protocol:        vpclattice.TargetGroupProtocolHttp,
				ProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp1,
				TargetGroupTagFields: model.TargetGroupTagFields{
					K8SServiceName:      service1.Name,
					K8SServiceNamespace: service1.Namespace,
					K8SRouteName:        route2.Name,
					K8SRouteNamespace:   route2.Namespace,
				},
			}

			// Create Kubernetes Resources
			testFramework.ExpectCreated(ctx,
				service1,
				deployment1,
				route1,
				route2,
			)

			// we want two separate target groups
			Eventually(func(g Gomega) {
				tg1, err = testFramework.FindTargetGroupFromSpec(ctx, r1TgSpec)
				g.Expect(err).To(BeNil())
				g.Expect(tg1).ToNot(BeNil())
				tg2, err = testFramework.FindTargetGroupFromSpec(ctx, r2TgSpec)
				g.Expect(err).To(BeNil())
				g.Expect(tg2).ToNot(BeNil())

				// without this we end up trying to delete while the tgs are still creating
				g.Expect(*tg1.Status).To(Equal(vpclattice.TargetGroupStatusActive))
				g.Expect(*tg2.Status).To(Equal(vpclattice.TargetGroupStatusActive))
			}).WithPolling(15 * time.Second).WithTimeout(2 * time.Minute).Should(Succeed())

			gwlog.FallbackLogger.Infof("Found TG1 %s and TG2 %s", aws.StringValue(tg1.Id), aws.StringValue(tg2.Id))
			Expect(aws.StringValue(tg1.Id) != aws.StringValue(tg2.Id)).To(BeTrue())

			// deletion of one should not affect the other
			testFramework.ExpectDeleted(ctx, route1)
			Eventually(func(g Gomega) {
				tg1, err = testFramework.FindTargetGroupFromSpec(ctx, r1TgSpec)
				g.Expect(err).To(BeNil())
				g.Expect(tg1).To(BeNil())
				tg2, err = testFramework.FindTargetGroupFromSpec(ctx, r2TgSpec)
				g.Expect(err).To(BeNil())
				g.Expect(tg2).ToNot(BeNil())
			}).WithPolling(15 * time.Second).WithTimeout(2 * time.Minute).Should(Succeed())
		})
	})

	AfterEach(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			route1,
			route2,
			deployment1,
			service1,
		)
	})
})
