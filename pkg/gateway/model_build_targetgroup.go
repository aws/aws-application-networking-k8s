package gateway

import (
	"context"
	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-sdk-go/service/vpclattice"
)

const (
	resourceIDTargetGroup = "TargetGroup"
)

type TargetGroupModelBuilder interface {
	Build(ctx context.Context, srvExport *mcs_api.ServiceExport) (core.Stack, *latticemodel.TargetGroup, error)
}

type targetGroupBuilder struct {
	client.Client

	serviceExport *mcs_api.ServiceExport
	Datastore     *latticestore.LatticeDataStore
	cloud         lattice_aws.Cloud

	defaultTags map[string]string
}

// triggered from serviceexport
func NewTargetGroupBuilder(client client.Client, datastore *latticestore.LatticeDataStore, cloud lattice_aws.Cloud) *targetGroupBuilder {
	return &targetGroupBuilder{
		Client:    client,
		Datastore: datastore,
		cloud:     cloud,
	}
}

type targetGroupModelBuildTask struct {
	client.Client
	serviceExport *mcs_api.ServiceExport
	targetGroup   *latticemodel.TargetGroup
	tgByResID     map[string]*latticemodel.TargetGroup
	stack         core.Stack

	Datastore *latticestore.LatticeDataStore
	cloud     lattice_aws.Cloud
}

// for serviceexport
func (b *targetGroupBuilder) Build(ctx context.Context, srvExport *mcs_api.ServiceExport) (core.Stack, *latticemodel.TargetGroup, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName((srvExport))))

	task := &targetGroupModelBuildTask{
		serviceExport: srvExport,
		stack:         stack,
		tgByResID:     make(map[string]*latticemodel.TargetGroup),

		Datastore: b.Datastore,
		cloud:     b.cloud,
		Client:    b.Client,
	}

	if err := task.run(ctx); err != nil {
		return task.stack, task.targetGroup, corev1.ErrIntOverflowGenerated
	}

	return task.stack, task.targetGroup, nil
}

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

func (t *targetGroupModelBuildTask) buildModel(ctx context.Context) error {
	err := t.BuildTargetGroup(ctx)

	if err != nil {
		glog.V(6).Infof("Failed to build TargetGroup when serviceExport buildModel for name %v namespace %v \n", t.serviceExport.Name, t.serviceExport.Namespace)
		return err
	}

	err = t.BuildTargets(ctx)

	if err != nil {
		glog.V(6).Infof("Failed to build Targets when serviceExport buildModel for name %v namespace %v \n ", t.serviceExport.Name, t.serviceExport.Namespace)
	}

	return nil
}

// triggered from service exports/targetgroups
func (t *targetGroupModelBuildTask) BuildTargets(ctx context.Context) error {

	targetTask := &latticeTargetsModelBuildTask{
		Client:      t.Client,
		tgName:      t.serviceExport.Name,
		tgNamespace: t.serviceExport.Namespace,
		stack:       t.stack,
		datastore:   t.Datastore,
	}

	err := targetTask.buildLatticeTargets(ctx)

	if err != nil {
		glog.V(6).Infof("Error buildTargets for serviceExport name %v, namespace %v \n", t.serviceExport.Name, t.serviceExport.Namespace)
		return err
	}
	return nil
}

// Triggered from httproute/service/targetgrou
func (t *latticeServiceModelBuildTask) buildTargets(ctx context.Context) error {
	for _, httpRule := range t.httpRoute.Spec.Rules {
		for _, httpBackendRef := range httpRule.BackendRefs {
			if string(*httpBackendRef.Kind) == "serviceimport" {
				glog.V(6).Infof("latticeServiceModelBuildTask: ignore service: %v \n", httpBackendRef)
				continue
			}
			backendNamespace := "default"

			if httpBackendRef.Namespace != nil {
				backendNamespace = string(*httpBackendRef.Namespace)

			}

			targetTask := &latticeTargetsModelBuildTask{
				Client:      t.Client,
				tgName:      string(httpBackendRef.Name),
				tgNamespace: backendNamespace,
				stack:       t.stack,
				datastore:   t.Datastore,
			}

			err := targetTask.buildLatticeTargets(ctx)

			if err != nil {
				glog.V(6).Infof("Error buildTargets for backend ref service %v \n", httpBackendRef)
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
	if err := t.Client.Get(ctx, k8s.NamespacedName(t.serviceExport), svc); err != nil {
		glog.V(6).Infof("Error finding corresponding service %v error :%v \n", k8s.NamespacedName(t.serviceExport), err)
		// mark there is no serviceexport dependence on the TG
		t.Datastore.SetTargetGroupByServiceExport(tgName, false, false)
		return err
	}

	//if t.serviceExport.
	tgSpec := latticemodel.TargetGroupSpec{
		Name: tgName,
		Type: latticemodel.TargetGroupTypeIP,
		Config: latticemodel.TargetGroupConfig{
			VpcID: config.VpcID,
			//Port:            backendServicePort,
			IsServiceImport: false,
			Protocol:        "HTTP",
			ProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp1,
		},
	}

	glog.V(6).Infof("Found backend service port %v , intval %v \n", svc.Spec.Ports[0].TargetPort,
		svc.Spec.Ports[0].TargetPort.IntVal)

	tgSpec.Config.Port = svc.Spec.Ports[0].TargetPort.IntVal

	tg := latticemodel.NewTargetGroup(t.stack, tgName, tgSpec)
	glog.V(6).Infof("buildTargetGroup, tg[%s], tgSpec%v \n", tgName, tg)

	// add targetgroup to localcache for service reconcile to reference
	t.Datastore.AddTargetGroup(tgName, "", "", "", tgSpec.Config.IsServiceImport)

	if !t.serviceExport.DeletionTimestamp.IsZero() {
		// triggered by serviceexport delete
		t.Datastore.SetTargetGroupByServiceExport(tgName, false, false)
	} else {
		// triggered by serviceexport add
		t.Datastore.SetTargetGroupByServiceExport(tgName, false, true)
	}

	dsTG, err := t.Datastore.GetTargetGroup(tgName, false)

	glog.V(6).Infof("TargetGroup cached in datastore: %v \n", dsTG)
	if (err != nil) || (!dsTG.ByBackendRef && !dsTG.ByServiceExport) {
		glog.V(6).Infof("BuildingTargetGroup: TG %v is NOT used anymore and can be delted\n", tgSpec)
		tg.Spec.IsDeleted = true
		tg.Spec.LatticeID = dsTG.ID
	}

	t.tgByResID[tgName] = tg
	t.targetGroup = tg
	return nil
}

// Build target group for backend service ref used in HTTPRoute
func (t *latticeServiceModelBuildTask) buildTargetGroup(ctx context.Context, client client.Client) (*latticemodel.TargetGroup, error) {

	for _, httpRule := range t.httpRoute.Spec.Rules {
		glog.V(6).Infof("buildTargetGroup: %v\n", httpRule)

		for _, httpBackendRef := range httpRule.BackendRefs {
			glog.V(6).Infof("buildTargetGroup -- backendRef %v \n", httpBackendRef)

			tgName := t.buildHTTPTargetGroupName(ctx, &httpBackendRef)

			tgSpec, err := t.buildHTTPTargetGroupSpec(ctx, client, &httpBackendRef)
			if err != nil {
				glog.V(6).Infof("buildHTTPTargetGroupSpec err %v \n", err)
				return nil, err
			}

			// add targetgroup to localcache for service reconcile to reference
			t.Datastore.AddTargetGroup(tgName, "", "", "", tgSpec.Config.IsServiceImport)

			if t.httpRoute.DeletionTimestamp.IsZero() {
				// to add
				t.Datastore.SetTargetGroupByBackendRef(tgName, false, true)
			} else {
				// to delete
				t.Datastore.SetTargetGroupByBackendRef(tgName, false, false)
			}

			tg := latticemodel.NewTargetGroup(t.stack, tgName, tgSpec)
			glog.V(6).Infof("buildTargetGroup, tg[%s], tgSpec%v \n", tgName, tg)

			t.tgByResID[tgName] = tg

		}

	}
	return nil, nil

}

func (t *latticeServiceModelBuildTask) buildHTTPTargetGroupSpec(ctx context.Context, client client.Client, httpBackendRef *v1alpha2.HTTPBackendRef) (latticemodel.TargetGroupSpec, error) {
	var namespace string

	if httpBackendRef.BackendRef.BackendObjectReference.Namespace != nil {
		namespace = string(*httpBackendRef.BackendRef.BackendObjectReference.Namespace)
	} else {
		namespace = "default"
	}

	backendKind := string(*httpBackendRef.BackendRef.BackendObjectReference.Kind)
	glog.V(6).Infof("buildHTTPTargetGroupSpec,  kind %s\n", backendKind)

	var vpc = config.VpcID
	var ekscluster = ""
	var isServiceImport bool
	var backendServicePort int32

	isServiceImport = false

	if backendKind == "ServiceImport" {
		namespaceName := types.NamespacedName{
			Namespace: namespace,
			Name:      string(httpBackendRef.Name),
		}
		serviceImport := &mcs_api.ServiceImport{}

		if err := client.Get(context.TODO(), namespaceName, serviceImport); err == nil {
			glog.V(6).Infof("buildHTTPTargetGroupSpec, using service Import %v\n", namespaceName)
			vpc = serviceImport.Annotations["multicluster.x-k8s.io/aws-vpc"]
			ekscluster = serviceImport.Annotations["multicluster.x-k8s.io/aws-eks-cluster-name"]
			isServiceImport = true

		} else {
			glog.V(6).Infof("buildHTTPTargetGroupSpec, using service Import %v, err :%v\n", namespaceName, err)
			return latticemodel.TargetGroupSpec{}, err
		}

	} else {
		var namespace = "default"

		if httpBackendRef.Namespace != nil {
			namespace = string(*httpBackendRef.Namespace)
		}
		// find out service target port
		serviceNamespaceName := types.NamespacedName{
			Namespace: namespace,
			Name:      string(httpBackendRef.Name),
		}

		svc := &corev1.Service{}
		if err := t.Client.Get(ctx, serviceNamespaceName, svc); err != nil {
			glog.V(6).Infof("Error finding backend service %v error :%v \n", serviceNamespaceName, err)
			return latticemodel.TargetGroupSpec{}, err
		}

		if svc.Spec.Ports != nil {
			glog.V(6).Infof("Found backend service port %v , intval %v \n", svc.Spec.Ports[0].TargetPort,
				svc.Spec.Ports[0].TargetPort.IntVal)

			backendServicePort = svc.Spec.Ports[0].TargetPort.IntVal
		}

	}

	tgName := latticestore.TargetGroupName(string(httpBackendRef.Name), namespace)

	var isDeleted bool

	if t.httpRoute.DeletionTimestamp.IsZero() {
		isDeleted = false
	} else {
		isDeleted = true
	}

	return latticemodel.TargetGroupSpec{
		Name: tgName,
		Type: latticemodel.TargetGroupTypeIP,
		Config: latticemodel.TargetGroupConfig{
			VpcID:           vpc,
			EKSClusterName:  ekscluster,
			IsServiceImport: isServiceImport,
			Protocol:        "HTTP",
			ProtocolVersion: vpclattice.TargetGroupProtocolVersionHttp1,
			Port:            backendServicePort,
		},
		IsDeleted: isDeleted,
	}, nil
}

func (t *latticeServiceModelBuildTask) buildHTTPTargetGroupName(_ context.Context, httpBackendRef *v1alpha2.HTTPBackendRef) string {
	if httpBackendRef.BackendRef.BackendObjectReference.Namespace != nil {
		return latticestore.TargetGroupName(string(httpBackendRef.BackendRef.BackendObjectReference.Name),
			string(*httpBackendRef.BackendRef.BackendObjectReference.Namespace))
	} else {
		return latticestore.TargetGroupName(string(httpBackendRef.BackendRef.BackendObjectReference.Name), "default")
	}
}
