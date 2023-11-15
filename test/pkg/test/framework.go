package test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	an_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/onsi/gomega/format"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/external-dns/endpoint"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"github.com/samber/lo/parallel"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/controllers"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
)

type TestObject struct {
	Type     client.Object
	ListType client.ObjectList
}

var (
	testScheme          = runtime.NewScheme()
	CurrentClusterVpcId = os.Getenv("CLUSTER_VPC_ID")
	TestObjects         = []TestObject{
		{&gwv1beta1.HTTPRoute{}, &gwv1beta1.HTTPRouteList{}},
		{&anv1alpha1.ServiceExport{}, &anv1alpha1.ServiceExportList{}},
		{&anv1alpha1.ServiceImport{}, &anv1alpha1.ServiceImportList{}},
		{&gwv1beta1.Gateway{}, &gwv1beta1.GatewayList{}},
		{&appsv1.Deployment{}, &appsv1.DeploymentList{}},
		{&corev1.Service{}, &corev1.ServiceList{}},
	}
)

func init() {
	format.MaxLength = 0
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(gwv1alpha2.AddToScheme(testScheme))
	utilruntime.Must(gwv1beta1.AddToScheme(testScheme))
	utilruntime.Must(anv1alpha1.AddToScheme(testScheme))
	addOptionalCRDs(testScheme)
}

func addOptionalCRDs(scheme *runtime.Scheme) {
	dnsEndpoint := schema.GroupVersion{
		Group:   "externaldns.k8s.io",
		Version: "v1alpha1",
	}
	scheme.AddKnownTypes(dnsEndpoint, &endpoint.DNSEndpoint{}, &endpoint.DNSEndpointList{})
	metav1.AddToGroupVersion(scheme, dnsEndpoint)

	awsGatewayControllerCRDGroupVersion := schema.GroupVersion{
		Group:   anv1alpha1.GroupName,
		Version: "v1alpha1",
	}
	scheme.AddKnownTypes(awsGatewayControllerCRDGroupVersion, &anv1alpha1.TargetGroupPolicy{}, &anv1alpha1.TargetGroupPolicyList{})
	metav1.AddToGroupVersion(scheme, awsGatewayControllerCRDGroupVersion)

	scheme.AddKnownTypes(awsGatewayControllerCRDGroupVersion, &anv1alpha1.VpcAssociationPolicy{}, &anv1alpha1.VpcAssociationPolicyList{})
	metav1.AddToGroupVersion(scheme, awsGatewayControllerCRDGroupVersion)

	scheme.AddKnownTypes(awsGatewayControllerCRDGroupVersion, &anv1alpha1.AccessLogPolicy{}, &anv1alpha1.AccessLogPolicyList{})
	metav1.AddToGroupVersion(scheme, awsGatewayControllerCRDGroupVersion)
}

type Framework struct {
	client.Client
	ctx                     context.Context
	k8sScheme               *runtime.Scheme
	namespace               string
	controllerRuntimeConfig *rest.Config
	Log                     gwlog.Logger
	LatticeClient           services.Lattice
	Ec2Client               *ec2.EC2
	GrpcurlRunner           *corev1.Pod
	DefaultTags             services.Tags
	Cloud                   an_aws.Cloud
}

func NewFramework(ctx context.Context, log gwlog.Logger, testNamespace string) *Framework {
	addOptionalCRDs(testScheme)
	config.ConfigInit()
	controllerRuntimeConfig := controllerruntime.GetConfigOrDie()
	cloudConfig := an_aws.CloudConfig{
		VpcId:       config.VpcID,
		AccountId:   config.AccountID,
		Region:      config.Region,
		ClusterName: config.ClusterName,
	}
	framework := &Framework{
		Client:                  lo.Must(client.New(controllerRuntimeConfig, client.Options{Scheme: testScheme})),
		LatticeClient:           services.NewDefaultLattice(session.Must(session.NewSession()), config.Region), // region is currently hardcoded
		Ec2Client:               ec2.New(session.Must(session.NewSession(&aws.Config{Region: aws.String(config.Region)}))),
		GrpcurlRunner:           &corev1.Pod{},
		ctx:                     ctx,
		Log:                     log,
		k8sScheme:               testScheme,
		namespace:               testNamespace,
		controllerRuntimeConfig: controllerRuntimeConfig,
		DefaultTags:             an_aws.NewDefaultCloud(nil, cloudConfig).DefaultTags(),
		Cloud:                   an_aws.NewDefaultCloud(nil, cloudConfig),
	}
	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(10 * time.Second)
	return framework
}

func (env *Framework) ExpectToBeClean(ctx context.Context) {
	env.Log.Info("Expecting the test environment to be clean")
	// Kubernetes API Objects
	parallel.ForEach(TestObjects, func(testObject TestObject, _ int) {
		defer GinkgoRecover()
		env.EventuallyExpectNoneFound(ctx, testObject.ListType)
	})

	retrievedServiceNetworkVpcAssociations, _ := env.LatticeClient.ListServiceNetworkVpcAssociationsAsList(ctx, &vpclattice.ListServiceNetworkVpcAssociationsInput{
		VpcIdentifier: aws.String(CurrentClusterVpcId),
	})
	env.Log.Infof("Expect VPC used by current cluster has no ServiceNetworkVPCAssociation, if it does you should manually delete it")
	Expect(len(retrievedServiceNetworkVpcAssociations)).To(Equal(0))
	Eventually(func(g Gomega) {
		retrievedServiceNetworks, _ := env.LatticeClient.ListServiceNetworksAsList(ctx, &vpclattice.ListServiceNetworksInput{})
		for _, sn := range retrievedServiceNetworks {
			env.Log.Infof("Found service network, checking if created by current EKS Cluster: %v", sn)
			retrievedTags, err := env.LatticeClient.ListTagsForResourceWithContext(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: sn.Arn,
			})
			if err == nil { // for err != nil, it is possible that this service network own by other account, and it is shared to current account by RAM
				env.Log.Infof("Found Tags for serviceNetwork %v tags: %v", *sn.Name, retrievedTags)

				value, ok := retrievedTags.Tags[model.K8SServiceNetworkOwnedByVPC]
				if ok {
					g.Expect(*value).To(Not(Equal(CurrentClusterVpcId)))
				}
			}
		}

		retrievedServices, _ := env.LatticeClient.ListServicesAsList(ctx, &vpclattice.ListServicesInput{})
		for _, service := range retrievedServices {
			env.Log.Infof("Found service, checking if created by current EKS Cluster: %v", service)
			retrievedTags, err := env.LatticeClient.ListTagsForResourceWithContext(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: service.Arn,
			})
			if err == nil { // for err != nil, it is possible that this service own by other account, and it is shared to current account by RAM
				env.Log.Infof("Found Tags for service %v tags: %v", *service.Name, retrievedTags)
				value, ok := retrievedTags.Tags[model.K8SServiceOwnedByVPC]
				if ok {
					g.Expect(*value).To(Not(Equal(CurrentClusterVpcId)))
				}
			}
		}

		retrievedTargetGroups, _ := env.LatticeClient.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{})
		for _, tg := range retrievedTargetGroups {
			env.Log.Infof("Found TargetGroup: %s, checking if created by current EKS Cluster", *tg.Id)
			if tg.VpcIdentifier != nil && CurrentClusterVpcId != *tg.VpcIdentifier {
				env.Log.Infof("Target group VPC Id: %s, does not match current EKS Cluster VPC Id: %s", *tg.VpcIdentifier, CurrentClusterVpcId)
				//This tg is not created by current EKS Cluster, skip it
				continue
			}
			retrievedTags, err := env.LatticeClient.ListTagsForResourceWithContext(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: tg.Arn,
			})
			if err == nil {
				env.Log.Infof("Found Tags for tg %v tags: %v", *tg.Name, retrievedTags)
				tagValue, ok := retrievedTags.Tags[model.K8SSourceTypeKey]
				if ok && *tagValue == string(model.SourceTypeSvcExport) {
					env.Log.Infof("TargetGroup: %s was created by k8s controller, by a ServiceExport", *tg.Id)
					//This tg is created by k8s controller, by a ServiceExport,
					//ServiceExport still have a known targetGroup leaking issue,
					//so we temporarily skip to verify whether ServiceExport created TargetGroup is deleted or not
					continue
				}
			}
		}
	}).Should(Succeed())
}

func objectsInfo(objs []client.Object) string {
	objInfos := utils.SliceMap(objs, func(obj client.Object) string {
		return fmt.Sprintf("%T/%s", obj, obj.GetName())
	})
	return strings.Join(objInfos, ", ")
}

func (env *Framework) ExpectCreated(ctx context.Context, objects ...client.Object) {
	env.Log.Infof("Creating objects: %s", objectsInfo(objects))
	parallel.ForEach(objects, func(obj client.Object, _ int) {
		Expect(env.Create(ctx, obj)).WithOffset(1).To(Succeed())
	})
}

func (env *Framework) ExpectUpdated(ctx context.Context, objects ...client.Object) {
	env.Log.Infof("Updating objects: %s", objectsInfo(objects))
	parallel.ForEach(objects, func(obj client.Object, _ int) {
		Expect(env.Update(ctx, obj)).WithOffset(1).To(Succeed())
	})
}

func (env *Framework) ExpectDeletedThenNotFound(ctx context.Context, objects ...client.Object) {
	env.ExpectDeleted(ctx, objects...)
	env.EventuallyExpectNotFound(ctx, objects...)
}

func (env *Framework) ExpectDeleted(ctx context.Context, objects ...client.Object) {
	httpRouteType := reflect.TypeOf(&gwv1beta1.HTTPRoute{})
	grpcRouteType := reflect.TypeOf(&gwv1alpha2.GRPCRoute{})

	routeObjects := []client.Object{}

	// first, find routes
	for _, object := range objects {
		t := reflect.TypeOf(object)
		if httpRouteType == t || grpcRouteType == t {
			routeObjects = append(routeObjects, object)
		}
	}

	if len(routeObjects) > 0 {
		env.Log.Infof("Found %d route objects", len(routeObjects))

		for _, route := range routeObjects {
			// for routes, we can speed up deletion by first removing their rules
			// get the latest version first tho
			t := reflect.TypeOf(route)
			nsName := types.NamespacedName{
				Name:      route.GetName(),
				Namespace: route.GetNamespace(),
			}

			if httpRouteType == t {
				http := &gwv1beta1.HTTPRoute{}
				err := env.Get(ctx, nsName, http)
				if err != nil {
					env.Log.Infof("Error getting http route %s", err)
					continue
				}

				env.Log.Infof("Clearing http route rules for %s", http.Name)
				http.Spec.Rules = make([]gwv1beta1.HTTPRouteRule, 0)
				err = env.Update(ctx, http)
				if err != nil {
					env.Log.Infof("Error clearing http route rules %s", err)
				}
			} else if grpcRouteType == t {
				grpc := &gwv1alpha2.GRPCRoute{}
				err := env.Get(ctx, nsName, grpc)
				if err != nil {
					env.Log.Infof("Error getting grpc route %s", err)
					continue
				}
				env.Log.Infof("Clearing grpc route rules for %s", grpc.Name)
				grpc.Spec.Rules = make([]gwv1alpha2.GRPCRouteRule, 0)
				err = env.Update(ctx, grpc)
				if err != nil {
					env.Log.Infof("Error clearing grpc route rules %s", err)
				}
			}
		}

		// sleep once for all routes
		env.SleepForRouteUpdate()
	}

	env.Log.Infof("Deleting objects: %s", objectsInfo(objects))
	parallel.ForEach(objects, func(obj client.Object, _ int) {
		err := env.Delete(ctx, obj)
		if err != nil {
			// not found is probably OK - means it was deleted elsewhere
			if !errors.IsNotFound(err) {
				Expect(err).ToNot(HaveOccurred())
			}
		}
	})
}

func (env *Framework) ExpectDeleteAllToSucceed(ctx context.Context, object client.Object, namespace string) {
	Expect(env.DeleteAllOf(ctx, object, client.InNamespace(namespace))).WithOffset(1).To(Succeed())
}

func (env *Framework) EventuallyExpectNotFound(ctx context.Context, objects ...client.Object) {
	env.Log.Infof("Waiting for NotFound, objects: %s", objectsInfo(objects))
	parallel.ForEach(objects, func(obj client.Object, _ int) {
		if obj != nil {
			Eventually(func(g Gomega) {
				g.Expect(errors.IsNotFound(env.Get(ctx, client.ObjectKeyFromObject(obj), obj))).To(BeTrue())
				// Wait for 7 minutes at maximum just in case the k8sService deletion triggered targets draining time
				// and httproute deletion need to wait for that targets draining time finish then it can return
			}).WithTimeout(7 * time.Minute).WithPolling(time.Second).WithOffset(1).Should(Succeed())
		}
	})
}

func (env *Framework) EventuallyExpectNoneFound(ctx context.Context, objectList client.ObjectList) {
	Eventually(func(g Gomega) {
		g.Expect(env.List(ctx, objectList, client.HasLabels([]string{DiscoveryLabel}))).To(Succeed())
		g.Expect(meta.ExtractList(objectList)).To(HaveLen(0), "Expected to not find any %q with label %q", reflect.TypeOf(objectList), DiscoveryLabel)
	}).WithOffset(1).Should(Succeed())
}

func (env *Framework) GetServiceNetwork(ctx context.Context, gateway *gwv1beta1.Gateway) *vpclattice.ServiceNetworkSummary {
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

func (env *Framework) GetVpcLatticeService(ctx context.Context, route core.Route) *vpclattice.ServiceSummary {
	var found *vpclattice.ServiceSummary
	latticeServiceName := utils.LatticeServiceName(route.Name(), route.Namespace())
	Eventually(func(g Gomega) {
		svc, err := env.LatticeClient.FindService(ctx, latticeServiceName)
		g.Expect(err).ToNot(HaveOccurred())
		found = svc
		g.Expect(found).ToNot(BeNil())
		g.Expect(found.Status).To(Equal(aws.String(vpclattice.ServiceStatusActive)))
		g.Expect(found.DnsEntry).To(ContainSubstring(latticeServiceName))
	}).WithOffset(1).Should(Succeed())

	return found
}

func (env *Framework) GetFullTargetGroupFromSummary(
	ctx context.Context,
	tgSummary *vpclattice.TargetGroupSummary) *vpclattice.GetTargetGroupOutput {

	tg, err := env.LatticeClient.GetTargetGroupWithContext(ctx, &vpclattice.GetTargetGroupInput{
		TargetGroupIdentifier: tgSummary.Arn,
	})

	if err != nil {
		panic(err)
	}

	return tg
}

func (env *Framework) GetTargetGroup(ctx context.Context, service *corev1.Service) *vpclattice.TargetGroupSummary {
	return env.GetTargetGroupWithProtocol(ctx, service, vpclattice.TargetGroupProtocolHttp, vpclattice.TargetGroupProtocolVersionHttp1)
}

func (env *Framework) GetTargetGroupWithProtocol(ctx context.Context, service *corev1.Service, protocol, protocolVersion string) *vpclattice.TargetGroupSummary {
	tgSpec := model.TargetGroupSpec{
		TargetGroupTagFields: model.TargetGroupTagFields{
			K8SServiceName:      service.Name,
			K8SServiceNamespace: service.Namespace,
		},
		Protocol:        strings.ToUpper(protocol),
		ProtocolVersion: strings.ToUpper(protocolVersion),
	}

	var found *vpclattice.TargetGroupSummary
	Eventually(func(g Gomega) {
		tg, err := env.FindTargetGroupFromSpec(ctx, tgSpec)
		if err != nil {
			gwlog.FallbackLogger.Infof("Error getting target group %s, %s due to %s",
				tgSpec.K8SServiceName, tgSpec.K8SServiceNamespace, err)
		}
		g.Expect(err).To(BeNil())
		g.Expect(tg).ToNot(BeNil())
		g.Expect(tg.Status).To(Equal(aws.String(vpclattice.TargetGroupStatusActive)))

		found = tg
	}).WithOffset(1).Should(Succeed())

	gwlog.FallbackLogger.Infof("Found target group %s, %s", *found.Name, *found.Id)
	return found
}

func (env *Framework) FindTargetGroupFromSpec(ctx context.Context, tgSpec model.TargetGroupSpec) (*vpclattice.TargetGroupSummary, error) {
	targetGroups, err := env.LatticeClient.ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{})
	if err != nil {
		return nil, err
	}

	for _, targetGroup := range targetGroups {
		if aws.StringValue(targetGroup.Protocol) != tgSpec.Protocol {
			continue
		}
		tg, err := env.LatticeClient.GetTargetGroupWithContext(ctx, &vpclattice.GetTargetGroupInput{TargetGroupIdentifier: targetGroup.Id})
		if err != nil {
			return nil, err
		}

		if aws.StringValue(tg.Config.ProtocolVersion) != tgSpec.ProtocolVersion {
			continue
		}

		res, err := env.LatticeClient.ListTagsForResourceWithContext(ctx,
			&vpclattice.ListTagsForResourceInput{ResourceArn: targetGroup.Arn})
		if err != nil {
			return nil, err
		}

		modelTags := model.TGTagFieldsFromTags(res.Tags)
		if modelTags.K8SServiceName != tgSpec.K8SServiceName || modelTags.K8SServiceNamespace != tgSpec.K8SServiceNamespace {
			continue
		}

		// we don't always specify these on the tgSpec, but use them if present
		// if they aren't present we will ignore. This isn't perfect logic but should be good enough for these tests
		if (tgSpec.K8SRouteName != "" && tgSpec.K8SRouteName != modelTags.K8SRouteName) ||
			(tgSpec.K8SRouteNamespace != "" && tgSpec.K8SRouteNamespace != modelTags.K8SRouteNamespace) {
			continue
		}

		// close enough :D
		return targetGroup, nil
	}
	return nil, nil
}

// TODO: Create a new function that only verifying deployment len(podList.Items)==*deployment.Spec.Replicas, and don't do lattice.ListTargets() api call
func (env *Framework) GetTargets(ctx context.Context, targetGroup *vpclattice.TargetGroupSummary, deployment *appsv1.Deployment) []*vpclattice.TargetSummary {
	var found []*vpclattice.TargetSummary
	Eventually(func(g Gomega) {
		podIps, retrievedTargets := GetTargets(targetGroup, deployment, env, ctx)

		targetIps := lo.Filter(retrievedTargets, func(target *vpclattice.TargetSummary, _ int) bool {
			return lo.Contains(podIps, *target.Id) &&
				(*target.Status == vpclattice.TargetStatusInitial ||
					*target.Status == vpclattice.TargetStatusHealthy)
		})

		g.Expect(targetIps).Should(HaveLen(len(podIps)))
		found = retrievedTargets
	}).WithPolling(15 * time.Second).WithTimeout(7 * time.Minute).Should(Succeed())
	return found
}

func (env *Framework) GetAllTargets(ctx context.Context, targetGroup *vpclattice.TargetGroupSummary, deployment *appsv1.Deployment) ([]string, []*vpclattice.TargetSummary) {
	return GetTargets(targetGroup, deployment, env, ctx)
}

func GetTargets(targetGroup *vpclattice.TargetGroupSummary, deployment *appsv1.Deployment, env *Framework, ctx context.Context) ([]string, []*vpclattice.TargetSummary) {
	env.Log.Infoln("Trying to retrieve registered targets for targetGroup", *targetGroup.Name)
	env.Log.Infoln("deployment.Spec.Selector.MatchLabels:", deployment.Spec.Selector.MatchLabels)
	podList := &corev1.PodList{}
	expectedMatchingLabels := make(map[string]string, len(deployment.Spec.Selector.MatchLabels))
	for k, v := range deployment.Spec.Selector.MatchLabels {
		expectedMatchingLabels[k] = v
	}
	expectedMatchingLabels[DiscoveryLabel] = "true"
	env.Log.Infoln("Expected matching labels:", expectedMatchingLabels)
	Expect(env.List(ctx, podList, client.MatchingLabels(expectedMatchingLabels))).To(Succeed())
	Expect(podList.Items).To(HaveLen(int(*deployment.Spec.Replicas)))
	retrievedTargets, err := env.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{TargetGroupIdentifier: targetGroup.Id})
	Expect(err).To(BeNil())

	podIps := utils.SliceMap(podList.Items, func(pod corev1.Pod) string { return pod.Status.PodIP })

	return podIps, retrievedTargets
}

func (env *Framework) VerifyTargetGroupNotFound(tg *vpclattice.TargetGroupSummary) {
	Eventually(func(g Gomega) {
		retrievedTargetGroup, err := env.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{
			TargetGroupIdentifier: tg.Id,
		})
		g.Expect(retrievedTargetGroup.Id).To(BeNil())
		g.Expect(err).To(Not(BeNil()))
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				g.Expect(aerr.Code()).To(Equal(vpclattice.ErrCodeResourceNotFoundException))
			}
		}
	}).Should(Succeed())
}

func (env *Framework) IsVpcAssociatedWithServiceNetwork(ctx context.Context, vpcId string, serviceNetwork *vpclattice.ServiceNetworkSummary) (bool, string, error) {
	vpcAssociations, err := env.LatticeClient.ListServiceNetworkVpcAssociationsAsList(ctx, &vpclattice.ListServiceNetworkVpcAssociationsInput{
		ServiceNetworkIdentifier: serviceNetwork.Id,
		VpcIdentifier:            &vpcId,
	})
	if err != nil {
		return false, "", err
	}
	if len(vpcAssociations) != 1 {
		return false, "", fmt.Errorf("Expect to have one VpcServiceNetworkAssociation len(vpcAssociations): %d", len(vpcAssociations))
	}
	association := vpcAssociations[0]
	if *association.Status != vpclattice.ServiceNetworkVpcAssociationStatusActive {
		return false, "", fmt.Errorf("Current cluster should have one Active status association *association.Status: %s, err: %w", *association.Status, err)
	}
	return true, *association.Id, nil
}

func (env *Framework) AreAllLatticeTargetsHealthy(ctx context.Context, tg *vpclattice.TargetGroupSummary) (bool, error) {
	env.Log.Infof("Checking whether AreAllLatticeTargetsHealthy for targetGroup: %v", tg)
	targets, err := env.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{TargetGroupIdentifier: tg.Id})
	if err != nil {
		return false, err
	}
	for _, target := range targets {
		env.Log.Infof("Checking target: %v", target)
		if *target.Status != vpclattice.TargetStatusHealthy {
			return false, nil
		}
	}
	return true, nil
}

func (env *Framework) GetLatticeServiceHttpsListenerNonDefaultRules(ctx context.Context, vpcLatticeService *vpclattice.ServiceSummary) ([]*vpclattice.GetRuleOutput, error) {

	listListenerResp, err := env.LatticeClient.ListListenersWithContext(ctx, &vpclattice.ListListenersInput{
		ServiceIdentifier: vpcLatticeService.Id,
	})
	if err != nil {
		return nil, err
	}

	httpsListenerId := ""
	for _, item := range listListenerResp.Items {
		if strings.Contains(*item.Name, "https") {
			httpsListenerId = *item.Id
			break
		}
	}
	if httpsListenerId == "" {
		return nil, fmt.Errorf("expect having 1 https listener for lattice service %s, but got 0", *vpcLatticeService.Id)
	}
	listRulesResp, err := env.LatticeClient.ListRulesWithContext(ctx, &vpclattice.ListRulesInput{
		ListenerIdentifier: &httpsListenerId,
		ServiceIdentifier:  vpcLatticeService.Id,
	})
	if err != nil {
		return nil, err
	}
	nonDefaultRules := utils.SliceFilter(listRulesResp.Items, func(rule *vpclattice.RuleSummary) bool {
		return rule.IsDefault != nil && *rule.IsDefault == false
	})

	nonDefaultRuleIds := utils.SliceMap(nonDefaultRules, func(rule *vpclattice.RuleSummary) *string {
		return rule.Id
	})

	var retrievedRules []*vpclattice.GetRuleOutput
	for _, ruleId := range nonDefaultRuleIds {
		rule, err := env.LatticeClient.GetRuleWithContext(ctx, &vpclattice.GetRuleInput{
			ServiceIdentifier:  vpcLatticeService.Id,
			ListenerIdentifier: &httpsListenerId,
			RuleIdentifier:     ruleId,
		})
		if err != nil {
			return nil, err
		}
		retrievedRules = append(retrievedRules, rule)
	}
	return retrievedRules, nil
}

func (env *Framework) GetVpcLatticeServiceDns(httpRouteName string, httpRouteNamespace string) string {
	env.Log.Infoln("GetVpcLatticeServiceDns: ", httpRouteName, httpRouteNamespace)
	httproute := gwv1beta1.HTTPRoute{}
	env.Get(env.ctx, types.NamespacedName{Name: httpRouteName, Namespace: httpRouteNamespace}, &httproute)
	vpcLatticeServiceDns := httproute.Annotations[controllers.LatticeAssignedDomainName]
	return vpcLatticeServiceDns
}

type RunGrpcurlCmdOptions struct {
	GrpcServerHostName  string
	GrpcServerPort      string
	Service             string
	Method              string
	Headers             [][2]string // a slice of string tuple
	ReqParamsJsonString string
	UseTLS              bool
}

// https://github.com/fullstorydev/grpcurl
// https://gallery.ecr.aws/a0j4q9e4/grpcurl-runner
func (env *Framework) RunGrpcurlCmd(opts RunGrpcurlCmdOptions) (string, string, error) {
	env.Log.Infoln("RunGrpcurlCmd")
	Expect(env.GrpcurlRunner).To(Not(BeNil()))

	tlsOption := ""
	if !opts.UseTLS {
		tlsOption = "-plaintext"
	}

	headers := ""
	for _, tuple := range opts.Headers {
		headers += fmt.Sprintf("-H '%s: %s' ", tuple[0], tuple[1])
	}

	reqParams := ""
	if opts.ReqParamsJsonString != "" {
		reqParams = fmt.Sprintf("-d '%s'", opts.ReqParamsJsonString)
	}

	cmd := fmt.Sprintf("/grpcurl "+
		"-proto /protos/addsvc.proto "+
		"-proto /protos/grpcbin.proto "+
		"-proto /protos/helloworld.proto "+
		"%s %s %s %s:%s %s/%s",
		tlsOption,
		headers,
		reqParams,
		opts.GrpcServerHostName,
		opts.GrpcServerPort,
		opts.Service,
		opts.Method)

	return env.PodExec(*env.GrpcurlRunner, cmd)
}

func (env *Framework) SleepForRouteUpdate() {
	time.Sleep(10 * time.Second)
}
