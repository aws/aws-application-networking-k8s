package integration

import (
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/types"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
)

var _ = Describe("HTTPRoute Update", func() {

	var (
		pathMatchHttpRouteOne *v1beta1.HTTPRoute
		pathMatchHttpRouteTwo *v1beta1.HTTPRoute
		deployment1           *appsv1.Deployment
		service1              *corev1.Service
		deployment2           *appsv1.Deployment
		service2              *corev1.Service
	)

	Context("Create a HTTPRoute with backendref to service1, then update the HTTPRoute with backendref to service1 "+
		"and service2, then update the HTTPRoute with backendref to just service2", func() {

		It("Updates rules correctly with corresponding target groups after each update", func() {
			deployment1, service1 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-v1", Namespace: k8snamespace})
			deployment2, service2 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-v2", Namespace: k8snamespace})

			pathMatchHttpRouteOne = testFramework.NewPathMatchHttpRoute(testGateway, []client.Object{service1}, "http",
				"", k8snamespace)
			pathMatchHttpRouteTwo = testFramework.NewPathMatchHttpRoute(testGateway, []client.Object{service1, service2}, "http",
				"", k8snamespace)

			// Create Kubernetes Resources
			testFramework.ExpectCreated(ctx,
				pathMatchHttpRouteOne,
				deployment1,
				service1,
				deployment2,
				service2,
			)

			log.Println("Set the pathMatchHttpRoute to backendRefs to just service1")
			checkTgs(service1, service2, true, false)

			testFramework.ExpectCreated(ctx,
				pathMatchHttpRouteTwo,
			)
			testFramework.Get(ctx, types.NamespacedName{Name: pathMatchHttpRouteTwo.Name, Namespace: pathMatchHttpRouteTwo.Namespace}, pathMatchHttpRouteTwo)
			testFramework.Update(ctx, pathMatchHttpRouteTwo)

			log.Println("Updated the pathMatchHttpRoute to backendRefs to service1 and service2")
			checkTgs(service1, service2, true, true)

			testFramework.Get(ctx, types.NamespacedName{Name: pathMatchHttpRouteOne.Name, Namespace: pathMatchHttpRouteOne.Namespace}, pathMatchHttpRouteOne)
			testFramework.Update(ctx, pathMatchHttpRouteOne) // Remove pathMatchHttpRouteTwo for service2 so service is free to use again
			testFramework.ExpectDeleted(ctx, pathMatchHttpRouteTwo)
			testFramework.EventuallyExpectNotFound(ctx, pathMatchHttpRouteTwo)
			pathMatchHttpRouteOne.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name = v1beta1.ObjectName(service2.Name)
			testFramework.Update(ctx, pathMatchHttpRouteOne)

			log.Println("Updated the pathMatchHttpRoute to backendRefs to just service2")
			checkTgs(service1, service2, false, true)
		})
	})

	AfterEach(func() {
		testFramework.ExpectDeleted(ctx, pathMatchHttpRouteOne, pathMatchHttpRouteTwo)
		testFramework.SleepForRouteDeletion()

		testFramework.ExpectDeletedThenNotFound(ctx,
			pathMatchHttpRouteOne,
			pathMatchHttpRouteTwo,
			deployment1,
			service1,
			deployment2,
			service2,
		)
	})
})

func checkTgs(service1 *corev1.Service, service2 *corev1.Service, expectedService1TgFound bool, expectedService2TgFound bool) {
	Eventually(func(g Gomega) bool {
		var service1TgFound = false
		var service2TgFound = false

		targetGroups, err := testFramework.LatticeClient.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{})
		Expect(err).To(BeNil())

		for _, targetGroup := range targetGroups {
			if lo.FromPtr(targetGroup.Name) == latticestore.TargetGroupName(service1.Name, service1.Namespace) {
				service1TgFound = true
			}
			if lo.FromPtr(targetGroup.Name) == latticestore.TargetGroupName(service2.Name, service2.Namespace) {
				service2TgFound = true
			}
		}

		return service1TgFound == expectedService1TgFound && service2TgFound == expectedService2TgFound
	}).WithPolling(15 * time.Second).WithTimeout(2 * time.Minute).Should(BeTrue())
}
