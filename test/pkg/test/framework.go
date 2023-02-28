package test

import (
	"context"
	"reflect"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/samber/lo"
	"github.com/samber/lo/parallel"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func init() {
	format.MaxLength = 0
}

type TestObject struct {
	Type     client.Object
	ListType client.ObjectList
}

var (
	TestObjects = []TestObject{
		{&v1.Service{}, &v1.ServiceList{}},
		{&appsv1.Deployment{}, &appsv1.DeploymentList{}},
		{&v1beta1.HTTPRoute{}, &v1beta1.HTTPRouteList{}},
		{&v1beta1.Gateway{}, &v1beta1.GatewayList{}},
	}
)

type Framework struct {
	client.Client
	LatticeClient services.Lattice
}

func NewFramework(ctx context.Context) *Framework {
	var scheme = scheme.Scheme
	lo.Must0(v1beta1.Install(scheme))
	lo.Must0(v1alpha1.Install(scheme))
	framework := &Framework{
		Client:        lo.Must(client.New(controllerruntime.GetConfigOrDie(), client.Options{Scheme: scheme})),
		LatticeClient: services.NewDefaultLattice(session.Must(session.NewSession()), ""), // region is currently hardcoded
	}
	SetDefaultEventuallyTimeout(180 * time.Second)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	BeforeEach(func() { framework.ExpectToBeClean(ctx) })
	AfterSuite(func() { framework.ExpectToClean(ctx) })
	return framework
}

func (env *Framework) ExpectToBeClean(ctx context.Context) {
	Logger(ctx).Info("Expecting the test environment to be clean")
	// Kubernetes API Objects
	parallel.ForEach(TestObjects, func(testObject TestObject, _ int) {
		defer GinkgoRecover()
		env.EventuallyExpectNoneFound(ctx, testObject.ListType)
	})

	// AWS API Objects
	Eventually(func(g Gomega) {
		g.Expect(env.LatticeClient.ListServicesWithContext(ctx, &vpclattice.ListServicesInput{})).To(HaveField("Items", BeEmpty()))
		g.Expect(env.LatticeClient.ListServiceNetworksWithContext(ctx, &vpclattice.ListServiceNetworksInput{})).To(HaveField("Items", BeEmpty()))
		g.Expect(env.LatticeClient.ListTargetGroupsWithContext(ctx, &vpclattice.ListTargetGroupsInput{})).To(HaveField("Items", BeEmpty()))
	}).Should(Succeed())
}

func (env *Framework) ExpectToClean(ctx context.Context) {
	Logger(ctx).Info("Cleaning the test environment")
	// Kubernetes API Objects
	namespaces := &v1.NamespaceList{}
	Expect(env.List(ctx, namespaces)).WithOffset(1).To(Succeed())
	for _, namespace := range namespaces.Items {
		parallel.ForEach(TestObjects, func(testObject TestObject, _ int) {
			defer GinkgoRecover()
			env.ExpectDeleteAllToSucceed(ctx, testObject.Type, namespace.Name)
			env.EventuallyExpectNoneFound(ctx, testObject.ListType)
		})
	}

	// AWS API Objects
	// Delete Services
	listServicesOutput, err := env.LatticeClient.ListServicesWithContext(ctx, &vpclattice.ListServicesInput{})
	Expect(err).ToNot(HaveOccurred())
	for _, service := range listServicesOutput.Items {
		// Delete ServiceNetworkServiceAssociations
		listServiceNetworkServiceAssociationsOutput, err := env.LatticeClient.ListServiceNetworkServiceAssociationsWithContext(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{ServiceIdentifier: service.Id})
		Expect(err).ToNot(HaveOccurred())
		for _, serviceNetworkServiceAssociation := range listServiceNetworkServiceAssociationsOutput.Items {
			_, err := env.LatticeClient.DeleteServiceNetworkServiceAssociationWithContext(ctx, &vpclattice.DeleteServiceNetworkServiceAssociationInput{ServiceNetworkServiceAssociationIdentifier: serviceNetworkServiceAssociation.Id})
			Expect(err).ToNot(HaveOccurred())
		}
		// Delete Listeners
		listListenersOutput, err := env.LatticeClient.ListListenersWithContext(ctx, &vpclattice.ListListenersInput{ServiceIdentifier: service.Id})
		Expect(err).ToNot(HaveOccurred())
		for _, listener := range listListenersOutput.Items {
			_, err = env.LatticeClient.DeleteListenerWithContext(ctx, &vpclattice.DeleteListenerInput{ServiceIdentifier: service.Id, ListenerIdentifier: listener.Id})
			Expect(err).ToNot(HaveOccurred())
		}
		// Delete Service
		_, err = env.LatticeClient.DeleteServiceWithContext(ctx, &vpclattice.DeleteServiceInput{ServiceIdentifier: service.Id})
		Expect(err).ToNot(HaveOccurred())
	}
	// Delete TargetGroups
	listTargetGroupsOutput, err := env.LatticeClient.ListTargetGroupsWithContext(ctx, &vpclattice.ListTargetGroupsInput{})
	Expect(err).ToNot(HaveOccurred())
	for _, targetGroup := range listTargetGroupsOutput.Items {
		// Delete Targets
		listTargetsOutput, err := env.LatticeClient.ListTargetsWithContext(ctx, &vpclattice.ListTargetsInput{TargetGroupIdentifier: targetGroup.Id})
		Expect(err).ToNot(HaveOccurred())
		if targets := lo.Map(listTargetsOutput.Items, func(target *vpclattice.TargetSummary, _ int) *vpclattice.Target {
			return &vpclattice.Target{Id: target.Id}
		}); len(targets) > 0 {
			_, err = env.LatticeClient.DeregisterTargetsWithContext(ctx, &vpclattice.DeregisterTargetsInput{TargetGroupIdentifier: targetGroup.Id, Targets: targets})
			Expect(err).ToNot(HaveOccurred())
		}
		// Delete TargetGroup
		_, err = env.LatticeClient.DeleteTargetGroupWithContext(ctx, &vpclattice.DeleteTargetGroupInput{TargetGroupIdentifier: targetGroup.Id})
		Expect(err).ToNot(HaveOccurred())
	}
	listServiceNetworksOutput, err := env.LatticeClient.ListServiceNetworksWithContext(ctx, &vpclattice.ListServiceNetworksInput{})
	Expect(err).ToNot(HaveOccurred())
	for _, serviceNetwork := range listServiceNetworksOutput.Items {
		_, err := env.LatticeClient.DeleteServiceNetworkWithContext(ctx, &vpclattice.DeleteServiceNetworkInput{ServiceNetworkIdentifier: serviceNetwork.Id})
		Expect(err).ToNot(HaveOccurred())
	}

	// Wait for objects to delete
	env.ExpectToBeClean(ctx)
}

func (env *Framework) ExpectCreated(ctx context.Context, objects ...client.Object) {
	for _, object := range objects {
		Logger(ctx).Infof("Creating %s %s/%s", reflect.TypeOf(object), object.GetNamespace(), object.GetName())
		Expect(env.Create(ctx, object)).WithOffset(1).To(Succeed())
	}
}

func (env *Framework) ExpectDeleted(ctx context.Context, objects ...client.Object) {
	for _, object := range objects {
		Logger(ctx).Infof("Deleting %s %s/%s", reflect.TypeOf(object), object.GetNamespace(), object.GetName())
		Expect(env.Delete(ctx, object)).WithOffset(1).To(Succeed())
	}
}

func (env *Framework) ExpectDeleteAllToSucceed(ctx context.Context, object client.Object, namespace string) {
	Expect(env.DeleteAllOf(ctx, object, client.InNamespace(namespace), client.HasLabels([]string{DiscoveryLabel}))).WithOffset(1).To(Succeed())
}

func (env *Framework) EventuallyExpectNotFound(ctx context.Context, objects ...client.Object) {
	Eventually(func(g Gomega) {
		for _, object := range objects {
			g.Expect(errors.IsNotFound(env.Get(ctx, client.ObjectKeyFromObject(object), object))).To(BeTrue())
		}
	}).Should(Succeed())
}

func (env *Framework) EventuallyExpectNoneFound(ctx context.Context, objectList client.ObjectList) {
	Eventually(func(g Gomega) {
		g.Expect(env.List(ctx, objectList, client.HasLabels([]string{DiscoveryLabel}))).To(Succeed())
		g.Expect(meta.ExtractList(objectList)).To(HaveLen(0), "Expected to not find any %q with label %q", reflect.TypeOf(objectList), DiscoveryLabel)
	}).WithOffset(1).Should(Succeed())
}

func (env *Framework) GetServiceNetwork(ctx context.Context, gateway *v1beta1.Gateway) *vpclattice.ServiceNetworkSummary {
	var found *vpclattice.ServiceNetworkSummary
	Eventually(func(g Gomega) {
		listServiceNetworksOutput, err := env.LatticeClient.ListServiceNetworksWithContext(ctx, &vpclattice.ListServiceNetworksInput{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(listServiceNetworksOutput.Items).ToNot(BeEmpty())
		for _, serviceNetwork := range listServiceNetworksOutput.Items {
			if lo.FromPtr(serviceNetwork.Name) == gateway.Name {
				found = serviceNetwork
			}
		}
		g.Expect(found).ToNot(BeNil())
	}).WithOffset(1).Should(Succeed())
	return found
}

func (env *Framework) GetService(ctx context.Context, httpRoute *v1beta1.HTTPRoute) *vpclattice.ServiceSummary {
	var found *vpclattice.ServiceSummary
	Eventually(func(g Gomega) {
		listServicesOutput, err := env.LatticeClient.ListServicesWithContext(ctx, &vpclattice.ListServicesInput{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(listServicesOutput.Items).ToNot(BeEmpty())
		for _, service := range listServicesOutput.Items {
			if lo.FromPtr(service.Name) == latticestore.AWSServiceName(httpRoute.Name, httpRoute.Namespace) {
				found = service
			}
		}
		g.Expect(found).ToNot(BeNil())
		g.Expect(found.Status).To(Equal(lo.ToPtr(vpclattice.ServiceStatusActive)))
	}).WithOffset(1).Should(Succeed())

	return found
}

func (env *Framework) GetTargetGroup(ctx context.Context, service *v1.Service) *vpclattice.TargetGroupSummary {
	var found *vpclattice.TargetGroupSummary
	Eventually(func(g Gomega) {
		listTargetGroupsOutput, err := env.LatticeClient.ListTargetGroupsWithContext(ctx, &vpclattice.ListTargetGroupsInput{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(listTargetGroupsOutput.Items).ToNot(BeEmpty())
		for _, targetGroup := range listTargetGroupsOutput.Items {
			if lo.FromPtr(targetGroup.Name) == latticestore.TargetGroupName(service.Name, service.Namespace) {
				found = targetGroup
			}
		}
		g.Expect(found).ToNot(BeNil())
		g.Expect(found.Status).To(Equal(lo.ToPtr(vpclattice.TargetGroupStatusActive)))
	}).WithOffset(1).Should(Succeed())
	return found
}

func (env *Framework) GetTargets(ctx context.Context, targetGroup *vpclattice.TargetGroupSummary, deployment *appsv1.Deployment) []*vpclattice.TargetSummary {
	var found []*vpclattice.TargetSummary
	Eventually(func(g Gomega) {
		podList := &v1.PodList{}
		g.Expect(env.List(ctx, podList, client.MatchingLabels(deployment.Spec.Selector.MatchLabels))).To(Succeed())
		g.Expect(podList.Items).To(HaveLen(int(*deployment.Spec.Replicas)))

		listTargetsOutput, err := env.LatticeClient.ListTargetsWithContext(ctx, &vpclattice.ListTargetsInput{TargetGroupIdentifier: targetGroup.Id})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(listTargetsOutput.Items).To(HaveLen(int(*deployment.Spec.Replicas)))

		podIps := lo.Map(podList.Items, func(pod v1.Pod, _ int) string { return pod.Status.PodIP })
		targetIps := lo.Filter(listTargetsOutput.Items, func(target *vpclattice.TargetSummary, _ int) bool {
			return *target.Status == vpclattice.TargetStatusInitial && lo.Contains(podIps, *target.Id)
		})
		g.Expect(targetIps).To(HaveLen(int(*deployment.Spec.Replicas)))

		found = listTargetsOutput.Items
	}).WithOffset(1).Should(Succeed())
	return found
}
