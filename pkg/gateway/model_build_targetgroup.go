package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

const (
	resourceIDTargetGroup = "TargetGroup"
)

type TargetGroupModelBuilder interface {
	Build(ctx context.Context, srvExport *mcs_api.ServiceExport) (core.Stack, *latticemodel.TargetGroup, error)
}

type TargetGroupBuilder struct {
	log           gwlog.Logger
	client        client.Client
	serviceExport *mcs_api.ServiceExport
	datastore     *latticestore.LatticeDataStore
	cloud         lattice_aws.Cloud
	defaultTags   map[string]string
}

// triggered from serviceexport
func NewTargetGroupBuilder(
	log gwlog.Logger,
	client client.Client,
	datastore *latticestore.LatticeDataStore,
	cloud lattice_aws.Cloud,
) *TargetGroupBuilder {
	return &TargetGroupBuilder{
		log:       log,
		client:    client,
		datastore: datastore,
		cloud:     cloud,
	}
}

type targetGroupModelBuildTask struct {
	log           gwlog.Logger
	client        client.Client
	serviceExport *mcs_api.ServiceExport
	targetGroup   *latticemodel.TargetGroup
	tgByResID     map[string]*latticemodel.TargetGroup
	stack         core.Stack
	datastore     *latticestore.LatticeDataStore
	cloud         lattice_aws.Cloud
}

// for serviceexport
func (b *TargetGroupBuilder) Build(
	ctx context.Context,
	srvExport *mcs_api.ServiceExport,
) (core.Stack, *latticemodel.TargetGroup, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(srvExport)))

	task := &targetGroupModelBuildTask{
		log:           b.log,
		serviceExport: srvExport,
		stack:         stack,
		tgByResID:     make(map[string]*latticemodel.TargetGroup),
		datastore:     b.datastore,
		cloud:         b.cloud,
		client:        b.client,
	}

	if err := task.run(ctx); err != nil {
		return task.stack, task.targetGroup, err
	}

	return task.stack, task.targetGroup, nil
}

// for serviceexport
func (t *targetGroupModelBuildTask) run(ctx context.Context) error {
	/*
		if !t.serviceExport.DeletionTimestamp.IsZero() {
			// TODO handle delete
			return nil
		}
	*/

	err := t.buildModel(ctx)

	return err
}

// for serviceexport
func (t *targetGroupModelBuildTask) buildModel(ctx context.Context) error {
	err := t.BuildTargetGroup(ctx)

	if err != nil {
		return fmt.Errorf("failed to build TargetGroup when serviceExport buildModel for name %v namespace %v, %w",
			t.serviceExport.Name, t.serviceExport.Namespace, err)
	}

	err = t.BuildTargets(ctx)

	if err != nil {
		t.log.Infof("Failed to build Targets when serviceExport buildModel for name %v namespace %v",
			t.serviceExport.Name, t.serviceExport.Namespace)
	}

	return nil
}

// triggered from service exports/targetgroups
func (t *targetGroupModelBuildTask) BuildTargets(ctx context.Context) error {
	targetTask := &latticeTargetsModelBuildTask{
		client:      t.client,
		tgName:      t.serviceExport.Name,
		tgNamespace: t.serviceExport.Namespace,
		stack:       t.stack,
		datastore:   t.datastore,
	}

	err := targetTask.buildLatticeTargets(ctx)

	if err != nil {
		t.log.Infof("Error buildTargets for serviceExport name %v, namespace %v",
			t.serviceExport.Name, t.serviceExport.Namespace)
		return err
	}
	return nil
}

// Triggered from route/service/targetgroup
func (t *latticeServiceModelBuildTask) buildTargets(ctx context.Context) error {
	for _, rule := range t.route.Spec().Rules() {
		for _, backendRef := range rule.BackendRefs() {
			if string(*backendRef.Kind()) == "ServiceImport" {
				t.log.Infof("latticeServiceModelBuildTask: ignore service: %v", backendRef)
				continue
			}

			backendNamespace := t.route.Namespace()
			if backendRef.Namespace() != nil {
				backendNamespace = string(*backendRef.Namespace())
			}

			var port int32
			if backendRef.Port() != nil {
				port = int32(*backendRef.Port())
			}

			targetTask := &latticeTargetsModelBuildTask{
				client:         t.client,
				tgName:         string(backendRef.Name()),
				tgNamespace:    backendNamespace,
				routeName:      t.route.Name(),
				backendRefPort: port,
				stack:          t.stack,
				datastore:      t.datastore,
				route:          t.route,
			}

			err := targetTask.buildLatticeTargets(ctx)

			if err != nil {
				t.log.Infof("Error buildTargets for backend ref service %v", backendRef)
				return err
			}
		}
	}
	return nil
}

// TODO have a same BuildTargetGroup for both targetGroupModelBuildTask, latticeServiceModelBuildTask
// Build target group for K8S serviceexport object
func (t *targetGroupModelBuildTask) BuildTargetGroup(ctx context.Context) error {
	tgName := latticestore.TargetGroupName(t.serviceExport.Name, t.serviceExport.Namespace)

	svc := &corev1.Service{}
	if err := t.client.Get(ctx, k8s.NamespacedName(t.serviceExport), svc); err != nil {
		// mark there is no serviceexport dependence on the TG
		t.datastore.SetTargetGroupByServiceExport(tgName, false, false)
		return fmt.Errorf("error finding corresponding service %v error :%w", k8s.NamespacedName(t.serviceExport), err)
	}

	ipAddressType, err := buildTargetGroupIpAdressType(svc)

	if err != nil {
		return err
	}

	tgp, err := getAttachedTargetGroupPolicy(ctx, t.client, t.serviceExport.Name, t.serviceExport.Namespace)
	if err != nil {
		return err
	}
	protocol := "HTTP"
	protocolVersion := vpclattice.TargetGroupProtocolVersionHttp1
	var healthCheckConfig *vpclattice.HealthCheckConfig
	if tgp != nil {
		if tgp.Spec.Protocol != nil {
			protocol = *tgp.Spec.Protocol
		}
		if tgp.Spec.ProtocolVersion != nil {
			protocolVersion = *tgp.Spec.ProtocolVersion
		}
		healthCheckConfig = parseHealthCheckConfig(tgp)
	}

	//if t.serviceExport.
	tgSpec := latticemodel.TargetGroupSpec{
		Name: tgName,
		Type: latticemodel.TargetGroupTypeIP,
		Config: latticemodel.TargetGroupConfig{
			VpcID: config.VpcID,
			// Fill in default HTTP port as we are using target port anyway.
			Port:                80,
			IsServiceImport:     false,
			IsServiceExport:     true,
			K8SServiceName:      t.serviceExport.Name,
			K8SServiceNamespace: t.serviceExport.Namespace,
			Protocol:            protocol,
			ProtocolVersion:     protocolVersion,
			HealthCheckConfig:   healthCheckConfig,
			IpAddressType:       ipAddressType,
		},
	}

	tg := latticemodel.NewTargetGroup(t.stack, tgName, tgSpec)
	tg.Spec.K8sServiceExists = true
	t.log.Infof("buildTargetGroup, tg[%s], tgSpec %v", tgName, tg)

	// add targetgroup to localcache for service reconcile to reference
	// for serviceexport, the httproutename is set to ""
	t.datastore.AddTargetGroup(tgName, "", "", "", tgSpec.Config.IsServiceImport, "")

	if !t.serviceExport.DeletionTimestamp.IsZero() {
		// triggered by serviceexport delete
		t.datastore.SetTargetGroupByServiceExport(tgName, false, false)
	} else {
		// triggered by serviceexport add
		t.datastore.SetTargetGroupByServiceExport(tgName, false, true)
	}

	// for serviceexport, the routeName is null
	dsTG, err := t.datastore.GetTargetGroup(tgName, "", false)

	t.log.Infof("TargetGroup cached in datastore: %v", dsTG)
	if (err != nil) || (!dsTG.ByBackendRef && !dsTG.ByServiceExport) {
		t.log.Infof("BuildingTargetGroup: TG %v is NOT used anymore and can be delted", tgSpec)
		tg.Spec.IsDeleted = true
		tg.Spec.LatticeID = dsTG.ID
	}

	t.tgByResID[tgName] = tg
	t.targetGroup = tg
	return nil
}

// Build target group for backend service ref used in Route
func (t *latticeServiceModelBuildTask) buildTargetGroup(
	ctx context.Context,
	client client.Client,
) (*latticemodel.TargetGroup, error) {
	for _, rule := range t.route.Spec().Rules() {
		t.log.Infof("buildTargetGroup: %v", rule)

		for _, backendRef := range rule.BackendRefs() {
			t.log.Infof("buildTargetGroup -- backendRef %v", backendRef)

			tgName := t.buildTargetGroupName(ctx, backendRef)

			tgSpec, err := t.buildTargetGroupSpec(ctx, client, backendRef)
			if err != nil {
				return nil, fmt.Errorf("buildTargetGroupSpec err %w", err)
			}

			// add targetgroup to localcache for service reconcile to reference
			if *backendRef.Kind() == "Service" {
				t.datastore.AddTargetGroup(tgName, "", "", "", tgSpec.Config.IsServiceImport, t.route.Name())
			} else {
				// for serviceimport, the httproutename is ""
				t.datastore.AddTargetGroup(tgName, "", "", "", tgSpec.Config.IsServiceImport, "")
			}

			if t.route.DeletionTimestamp().IsZero() {
				// to add
				t.datastore.SetTargetGroupByBackendRef(tgName, t.route.Name(), tgSpec.Config.IsServiceImport, true)
			} else {
				// to delete
				t.datastore.SetTargetGroupByBackendRef(tgName, t.route.Name(), tgSpec.Config.IsServiceImport, false)
				dsTG, _ := t.datastore.GetTargetGroup(tgName, t.route.Name(), tgSpec.Config.IsServiceImport)
				tgSpec.IsDeleted = true
				tgSpec.LatticeID = dsTG.ID
			}

			tg := latticemodel.NewTargetGroup(t.stack, tgName, tgSpec)
			t.log.Infof("buildTargetGroup, tg[%s], tgSpec%v \n", tgName, tg)
			t.tgByResID[tgName] = tg
		}
	}
	return nil, nil

}

// Now, Only k8sService and serviceImport creation deletion use this function to build TargetGroupSpec, serviceExport does not use this function to create TargetGroupSpec
func (t *latticeServiceModelBuildTask) buildTargetGroupSpec(
	ctx context.Context,
	client client.Client,
	backendRef core.BackendRef,
) (latticemodel.TargetGroupSpec, error) {
	var namespace string

	if backendRef.Namespace() != nil {
		namespace = string(*backendRef.Namespace())
	} else {
		namespace = t.route.Namespace()
	}

	backendKind := string(*backendRef.Kind())
	t.log.Infof("buildTargetGroupSpec, kind %s", backendKind)

	var vpc = config.VpcID
	var eksCluster = ""
	var isServiceImport bool
	var isDeleted bool

	if t.route.DeletionTimestamp().IsZero() {
		isDeleted = false
	} else {
		isDeleted = true
	}
	k8sServiceExists := false
	ipAddressType := vpclattice.IpAddressTypeIpv4

	if backendKind == "ServiceImport" {
		isServiceImport = true
		namespaceName := types.NamespacedName{
			Namespace: namespace,
			Name:      string(backendRef.Name()),
		}
		serviceImport := &mcs_api.ServiceImport{}

		if err := client.Get(context.TODO(), namespaceName, serviceImport); err == nil {
			t.log.Infof("buildTargetGroupSpec, using service Import %v", namespaceName)
			vpc = serviceImport.Annotations["multicluster.x-k8s.io/aws-vpc"]
			eksCluster = serviceImport.Annotations["multicluster.x-k8s.io/aws-eks-cluster-name"]

		} else {
			t.log.Infof("buildTargetGroupSpec, using service Import %v, err :%v", namespaceName, err)
			if !isDeleted {
				//Return error for creation request only.
				//For ServiceImport deletion request, we should go ahead to build TargetGroupSpec model,
				//although the targetGroupSynthesizer could skip TargetGroup deletion triggered by ServiceImport deletion
				return latticemodel.TargetGroupSpec{}, err
			}
		}

	} else {
		var namespace = t.route.Namespace()

		if backendRef.Namespace() != nil {
			namespace = string(*backendRef.Namespace())
		}
		// find out service target port
		serviceNamespaceName := types.NamespacedName{
			Namespace: namespace,
			Name:      string(backendRef.Name()),
		}

		svc := &corev1.Service{}
		if err := t.client.Get(ctx, serviceNamespaceName, svc); err == nil {
			ipAddressType, _ = buildTargetGroupIpAdressType(svc)
			k8sServiceExists = true
		} else {
			t.log.Infof("Error finding backend service %v error :%v", serviceNamespaceName, err)
			k8sServiceExists = false
		}
	}

	tgName := latticestore.TargetGroupName(string(backendRef.Name()), namespace)

	tgp, err := getAttachedTargetGroupPolicy(ctx, client, string(backendRef.Name()), namespace)
	if err != nil {
		return latticemodel.TargetGroupSpec{}, err
	}
	protocol := "HTTP"
	protocolVersion := vpclattice.TargetGroupProtocolVersionHttp1
	var healthCheckConfig *vpclattice.HealthCheckConfig
	if tgp != nil {
		if tgp.Spec.Protocol != nil {
			protocol = *tgp.Spec.Protocol
		}

		if tgp.Spec.ProtocolVersion != nil {
			protocolVersion = *tgp.Spec.ProtocolVersion
		}
		healthCheckConfig = parseHealthCheckConfig(tgp)
	}

	// GRPC takes precedence over other protocolVersions.
	if _, ok := t.route.(*core.GRPCRoute); ok {
		protocolVersion = vpclattice.TargetGroupProtocolVersionGrpc
	}

	return latticemodel.TargetGroupSpec{
		Name: tgName,
		Type: latticemodel.TargetGroupTypeIP,
		Config: latticemodel.TargetGroupConfig{
			VpcID:                 vpc,
			EKSClusterName:        eksCluster,
			IsServiceImport:       isServiceImport,
			IsServiceExport:       false,
			K8SServiceName:        string(backendRef.Name()),
			K8SServiceNamespace:   namespace,
			K8SHTTPRouteName:      t.route.Name(),
			K8SHTTPRouteNamespace: t.route.Namespace(),
			Protocol:              protocol,
			ProtocolVersion:       protocolVersion,
			HealthCheckConfig:     healthCheckConfig,
			// Fill in default HTTP port as we are using target port anyway.
			Port:          80,
			IpAddressType: ipAddressType,
		},
		K8sServiceExists: k8sServiceExists,
		IsDeleted:        isDeleted,
	}, nil
}

func (t *latticeServiceModelBuildTask) buildTargetGroupName(_ context.Context, backendRef core.BackendRef) string {
	if backendRef.Namespace() != nil {
		return latticestore.TargetGroupName(string(backendRef.Name()), string(*backendRef.Namespace()))
	} else {
		return latticestore.TargetGroupName(string(backendRef.Name()), t.route.Namespace())
	}
}

func getAttachedTargetGroupPolicy(ctx context.Context, k8sClient client.Client, svcName, svcNamespace string) (*v1alpha1.TargetGroupPolicy, error) {
	policyList := &v1alpha1.TargetGroupPolicyList{}
	err := k8sClient.List(ctx, policyList, &client.ListOptions{
		Namespace: svcNamespace,
	})
	if err != nil {
		if meta.IsNoMatchError(err) {
			// CRD does not exist
			return nil, nil
		}
		return nil, err
	}
	for _, policy := range policyList.Items {
		targetRef := policy.Spec.TargetRef
		if targetRef == nil {
			continue
		}

		groupKindMatch := targetRef.Group == "" && targetRef.Kind == "Service"
		nameMatch := string(targetRef.Name) == svcName

		namespace := policy.Namespace
		if targetRef.Namespace != nil {
			namespace = string(*targetRef.Namespace)
		}
		namespaceMatch := namespace == svcNamespace

		if groupKindMatch && nameMatch && namespaceMatch {
			return &policy, nil
		}
	}
	return nil, nil
}

func parseHealthCheckConfig(tgp *v1alpha1.TargetGroupPolicy) *vpclattice.HealthCheckConfig {
	hc := tgp.Spec.HealthCheck
	if hc == nil {
		return nil
	}
	var matcher *vpclattice.Matcher
	if hc.StatusMatch != nil {
		matcher = &vpclattice.Matcher{HttpCode: hc.StatusMatch}
	}
	return &vpclattice.HealthCheckConfig{
		Enabled:                    hc.Enabled,
		HealthCheckIntervalSeconds: hc.IntervalSeconds,
		HealthCheckTimeoutSeconds:  hc.TimeoutSeconds,
		HealthyThresholdCount:      hc.HealthyThresholdCount,
		UnhealthyThresholdCount:    hc.UnhealthyThresholdCount,
		Matcher:                    matcher,
		Path:                       hc.Path,
		Port:                       hc.Port,
		Protocol:                   (*string)(hc.Protocol),
		ProtocolVersion:            (*string)(hc.ProtocolVersion),
	}
}

func buildTargetGroupIpAdressType(svc *corev1.Service) (string, error) {
	ipFamilies := svc.Spec.IPFamilies

	glog.V(6).Infof("buildTargetGroupIpAddressType ipFamilies: %v\n", ipFamilies)

	if len(ipFamilies) != 1 {
		return "", errors.New("Lattice Target Group only support single stack ip addresses")
	}

	// IpFamilies will always have at least 1 element
	ipFamily := ipFamilies[0]

	switch ipFamily {
	case corev1.IPv4Protocol:
		return vpclattice.IpAddressTypeIpv4, nil
	case corev1.IPv6Protocol:
		return vpclattice.IpAddressTypeIpv6, nil
	default:
		return "", fmt.Errorf("unknown ipFamily: %v", ipFamily)
	}
}
