package gateway

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"k8s.io/utils/pointer"

	"github.com/golang/glog"

	"github.com/aws/aws-sdk-go/aws"

	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

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
	for _, parentRef := range t.route.Spec().ParentRefs() {
		if parentRef.Name != t.route.Spec().ParentRefs()[0].Name {
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

		for _, rule := range t.route.Spec().Rules() {
			glog.V(6).Infof("Parsing http rule spec: %v\n", rule)
			var ruleSpec latticemodel.RuleSpec

			if len(rule.Matches()) > 1 {
				// only support 1 match today
				glog.V(2).Infof("Do not support multiple matches: matches == %v ", len(rule.Matches()))
				return errors.New(LATTICE_NO_SUPPORT_FOR_MULTIPLE_MATCHES)
			}

			if len(rule.Matches()) == 0 {
				glog.V(6).Infof("Continue next rule, no matches specified in current rule")
				continue
			}

			headersUsedByMethodMatch := 0

			// only support 1 match today
			match := rule.Matches()[0]

			switch m := match.(type) {
			case *core.HTTPRouteMatch:
				if m.Path() != nil && m.Path().Type != nil {
					glog.V(6).Infof("Examing pathmatch type %v value %v for for httproute %s namespace %s ",
						*m.Path().Type, *m.Path().Value, t.route.Name(), t.route.Namespace())

					switch *m.Path().Type {
					case gateway_api_v1beta1.PathMatchExact:
						glog.V(6).Infof("Using PathMatchExact for httproute %s namespace %s ",
							t.route.Name(), t.route.Namespace())
						ruleSpec.PathMatchExact = true

					case gateway_api_v1beta1.PathMatchPathPrefix:
						glog.V(6).Infof("Using PathMatchPathPrefix for httproute %s namespace %s ",
							t.route.Name(), t.route.Namespace())
						ruleSpec.PathMatchPrefix = true
					default:
						glog.V(2).Infof("Unsupported path match type %v for httproute %s namespace %s",
							*m.Path().Type, t.route.Name(), t.route.Namespace())
						return errors.New(LATTICE_UNSUPPORTED_PATH_MATCH_TYPE)
					}
					ruleSpec.PathMatchValue = *m.Path().Value
				}

				// controller do not support these match type today
				if m.Method() != nil || m.QueryParams() != nil {
					glog.V(2).Infof("Unsupported Match Method %v for httproute %v, namespace %v",
						m.Method(), t.route.Name(), t.route.Namespace())
					return errors.New(LATTICE_UNSUPPORTED_MATCH_TYPE)
				}
			case *core.GRPCRouteMatch:
				glog.V(6).Infof("Building rule with GRPCRouteMatch, %v", *m)
				ruleSpec.Method = string(gateway_api_v1beta1.HTTPMethodPost)
				method := m.Method()
				// VPC Lattice doesn't support suffix/regex matching, so we can't support method match without service
				if method.Service == nil {
					return fmt.Errorf("expected gRPC method match to include a service, but was nil")
				}
				switch *method.Type {
				case gateway_api_v1alpha2.GRPCMethodMatchExact:
					if method.Method != nil {
						glog.V(6).Infof("Match by specific gRPC service %s and method %s", *method.Service, *method.Method)
						ruleSpec.MatchedHeaders[0] = vpclattice.HeaderMatch{
							Name: pointer.String(":path"),
							Match: &vpclattice.HeaderMatchType{
								Exact: pointer.String(fmt.Sprintf("/%s/%s", *method.Service, *method.Method)),
							},
						}
					} else {
						glog.V(6).Infof("Match by specific gRPC service %s, regardless of method", *method.Service)
						ruleSpec.MatchedHeaders[0] = vpclattice.HeaderMatch{
							Name: pointer.String(":path"),
							Match: &vpclattice.HeaderMatchType{
								Prefix: pointer.String(fmt.Sprintf("/%s/", *method.Service)),
							},
						}
					}
					headersUsedByMethodMatch = 1
				default:
					return fmt.Errorf("unsupported gRPC method match type %v", *method.Type)
				}
			default:
				return fmt.Errorf("unsupported rule match: %T", m)
			}

			ruleSpec.NumOfHeaderMatches = len(match.Headers()) + headersUsedByMethodMatch

			// header based match
			// today, only support EXACT match
			if match.Headers() != nil {
				if len(match.Headers()) > LATTICE_MAX_HEADER_MATCHES {
					return errors.New(LATTICE_EXCEED_MAX_HEADER_MATCHES)
				}
				if len(match.Headers())+headersUsedByMethodMatch > LATTICE_MAX_HEADER_MATCHES {
					return fmt.Errorf("expected max of %d header matches when using gRPC method match, but was actually %d",
						LATTICE_MAX_HEADER_MATCHES-1, LATTICE_MAX_HEADER_MATCHES)
				}

				glog.V(6).Infof("Examining match.Headers %v for httproute %s namespace %s",
					match.Headers(), t.route.Name(), t.route.Namespace())

				for i, header := range match.Headers() {
					glog.V(6).Infof("Examining match.Header: i = %d header.Type %v", i, *header.Type())
					if header.Type() != nil && *header.Type() != gateway_api_v1beta1.HeaderMatchExact {
						glog.V(2).Infof("Unsupported header matchtype %v for httproute %v namespace %s",
							*header.Type(), t.route.Name(), t.route.Namespace())
						return errors.New(LATTICE_UNSUPPORTED_HEADER_MATCH_TYPE)
					}

					glog.V(6).Infof("Found HeaderExactMatch==%v for HTTPRoute %v, namespace %v",
						header.Value(), t.route.Name(), t.route.Namespace())

					matchType := vpclattice.HeaderMatchType{
						Exact: aws.String(header.Value()),
					}
					ruleSpec.MatchedHeaders[i+headersUsedByMethodMatch].Match = &matchType
					headerName := header.Name()

					glog.V(6).Infof("Found matching i = %d headerName %v", i, &headerName)

					ruleSpec.MatchedHeaders[i+headersUsedByMethodMatch].Name = &headerName
				}
			}

			glog.V(6).Infof("Generated ruleSpec is: %v", ruleSpec)

			tgList := []*latticemodel.RuleTargetGroup{}

			for _, backendRef := range rule.BackendRefs() {
				glog.V(6).Infof("buildRoutingPolicy - examining backendRef %v, backendRef kind: %v",
					backendRef, *backendRef.Kind())

				ruleTG := latticemodel.RuleTargetGroup{}

				if string(*backendRef.Kind()) == "Service" {
					namespace := t.route.Namespace()
					if backendRef.Namespace() != nil {
						namespace = string(*backendRef.Namespace())
					}
					ruleTG.Name = string(backendRef.Name())
					ruleTG.Namespace = namespace
					ruleTG.RouteName = t.route.Name()
					ruleTG.IsServiceImport = false
					if backendRef.Weight() != nil {
						ruleTG.Weight = int64(*backendRef.Weight())
					}

				}

				if string(*backendRef.Kind()) == "ServiceImport" {
					// TODO
					glog.V(6).Infof("Handle ServiceImport Routing Policy\n")
					/* I think this need to be done at policy manager API call
					tg, err := t.Datastore.GetTargetGroup(string(backendRef.Name()),
						"default", true) // isServiceImport==true
					if err != nil {
						glog.V(6).Infof("ServiceImport %s Not found, continue \n",
							string(backendRef.Name()))
						continue

					}
					*/
					ruleTG.Name = string(backendRef.Name())
					ruleTG.Namespace = t.route.Namespace()
					if backendRef.Namespace() != nil {
						ruleTG.Namespace = string(*backendRef.Namespace())
					}
					// the routename for serviceimport is always ""
					ruleTG.RouteName = ""
					ruleTG.IsServiceImport = true

					if backendRef.Weight() != nil {
						ruleTG.Weight = int64(*backendRef.Weight())
					}

				}

				tgList = append(tgList, &ruleTG)
			}

			ruleIDName := fmt.Sprintf("rule-%d", ruleID)
			ruleAction := latticemodel.RuleAction{
				TargetGroups: tgList,
			}
			latticemodel.NewRule(t.stack, ruleIDName, t.route.Name(), t.route.Namespace(), port,
				protocol, ruleAction, ruleSpec)
			ruleID++

		}
	}

	return nil
}
