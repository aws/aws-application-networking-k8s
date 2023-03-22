package gateway

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/glog"

	"github.com/aws/aws-sdk-go/aws"

	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
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
	// when a service is associate to multiple service network(s), all listener config MUST be same
	// so here we are only using the 1st parentRef
	var ruleID = 1
	if len(t.httpRoute.Spec.ParentRefs) > 0 {
		parentRef := t.httpRoute.Spec.ParentRefs[0]
		port, protocol, _, err := t.extractListnerInfo(ctx, parentRef)

		if err != nil {
			glog.V(2).Infof("Error on buildRules %v \n", err)
			return err
		}

		for _, httpRule := range t.httpRoute.Spec.Rules {
			glog.V(6).Infof("Parsing http rule spec: %v\n", httpRule)
			var ruleSpec latticemodel.RuleSpec

			if len(httpRule.Matches) > 1 {
				// only support 1 match today
				glog.V(2).Infof("Do not support multiple matches: matches == %v ", len(httpRule.Matches))
				return errors.New(LATTICE_NO_SUPPORT_FOR_MULTIPLE_MATCHES)
			}

			if len(httpRule.Matches) == 0 {
				glog.V(6).Infof("Continue next rule, no matches specified in current rule")
				continue
			}

			// only support 1 match today
			match := httpRule.Matches[0]

			if match.Path != nil && match.Path.Type != nil {
				glog.V(6).Infof("Examing pathmatch type %v value %v for for httproute %s namespace %s ",
					*match.Path.Type, *match.Path.Value, t.httpRoute.Name, t.httpRoute.Namespace)

				switch *match.Path.Type {
				case gateway_api.PathMatchExact:
					glog.V(6).Infof("Using PathMatchExact for httproute %s namespace %s ",
						t.httpRoute.Name, t.httpRoute.Namespace)
					ruleSpec.PathMatchExact = true

				case gateway_api.PathMatchPathPrefix:
					glog.V(6).Infof("Using PathMatchPathPrefix for httproute %s namespace %s ",
						t.httpRoute.Name, t.httpRoute.Namespace)
					ruleSpec.PathMatchPrefix = true
				default:
					glog.V(2).Infof("Unsupported path match type %v for httproute %s namespace %s",
						*match.Path.Type, t.httpRoute.Name, t.httpRoute.Namespace)
					return errors.New(LATTICE_UNSUPPORTED_PATH_MATCH_TYPE)
				}
				ruleSpec.PathMatchValue = *match.Path.Value
			}

			// header based match
			// today, only support EXACT match
			if match.Headers != nil {
				if len(match.Headers) > LATTICE_MAX_HEADER_MATCHES {
					return errors.New(LATTICE_EXCEED_MAX_HEADER_MATCHES)
				}

				ruleSpec.NumOfHeaderMatches = len(match.Headers)

				glog.V(6).Infof("Examing match.Headers %v for httproute %s namespace %s",
					match.Headers, t.httpRoute.Name, t.httpRoute.Namespace)

				for i, header := range match.Headers {
					glog.V(6).Infof("Examing match.Header: i = %d header.Type %v", i, *header.Type)
					if header.Type != nil && gateway_api.HeaderMatchType(*header.Type) != gateway_api.HeaderMatchExact {
						glog.V(2).Infof("Unsupported header matchtype %v for httproute %v namespace %s",
							*header.Type, t.httpRoute.Name, t.httpRoute.Namespace)
						return errors.New(LATTICE_UNSUPPORTED_HEADER_MATCH_TYPE)
					}

					glog.V(6).Infof("Found HeaderExactMatch==%v for HTTPRoute %v, namespace %v",
						header.Value, t.httpRoute.Name, t.httpRoute.Namespace)

					matchType := vpclattice.HeaderMatchType{
						Exact: aws.String(header.Value),
					}
					ruleSpec.MatchedHeaders[i].Match = &matchType
					header_name := header.Name

					glog.V(6).Infof("Found matching i = %d header_name %v", i, &header_name)

					ruleSpec.MatchedHeaders[i].Name = (*string)(&header_name)
				}
			}

			// controller do not support these match type today
			if match.Method != nil || match.QueryParams != nil {
				glog.V(2).Infof("Unsupported Match Method %v for httproute %v, namespace %v",
					match.Method, t.httpRoute.Name, t.httpRoute.Namespace)
				return errors.New(LATTICE_UNSUPPORTED_MATCH_TYPE)
			}
			glog.V(6).Infof("Generated ruleSpec is: %v", ruleSpec)

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
				protocol, ruleAction, ruleSpec)
			ruleID++

		}
	}

	return nil
}
