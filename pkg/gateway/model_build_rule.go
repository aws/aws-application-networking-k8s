package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"

	"github.com/aws/aws-sdk-go/aws"

	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
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
			t.log.Debugf("Ignore parentref of different gateway %s-%s", parentRef.Name, *parentRef.Namespace)
			continue
		}

		port, protocol, _, err := t.extractListenerInfo(ctx, parentRef)
		if err != nil {
			return err
		}

		for _, rule := range t.route.Spec().Rules() {
			var ruleSpec latticemodel.RuleSpec

			if len(rule.Matches()) > 1 {
				// only support 1 match today
				return errors.New(LATTICE_NO_SUPPORT_FOR_MULTIPLE_MATCHES)
			}

			if len(rule.Matches()) == 0 {
				t.log.Debugf("Continue next rule, no matches specified in current rule")
				continue
			}

			// only support 1 match today
			match := rule.Matches()[0]

			switch m := match.(type) {
			case *core.HTTPRouteMatch:
				if m.Path() != nil && m.Path().Type != nil {
					t.log.Debugf("Examining pathmatch type %s value %s for for httproute %s-%s ",
						*m.Path().Type, *m.Path().Value, t.route.Name(), t.route.Namespace())

					switch *m.Path().Type {
					case gateway_api_v1beta1.PathMatchExact:
						t.log.Debugf("Using PathMatchExact for httproute %s-%s ",
							t.route.Name(), t.route.Namespace())
						ruleSpec.PathMatchExact = true

					case gateway_api_v1beta1.PathMatchPathPrefix:
						t.log.Debugf("Using PathMatchPathPrefix for httproute %s-%s ",
							t.route.Name(), t.route.Namespace())
						ruleSpec.PathMatchPrefix = true
					default:
						t.log.Debugf("Unsupported path match type %s for httproute %s-%s",
							*m.Path().Type, t.route.Name(), t.route.Namespace())
						return errors.New(LATTICE_UNSUPPORTED_PATH_MATCH_TYPE)
					}
					ruleSpec.PathMatchValue = *m.Path().Value
				}

				// method based match
				if m.Method() != nil {
					t.log.Infof("Examining http method %s for httproute %s-%s",
						*m.Method(), t.route.Name(), t.route.Namespace())

					ruleSpec.Method = string(*m.Method())
				}

				// controller does not support query matcher type today
				if m.QueryParams() != nil {
					t.log.Infof("Unsupported match type for httproute %s, namespace %s",
						t.route.Name(), t.route.Namespace())
					return errors.New(LATTICE_UNSUPPORTED_MATCH_TYPE)
				}
			case *core.GRPCRouteMatch:
				t.log.Debugf("Building rule with GRPCRouteMatch, %+v", *m)
				ruleSpec.Method = string(gateway_api_v1beta1.HTTPMethodPost)
				method := m.Method()
				// VPC Lattice doesn't support suffix/regex matching, so we can't support method match without service
				if method.Service == nil && method.Method != nil {
					return fmt.Errorf("cannot create GRPCRouteMatch for nil service and non-nil method")
				}
				switch *method.Type {
				case gateway_api_v1alpha2.GRPCMethodMatchExact:
					if method.Service == nil {
						t.log.Debugf("Match all paths due to nil service and nil method")
						ruleSpec.PathMatchPrefix = true
						ruleSpec.PathMatchValue = "/"
					} else if method.Method == nil {
						t.log.Debugf("Match by specific gRPC service %s, regardless of method", *method.Service)
						ruleSpec.PathMatchPrefix = true
						ruleSpec.PathMatchValue = fmt.Sprintf("/%s/", *method.Service)
					} else {
						t.log.Debugf("Match by specific gRPC service %s and method %s", *method.Service, *method.Method)
						ruleSpec.PathMatchExact = true
						ruleSpec.PathMatchValue = fmt.Sprintf("/%s/%s", *method.Service, *method.Method)
					}
				default:
					return fmt.Errorf("unsupported gRPC method match type %s", *method.Type)
				}
			default:
				return fmt.Errorf("unsupported rule match: %T", m)
			}

			// header based match
			// today, only support EXACT match
			if match.Headers() != nil {
				if len(match.Headers()) > LATTICE_MAX_HEADER_MATCHES {
					return errors.New(LATTICE_EXCEED_MAX_HEADER_MATCHES)
				}

				ruleSpec.NumOfHeaderMatches = len(match.Headers())

				t.log.Debugf("Examining match.Headers for route %s-%s", t.route.Name(), t.route.Namespace())

				for i, header := range match.Headers() {
					t.log.Debugf("Examining match.Header: i = %d header.Type %s", i, *header.Type())
					if header.Type() != nil && *header.Type() != gateway_api_v1beta1.HeaderMatchExact {
						t.log.Debugf("Unsupported header matchtype %s for httproute %s-%s",
							*header.Type(), t.route.Name(), t.route.Namespace())
						return errors.New(LATTICE_UNSUPPORTED_HEADER_MATCH_TYPE)
					}

					matchType := vpclattice.HeaderMatchType{
						Exact: aws.String(header.Value()),
					}
					ruleSpec.MatchedHeaders[i].Match = &matchType
					headerName := header.Name()
					ruleSpec.MatchedHeaders[i].Name = &headerName
				}
			}

			var tgList []*latticemodel.RuleTargetGroup

			for _, backendRef := range rule.BackendRefs() {
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
					/* I think this need to be done at policy manager API call
					tg, err := t.datastore.GetTargetGroup(string(backendRef.Name()),
						"default", true) // isServiceImport==true
					if err != nil {
						t.log.Infof("ServiceImport %s Not found, continue \n",
							string(backendRef.Name()))
						continue

					}
					*/
					ruleTG.Name = string(backendRef.Name())
					ruleTG.Namespace = t.route.Namespace()
					if backendRef.Namespace() != nil {
						ruleTG.Namespace = string(*backendRef.Namespace())
					}
					// the routeName for serviceimport is always ""
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
