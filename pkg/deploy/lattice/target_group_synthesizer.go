package lattice

import (
	"context"
	"errors"
	"strings"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func NewTargetGroupSynthesizer(cloud lattice_aws.Cloud, client client.Client, tgManager TargetGroupManager, stack core.Stack, latticeDataStore *latticestore.LatticeDataStore) *targetGroupSynthesizer {
	return &targetGroupSynthesizer{
		cloud:              cloud,
		client:             client,
		targetGroupManager: tgManager,
		stack:              stack,
		latticeDataStore:   latticeDataStore,
	}
}

type targetGroupSynthesizer struct {
	cloud              lattice_aws.Cloud
	client             client.Client
	targetGroupManager TargetGroupManager
	stack              core.Stack
	latticeDataStore   *latticestore.LatticeDataStore
}

func (t *targetGroupSynthesizer) Synthesize(ctx context.Context) error {

	glog.V(6).Infof("Start synthesizing TargetGroupss ...\n")

	if err := t.SynthesizeTriggeredTargetGroupsCreation(ctx); err != nil {
		return errors.New(LATTICE_RETRY)
	}

	/* TODO,  resolve bug that this might delete other HTTPRoute's TG before they have chance
	 * to be reconcile during controller restart
	 */
	return nil
}

func (t *targetGroupSynthesizer) SynthesizeTriggeredTargetGroupsCreation(ctx context.Context) error {
	var resTargetGroups []*latticemodel.TargetGroup
	var returnErr = false

	t.stack.ListResources(&resTargetGroups)

	glog.V(6).Infof("SynthesizeTriggeredTargetGroupsCreation TargetGroups: [%v]\n", resTargetGroups)

	for _, resTargetGroup := range resTargetGroups {
		if resTargetGroup.Spec.IsDeleted {
			// in SynthesizeTriggeredTargetGroupsCreation(), we only handle TG Creation request
			continue
		}
		// find out VPC for service import
		if resTargetGroup.Spec.Config.IsServiceImport {
			/* right now, TG are unique across VPC, we do NOT need to get VPC
			if resTargetGroup.Spec.Config.EKSClusterName != "" {
				eksSess := t.cloud.EKS()

				input := &eks.DescribeClusterInput{
					Name: aws.String(resTargetGroup.Spec.Config.EKSClusterName),
				}
				result, err := eksSess.DescribeCluster(input)

				if err != nil {
					glog.V(6).Infof("Error eks DescribeCluster %v\n", err)
					returnErr = true
					continue
				} else {
					glog.V(6).Infof("Found VPCID =%s for EKS cluster %s \n", result.String(), resTargetGroup.Spec.Config.EKSClusterName)
					resTargetGroup.Spec.Config.VpcID = *result.Cluster.ResourcesVpcConfig.VpcId
					glog.V(6).Infof("targetGroup.Spec.Config.VpcID = %s\n", resTargetGroup.Spec.Config.VpcID)
				}
			}
			// TODO today, targetGroupManager.Create() will list all target and find out the matching one
			resTargetGroup.Spec.Config.VpcID = resTargetGroup.Spec.Config.VpcID
			*/

			// TODO in future, we might want to use annotation to specify lattice TG arn or ID

			tgStatus, err := t.targetGroupManager.Get(ctx, resTargetGroup)
			if err != nil {
				glog.V(6).Infof("Error on t.targetGroupManager.Get for %v err %v\n", resTargetGroup, err)
				returnErr = true
				continue
			}

			// for serviceimport, the httproutename is ""

			t.latticeDataStore.AddTargetGroup(resTargetGroup.Spec.Name,
				resTargetGroup.Spec.Config.VpcID, tgStatus.TargetGroupARN, tgStatus.TargetGroupID,
				resTargetGroup.Spec.Config.IsServiceImport, "")

			glog.V(6).Infof("targetGroup Synthesized successfully for %s: %v\n", resTargetGroup.Spec.Name, tgStatus)

		} else { // handle TargetGroup creation request that triggered by httproute with backendref k8sService creation or serviceExport creation

			resTargetGroup.Spec.Config.VpcID = config.VpcID

			tgStatus, err := t.targetGroupManager.Create(ctx, resTargetGroup)

			if err != nil {
				glog.V(6).Infof("Error on t.targetGroupManager.Create for %v err %v\n", resTargetGroup, err)
				returnErr = true
				continue
			}

			//In the ModelBuildTask, it should already add a tg entry in the latticeDataStore,
			//in here, only UPDATE the entry with tgStatus.TargetGroupARN and tgStatus.TargetGroupID
			t.latticeDataStore.AddTargetGroup(resTargetGroup.Spec.Name,
				resTargetGroup.Spec.Config.VpcID, tgStatus.TargetGroupARN,
				tgStatus.TargetGroupID, resTargetGroup.Spec.Config.IsServiceImport,
				resTargetGroup.Spec.Config.K8SHTTPRouteName)

			glog.V(6).Infof("targetGroup Synthesized successfully for %v: %v\n", resTargetGroup.Spec, tgStatus)
		}

	}

	glog.V(6).Infof("Done -- SynthesizeTriggeredTargetGroupsCreation %v\n", resTargetGroups)

	if returnErr {
		return errors.New(LATTICE_RETRY)
	} else {
		return nil
	}

}

// TODO: run t.SynthesizeSDKTargetGroups(ctx) as a global garbage collector schedule backgroud task (i.e., run it as a goroutine in main.go)
func (t *targetGroupSynthesizer) SynthesizeSDKTargetGroups(ctx context.Context) error {

	staleSDKTGs := []latticemodel.TargetGroup{}
	sdkTGs, err := t.targetGroupManager.List(ctx)

	if err != nil {
		glog.V(2).Infof("SynthesizeSDKTargetGroups: failed to retrieve sdk TGs %v\n", err)
		return nil
	}

	glog.V(6).Infof("SynthesizeSDKTargetGroups: here is sdkTGs %v len %v \n", sdkTGs, len(sdkTGs))

	for _, sdkTG := range sdkTGs {
		tgRouteName := ""

		if *sdkTG.getTargetGroupOutput.Config.VpcIdentifier != config.VpcID {
			glog.V(6).Infof("Ignore target group ARN %v Name %v for other VPCs",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
			continue
		}

		// does target group have K8S tags,  ignore if it is not tagged
		tgTags := sdkTG.targetGroupTags

		if tgTags == nil || tgTags.Tags == nil {
			glog.V(6).Infof("Ignore target group not tagged for K8S, %v, %v \n",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
			continue
		}

		parentRef, ok := tgTags.Tags[latticemodel.K8SParentRefTypeKey]
		if !ok || parentRef == nil {
			glog.V(6).Infof("Ignore target group that have no K8S parentRef tag :%v, %v \n",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
			continue
		}

		srvName, ok := tgTags.Tags[latticemodel.K8SServiceNameKey]

		if !ok || srvName == nil {
			glog.V(6).Infof("Ignore TargetGroup have no servicename tag: %v, %v",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
			continue
		}

		srvNamespace, ok := tgTags.Tags[latticemodel.K8SServiceNamespaceKey]

		if !ok || srvNamespace == nil {
			glog.V(6).Infof("Ignore TargetGroup have no servicenamespace tag: %v, %v",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
			continue
		}

		// if its parentref is service export,  check the parent service export exist
		// Ignore if service export exists
		if *parentRef == latticemodel.K8SServiceExportType {
			glog.V(6).Infof("TargetGroup %v, %v is referenced by ServiceExport",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)

			glog.V(6).Infof("Determine serviceexport name=%v, namespace=%v exists for targetGroup %v",
				*srvName, *srvNamespace, *sdkTG.getTargetGroupOutput.Arn)

			srvExportName := types.NamespacedName{
				Namespace: *srvNamespace,
				Name:      *srvName,
			}
			srvExport := &mcs_api.ServiceExport{}
			if err := t.client.Get(ctx, srvExportName, srvExport); err == nil {

				glog.V(6).Infof("Ignore TargetGroup(triggered by serviceexport) %v, %v since serviceexport object is found",
					*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
				continue
			}
		}

		// if its parentref is HTTP/route, check the parent HTTPRoute exist
		// Ignore if httpRoute does NOT exist
		if *parentRef == latticemodel.K8SHTTPRouteType {
			glog.V(6).Infof("TargetGroup %v, %v is referenced by HTTPRoute",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)

			httpName, ok := tgTags.Tags[latticemodel.K8SHTTPRouteNameKey]
			tgRouteName = *httpName

			if !ok || httpName == nil {
				glog.V(6).Infof("Ignore TargetGroup(triggered by httpRoute) %v, %v have no httproute name tag",
					*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
				continue
			}

			httpNamespace, ok := tgTags.Tags[latticemodel.K8SHTTPRouteNamespaceKey]

			if !ok || httpNamespace == nil {
				glog.V(6).Infof("Ignore TargetGroup(triggered by httpRoute) %v, %v have no httproute namespace tag",
					*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
				continue
			}

			httprouteName := types.NamespacedName{
				Namespace: *httpNamespace,
				Name:      *httpName,
			}

			httpRoute := &gateway_api.HTTPRoute{}

			tgName := latticestore.TargetGroupName(*srvName, *srvNamespace)

			if err := t.client.Get(ctx, httprouteName, httpRoute); err != nil {
				glog.V(6).Infof("tgname %v is not used by httproute %v\n", tgName, httpRoute)
			} else {
				isUsed := t.isTargetGroupUsedByaHTTPRoute(ctx, tgName, httpRoute)
				if isUsed {
					glog.V(6).Infof("Ignore TargetGroup(triggered by HTTProute) %v, %v since httproute object is found",
						*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
					continue
				} else {
					glog.V(6).Infof("tgname %v is not used by httproute %v\n", tgName, httpRoute)
				}
			}

		}

		// the routename for serviceimport is ""
		if tg, err := t.latticeDataStore.GetTargetGroup(*sdkTG.getTargetGroupOutput.Name, "", true); err == nil {
			glog.V(6).Infof("Ignore target group created by service import %v\n", tg)
			continue
		}

		glog.V(2).Infof("Append stale SDK TG to stale list Name %v, routename %v, ARN %v",
			*sdkTG.getTargetGroupOutput.Name, tgRouteName, *sdkTG.getTargetGroupOutput.Id)

		staleSDKTGs = append(staleSDKTGs, latticemodel.TargetGroup{
			Spec: latticemodel.TargetGroupSpec{
				Name: *sdkTG.getTargetGroupOutput.Name,
				Config: latticemodel.TargetGroupConfig{
					K8SHTTPRouteName: tgRouteName,
				},
				LatticeID: *sdkTG.getTargetGroupOutput.Id,
			},
		})

	}

	glog.V(6).Infof("SynthesizeSDKTargetGroups, here is the stale target groups list %v stalelen %d\n", staleSDKTGs, len(staleSDKTGs))

	ret_err := false

	for _, sdkTG := range staleSDKTGs {

		err := t.targetGroupManager.Delete(ctx, &sdkTG)
		glog.V(2).Infof("SynthesizeSDKTargetGroups, deleting stale target group %v \n", err)

		if err != nil && !strings.Contains(err.Error(), "TargetGroup is referenced in routing configuration, listeners or rules of service.") {
			ret_err = true
		}
		// continue on even when there is an err

	}

	if ret_err {
		return errors.New(LATTICE_RETRY)
	} else {
		return nil
	}

}

func (t *targetGroupSynthesizer) isTargetGroupUsedByaHTTPRoute(ctx context.Context, tgName string, httpRoute *gateway_api.HTTPRoute) bool {

	for _, httpRule := range httpRoute.Spec.Rules {
		for _, httpBackendRef := range httpRule.BackendRefs {
			if string(*httpBackendRef.BackendObjectReference.Kind) != "Service" {
				continue
			}
			namespace := httpRoute.Namespace
			if httpBackendRef.BackendObjectReference.Namespace != nil {
				namespace = string(*httpBackendRef.BackendObjectReference.Namespace)
			}
			refTGName := latticestore.TargetGroupName(string(httpBackendRef.BackendObjectReference.Name), namespace)

			if tgName == refTGName {
				return true
			}

		}
	}

	return false
}

func (t *targetGroupSynthesizer) SynthesizeTriggeredTargetGroupsDeletion(ctx context.Context) error {
	var resTargetGroups []*latticemodel.TargetGroup
	var returnErr = false

	t.stack.ListResources(&resTargetGroups)

	glog.V(2).Infof("SynthesizeTriggeredTargetGroupsDeletion: TargetGroups ==[%v]\n", resTargetGroups)
	for _, resTargetGroup := range resTargetGroups {
		if resTargetGroup.Spec.IsDeleted && !resTargetGroup.Spec.Config.IsServiceImport {
			//Handle deleting target groups request triggered by httproute with BackendRef k8sService deletion or ServiceExport deletion,
			//ignore target group deletion request triggered by ServiceImport deletion
			//ignore any target group creation request
			err := t.targetGroupManager.Delete(ctx, resTargetGroup)
			if err != nil {
				glog.V(6).Infof("Delete Target Group in PostSynthesize: failed to delete target group %v, err %v \n", resTargetGroup, err)
				returnErr = true
			} else {
				glog.V(6).Infof("Delete Target Group in PostSynthesize: successfully deleted target group %v\n", resTargetGroup)
				t.latticeDataStore.DelTargetGroup(resTargetGroup.Spec.Name, resTargetGroup.Spec.Config.K8SHTTPRouteName, resTargetGroup.Spec.Config.IsServiceImport)
			}
		}
	}
	if returnErr {
		return errors.New(LATTICE_RETRY)
	} else {
		return nil
	}
}

func (t *targetGroupSynthesizer) PostSynthesize(ctx context.Context) error {

	glog.V(2).Infof("PostSynthesize TargetGroups\n")

	var returnErr = false
	if err := t.SynthesizeTriggeredTargetGroupsDeletion(ctx); err != nil {
		returnErr = true
	}

	//TODO: run t.SynthesizeSDKTargetGroups(ctx) as a global garbage collector scheduled backgroud task (i.e., run it as a goroutine in main.go)
	if err := t.SynthesizeSDKTargetGroups(ctx); err != nil {
		returnErr = true
	}

	if returnErr {
		return errors.New(LATTICE_RETRY)
	} else {
		return nil
	}

}
