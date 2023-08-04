package gateway

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"

	"github.com/golang/glog"

	"github.com/aws/aws-sdk-go/aws"

	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-sdk-go/service/vpclattice"
)

const (
	resourceIDRuleConfig = "RuleConfig"
	// error code
	LATTICE_NO_SUPPORT_FOR_MULTIPLE_MATCHES = "LATTICE_NO_SUPPORT_FOR_MULTIPLE_MATCHES"

	LATTICE_EXCEED_MAX_HEADER_MATCHES = "LATTICE_EXCEED_MAX_HEADER_MATCHES"

	LATTICE_UNSUPPORTED_MATCH_TYPE = "LATTICE_UNSUPPORTED_MATCH_TYPE"

	LATTICE_UNSUPPORTED_HEADER_MATCH_TYPE = "LATTICE_UNSUPPORTED_HEADER_MATCH_TYPE"

	LATTICE_UNSUPPORTED_PATH_MATCH_TYPE = "LATTICE_UNSUPPORTED_PATH_MATCH_TYPE"

	LATTICE_MAX_HEADER_MATCHES = 5
)

func (t *latticeServiceModelBuildTask) buildRules(ctx context.Context) error {

	var ruleID = 1
	for _, parentRef := range t.route.GetSpec().GetParentRefs() {
		if parentRef.Name != t.route.GetSpec().GetParentRefs()[0].Name {
			// when a service is associate to multiple service network(s), all listener config MUST be same
			// so here we are only using the 1st gateway
			glog.V(2).Infof("Ignore parentref of different gateway %v", parentRef.Name)

			continue
		}
		port, protocol, _, err := t.extractListenerInfo(ctx, parentRef)

		if err != nil {
			glog.V(2).Infof("Error on buildRules %v \n", err)
			return err
		}

		for _, rule := range t.route.GetSpec().GetRules() {
			glog.V(6).Infof("Parsing http rule spec: %v\n", rule)
			var ruleSpec latticemodel.RuleSpec

			if len(rule.GetMatches()) > 1 {
				// only support 1 match today
				glog.V(2).Infof("Do not support multiple matches: matches == %v ", len(rule.GetMatches()))
				return errors.New(LATTICE_NO_SUPPORT_FOR_MULTIPLE_MATCHES)
			}

			if len(rule.GetMatches()) == 0 {
				glog.V(6).Infof("Continue next rule, no matches specified in current rule")
				continue
			}

			// only support 1 match today
			match := rule.GetMatches()[0]

			if httpMatch, ok := match.(core.HTTPRouteMatch); ok {
				if httpMatch.Path != nil && httpMatch.Path.Type != nil {
					glog.V(6).Infof("Examing pathmatch type %v value %v for for httproute %s namespace %s ",
						*httpMatch.Path.Type, *httpMatch.Path.Value, t.route.GetName(), t.route.GetNamespace())

					switch *httpMatch.Path.Type {
					case gateway_api.PathMatchExact:
						glog.V(6).Infof("Using PathMatchExact for httproute %s namespace %s ",
							t.route.GetName(), t.route.GetNamespace())
						ruleSpec.PathMatchExact = true

					case gateway_api.PathMatchPathPrefix:
						glog.V(6).Infof("Using PathMatchPathPrefix for httproute %s namespace %s ",
							t.route.GetName(), t.route.GetNamespace())
						ruleSpec.PathMatchPrefix = true
					default:
						glog.V(2).Infof("Unsupported path match type %v for httproute %s namespace %s",
							*httpMatch.Path.Type, t.route.GetName(), t.route.GetNamespace())
						return errors.New(LATTICE_UNSUPPORTED_PATH_MATCH_TYPE)
					}
					ruleSpec.PathMatchValue = *httpMatch.Path.Value
				}

				// controller do not support these match type today
				if httpMatch.Method != nil || httpMatch.QueryParams != nil {
					glog.V(2).Infof("Unsupported Match Method %v for httproute %v, namespace %v",
						httpMatch.Method, t.route.GetName(), t.route.GetNamespace())
					return errors.New(LATTICE_UNSUPPORTED_MATCH_TYPE)
				}
			}

			// header based match
			// today, only support EXACT match
			if match.GetHeaders() != nil {
				if len(match.GetHeaders()) > LATTICE_MAX_HEADER_MATCHES {
					return errors.New(LATTICE_EXCEED_MAX_HEADER_MATCHES)
				}

				ruleSpec.NumOfHeaderMatches = len(match.GetHeaders())

				glog.V(6).Infof("Examing match.Headers %v for httproute %s namespace %s",
					match.GetHeaders(), t.route.GetName(), t.route.GetNamespace())

				for i, header := range match.GetHeaders() {
					glog.V(6).Infof("Examing match.Header: i = %d header.Type %v", i, *header.GetType())
					if header.GetType() != nil && *header.GetType() != gateway_api.HeaderMatchExact {
						glog.V(2).Infof("Unsupported header matchtype %v for httproute %v namespace %s",
							*header.GetType(), t.route.GetName(), t.route.GetNamespace())
						return errors.New(LATTICE_UNSUPPORTED_HEADER_MATCH_TYPE)
					}

					glog.V(6).Infof("Found HeaderExactMatch==%v for HTTPRoute %v, namespace %v",
						header.GetValue(), t.route.GetName(), t.route.GetNamespace())

					matchType := vpclattice.HeaderMatchType{
						Exact: aws.String(header.GetValue()),
					}
					ruleSpec.MatchedHeaders[i].Match = &matchType
					header_name := header.GetName()

					glog.V(6).Infof("Found matching i = %d header_name %v", i, &header_name)

					ruleSpec.MatchedHeaders[i].Name = &header_name
				}
			}

			glog.V(6).Infof("Generated ruleSpec is: %v", ruleSpec)

			tgList := []*latticemodel.RuleTargetGroup{}

			for _, backendRef := range rule.GetBackendRefs() {
				glog.V(6).Infof("buildRoutingPolicy - examining backendRef %v\n", backendRef)
				glog.V(6).Infof("backendRef kind: %v\n", *backendRef.GetKind())

				ruleTG := latticemodel.RuleTargetGroup{}

				if string(*backendRef.GetKind()) == "Service" {
					namespace := t.route.GetNamespace()
					if backendRef.GetNamespace() != nil {
						namespace = string(*backendRef.GetNamespace())
					}
					ruleTG.Name = string(backendRef.GetName())
					ruleTG.Namespace = namespace
					ruleTG.RouteName = t.route.GetName()
					ruleTG.IsServiceImport = false
					if backendRef.GetWeight() != nil {
						ruleTG.Weight = int64(*backendRef.GetWeight())
					}

				}

				if string(*backendRef.GetKind()) == "ServiceImport" {
					// TODO
					glog.V(6).Infof("Handle ServiceImport Routing Policy\n")
					/* I think this need to be done at policy manager API call
					tg, err := t.Datastore.GetTargetGroup(string(backendRef.GetName()),
						"default", true) // isServiceImport==true
					if err != nil {
						glog.V(6).Infof("ServiceImport %s Not found, continue \n",
							string(backendRef.GetName()))
						continue

					}
					*/
					ruleTG.Name = string(backendRef.GetName())
					ruleTG.Namespace = t.route.GetNamespace()
					if backendRef.GetNamespace() != nil {
						ruleTG.Namespace = string(*backendRef.GetNamespace())
					}
					// the routename for serviceimport is always ""
					ruleTG.RouteName = ""
					ruleTG.IsServiceImport = true

					if backendRef.GetWeight() != nil {
						ruleTG.Weight = int64(*backendRef.GetWeight())
					}

				}

				tgList = append(tgList, &ruleTG)
			}

			ruleIDName := fmt.Sprintf("rule-%d", ruleID)
			ruleAction := latticemodel.RuleAction{
				TargetGroups: tgList,
			}
			latticemodel.NewRule(t.stack, ruleIDName, t.route.GetName(), t.route.GetNamespace(), port,
				protocol, ruleAction, ruleSpec)
			ruleID++

		}
	}

	return nil
}
