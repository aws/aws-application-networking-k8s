package test

import (
	"context"
	"log"
	"os"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"

	"github.com/aws/aws-application-networking-k8s/controllers"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"

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

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/vpclattice"
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
		{&v1alpha1.ServiceExport{}, &v1alpha1.ServiceExportList{}},
		{&v1alpha1.ServiceImport{}, &v1alpha1.ServiceImportList{}},
		{&appsv1.Deployment{}, &appsv1.DeploymentList{}},
		{&v1beta1.HTTPRoute{}, &v1beta1.HTTPRouteList{}},
		{&v1beta1.Gateway{}, &v1beta1.GatewayList{}},
	}
)

type Framework struct {
	client.Client
	ctx                                 context.Context
	k8sScheme                           *runtime.Scheme
	controllerRuntimeConfig             *rest.Config
	LatticeClient                       services.Lattice
	TestCasesCreatedServiceNetworkNames map[string]bool //key: ServiceNetworkName; value: not in use, meaningless
	TestCasesCreatedServiceNames        map[string]bool //key: ServiceName; value not in use, meaningless
	TestCasesCreatedTargetGroupNames    map[string]bool //key: TargetGroupName; value: not in use, meaningless
	TestCasesCreatedK8sResource         []client.Object
}

func NewFramework(ctx context.Context) *Framework {
	var scheme = scheme.Scheme
	lo.Must0(v1beta1.Install(scheme))
	lo.Must0(v1alpha1.Install(scheme))
	config.ConfigInit()
	controllerRuntimeConfig := controllerruntime.GetConfigOrDie()
	framework := &Framework{
		Client:                              lo.Must(client.New(controllerRuntimeConfig, client.Options{Scheme: scheme})),
		LatticeClient:                       services.NewDefaultLattice(session.Must(session.NewSession()), config.Region), // region is currently hardcoded
		ctx:                                 ctx,
		k8sScheme:                           scheme,
		controllerRuntimeConfig:             controllerRuntimeConfig,
		TestCasesCreatedServiceNetworkNames: make(map[string]bool),
		TestCasesCreatedServiceNames:        make(map[string]bool),
		TestCasesCreatedTargetGroupNames:    make(map[string]bool),
	}

	SetDefaultEventuallyTimeout(180 * time.Second)
	SetDefaultEventuallyPollingInterval(10 * time.Second)
	BeforeEach(func() { framework.ExpectToBeClean(ctx) })
	AfterEach(func() { framework.ExpectToBeClean(ctx) })
	return framework
}

func (env *Framework) ExpectToBeClean(ctx context.Context) {
	Logger(ctx).Info("Expecting the test environment to be clean")
	// Kubernetes API Objects
	parallel.ForEach(TestObjects, func(testObject TestObject, _ int) {
		defer GinkgoRecover()
		env.EventuallyExpectNoneFound(ctx, testObject.ListType)
	})

	currentClusterVpcId := os.Getenv("CLUSTER_VPC_ID")
	retrievedServiceNetworkVpcAssociations, _ := env.LatticeClient.ListServiceNetworkVpcAssociationsAsList(ctx, &vpclattice.ListServiceNetworkVpcAssociationsInput{
		VpcIdentifier: aws.String(currentClusterVpcId),
	})
	Logger(ctx).Infof("Expect VPC used by current cluster don't have any ServiceNetworkVPCAssociation, if it has you should manually delete it")
	Expect(len(retrievedServiceNetworkVpcAssociations)).To(Equal(0))
	Eventually(func(g Gomega) {
		retrievedServiceNetworks, _ := env.LatticeClient.ListServiceNetworksAsList(ctx, &vpclattice.ListServiceNetworksInput{})
		for _, sn := range retrievedServiceNetworks {
			Logger(ctx).Infof("Found service network, checking whether it's created by current EKS Cluster: %v", sn)
			g.Expect(*sn.Name).Should(Not(BeKeyOf(env.TestCasesCreatedServiceNetworkNames)))
			retrievedTags, err := env.LatticeClient.ListTagsForResourceWithContext(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: sn.Arn,
			})
			if err == nil { // for err != nil, it is possible that this service network own by other account, and it is shared to current account by RAM
				Logger(ctx).Infof("Found Tags for serviceNetwork %v tags: %v", *sn.Name, retrievedTags)

				value, ok := retrievedTags.Tags[lattice.K8SServiceNetworkOwnedByVPC]
				if ok {
					g.Expect(*value).To(Not(Equal(currentClusterVpcId)))
				}
			}
		}

		retrievedServices, _ := env.LatticeClient.ListServicesAsList(ctx, &vpclattice.ListServicesInput{})
		for _, service := range retrievedServices {
			Logger(ctx).Infof("Found service, checking whether it's created by current EKS Cluster: %v", service)
			g.Expect(*service.Name).Should(Not(BeKeyOf(env.TestCasesCreatedServiceNames)))
			retrievedTags, err := env.LatticeClient.ListTagsForResourceWithContext(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: service.Arn,
			})
			if err == nil { // for err != nil, it is possible that this service own by other account, and it is shared to current account by RAM
				Logger(ctx).Infof("Found Tags for service %v tags: %v", *service.Name, retrievedTags)
				value, ok := retrievedTags.Tags[lattice.K8SServiceOwnedByVPC]
				if ok {
					g.Expect(*value).To(Not(Equal(currentClusterVpcId)))
				}
			}
		}

		retrievedTargetGroups, _ := env.LatticeClient.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{})
		for _, tg := range retrievedTargetGroups {
			Logger(ctx).Infof("Found TargetGroup: %s, checking it whether it's created by current EKS Cluster", *tg.Id)
			if tg.VpcIdentifier != nil && currentClusterVpcId != *tg.VpcIdentifier {
				Logger(ctx).Infof("Target group VPC Id: %s, does not match current EKS Cluster VPC Id: %s", *tg.VpcIdentifier, currentClusterVpcId)
				//This tg is not created by current EKS Cluster, skip it
				continue
			}
			retrievedTags, err := env.LatticeClient.ListTagsForResourceWithContext(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: tg.Arn,
			})
			if err == nil {
				Logger(ctx).Infof("Found Tags for tg %v tags: %v", *tg.Name, retrievedTags)
				tagValue, ok := retrievedTags.Tags[lattice.K8SParentRefTypeKey]
				if ok && *tagValue == lattice.K8SServiceExportType {
					Logger(ctx).Infof("TargetGroup: %s was created by k8s controller, by a ServiceExport", *tg.Id)
					//This tg is created by k8s controller, by a ServiceExport,
					//ServiceExport still have a known targetGroup leaking issue,
					//so we temporarily skip to verify whether ServiceExport created TargetGroup is deleted or not
					continue
				}
				Expect(env.TestCasesCreatedServiceNames).To(Not(ContainElements(BeKeyOf(*tg.Name))))
			}
		}
	}).Should(Succeed())
}

func (env *Framework) CleanTestEnvironment(ctx context.Context) {
	defer GinkgoRecover()
	Logger(ctx).Info("Cleaning the test environment")
	// Kubernetes API Objects
	namespaces := &v1.NamespaceList{}
	Expect(env.List(ctx, namespaces)).WithOffset(1).To(Succeed())
	for _, object := range env.TestCasesCreatedK8sResource {
		Logger(ctx).Infof("Deleting k8s resource %s %s/%s", reflect.TypeOf(object), object.GetNamespace(), object.GetName())
		env.Delete(ctx, object)
		//Ignore resource-not-found error here, as the test case logic itself could already clear the resources
	}

	//Theoretically, Deleting all k8s resource by `env.ExpectDeleteAllToSucceed()`, will make controller delete all related VPC Lattice resource,
	//but the controller is still developing in the progress and may leaking some vPCLattice resource, need to invoke vpcLattice api to double confirm and delete leaking resource.
	env.DeleteAllFrameworkTracedServiceNetworks(ctx)
	env.DeleteAllFrameworkTracedVpcLatticeServices(ctx)
	env.DeleteAllFrameworkTracedTargetGroups(ctx)
	env.EventuallyExpectNotFound(ctx, env.TestCasesCreatedK8sResource...)
	env.TestCasesCreatedK8sResource = nil

}

func (env *Framework) ExpectCreated(ctx context.Context, objects ...client.Object) {
	for _, object := range objects {
		Logger(ctx).Infof("Creating %s %s/%s", reflect.TypeOf(object), object.GetNamespace(), object.GetName())
		Expect(env.Create(ctx, object)).WithOffset(1).To(Succeed())
	}
}

func (env *Framework) ExpectUpdated(ctx context.Context, objects ...client.Object) {
	for _, object := range objects {
		Logger(ctx).Infof("Updating %s %s/%s", reflect.TypeOf(object), object.GetNamespace(), object.GetName())
		Expect(env.Update(ctx, object)).WithOffset(1).To(Succeed())
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
			Logger(ctx).Infof("Checking whether %s %s %s is not found", reflect.TypeOf(object), object.GetNamespace(), object.GetName())
			g.Expect(errors.IsNotFound(env.Get(ctx, client.ObjectKeyFromObject(object), object))).To(BeTrue())
		}
		// Wait for 7 minutes at maximum just in case the k8sService deletion triggered targets draining time
		// and httproute deletion need to wait for that targets draining time finish then it can return
	}).WithTimeout(7 * time.Minute).WithOffset(1).Should(Succeed())
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

func (env *Framework) GetVpcLatticeService(ctx context.Context, httpRoute *v1beta1.HTTPRoute) *vpclattice.ServiceSummary {
	var found *vpclattice.ServiceSummary
	Eventually(func(g Gomega) {
		listServicesOutput, err := env.LatticeClient.ListServicesWithContext(ctx, &vpclattice.ListServicesInput{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(listServicesOutput.Items).ToNot(BeEmpty())
		for _, service := range listServicesOutput.Items {
			if lo.FromPtr(service.Name) == latticestore.LatticeServiceName(httpRoute.Name, httpRoute.Namespace) {
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
		targetGroups, err := env.LatticeClient.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{})
		g.Expect(err).To(BeNil())
		for _, targetGroup := range targetGroups {
			if lo.FromPtr(targetGroup.Name) == latticestore.TargetGroupName(service.Name, service.Namespace) {
				found = targetGroup
				break
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
		podIps, retrievedTargets := GetTargets(targetGroup, deployment, env, ctx)

		targetIps := lo.Filter(retrievedTargets, func(target *vpclattice.TargetSummary, _ int) bool {
			return lo.Contains(podIps, *target.Id) &&
				(*target.Status == vpclattice.TargetStatusInitial ||
					*target.Status == vpclattice.TargetStatusHealthy)
		})

		g.Expect(retrievedTargets).Should(HaveLen(len(targetIps)))
		found = retrievedTargets
	}).WithPolling(time.Minute).WithTimeout(7 * time.Minute).Should(Succeed())
	return found
}

func (env *Framework) GetAllTargets(ctx context.Context, targetGroup *vpclattice.TargetGroupSummary, deployment *appsv1.Deployment) ([]string, []*vpclattice.TargetSummary) {
	return GetTargets(targetGroup, deployment, env, ctx)
}

func GetTargets(targetGroup *vpclattice.TargetGroupSummary, deployment *appsv1.Deployment, env *Framework, ctx context.Context) ([]string, []*vpclattice.TargetSummary) {
	log.Println("Trying to retrieve registered targets for targetGroup", targetGroup.Name)
	log.Println("deployment.Spec.Selector.MatchLabels:", deployment.Spec.Selector.MatchLabels)
	podList := &v1.PodList{}
	expectedMatchingLabels := make(map[string]string, len(deployment.Spec.Selector.MatchLabels))
	for k, v := range deployment.Spec.Selector.MatchLabels {
		expectedMatchingLabels[k] = v
	}
	expectedMatchingLabels[DiscoveryLabel] = "true"
	log.Println("Expected matching labels:", expectedMatchingLabels)
	Expect(env.List(ctx, podList, client.MatchingLabels(expectedMatchingLabels))).To(Succeed())
	Expect(podList.Items).To(HaveLen(int(*deployment.Spec.Replicas)))
	retrievedTargets, err := env.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{TargetGroupIdentifier: targetGroup.Id})
	Expect(err).To(BeNil())

	podIps := utils.SliceMap(podList.Items, func(pod v1.Pod) string { return pod.Status.PodIP })

	return podIps, retrievedTargets
}

func (env *Framework) DeleteAllFrameworkTracedServiceNetworks(ctx aws.Context) {
	log.Println("DeleteAllFrameworkTracedServiceNetworks ", env.TestCasesCreatedServiceNetworkNames)
	sns, err := env.LatticeClient.ListServiceNetworksAsList(ctx, &vpclattice.ListServiceNetworksInput{})
	Expect(err).ToNot(HaveOccurred())
	filteredSns := lo.Filter(sns, func(sn *vpclattice.ServiceNetworkSummary, _ int) bool {
		_, ok := env.TestCasesCreatedServiceNames[*sn.Name]
		return ok
	})
	snIds := lo.Map(filteredSns, func(svc *vpclattice.ServiceNetworkSummary, _ int) *string {
		return svc.Id
	})
	var serviceNetworkIdsWithRemainingAssociations []*string
	for _, snId := range snIds {
		_, err := env.LatticeClient.DeleteServiceNetworkWithContext(ctx, &vpclattice.DeleteServiceNetworkInput{
			ServiceNetworkIdentifier: snId,
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case vpclattice.ErrCodeResourceNotFoundException:
					continue
				case vpclattice.ErrCodeConflictException:
					serviceNetworkIdsWithRemainingAssociations = append(serviceNetworkIdsWithRemainingAssociations, snId)
				}
			}
		}
	}

	var allServiceNetworkVpcAssociationIdsToBeDeleted []*string
	for _, snIdWithRemainingAssociations := range serviceNetworkIdsWithRemainingAssociations {
		associations, err := env.LatticeClient.ListServiceNetworkVpcAssociationsAsList(ctx, &vpclattice.ListServiceNetworkVpcAssociationsInput{
			ServiceNetworkIdentifier: snIdWithRemainingAssociations,
		})
		Expect(err).ToNot(HaveOccurred())

		snvaIds := lo.Map(associations, func(association *vpclattice.ServiceNetworkVpcAssociationSummary, _ int) *string {
			return association.Id
		})
		allServiceNetworkVpcAssociationIdsToBeDeleted = append(allServiceNetworkVpcAssociationIdsToBeDeleted, snvaIds...)
	}

	for _, snvaId := range allServiceNetworkVpcAssociationIdsToBeDeleted {
		_, err := env.LatticeClient.DeleteServiceNetworkVpcAssociationWithContext(ctx, &vpclattice.DeleteServiceNetworkVpcAssociationInput{
			ServiceNetworkVpcAssociationIdentifier: snvaId,
		})
		Expect(err).ToNot(HaveOccurred())
	}

	var allServiceNetworkServiceAssociationIdsToBeDeleted []*string

	for _, snIdWithRemainingAssociations := range serviceNetworkIdsWithRemainingAssociations {
		associations, err := env.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
			ServiceNetworkIdentifier: snIdWithRemainingAssociations,
		})
		Expect(err).ToNot(HaveOccurred())

		snsaIds := lo.Map(associations, func(association *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) *string {
			return association.Id
		})
		allServiceNetworkServiceAssociationIdsToBeDeleted = append(allServiceNetworkServiceAssociationIdsToBeDeleted, snsaIds...)
	}

	for _, snsaId := range allServiceNetworkServiceAssociationIdsToBeDeleted {
		_, err := env.LatticeClient.DeleteServiceNetworkServiceAssociationWithContext(ctx, &vpclattice.DeleteServiceNetworkServiceAssociationInput{
			ServiceNetworkServiceAssociationIdentifier: snsaId,
		})
		Expect(err).ToNot(HaveOccurred())
	}

	Eventually(func(g Gomega) {
		for _, snvaId := range allServiceNetworkVpcAssociationIdsToBeDeleted {
			_, err := env.LatticeClient.GetServiceNetworkVpcAssociationWithContext(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: snvaId,
			})
			if err != nil {
				g.Expect(err.(awserr.Error).Code()).To(Equal(vpclattice.ErrCodeResourceNotFoundException))
			}
		}
		for _, snsaId := range allServiceNetworkServiceAssociationIdsToBeDeleted {
			_, err := env.LatticeClient.GetServiceNetworkServiceAssociationWithContext(ctx, &vpclattice.GetServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: snsaId,
			})
			if err != nil {
				g.Expect(err.(awserr.Error).Code()).To(Equal(vpclattice.ErrCodeResourceNotFoundException))
			}
		}
	}).Should(Succeed())

	for _, snId := range serviceNetworkIdsWithRemainingAssociations {
		env.LatticeClient.DeleteServiceNetworkWithContext(ctx, &vpclattice.DeleteServiceNetworkInput{
			ServiceNetworkIdentifier: snId,
		})
	}

	env.TestCasesCreatedServiceNetworkNames = make(map[string]bool)
}

// In the VPC Lattice backend code, delete VPC Lattice services will also make all its listeners and rules to be deleted asynchronously
func (env *Framework) DeleteAllFrameworkTracedVpcLatticeServices(ctx aws.Context) {
	log.Println("DeleteAllFrameworkTracedVpcLatticeServices", env.TestCasesCreatedServiceNames)
	services, err := env.LatticeClient.ListServicesAsList(ctx, &vpclattice.ListServicesInput{})
	Expect(err).ToNot(HaveOccurred())
	filteredServices := lo.Filter(services, func(service *vpclattice.ServiceSummary, _ int) bool {
		_, ok := env.TestCasesCreatedServiceNames[*service.Name]
		return ok
	})
	serviceIds := lo.Map(filteredServices, func(svc *vpclattice.ServiceSummary, _ int) *string {
		return svc.Id
	})
	var serviceWithRemainingAssociations []*string
	for _, serviceId := range serviceIds {
		_, err := env.LatticeClient.DeleteServiceWithContext(ctx, &vpclattice.DeleteServiceInput{
			ServiceIdentifier: serviceId,
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case vpclattice.ErrCodeResourceNotFoundException:
					delete(env.TestCasesCreatedServiceNames, *serviceId)
					continue
				case vpclattice.ErrCodeConflictException:
					serviceWithRemainingAssociations = append(serviceWithRemainingAssociations, serviceId)
				}
			}

		}
	}
	var allServiceNetworkServiceAssociationIdsToBeDeleted []*string

	for _, serviceIdWithRemainingAssociations := range serviceWithRemainingAssociations {

		associations, err := env.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
			ServiceIdentifier: serviceIdWithRemainingAssociations,
		})
		Expect(err).ToNot(HaveOccurred())

		snsaIds := lo.Map(associations, func(association *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) *string {
			return association.Id
		})
		allServiceNetworkServiceAssociationIdsToBeDeleted = append(allServiceNetworkServiceAssociationIdsToBeDeleted, snsaIds...)
	}

	for _, snsaId := range allServiceNetworkServiceAssociationIdsToBeDeleted {
		_, err := env.LatticeClient.DeleteServiceNetworkServiceAssociationWithContext(ctx, &vpclattice.DeleteServiceNetworkServiceAssociationInput{
			ServiceNetworkServiceAssociationIdentifier: snsaId,
		})
		if err != nil {
			Expect(err.(awserr.Error).Code()).To(Equal(vpclattice.ErrCodeResourceNotFoundException))
		}
	}

	Eventually(func(g Gomega) {
		for _, snsaId := range allServiceNetworkServiceAssociationIdsToBeDeleted {
			_, err := env.LatticeClient.GetServiceNetworkServiceAssociationWithContext(ctx, &vpclattice.GetServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: snsaId,
			})
			if err != nil {
				g.Expect(err.(awserr.Error).Code()).To(Equal(vpclattice.ErrCodeResourceNotFoundException))
			}
		}
	}).Should(Succeed())

	for _, serviceId := range serviceWithRemainingAssociations {
		env.LatticeClient.DeleteServiceWithContext(ctx, &vpclattice.DeleteServiceInput{
			ServiceIdentifier: serviceId,
		})
	}
	env.TestCasesCreatedServiceNames = make(map[string]bool)
}

func (env *Framework) DeleteAllFrameworkTracedTargetGroups(ctx aws.Context) {
	log.Println("DeleteAllFrameworkTracedTargetGroups ", env.TestCasesCreatedTargetGroupNames)
	var tgIdsThatNeedToWaitForDrainingTargetsToBeDeleted []string
	targetGroups, err := env.LatticeClient.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{})
	Expect(err).ToNot(HaveOccurred())
	filteredTgs := lo.Filter(targetGroups, func(targetGroup *vpclattice.TargetGroupSummary, _ int) bool {
		_, ok := env.TestCasesCreatedTargetGroupNames[*targetGroup.Name]
		return ok
	})
	tgIds := lo.Map(filteredTgs, func(targetGroup *vpclattice.TargetGroupSummary, _ int) *string {
		return targetGroup.Id
	})

	log.Println("Number of traced target groups to delete is:", len(tgIds))

	for _, tgId := range tgIds {
		targetSummaries, err := env.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{
			TargetGroupIdentifier: tgId,
		})
		Expect(err).ToNot(HaveOccurred())
		if len(targetSummaries) > 0 {
			tgIdsThatNeedToWaitForDrainingTargetsToBeDeleted = append(tgIdsThatNeedToWaitForDrainingTargetsToBeDeleted, *tgId)
			var targets []*vpclattice.Target = lo.Map(targetSummaries, func(targetSummary *vpclattice.TargetSummary, _ int) *vpclattice.Target {
				return &vpclattice.Target{
					Id:   targetSummary.Id,
					Port: targetSummary.Port,
				}
			})
			env.LatticeClient.DeregisterTargetsWithContext(ctx, &vpclattice.DeregisterTargetsInput{
				TargetGroupIdentifier: tgId,
				Targets:               targets,
			})
		} else {
			Logger(ctx).Infof("Target group %s no longer has targets registered. Deleting now.", *tgId)
			Eventually(func() bool {
				_, err := env.LatticeClient.DeleteTargetGroup(&vpclattice.DeleteTargetGroupInput{
					TargetGroupIdentifier: tgId,
				})
				if err != nil {
					// Allow time for related service to be deleted prior
					return err.(awserr.Error).Code() == vpclattice.ErrCodeResourceNotFoundException
				}
				return true
			}).WithPolling(15 * time.Second).WithTimeout(2 * time.Minute).Should(BeTrue())
		}
	}

	if len(tgIdsThatNeedToWaitForDrainingTargetsToBeDeleted) > 0 {
		log.Println("Need to wait for draining targets to be deregistered", tgIdsThatNeedToWaitForDrainingTargetsToBeDeleted)
		//After initiating the DeregisterTargets call, the Targets will be in `draining` status for the next 5 minutes,
		//And VPC lattice backend will run a background job to completely delete the targets within 6 minutes at maximum in total.
		Eventually(func(g Gomega) {
			log.Println("Trying to clear Target group", tgIdsThatNeedToWaitForDrainingTargetsToBeDeleted, "need to wait for draining targets to be deregistered")

			for _, tgId := range tgIdsThatNeedToWaitForDrainingTargetsToBeDeleted {
				_, err := env.LatticeClient.DeleteTargetGroupWithContext(ctx, &vpclattice.DeleteTargetGroupInput{
					TargetGroupIdentifier: &tgId,
				})
				if err != nil {
					g.Expect(err.(awserr.Error).Code()).To(Equal(vpclattice.ErrCodeResourceNotFoundException))
				}
			}
		}).WithPolling(time.Minute).WithTimeout(7 * time.Minute).Should(Succeed())

	}
	env.TestCasesCreatedServiceNames = make(map[string]bool)
}

func (env *Framework) GetVpcLatticeServiceDns(httpRouteName string, httpRouteNamespace string) string {
	log.Println("GetVpcLatticeServiceDns: ", httpRouteName, httpRouteNamespace)
	httproute := v1beta1.HTTPRoute{}
	env.Get(env.ctx, types.NamespacedName{Name: httpRouteName, Namespace: httpRouteNamespace}, &httproute)
	vpcLatticeServiceDns := httproute.Annotations[controllers.LatticeAssignedDomainName]
	return vpcLatticeServiceDns
}
