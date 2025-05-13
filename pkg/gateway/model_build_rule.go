package gateway

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

const (
	LATTICE_NO_SUPPORT_FOR_MULTIPLE_MATCHES = "LATTICE_NO_SUPPORT_FOR_MULTIPLE_MATCHES"
	LATTICE_EXCEED_MAX_HEADER_MATCHES       = "LATTICE_EXCEED_MAX_HEADER_MATCHES"
	LATTICE_UNSUPPORTED_MATCH_TYPE          = "LATTICE_UNSUPPORTED_MATCH_TYPE"
	LATTICE_UNSUPPORTED_HEADER_MATCH_TYPE   = "LATTICE_UNSUPPORTED_HEADER_MATCH_TYPE"
	LATTICE_UNSUPPORTED_PATH_MATCH_TYPE     = "LATTICE_UNSUPPORTED_PATH_MATCH_TYPE"
	LATTICE_MAX_HEADER_MATCHES              = 5
)

func (t *latticeServiceModelBuildTask) buildRules(ctx context.Context, stackListenerId string) error {
	// note we only build rules for non-deleted routes
	t.log.Debugf(ctx, "Processing %d rules", len(t.route.Spec().Rules()))

	// Skip if route is being deleted
	if !t.route.DeletionTimestamp().IsZero() {
		t.log.Debugf(ctx, "Skipping rule creation since the route is deleted")
		return nil
	}

	// Create a priority queue to organize rules
	pq := make(utils.PriorityQueue, 0, len(t.route.Spec().Rules()))

	// First pass: build all rules and add them to priority queue
	for i, rule := range t.route.Spec().Rules() {
		// Default priority is index + 1
		priority := int64(i + 1)

		// Check for priority annotation in format: application-networking.k8s.aws/rule-{index}-priority
		if priorityStr, ok := t.route.K8sObject().GetAnnotations()[fmt.Sprintf("application-networking.k8s.aws/rule-%d-priority", i)]; ok {
			if p, err := strconv.ParseInt(priorityStr, 10, 64); err == nil {
				priority = p
				t.log.Debugf(ctx, "Using priority %d from annotation for rule %d", priority, i)
			} else {
				t.log.Warnf(ctx, "Invalid priority value in annotation for rule %d: %s", i, priorityStr)
			}
		}

		ruleSpec := model.RuleSpec{
			StackListenerId: stackListenerId,
			Priority:        priority,
		}

		if len(rule.Matches()) > 1 {
			// only support 1 match today
			return errors.New(LATTICE_NO_SUPPORT_FOR_MULTIPLE_MATCHES)
		} else if len(rule.Matches()) > 0 {
			t.log.Debugf(ctx, "Processing rule match")
			match := rule.Matches()[0]

			switch m := match.(type) {
			case *core.HTTPRouteMatch:
				if err := t.updateRuleSpecForHttpRoute(m, &ruleSpec); err != nil {
					return err
				}
			case *core.GRPCRouteMatch:
				if err := t.updateRuleSpecForGrpcRoute(m, &ruleSpec); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported rule match: %T", m)
			}

			if err := t.updateRuleSpecWithHeaderMatches(match, &ruleSpec); err != nil {
				return err
			}
		} else {

			// Match every traffic on no matches
			ruleSpec.PathMatchValue = "/"
			ruleSpec.PathMatchPrefix = true
			if _, ok := rule.(*core.GRPCRouteRule); ok {
				ruleSpec.Method = string(gwv1.HTTPMethodPost)
			}

		}

		ruleTgList, err := t.getTargetGroupsForRuleAction(ctx, rule)
		if err != nil {
			return err
		}

		ruleSpec.Action = model.RuleAction{
			TargetGroups: ruleTgList,
		}

		// Create the rule but don't add to stack yet
		stackRule, err := model.NewRule(t.stack, ruleSpec)
		if err != nil {
			return err
		}

		// Add to priority queue
		pq.Push(&utils.Item{
			Value:    stackRule,
			Priority: int32(priority),
		})
	}

	// Initialize heap
	heap.Init(&pq)

	// Add rules to stack in priority order
	for pq.Len() > 0 {
		item := pq.Pop().(*utils.Item)
		stackRule := item.Value.(*model.Rule)
		t.stack.AddResource(stackRule)
		t.log.Debugf(ctx, "Added rule %d to the stack (ID %s)", stackRule.Spec.Priority, stackRule.ID())
	}

	return nil
}

func (t *latticeServiceModelBuildTask) updateRuleSpecForHttpRoute(m *core.HTTPRouteMatch, ruleSpec *model.RuleSpec) error {
	hasPath := m.Path() != nil
	hasType := hasPath && m.Path().Type != nil

	if hasPath && !hasType {
		return errors.New("type is required on path match")
	}

	if hasPath {
		t.log.Debugf(context.TODO(), "Examining pathmatch type %s value %s for for httproute %s-%s ",
			*m.Path().Type, *m.Path().Value, t.route.Name(), t.route.Namespace())

		switch *m.Path().Type {
		case gwv1.PathMatchExact:
			t.log.Debugf(context.TODO(), "Using PathMatchExact for httproute %s-%s ",
				t.route.Name(), t.route.Namespace())
			ruleSpec.PathMatchExact = true

		case gwv1.PathMatchPathPrefix:
			t.log.Debugf(context.TODO(), "Using PathMatchPathPrefix for httproute %s-%s ",
				t.route.Name(), t.route.Namespace())
			ruleSpec.PathMatchPrefix = true
		default:
			t.log.Debugf(context.TODO(), "Unsupported path match type %s for httproute %s-%s",
				*m.Path().Type, t.route.Name(), t.route.Namespace())
			return errors.New(LATTICE_UNSUPPORTED_PATH_MATCH_TYPE)
		}
		ruleSpec.PathMatchValue = *m.Path().Value
	}

	// method based match
	if m.Method() != nil {
		t.log.Infof(context.TODO(), "Examining http method %s for httproute %s-%s",
			*m.Method(), t.route.Name(), t.route.Namespace())

		ruleSpec.Method = string(*m.Method())
	}

	// controller does not support query matcher type today
	if m.QueryParams() != nil {
		t.log.Infof(context.TODO(), "Unsupported match type for httproute %s, namespace %s",
			t.route.Name(), t.route.Namespace())
		return errors.New(LATTICE_UNSUPPORTED_MATCH_TYPE)
	}
	return nil
}

func (t *latticeServiceModelBuildTask) updateRuleSpecForGrpcRoute(m *core.GRPCRouteMatch, ruleSpec *model.RuleSpec) error {
	t.log.Debugf(context.TODO(), "Building rule with GRPCRouteMatch, %+v", *m)
	ruleSpec.Method = string(gwv1.HTTPMethodPost) // GRPC is always POST
	method := m.Method()
	// VPC Lattice doesn't support suffix/regex matching, so we can't support method match without service
	if method.Service == nil && method.Method != nil {
		return fmt.Errorf("cannot create GRPCRouteMatch for nil service and non-nil method")
	}
	switch *method.Type {
	case gwv1.GRPCMethodMatchExact:
		if method.Service == nil {
			t.log.Debugf(context.TODO(), "Match all paths due to nil service and nil method")
			ruleSpec.PathMatchPrefix = true
			ruleSpec.PathMatchValue = "/"
		} else if method.Method == nil {
			t.log.Debugf(context.TODO(), "Match by specific gRPC service %s, regardless of method", *method.Service)
			ruleSpec.PathMatchPrefix = true
			ruleSpec.PathMatchValue = fmt.Sprintf("/%s/", *method.Service)
		} else {
			t.log.Debugf(context.TODO(), "Match by specific gRPC service %s and method %s", *method.Service, *method.Method)
			ruleSpec.PathMatchExact = true
			ruleSpec.PathMatchValue = fmt.Sprintf("/%s/%s", *method.Service, *method.Method)
		}
	default:
		return fmt.Errorf("unsupported gRPC method match type %s", *method.Type)
	}
	return nil
}

func (t *latticeServiceModelBuildTask) updateRuleSpecWithHeaderMatches(match core.RouteMatch, ruleSpec *model.RuleSpec) error {
	if match.Headers() == nil {
		return nil
	}

	if len(match.Headers()) > LATTICE_MAX_HEADER_MATCHES {
		return errors.New(LATTICE_EXCEED_MAX_HEADER_MATCHES)
	}

	t.log.Debugf(context.TODO(), "Examining match headers for route %s-%s", t.route.Name(), t.route.Namespace())

	for _, header := range match.Headers() {
		t.log.Debugf(context.TODO(), "Examining match.Header: header.Type %s", *header.Type())
		if header.Type() != nil && *header.Type() != gwv1.HeaderMatchExact {
			t.log.Debugf(context.TODO(), "Unsupported header matchtype %s for httproute %s-%s",
				*header.Type(), t.route.Name(), t.route.Namespace())
			return errors.New(LATTICE_UNSUPPORTED_HEADER_MATCH_TYPE)
		}

		matchType := vpclattice.HeaderMatchType{
			Exact: aws.String(header.Value()),
		}
		headerName := header.Name()

		headerMatch := vpclattice.HeaderMatch{}
		headerMatch.Match = &matchType
		headerMatch.Name = &headerName

		ruleSpec.MatchedHeaders = append(ruleSpec.MatchedHeaders, headerMatch)
	}

	return nil
}

func (t *latticeServiceModelBuildTask) getTargetGroupsForRuleAction(ctx context.Context, rule core.RouteRule) ([]*model.RuleTargetGroup, error) {
	var tgList []*model.RuleTargetGroup

	for _, backendRef := range rule.BackendRefs() {
		ruleTG := model.RuleTargetGroup{
			Weight: 1, // default value according to spec
		}
		if backendRef.Weight() != nil {
			ruleTG.Weight = int64(*backendRef.Weight())
		}

		namespace := t.route.Namespace()
		if backendRef.Namespace() != nil {
			namespace = string(*backendRef.Namespace())
		}

		t.log.Debugf(ctx, "Processing %s backendRef %s-%s", string(*backendRef.Kind()), backendRef.Name(), namespace)

		if string(*backendRef.Kind()) == "ServiceImport" {
			// there needs to be a pre-existing target group, we fetch all the fields
			// needed to identify it
			svcImportTg := model.SvcImportTargetGroup{
				K8SServiceNamespace: namespace,
				K8SServiceName:      string(backendRef.Name()),
			}

			// if there's a matching top-level service import, we can get additional fields
			svcImportName := types.NamespacedName{
				Namespace: namespace,
				Name:      string(backendRef.Name()),
			}
			svcImport := &anv1alpha1.ServiceImport{}
			if err := t.client.Get(ctx, svcImportName, svcImport); err != nil {
				if !apierrors.IsNotFound(err) {
					return nil, err
				}
			}
			vpc, ok := svcImport.Annotations["application-networking.k8s.aws/aws-vpc"]
			if ok {
				svcImportTg.VpcId = vpc
			}

			eksCluster, ok := svcImport.Annotations["application-networking.k8s.aws/aws-eks-cluster-name"]
			if ok {
				svcImportTg.K8SClusterName = eksCluster
			}
			ruleTG.SvcImportTG = &svcImportTg
		}

		if string(*backendRef.Kind()) == "Service" {
			// generate the actual target group model for the backendRef
			_, tg, err := t.brTgBuilder.Build(ctx, t.route, backendRef, t.stack)
			if err != nil {
				ibre := &InvalidBackendRefError{}
				if !errors.As(err, &ibre) {
					return nil, err
				}

				t.log.Infof(ctx, "Invalid backendRef found on route %s", t.route.Name())
				ruleTG.StackTargetGroupId = model.InvalidBackendRefTgId
			} else {
				ruleTG.StackTargetGroupId = tg.ID()
			}
		}

		tgList = append(tgList, &ruleTG)
	}

	return tgList, nil
}
