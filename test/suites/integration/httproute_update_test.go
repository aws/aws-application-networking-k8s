package integration

import (
	"fmt"
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
		gateway               *v1beta1.Gateway   = nil
		pathMatchHttpRouteOne *v1beta1.HTTPRoute = nil
		pathMatchHttpRouteTwo *v1beta1.HTTPRoute = nil
		deployment1           *appsv1.Deployment = nil
		service1              *corev1.Service    = nil
		deployment2           *appsv1.Deployment = nil
		service2              *corev1.Service    = nil
	)

	var resourceCreationWaitTime = 30 * time.Second

	Context("Create a HTTPRoute with backendref to service1, then update the HTTPRoute with backendref to service1 "+
		"and service2, then update the HTTPRoute with backendref to just service2", func() {
		It("Updates rules correctly with corresponding target groups after each update", func() {
			gateway = testFramework.NewGateway("", "default")
			deployment1, service1 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-v1", Namespace: "default"})
			deployment2, service2 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-v2", Namespace: "default"})

			pathMatchHttpRouteOne = testFramework.NewPathMatchHttpRoute(gateway, []client.Object{service1}, "http",
				"", "default")
			pathMatchHttpRouteTwo = testFramework.NewPathMatchHttpRoute(gateway, []client.Object{service1, service2}, "http",
				"", "default")

			// Create Kubernetes Resources
			testFramework.ExpectCreated(ctx,
				gateway,
				pathMatchHttpRouteOne,
				service1,
				deployment1,
				service2,
				deployment2,
			)

			log.Println("service1.Name: ", service1.Name)
			log.Println("service2.Name: ", service2.Name)
			log.Println(fmt.Sprintf("Waiting %s for Amazon VPC Lattice resource creation.", resourceCreationWaitTime))
			time.Sleep(resourceCreationWaitTime)

			service1TgFound := false
			service2TgFound := false

			targetGroups, err := testFramework.LatticeClient.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{})
			Expect(err).To(BeNil())
			for _, targetGroup := range targetGroups {
				log.Println("targetGroup.Name: ", *targetGroup.Name)

				if lo.FromPtr(targetGroup.Name) == latticestore.TargetGroupName(service1.Name, service1.Namespace) {
					service1TgFound = true
				}
				if lo.FromPtr(targetGroup.Name) == latticestore.TargetGroupName(service2.Name, service2.Namespace) {
					service2TgFound = true
				}
			}
			Expect(service1TgFound).To(BeTrue())
			Expect(service2TgFound).To(BeFalse())

			testFramework.ExpectCreated(ctx,
				pathMatchHttpRouteTwo,
			)
			testFramework.Get(ctx, types.NamespacedName{Name: pathMatchHttpRouteTwo.Name, Namespace: pathMatchHttpRouteTwo.Namespace}, pathMatchHttpRouteTwo)

			log.Println("Will update the pathMatchHttpRoute to backendRefs to service1 and service2")
			testFramework.Update(ctx, pathMatchHttpRouteTwo)
			time.Sleep(30 * time.Second)

			service1TgFound = false
			service2TgFound = false
			targetGroups, err = testFramework.LatticeClient.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{})
			log.Println("Retrieved target groups:", len(targetGroups))
			Expect(err).To(BeNil())
			for _, targetGroup := range targetGroups {
				log.Println("targetGroup.Name:", *targetGroup.Name)

				if lo.FromPtr(targetGroup.Name) == latticestore.TargetGroupName(service1.Name, service1.Namespace) {
					service1TgFound = true
				}
				if lo.FromPtr(targetGroup.Name) == latticestore.TargetGroupName(service2.Name, service2.Namespace) {
					service2TgFound = true
				}
			}
			Expect(service1TgFound).To(BeTrue())
			Expect(service2TgFound).To(BeTrue())

			testFramework.Get(ctx, types.NamespacedName{Name: pathMatchHttpRouteOne.Name, Namespace: pathMatchHttpRouteOne.Namespace}, pathMatchHttpRouteOne)

			log.Println("Will update the pathMatchHttpRoute to backendRefs to just service2")

			// Remove pathMatchHttpRouteTwo for service2 so service is free to use again
			testFramework.Update(ctx, pathMatchHttpRouteOne)
			testFramework.ExpectDeleted(ctx, pathMatchHttpRouteTwo)
			testFramework.EventuallyExpectNotFound(ctx, pathMatchHttpRouteTwo)

			pathMatchHttpRouteOne.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name = v1beta1.ObjectName(service2.Name)
			testFramework.Update(ctx, pathMatchHttpRouteOne)
			time.Sleep(30 * time.Second)

			service1TgFound = false
			service2TgFound = false
			targetGroups, err = testFramework.LatticeClient.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{})
			log.Println("Retrieved target groups:", len(targetGroups))
			Expect(err).To(BeNil())
			for _, targetGroup := range targetGroups {
				log.Println("targetGroup.Name: ", *targetGroup.Name)

				if lo.FromPtr(targetGroup.Name) == latticestore.TargetGroupName(service1.Name, service1.Namespace) {
					service1TgFound = true
				}
				if lo.FromPtr(targetGroup.Name) == latticestore.TargetGroupName(service2.Name, service2.Namespace) {
					service2TgFound = true
				}
			}
			Expect(service1TgFound).To(BeFalse())
			Expect(service2TgFound).To(BeTrue())
		})
	})

	AfterEach(func() {
		testFramework.CleanTestEnvironment(ctx)
	})
})
