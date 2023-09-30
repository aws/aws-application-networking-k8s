package lattice

import (
	"context"
	"errors"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func NewTargetGroupSynthesizer(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
	client client.Client,
	tgManager TargetGroupManager,
	stack core.Stack,
	latticeDataStore *latticestore.LatticeDataStore,
) *TargetGroupSynthesizer {
	return &TargetGroupSynthesizer{
		log:                log,
		cloud:              cloud,
		client:             client,
		targetGroupManager: tgManager,
		stack:              stack,
		latticeDataStore:   latticeDataStore,
	}
}

type TargetGroupSynthesizer struct {
	log                gwlog.Logger
	cloud              pkg_aws.Cloud
	client             client.Client
	targetGroupManager TargetGroupManager
	stack              core.Stack
	latticeDataStore   *latticestore.LatticeDataStore
}

func (t *TargetGroupSynthesizer) Synthesize(ctx context.Context) error {
	var ret = ""

	t.log.Infof("Started synthesizing TargetGroups")

	if err := t.SynthesizeTriggeredTargetGroup(ctx); err != nil {
		ret = LATTICE_RETRY
	}

	/*
	 * TODO: resolve bug that this might delete other Route's TG before they have chance
	 *       to be reconcile during controller restart
	 */
	// This might conflict and try to delete other TGs in the middle of creation, because
	// this is coming from TargetGroupStackDeployer, which can run before all rules are reconciled.
	//
	// Since the same cleaner logic is running from ServiceStackDeployer, we may not need this here.
	//
	//if err := t.SynthesizeSDKTargetGroups(ctx); err != nil {
	//	ret = LATTICE_RETRY
	//}

	if ret != "" {
		return errors.New(ret)
	} else {
		return nil
	}
}

func (t *TargetGroupSynthesizer) SynthesizeTriggeredTargetGroup(ctx context.Context) error {
	var resTargetGroups []*model.TargetGroup
	var returnErr = false

	t.stack.ListResources(&resTargetGroups)

	t.log.Infof("Synthesize TargetGroups ==[%v]", resTargetGroups)

	for _, resTargetGroup := range resTargetGroups {

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
					t.log.Infof("Error eks DescribeCluster %v", err)
					returnErr = true
					continue
				} else {
					t.log.Infof("Found VPCID =%s for EKS cluster %s", result.String(), resTargetGroup.Spec.Config.EKSClusterName)
					resTargetGroup.Spec.Config.VpcID = *result.Cluster.ResourcesVpcConfig.VpcId
					t.log.Infof("targetGroup.Spec.Config.VpcID = %s", resTargetGroup.Spec.Config.VpcID)
				}
			}
			// TODO today, targetGroupManager.Create() will list all target and find out the matching one
			resTargetGroup.Spec.Config.VpcID = resTargetGroup.Spec.Config.VpcID
			*/

			// TODO in future, we might want to use annotation to specify lattice TG arn or ID
			if resTargetGroup.Spec.IsDeleted {
				//Ingnore TG delete since this is an import from elsewhere
				continue
			}
			tgStatus, err := t.targetGroupManager.Get(ctx, resTargetGroup)

			if err != nil {
				t.log.Infof("Error on t.targetGroupManager.Get for %v err %v", resTargetGroup, err)
				returnErr = true
				continue
			}

			// for serviceimport, the httproutename is ""

			t.latticeDataStore.AddTargetGroup(resTargetGroup.Spec.Name,
				resTargetGroup.Spec.Config.VpcID, tgStatus.TargetGroupARN, tgStatus.TargetGroupID,
				resTargetGroup.Spec.Config.IsServiceImport, "")

			t.log.Infof("targetGroup Synthesized successfully for %s: %v", resTargetGroup.Spec.Name, tgStatus)

		} else {
			if resTargetGroup.Spec.IsDeleted {
				err := t.targetGroupManager.Delete(ctx, resTargetGroup)

				if err != nil {
					returnErr = true
					continue
				} else {
					t.log.Infof("Synthesizing Target Group: successfully deleted target group %v", resTargetGroup)
					t.latticeDataStore.DelTargetGroup(resTargetGroup.Spec.Name, resTargetGroup.Spec.Config.K8SHTTPRouteName, false)
				}

			} else {
				resTargetGroup.Spec.Config.VpcID = config.VpcID

				tgStatus, err := t.targetGroupManager.Create(ctx, resTargetGroup)

				if err != nil {
					t.log.Infof("Error on t.targetGroupManager.Create for %v err %v", resTargetGroup, err)
					returnErr = true
					continue
				}

				t.latticeDataStore.AddTargetGroup(resTargetGroup.Spec.Name,
					resTargetGroup.Spec.Config.VpcID, tgStatus.TargetGroupARN,
					tgStatus.TargetGroupID, resTargetGroup.Spec.Config.IsServiceImport,
					resTargetGroup.Spec.Config.K8SHTTPRouteName)

				t.log.Infof("targetGroup Synthesized successfully for %v: %v", resTargetGroup.Spec, tgStatus)
			}
		}

	}

	t.log.Infof("Done -- SynthesizeTriggeredTargetGroup %v", resTargetGroups)

	if returnErr {
		return errors.New("LATTICE-RETRY")
	} else {
		return nil
	}

}

func (t *TargetGroupSynthesizer) SynthesizeSDKTargetGroups(ctx context.Context) error {
	var staleSDKTGs []model.TargetGroup
	sdkTGs, err := t.targetGroupManager.List(ctx)

	if err != nil {
		t.log.Debugf("SynthesizeSDKTargetGroups: failed to retrieve sdk TGs %v", err)
		return nil
	}

	t.log.Infof("SynthesizeSDKTargetGroups: here is sdkTGs %v len %v", sdkTGs, len(sdkTGs))

	for _, sdkTG := range sdkTGs {
		tgRouteName := ""

		if *sdkTG.getTargetGroupOutput.Config.VpcIdentifier != config.VpcID {
			t.log.Infof("Ignore target group ARN %v Name %v for other VPCs",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
			continue
		}

		// does target group have K8S tags,  ignore if it is not tagged
		tgTags := sdkTG.targetGroupTags

		if tgTags == nil || tgTags.Tags == nil {
			t.log.Infof("Ignore target group not tagged for K8S, %v, %v",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
			continue
		}

		parentRef, ok := tgTags.Tags[model.K8SParentRefTypeKey]
		if !ok || parentRef == nil {
			t.log.Infof("Ignore target group that have no K8S parentRef tag :%v, %v",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
			continue
		}

		srvName, ok := tgTags.Tags[model.K8SServiceNameKey]

		if !ok || srvName == nil {
			t.log.Infof("Ignore TargetGroup have no servicename tag: %v, %v",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
			continue
		}

		srvNamespace, ok := tgTags.Tags[model.K8SServiceNamespaceKey]

		if !ok || srvNamespace == nil {
			t.log.Infof("Ignore TargetGroup have no servicenamespace tag: %v, %v",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
			continue
		}

		// if its parentref is service export,  check the parent service export exist
		// Ignore if service export exists
		if *parentRef == model.K8SServiceExportType {
			t.log.Infof("TargetGroup %v, %v is referenced by ServiceExport",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)

			t.log.Infof("Determine serviceexport name=%v, namespace=%v exists for targetGroup %v",
				*srvName, *srvNamespace, *sdkTG.getTargetGroupOutput.Arn)

			srvExportName := types.NamespacedName{
				Namespace: *srvNamespace,
				Name:      *srvName,
			}
			srvExport := &mcsv1alpha1.ServiceExport{}
			if err := t.client.Get(ctx, srvExportName, srvExport); err == nil {

				t.log.Infof("Ignore TargetGroup(triggered by serviceexport) %v, %v since serviceexport object is found",
					*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
				continue
			}
		}

		// if its parentRef is a route, check that the parent route exists
		// Ignore if route does not exist
		if *parentRef == model.K8SHTTPRouteType {
			t.log.Infof("TargetGroup %v, %v is referenced by a route",
				*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)

			routeNameValue, ok := tgTags.Tags[model.K8SHTTPRouteNameKey]
			tgRouteName = *routeNameValue

			if !ok || routeNameValue == nil {
				t.log.Infof("Ignore TargetGroup(triggered by route) %v, %v has no route name tag",
					*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
				continue
			}

			routeNamespaceValue, ok := tgTags.Tags[model.K8SHTTPRouteNamespaceKey]

			if !ok || routeNamespaceValue == nil {
				t.log.Infof("Ignore TargetGroup(triggered by route) %v, %v has no route namespace tag",
					*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
				continue
			}

			routeName := types.NamespacedName{
				Namespace: *routeNamespaceValue,
				Name:      *routeNameValue,
			}

			var route core.Route
			if *sdkTG.getTargetGroupOutput.Config.ProtocolVersion == vpclattice.TargetGroupProtocolVersionGrpc {
				if route, err = core.GetGRPCRoute(ctx, t.client, routeName); err != nil {
					t.log.Infof("Could not find GRPCRoute for target group %s", err)
				}
			} else {
				if route, err = core.GetHTTPRoute(ctx, t.client, routeName); err != nil {
					t.log.Infof("Could not find HTTPRoute for target group %s", err)
				}
			}

			if route != nil {
				tgName := latticestore.TargetGroupName(*srvName, *srvNamespace)

				// We have finished rule reconciliation at this point.
				// If a target group under HTTPRoute does not have any service, it is stale.
				isUsed := t.isTargetGroupUsedByRoute(ctx, tgName, route) &&
					len(sdkTG.getTargetGroupOutput.ServiceArns) > 0
				if isUsed {
					t.log.Infof("Ignore TargetGroup(triggered by route) %v, %v since route object is found",
						*sdkTG.getTargetGroupOutput.Arn, *sdkTG.getTargetGroupOutput.Name)
					continue
				} else {
					t.log.Infof("tgname %v is not used by route %v", tgName, route)
				}
			}
		}

		// the routename for serviceimport is ""
		if tg, err := t.latticeDataStore.GetTargetGroup(*sdkTG.getTargetGroupOutput.Name, "", true); err == nil {
			t.log.Infof("Ignore target group created by service import %v", tg)
			continue
		}

		t.log.Debugf("Append stale SDK TG to stale list Name %v, routename %v, ARN %v",
			*sdkTG.getTargetGroupOutput.Name, tgRouteName, *sdkTG.getTargetGroupOutput.Id)

		staleSDKTGs = append(staleSDKTGs, model.TargetGroup{
			Spec: model.TargetGroupSpec{
				Name: *sdkTG.getTargetGroupOutput.Name,
				Config: model.TargetGroupConfig{
					K8SHTTPRouteName: tgRouteName,
				},
				LatticeID: *sdkTG.getTargetGroupOutput.Id,
			},
		})

	}

	t.log.Infof("SynthesizeSDKTargetGroups, here is the stale target groups list %v stalelen %d", staleSDKTGs, len(staleSDKTGs))

	ret_err := false

	for _, sdkTG := range staleSDKTGs {

		err := t.targetGroupManager.Delete(ctx, &sdkTG)
		t.log.Debugf("SynthesizeSDKTargetGroups, deleting stale target group %v", err)

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

func (t *TargetGroupSynthesizer) isTargetGroupUsedByRoute(ctx context.Context, tgName string, route core.Route) bool {
	for _, rule := range route.Spec().Rules() {
		for _, backendRef := range rule.BackendRefs() {
			if string(*backendRef.Kind()) != "Service" {
				continue
			}
			namespace := route.Namespace()
			if backendRef.Namespace() != nil {
				namespace = string(*backendRef.Namespace())
			}
			refTGName := latticestore.TargetGroupName(string(backendRef.Name()), namespace)

			if tgName == refTGName {
				return true
			}
		}
	}

	return false
}

func (t *TargetGroupSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}

func (t *TargetGroupSynthesizer) SynthesizeTriggeredTargetGroupsCreation(ctx context.Context) error {
	var resTargetGroups []*model.TargetGroup
	var returnErr = false
	t.stack.ListResources(&resTargetGroups)
	t.log.Infof("SynthesizeTriggeredTargetGroupsCreation TargetGroups: [%v]", resTargetGroups)
	for _, resTargetGroup := range resTargetGroups {
		if resTargetGroup.Spec.IsDeleted {
			t.log.Infof("In the SynthesizeTriggeredTargetGroupsCreation(), we only handle TG Creation request and skip any deletion request [%v]", resTargetGroup)
			continue
		}
		if resTargetGroup.Spec.Config.IsServiceImport {
			tgStatus, err := t.targetGroupManager.Get(ctx, resTargetGroup)
			if err != nil {
				t.log.Infof("Error on t.targetGroupManager.Get for %v err %v", resTargetGroup, err)
				returnErr = true
				continue
			}
			// for serviceimport, the httproutename is ""
			t.latticeDataStore.AddTargetGroup(resTargetGroup.Spec.Name,
				resTargetGroup.Spec.Config.VpcID, tgStatus.TargetGroupARN, tgStatus.TargetGroupID,
				resTargetGroup.Spec.Config.IsServiceImport, "")
			t.log.Infof("targetGroup Synthesized successfully for %s: %v", resTargetGroup.Spec.Name, tgStatus)
		} else { // handle TargetGroup creation request that triggered by httproute with backendref k8sService creation or serviceExport creation
			resTargetGroup.Spec.Config.VpcID = config.VpcID
			tgStatus, err := t.targetGroupManager.Create(ctx, resTargetGroup)
			if err != nil {
				t.log.Infof("Error on t.targetGroupManager.Create for %v err %v", resTargetGroup, err)
				returnErr = true
				continue
			}
			//In the ModelBuildTask, it should already add a tg entry in the latticeDataStore,
			//in here, only UPDATE the entry with tgStatus.TargetGroupARN and tgStatus.TargetGroupID
			t.latticeDataStore.AddTargetGroup(resTargetGroup.Spec.Name,
				resTargetGroup.Spec.Config.VpcID, tgStatus.TargetGroupARN,
				tgStatus.TargetGroupID, resTargetGroup.Spec.Config.IsServiceImport,
				resTargetGroup.Spec.Config.K8SHTTPRouteName)
			t.log.Infof("targetGroup Synthesized successfully for %v: %v", resTargetGroup.Spec, tgStatus)
		}
	}
	t.log.Infof("Done -- SynthesizeTriggeredTargetGroupsCreation %v", resTargetGroups)
	if returnErr {
		return errors.New(LATTICE_RETRY)
	} else {
		return nil
	}

}

func (t *TargetGroupSynthesizer) SynthesizeTriggeredTargetGroupsDeletion(ctx context.Context) error {
	var resTargetGroups []*model.TargetGroup
	var returnErr = false
	t.stack.ListResources(&resTargetGroups)

	for _, resTargetGroup := range resTargetGroups {
		t.log.Debugf("SynthesizeTriggeredTargetGroupsDeletion: TargetGroup ==[%v]", *resTargetGroup)

		if !resTargetGroup.Spec.IsDeleted {
			t.log.Infof("SynthesizeTriggeredTargetGroupsDeletion ignoring target group deletion request for tg: [%v]", resTargetGroup)
			continue
		}

		if resTargetGroup.Spec.Config.IsServiceImport {
			t.log.Debugf("Deleting service import target group from local datastore %v", resTargetGroup.Spec.LatticeID)
			t.latticeDataStore.DelTargetGroup(resTargetGroup.Spec.Name, resTargetGroup.Spec.Config.K8SHTTPRouteName, resTargetGroup.Spec.Config.IsServiceImport)
		} else {
			// For delete TargetGroup request triggered by k8s service, invoke vpc lattice api to delete it, if success, delete the tg in the datastore as well
			err := t.targetGroupManager.Delete(ctx, resTargetGroup)
			t.log.Infof("err := t.targetGroupManager.Delete(ctx, resTargetGroup) err: %v", err)
			if err == nil {
				t.log.Infof("Delete Target Group in SynthesizeTriggeredTargetGroupsDeletion: successfully deleted target group %v", resTargetGroup)
				t.latticeDataStore.DelTargetGroup(resTargetGroup.Spec.Name, resTargetGroup.Spec.Config.K8SHTTPRouteName, resTargetGroup.Spec.Config.IsServiceImport)
			} else {
				t.log.Infof("Delete Target Group in SynthesizeTriggeredTargetGroupsDeletion: failed to delete target group %v, err %v", resTargetGroup, err)
				returnErr = true
			}
		}
	}
	if returnErr {
		return errors.New(LATTICE_RETRY)
	} else {
		return nil
	}
}
