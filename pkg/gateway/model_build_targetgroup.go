package gateway

import (
	"context"

	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
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
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(srvExport)))

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

// Triggered from httproute/service/targetgroup
func (t *latticeServiceModelBuildTask) buildTargets(ctx context.Context) error {
	for _, httpRule := range t.route.Spec().Rules() {
		for _, httpBackendRef := range httpRule.BackendRefs() {
			if string(*httpBackendRef.Kind()) == "serviceimport" {
				glog.V(6).Infof("latticeServiceModelBuildTask: ignore service: %v \n", httpBackendRef)
				continue
			}

			backendNamespace := t.route.Namespace()
			if httpBackendRef.Namespace() != nil {
				backendNamespace = string(*httpBackendRef.Namespace())
			}

			port := int64(0)
			if httpBackendRef.Port() != nil {
				port = int64(*httpBackendRef.Port())
			}

			targetTask := &latticeTargetsModelBuildTask{
				Client:      t.Client,
				tgName:      string(httpBackendRef.Name()),
				tgNamespace: backendNamespace,
				routename:   t.route.Name(),
				port:        port,
				stack:       t.stack,
				datastore:   t.Datastore,
				route:       t.route,
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
			// Fill in default HTTP port as we are using target port anyway.
			Port:                80,
			IsServiceImport:     false,
			IsServiceExport:     true,
			K8SServiceName:      t.serviceExport.Name,
			K8SServiceNamespace: t.serviceExport.Namespace,
			Protocol:            "HTTP",
			ProtocolVersion:     vpclattice.TargetGroupProtocolVersionHttp1,
		},
	}

	tg := latticemodel.NewTargetGroup(t.stack, tgName, tgSpec)
	glog.V(6).Infof("buildTargetGroup, tg[%s], tgSpec%v \n", tgName, tg)

	// add targetgroup to localcache for service reconcile to reference
	// for serviceexport, the httproutename is set to ""
	t.Datastore.AddTargetGroup(tgName, "", "", "", tgSpec.Config.IsServiceImport, "")

	if !t.serviceExport.DeletionTimestamp.IsZero() {
		// triggered by serviceexport delete
		t.Datastore.SetTargetGroupByServiceExport(tgName, false, false)
	} else {
		// triggered by serviceexport add
		t.Datastore.SetTargetGroupByServiceExport(tgName, false, true)
	}

	// for serviceexport, the routename is null
	dsTG, err := t.Datastore.GetTargetGroup(tgName, "", false)

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

	for _, httpRule := range t.route.Spec().Rules() {
		glog.V(6).Infof("buildTargetGroup: %v\n", httpRule)

		for _, httpBackendRef := range httpRule.BackendRefs() {
			glog.V(6).Infof("buildTargetGroup -- backendRef %v \n", httpBackendRef)

			tgName := t.buildTargetGroupName(ctx, httpBackendRef)

			tgSpec, err := t.buildTargetGroupSpec(ctx, client, httpBackendRef)
			if err != nil {
				glog.V(6).Infof("buildTargetGroupSpec err %v \n", err)
				return nil, err
			}

			// add targetgroup to localcache for service reconcile to reference
			if *httpBackendRef.Kind() == "Service" {
				t.Datastore.AddTargetGroup(tgName, "", "", "", tgSpec.Config.IsServiceImport, t.route.Name())
			} else {
				// for serviceimport, the httproutename is ""
				t.Datastore.AddTargetGroup(tgName, "", "", "", tgSpec.Config.IsServiceImport, "")
			}

			if t.route.DeletionTimestamp().IsZero() {
				// to add
				t.Datastore.SetTargetGroupByBackendRef(tgName, t.route.Name(), tgSpec.Config.IsServiceImport, true)
			} else {
				// to delete
				t.Datastore.SetTargetGroupByBackendRef(tgName, t.route.Name(), tgSpec.Config.IsServiceImport, false)
				dsTG, _ := t.Datastore.GetTargetGroup(tgName, t.route.Name(), tgSpec.Config.IsServiceImport)
				tgSpec.IsDeleted = true
				tgSpec.LatticeID = dsTG.ID
			}

			tg := latticemodel.NewTargetGroup(t.stack, tgName, tgSpec)
			glog.V(6).Infof("buildTargetGroup, tg[%s], tgSpec%v \n", tgName, tg)

			t.tgByResID[tgName] = tg

		}

	}
	return nil, nil

}

// Now, Only k8sService and serviceImport creation deletion use this function to build TargetGroupSpec, serviceExport does not use this function to create TargetGroupSpec
func (t *latticeServiceModelBuildTask) buildTargetGroupSpec(ctx context.Context, client client.Client, httpBackendRef core.BackendRef) (latticemodel.TargetGroupSpec, error) {
	var namespace string

	if httpBackendRef.Namespace() != nil {
		namespace = string(*httpBackendRef.Namespace())
	} else {
		namespace = t.route.Namespace()
	}

	backendKind := string(*httpBackendRef.Kind())
	glog.V(6).Infof("buildTargetGroupSpec,  kind %s\n", backendKind)

	var vpc = config.VpcID
	var ekscluster = ""
	var isServiceImport bool
	var isDeleted bool

	if t.route.DeletionTimestamp().IsZero() {
		isDeleted = false
	} else {
		isDeleted = true
	}
	if backendKind == "ServiceImport" {
		isServiceImport = true
		namespaceName := types.NamespacedName{
			Namespace: namespace,
			Name:      string(httpBackendRef.Name()),
		}
		serviceImport := &mcs_api.ServiceImport{}

		if err := client.Get(context.TODO(), namespaceName, serviceImport); err == nil {
			glog.V(6).Infof("buildTargetGroupSpec, using service Import %v\n", namespaceName)
			vpc = serviceImport.Annotations["multicluster.x-k8s.io/aws-vpc"]
			ekscluster = serviceImport.Annotations["multicluster.x-k8s.io/aws-eks-cluster-name"]

		} else {
			glog.V(6).Infof("buildTargetGroupSpec, using service Import %v, err :%v\n", namespaceName, err)
			if !isDeleted {
				//Return error for creation request only.
				//For ServiceImport deletion request, we should go ahead to build TargetGroupSpec model,
				//although the targetGroupSynthesizer could skip TargetGroup deletion triggered by ServiceImport deletion
				return latticemodel.TargetGroupSpec{}, err
			}
		}

	} else {
		var namespace = t.route.Namespace()

		if httpBackendRef.Namespace() != nil {
			namespace = string(*httpBackendRef.Namespace())
		}
		// find out service target port
		serviceNamespaceName := types.NamespacedName{
			Namespace: namespace,
			Name:      string(httpBackendRef.Name()),
		}

		svc := &corev1.Service{}
		if err := t.Client.Get(ctx, serviceNamespaceName, svc); err != nil {
			glog.V(6).Infof("Error finding backend service %v error :%v \n", serviceNamespaceName, err)
			if !isDeleted {
				//Return error for creation request only,
				//For k8sService deletion request, we should go ahead to build TargetGroupSpec model
				return latticemodel.TargetGroupSpec{}, err
			}
		}
	}

	tgName := latticestore.TargetGroupName(string(httpBackendRef.Name()), namespace)

	return latticemodel.TargetGroupSpec{
		Name: tgName,
		Type: latticemodel.TargetGroupTypeIP,
		Config: latticemodel.TargetGroupConfig{
			VpcID:                 vpc,
			EKSClusterName:        ekscluster,
			IsServiceImport:       isServiceImport,
			IsServiceExport:       false,
			K8SServiceName:        string(httpBackendRef.Name()),
			K8SServiceNamespace:   namespace,
			K8SHTTPRouteName:      t.route.Name(),
			K8SHTTPRouteNamespace: t.route.Namespace(),
			Protocol:              "HTTP",
			ProtocolVersion:       vpclattice.TargetGroupProtocolVersionHttp1,
			// Fill in default HTTP port as we are using target port anyway.
			Port: 80,
		},
		IsDeleted: isDeleted,
	}, nil
}

func (t *latticeServiceModelBuildTask) buildTargetGroupName(_ context.Context, backendRef core.BackendRef) string {
	if backendRef.Namespace() != nil {
		return latticestore.TargetGroupName(string(backendRef.Name()),
			string(*backendRef.Namespace()))
	} else {
		return latticestore.TargetGroupName(string(backendRef.Name()),
			t.route.Namespace())
	}
}
