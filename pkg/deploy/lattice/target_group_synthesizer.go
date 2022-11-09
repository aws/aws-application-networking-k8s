package lattice

import (
	"context"
	"errors"
	"github.com/golang/glog"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

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
	var ret = ""

	glog.V(6).Infof("Start synthesizing TargetGroupss ...\n")

	if err := t.SynthesizeTriggeredTargetGroup(ctx); err != nil {
		ret = LATTICE_RETRY
	}

	/* TODO,  resolve bug that this might delete other HTTPRoute's TG before they have chance
	 * to be reconcile during controller restart
	if err := t.SynthesizeSDKTargetGroups(ctx); err != nil {
		ret = LATTICE_RETRY
	}
	*/

	if ret != "" {
		return errors.New(ret)
	} else {
		return nil
	}
}

func (t *targetGroupSynthesizer) SynthesizeTriggeredTargetGroup(ctx context.Context) error {
	var resTargetGroups []*latticemodel.TargetGroup
	var returnErr = false

	t.stack.ListResources(&resTargetGroups)

	glog.V(6).Infof("Synthesize TargetGroups ==[%v]\n", resTargetGroups)

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
			if resTargetGroup.Spec.IsDeleted {
				//Ingnore TG delete since this is an import from elsewhere
				continue
			}
			tgStatus, err := t.targetGroupManager.Get(ctx, resTargetGroup)

			if err != nil {
				glog.V(6).Infof("Error on t.targetGroupManager.Get for %v err %v\n", resTargetGroup, err)
				returnErr = true
				continue
			}

			t.latticeDataStore.AddTargetGroup(resTargetGroup.Spec.Name,
				resTargetGroup.Spec.Config.VpcID, tgStatus.TargetGroupARN, tgStatus.TargetGroupID, resTargetGroup.Spec.Config.IsServiceImport)

			glog.V(6).Infof("targetGroup Synthesized successfully for %s: %v\n", resTargetGroup.Spec.Name, tgStatus)

		} else {
			if resTargetGroup.Spec.IsDeleted {
				err := t.targetGroupManager.Delete(ctx, resTargetGroup)

				if err != nil {
					returnErr = true
					continue
				} else {
					glog.V(6).Infof("Synthersizing Target Group: successfully deleted target group %v\n", resTargetGroup)
					t.latticeDataStore.DelTargetGroup(resTargetGroup.Spec.Name, false)
				}

			} else {
				resTargetGroup.Spec.Config.VpcID = config.VpcID

				tgStatus, err := t.targetGroupManager.Create(ctx, resTargetGroup)

				if err != nil {
					glog.V(6).Infof("Error on t.targetGroupManager.Create for %v err %v\n", resTargetGroup, err)
					returnErr = true
					continue
				}

				t.latticeDataStore.AddTargetGroup(resTargetGroup.Spec.Name,
					resTargetGroup.Spec.Config.VpcID, tgStatus.TargetGroupARN, tgStatus.TargetGroupID, resTargetGroup.Spec.Config.IsServiceImport)

				glog.V(6).Infof("targetGroup Synthesized successfully for %v: %v\n", resTargetGroup.Spec, tgStatus)
			}
		}

	}

	glog.V(6).Infof("Done -- SynthesizeTriggeredTargetGroup %v\n", resTargetGroups)

	if returnErr {
		return errors.New("LATTICE-RETRY")
	} else {
		return nil
	}

}

func (t *targetGroupSynthesizer) SynthesizeSDKTargetGroups(ctx context.Context) error {

	staleSDKTGs := []latticemodel.TargetGroup{}
	sdkTGs, err := t.targetGroupManager.List(ctx)

	if err != nil {
		glog.V(6).Infof("SynthesizeSDKTargetGroups: failed to retrieve sdk TGs %v\n", err)
		return nil
	}

	glog.V(6).Infof("SynthesizeSDKTargetGroups: here is sdkTGs %v len %v \n", sdkTGs, len(sdkTGs))

	for _, sdkTG := range sdkTGs {

		if *sdkTG.Config.VpcIdentifier != config.VpcID {
			glog.V(6).Infof("Ignore target group for other VPCs, other :%v, current vpc %v\n", *sdkTG.Config.VpcIdentifier, config.VpcID)
			continue
		}
		if tg, err := t.latticeDataStore.GetTargetGroup(*sdkTG.Name, true); err == nil {
			glog.V(6).Infof("Ignore target group created by service import %v\n", tg)
			continue
		}

		tg, err := t.latticeDataStore.GetTargetGroup(*sdkTG.Name, false)
		if err != nil {
			staleSDKTGs = append(staleSDKTGs, latticemodel.TargetGroup{
				Spec: latticemodel.TargetGroupSpec{
					Name:      *sdkTG.Name,
					LatticeID: *sdkTG.Id,
				},
			})
		} else {
			if tg.ByServiceExport {
				continue
			}

			if t.isTargetGroupUsedByHTTPRoute(ctx, *sdkTG.Name) {
				continue
			}
			staleSDKTGs = append(staleSDKTGs, latticemodel.TargetGroup{
				Spec: latticemodel.TargetGroupSpec{
					Name:      *sdkTG.Name,
					LatticeID: *sdkTG.Id,
				},
			})

		}

	}

	glog.V(6).Infof("SynthesizeSDKTargetGroups, here is the stale target groups list %v stalelen %d\n", staleSDKTGs, len(staleSDKTGs))

	ret_err := false

	for _, sdkTG := range staleSDKTGs {

		err := t.targetGroupManager.Delete(ctx, &sdkTG)
		glog.V(6).Infof("SynthesizeSDKTargetGroups, deleting stale target group %v \n", err)

		// TODO find out the error code
		if err != nil && !strings.Contains(err.Error(), "ConflictException") {
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

// TODO put following routine to common library
func (t *targetGroupSynthesizer) isTargetGroupUsedByHTTPRoute(ctx context.Context, tgName string) bool {

	httpRouteList := &v1alpha2.HTTPRouteList{}

	t.client.List(ctx, httpRouteList)

	//glog.V(6).Infof("isTargetGroupUsedByHTTPRoute: tgName %v-- %v\n", tgName, httpRouteList)

	for _, httpRoute := range httpRouteList.Items {
		for _, httpRule := range httpRoute.Spec.Rules {
			for _, httpBackendRef := range httpRule.BackendRefs {
				//glog.V(6).Infof("isTargetGroupUsedByHTTPRoute: httpBackendRef: %v \n", httpBackendRef)
				if string(*httpBackendRef.BackendObjectReference.Kind) != "Service" {
					continue
				}
				namespace := "default"

				if httpBackendRef.BackendObjectReference.Namespace != nil {
					namespace = string(*httpBackendRef.BackendObjectReference.Namespace)
				}
				refTGName := latticestore.TargetGroupName(string(httpBackendRef.BackendObjectReference.Name), namespace)
				//glog.V(6).Infof("refTGName: %s\n", refTGName)

				if tgName == refTGName {
					return true
				}

			}
		}
	}

	return false
}

func (t *targetGroupSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
