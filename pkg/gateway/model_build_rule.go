package gateway

import (
	"context"
	"fmt"
	"github.com/golang/glog"

	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

const (
	resourceIDRuleConfig = "RuleConfig"
)

func (t *latticeServiceModelBuildTask) buildRules(ctx context.Context) error {
	var ruleID = 1
	for _, parentRef := range t.httpRoute.Spec.ParentRefs {
		port, protocol, err := t.extractListnerInfo(ctx, parentRef)

		if err != nil {
			glog.V(6).Infof("Error on buildRules %v \n", err)
			return err
		}

		for _, httpRule := range t.httpRoute.Spec.Rules {
			glog.V(6).Infof("buildRoutingPolicy, examping rules spec: %v\n", httpRule)
			var ruleValue string

			if len(httpRule.Matches) != 0 {
				// only handle 1 match for now using  path
				if httpRule.Matches[0].Path.Value != nil {
					ruleValue = *httpRule.Matches[0].Path.Value
				}
			}

			tgList := []*latticemodel.RuleTargetGroup{}

			for _, httpBackendRef := range httpRule.BackendRefs {
				glog.V(6).Infof("buildRoutingPolicy - examing backendRef %v\n", httpBackendRef)
				glog.V(6).Infof("backendref kind: %v\n", *httpBackendRef.BackendObjectReference.Kind)

				ruleTG := latticemodel.RuleTargetGroup{}

				if string(*httpBackendRef.BackendObjectReference.Kind) == "Service" {
					namespace := "default"
					if httpBackendRef.BackendObjectReference.Namespace != nil {
						namespace = string(*httpBackendRef.BackendObjectReference.Namespace)
					}
					ruleTG.Name = string(httpBackendRef.BackendObjectReference.Name)
					ruleTG.Namespace = namespace
					ruleTG.IsServiceImport = false
					if httpBackendRef.Weight != nil {
						ruleTG.Weight = int64(*httpBackendRef.Weight)
					}

				}

				if string(*httpBackendRef.BackendObjectReference.Kind) == "ServiceImport" {
					// TODO
					glog.V(6).Infof("Handle ServiceImport Routing Policy\n")
					/* I think this need to be done at policy manager API call
					tg, err := t.Datastore.GetTargetGroup(string(httpBackendRef.BackendObjectReference.Name),
						"default", true) // isServiceImport==true
					if err != nil {
						glog.V(6).Infof("ServiceImport %s Not found, continue \n",
							string(httpBackendRef.BackendObjectReference.Name))
						continue

					}
					*/
					ruleTG.Name = string(httpBackendRef.BackendObjectReference.Name)
					ruleTG.Namespace = "default"
					if httpBackendRef.BackendObjectReference.Namespace != nil {
						ruleTG.Namespace = string(*httpBackendRef.BackendObjectReference.Namespace)
					}
					ruleTG.IsServiceImport = true

					if httpBackendRef.Weight != nil {
						ruleTG.Weight = int64(*httpBackendRef.Weight)
					}

				}

				tgList = append(tgList, &ruleTG)
			}

			ruleIDName := fmt.Sprintf("rule-%d", ruleID)
			ruleAction := latticemodel.RuleAction{
				TargetGroups: tgList,
			}
			latticemodel.NewRule(t.stack, ruleIDName, t.httpRoute.Name, t.httpRoute.Namespace, port,
				protocol, latticemodel.MatchByPath, ruleValue, ruleAction)
			ruleID++

		}
	}

	return nil
}
