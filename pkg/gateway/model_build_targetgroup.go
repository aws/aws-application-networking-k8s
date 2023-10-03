package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type SvcExportTargetGroupModelBuilder interface {
	Build(ctx context.Context, srvExport *mcsv1alpha1.ServiceExport) (core.Stack, *model.TargetGroup, error)
}

type SvcExportTargetGroupBuilder struct {
	log           gwlog.Logger
	client        client.Client
	serviceExport *mcsv1alpha1.ServiceExport
	datastore     *latticestore.LatticeDataStore
	cloud         pkg_aws.Cloud
	defaultTags   map[string]string
}

func NewSvcExportTargetGroupBuilder(
	log gwlog.Logger,
	client client.Client,
	datastore *latticestore.LatticeDataStore,
	cloud pkg_aws.Cloud,
) *SvcExportTargetGroupBuilder {
	return &SvcExportTargetGroupBuilder{
		log:       log,
		client:    client,
		datastore: datastore,
		cloud:     cloud,
	}
}

type svcExportTargetGroupModelBuildTask struct {
	log           gwlog.Logger
	client        client.Client
	serviceExport *mcsv1alpha1.ServiceExport
	targetGroup   *model.TargetGroup
	tgByResID     map[string]*model.TargetGroup
	stack         core.Stack
	datastore     *latticestore.LatticeDataStore
	cloud         pkg_aws.Cloud
}

func (b *SvcExportTargetGroupBuilder) Build(
	ctx context.Context,
	srvExport *mcsv1alpha1.ServiceExport,
) (core.Stack, *model.TargetGroup, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(srvExport)))

	task := &svcExportTargetGroupModelBuildTask{
		log:           b.log,
		serviceExport: srvExport,
		stack:         stack,
		tgByResID:     make(map[string]*model.TargetGroup),
		datastore:     b.datastore,
		cloud:         b.cloud,
		client:        b.client,
	}

	if err := task.run(ctx); err != nil {
		return task.stack, task.targetGroup, err
	}

	return task.stack, task.targetGroup, nil
}

func (t *svcExportTargetGroupModelBuildTask) run(ctx context.Context) error {
	err := t.BuildTargetGroupForServiceExport(ctx)
	if err != nil {
		return fmt.Errorf("failed to build target group for service export %s-%s due to %w",
			t.serviceExport.Name, t.serviceExport.Namespace, err)
	}

	err = t.BuildTargets(ctx)
	if err != nil {
		t.log.Debugf("Failed to build targets for service export %s-%s due to %s",
			t.serviceExport.Name, t.serviceExport.Namespace, err)
	}

	return nil
}

func (t *svcExportTargetGroupModelBuildTask) BuildTargets(ctx context.Context) error {
	targetTask := &latticeTargetsModelBuildTask{
		log:         t.log,
		client:      t.client,
		tgName:      t.serviceExport.Name,
		tgNamespace: t.serviceExport.Namespace,
		stack:       t.stack,
		datastore:   t.datastore,
	}

	err := targetTask.buildLatticeTargets(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (t *svcExportTargetGroupModelBuildTask) BuildTargetGroupForServiceExport(ctx context.Context) error {
	tgName := latticestore.TargetGroupName(t.serviceExport.Name, t.serviceExport.Namespace)
	var tg *model.TargetGroup
	var err error
	if t.serviceExport.DeletionTimestamp.IsZero() {
		tg, err = t.buildTargetGroupForServiceExportCreation(ctx, tgName)
	} else {
		tg, err = t.buildTargetGroupForServiceExportDeletion(ctx, tgName)
	}

	if err != nil {
		return err
	}

	t.tgByResID[tgName] = tg
	t.targetGroup = tg
	return nil
}

func (t *svcExportTargetGroupModelBuildTask) buildTargetGroupForServiceExportCreation(ctx context.Context, targetGroupName string) (*model.TargetGroup, error) {
	svc := &corev1.Service{}
	if err := t.client.Get(ctx, k8s.NamespacedName(t.serviceExport), svc); err != nil {
		t.datastore.SetTargetGroupByServiceExport(targetGroupName, false, false)
		return nil, fmt.Errorf("Failed to find corresponding k8sService %s, error :%w ", k8s.NamespacedName(t.serviceExport), err)
	}

	ipAddressType, err := buildTargetGroupIpAdressType(svc)
	if err != nil {
		return nil, err
	}

	tgp, err := GetAttachedPolicy(ctx, t.client, k8s.NamespacedName(t.serviceExport), &anv1alpha1.TargetGroupPolicy{})
	if err != nil {
		return nil, err
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

	stackTG := model.NewTargetGroup(t.stack, targetGroupName, model.TargetGroupSpec{
		Name: targetGroupName,
		Type: model.TargetGroupTypeIP,
		Config: model.TargetGroupConfig{
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
	})

	t.log.Debugw("stackTG:",
		"targetGroupName", stackTG.Spec.Name,
		"K8SServiceName", stackTG.Spec.Config.K8SServiceName,
		"K8SServiceNamespace", stackTG.Spec.Config.K8SServiceNamespace,
		"Protocol", stackTG.Spec.Config.Protocol,
		"ProtocolVersion", stackTG.Spec.Config.ProtocolVersion,
		"IpAddressType", stackTG.Spec.Config.IpAddressType,
		"HealthCheckConfig", stackTG.Spec.Config.HealthCheckConfig,
	)

	t.datastore.AddTargetGroup(targetGroupName, "", "", "", false, "")
	t.datastore.SetTargetGroupByServiceExport(targetGroupName, false, true)
	return stackTG, nil
}

func (t *svcExportTargetGroupModelBuildTask) buildTargetGroupForServiceExportDeletion(ctx context.Context, targetGroupName string) (*model.TargetGroup, error) {
	stackTG := model.NewTargetGroup(t.stack, targetGroupName, model.TargetGroupSpec{
		Name:      targetGroupName,
		LatticeID: "",
		IsDeleted: true,
	})
	t.datastore.SetTargetGroupByServiceExport(targetGroupName, false, false)
	dsTG, err := t.datastore.GetTargetGroup(targetGroupName, "", false)
	if err != nil {
		return nil, fmt.Errorf("%w: targetGroupName: %s", err, targetGroupName)
	}

	if !dsTG.ByBackendRef {
		// When handling the serviceExport deletion request while having dsTG.ByBackendRef==false,
		// That means this target group is not in use anymore, i.e., it is not referenced by latticeService rules(aka http/grpc route rules),
		// so, it can be deleted. Assign the stackTG.Spec.LatticeID to make target group manager can delete it
		t.log.Debugf("Target group %s is not in use anymore and can be deleted", stackTG.Spec.Name)
		stackTG.Spec.LatticeID = dsTG.ID
	}

	return stackTG, nil
}

// Build target group for backend service ref used in Route
func (t *latticeServiceModelBuildTask) buildTargetGroupsForRoute(
	ctx context.Context,
	client client.Client,
) error {
	for _, rule := range t.route.Spec().Rules() {
		for _, backendRef := range rule.BackendRefs() {
			tgName := t.buildTargetGroupName(ctx, backendRef)
			tgSpec, err := t.buildTargetGroupSpec(ctx, client, backendRef)
			if err != nil {
				return fmt.Errorf("buildTargetGroupSpec err %w", err)
			}

			// add targetgroup to localcache for service reconcile to reference
			if *backendRef.Kind() == "Service" {
				t.datastore.AddTargetGroup(tgName, "", "", "", tgSpec.Config.IsServiceImport, t.route.Name())
			} else {
				// for serviceimport, the httproutename is ""
				t.datastore.AddTargetGroup(tgName, "", "", "", tgSpec.Config.IsServiceImport, "")
			}

			if t.route.DeletionTimestamp().IsZero() {
				t.datastore.SetTargetGroupByBackendRef(tgName, t.route.Name(), tgSpec.Config.IsServiceImport, true)
			} else {
				t.datastore.SetTargetGroupByBackendRef(tgName, t.route.Name(), tgSpec.Config.IsServiceImport, false)
				dsTG, _ := t.datastore.GetTargetGroup(tgName, t.route.Name(), tgSpec.Config.IsServiceImport)
				tgSpec.IsDeleted = true
				tgSpec.LatticeID = dsTG.ID
			}

			tg := model.NewTargetGroup(t.stack, tgName, tgSpec)
			t.tgByResID[tgName] = tg
		}
	}
	return nil
}

// Triggered from route/service/targetgroup
func (t *latticeServiceModelBuildTask) buildTargetsForRoute(ctx context.Context) error {
	for _, rule := range t.route.Spec().Rules() {
		for _, backendRef := range rule.BackendRefs() {
			if string(*backendRef.Kind()) == "ServiceImport" {
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
				log:            t.log,
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
				return err
			}
		}
	}
	return nil
}

// Now, Only k8sService and serviceImport creation deletion use this function to build TargetGroupSpec, serviceExport does not use this function to create TargetGroupSpec
func (t *latticeServiceModelBuildTask) buildTargetGroupSpec(
	ctx context.Context,
	client client.Client,
	backendRef core.BackendRef,
) (model.TargetGroupSpec, error) {
	var namespace string

	if backendRef.Namespace() != nil {
		namespace = string(*backendRef.Namespace())
	} else {
		namespace = t.route.Namespace()
	}

	backendKind := string(*backendRef.Kind())
	t.log.Debugf("buildTargetGroupSpec, kind %s", backendKind)

	var vpc = config.VpcID
	var eksCluster = ""
	var isServiceImport bool
	var isDeleted bool

	if t.route.DeletionTimestamp().IsZero() {
		isDeleted = false
	} else {
		isDeleted = true
	}

	ipAddressType := vpclattice.IpAddressTypeIpv4

	if backendKind == "ServiceImport" {
		isServiceImport = true
		namespaceName := types.NamespacedName{
			Namespace: namespace,
			Name:      string(backendRef.Name()),
		}
		serviceImport := &mcsv1alpha1.ServiceImport{}

		if err := client.Get(context.TODO(), namespaceName, serviceImport); err == nil {
			t.log.Debugf("Building target group spec using service import %s", namespaceName)
			vpc = serviceImport.Annotations["multicluster.x-k8s.io/aws-vpc"]
			eksCluster = serviceImport.Annotations["multicluster.x-k8s.io/aws-eks-cluster-name"]
		} else {
			t.log.Errorf("Error building target group spec using service import %s due to %s", namespaceName, err)
			if !isDeleted {
				//Return error for creation request only.
				//For ServiceImport deletion request, we should go ahead to build TargetGroupSpec model,
				//although the targetGroupSynthesizer could skip TargetGroup deletion triggered by ServiceImport deletion
				return model.TargetGroupSpec{}, err
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
		if err := t.client.Get(ctx, serviceNamespaceName, svc); err != nil {
			t.log.Infof("Error finding backend service %s due to %s", serviceNamespaceName, err)
			if !isDeleted {
				//Return error for creation request only,
				//For k8sService deletion request, we should go ahead to build TargetGroupSpec model
				return model.TargetGroupSpec{}, err
			}
		}

		var err error

		ipAddressType, err = buildTargetGroupIpAdressType(svc)

		// Ignore error for creation request
		if !isDeleted && err != nil {
			return model.TargetGroupSpec{}, err
		}
	}

	tgName := latticestore.TargetGroupName(string(backendRef.Name()), namespace)

	refObjNamespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      string(backendRef.Name()),
	}
	tgp, err := GetAttachedPolicy(ctx, t.client, refObjNamespacedName, &anv1alpha1.TargetGroupPolicy{})

	if err != nil {
		return model.TargetGroupSpec{}, err
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

	return model.TargetGroupSpec{
		Name: tgName,
		Type: model.TargetGroupTypeIP,
		Config: model.TargetGroupConfig{
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
		IsDeleted: isDeleted,
	}, nil
}

func (t *latticeServiceModelBuildTask) buildTargetGroupName(_ context.Context, backendRef core.BackendRef) string {
	if backendRef.Namespace() != nil {
		return latticestore.TargetGroupName(string(backendRef.Name()), string(*backendRef.Namespace()))
	} else {
		return latticestore.TargetGroupName(string(backendRef.Name()), t.route.Namespace())
	}
}

func parseHealthCheckConfig(tgp *anv1alpha1.TargetGroupPolicy) *vpclattice.HealthCheckConfig {
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

	if len(ipFamilies) != 1 {
		return "", errors.New("Lattice Target Group only supports single stack IP addresses")
	}

	// IpFamilies will always have at least 1 element
	ipFamily := ipFamilies[0]

	switch ipFamily {
	case corev1.IPv4Protocol:
		return vpclattice.IpAddressTypeIpv4, nil
	case corev1.IPv6Protocol:
		return vpclattice.IpAddressTypeIpv6, nil
	default:
		return "", fmt.Errorf("unknown ipFamily: %s", ipFamily)
	}
}

func GetServiceForBackendRef(ctx context.Context, client client.Client, route core.Route, backendRef core.BackendRef) (*corev1.Service, error) {
	svc := &corev1.Service{}
	key := types.NamespacedName{
		Name: string(backendRef.Name()),
	}

	if backendRef.Namespace() != nil {
		key.Namespace = string(*backendRef.Namespace())
	} else {
		key.Namespace = route.Namespace()
	}

	if err := client.Get(ctx, key, svc); err != nil {
		return nil, err
	}

	return svc, nil
}
