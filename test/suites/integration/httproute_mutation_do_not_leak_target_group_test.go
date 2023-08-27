package integration

import (
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"strings"
)

var _ = Describe("HTTPRoute Mutation", func() {
	var (
		pathMatchHttpRoute *v1beta1.HTTPRoute = nil
		deployment1        *appsv1.Deployment = nil
		service1           *corev1.Service    = nil
		deployment2        *appsv1.Deployment = nil
		service2           *corev1.Service    = nil
		deployment3        *appsv1.Deployment = nil
		service3           *corev1.Service    = nil
	)

	It("Create a HTTPRoute that backendref to service1 and service2 first, tg1 and tg2 should be created, tg3 should not be created. "+
		"Then, update the HTTPRoute to backendref to service1 and service3, tg1 should still exist, tg2 should be deleted, tg3 should be created", func() {
		deployment1, service1 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-v1", Namespace: k8snamespace})
		deployment2, service2 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-v2", Namespace: k8snamespace})
		deployment3, service3 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-v3", Namespace: k8snamespace})

		pathMatchHttpRoute = testFramework.NewPathMatchHttpRoute(testGateway, []client.Object{service1, service2}, "http",
			"", k8snamespace)

		// Create Kubernetes Resources
		testFramework.ExpectCreated(ctx,
			pathMatchHttpRoute,
			service1,
			deployment1,
			service2,
			deployment2,
			service3,
			deployment3,
		)

		Eventually(func(g Gomega) {
			service1TgFound := false
			service2TgFound := false
			service3TgFound := false

			targetGroups, err := testFramework.LatticeClient.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{})
			g.Expect(err).To(BeNil())
			for _, targetGroup := range targetGroups {
				fmt.Println("targetGroup.Name: ", *targetGroup.Name)

				if strings.HasPrefix(lo.FromPtr(targetGroup.Name), latticestore.TargetGroupName(service1.Name, service1.Namespace)) {
					service1TgFound = true
				}
				if strings.HasPrefix(lo.FromPtr(targetGroup.Name), latticestore.TargetGroupName(service2.Name, service2.Namespace)) {
					service2TgFound = true
				}
				if strings.HasPrefix(lo.FromPtr(targetGroup.Name), latticestore.TargetGroupName(service3.Name, service3.Namespace)) {
					service3TgFound = true
				}
			}
			g.Expect(service1TgFound).To(BeTrue())
			g.Expect(service2TgFound).To(BeTrue())
			g.Expect(service3TgFound).To(BeFalse())
		}).Should(Succeed())

		testFramework.Get(ctx, types.NamespacedName{Name: pathMatchHttpRoute.Name, Namespace: pathMatchHttpRoute.Namespace}, pathMatchHttpRoute)

		fmt.Println("Will update the pathMatchHttpRoute to backendRefs to service1 and service3")
		pathMatchHttpRoute.Spec.Rules[1].BackendRefs[0].BackendObjectReference.Name = v1beta1.ObjectName(service3.Name)
		testFramework.Update(ctx, pathMatchHttpRoute)

		// Verify the targetGroup that corresponds to the service2 is deleted
		// And the targetGroup that corresponds to the service3 is created
		Eventually(func(g Gomega) {
			service1TgFound := false
			service2TgFound := false
			service3TgFound := false
			targetGroups, err := testFramework.LatticeClient.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{})
			fmt.Println("Retrieved targetGroups len: ", len(targetGroups))
			g.Expect(err).To(BeNil())
			for _, targetGroup := range targetGroups {
				fmt.Println("targetGroup.Name: ", *targetGroup.Name)

				if strings.HasPrefix(lo.FromPtr(targetGroup.Name), latticestore.TargetGroupName(service1.Name, service1.Namespace)) {
					service1TgFound = true
				}
				if strings.HasPrefix(lo.FromPtr(targetGroup.Name), latticestore.TargetGroupName(service2.Name, service2.Namespace)) {
					service2TgFound = true
				}
				if strings.HasPrefix(lo.FromPtr(targetGroup.Name), latticestore.TargetGroupName(service3.Name, service3.Namespace)) {
					service3TgFound = true
				}
			}
			g.Expect(service1TgFound).To(BeTrue())
			g.Expect(service2TgFound).To(BeFalse())
			g.Expect(service3TgFound).To(BeTrue())
		}).Should(Succeed())
	})

	AfterEach(func() {
		testFramework.ExpectDeleted(ctx, pathMatchHttpRoute)
		testFramework.SleepForRouteDeletion()
		testFramework.ExpectDeletedThenNotFound(ctx,
			pathMatchHttpRoute,
			deployment1,
			service1,
			deployment2,
			service2,
			deployment3,
			service3,
		)
	})
})
