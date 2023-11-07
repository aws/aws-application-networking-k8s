package gateway

import (
	"context"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type LatticeServiceBuilder interface {
	Build(ctx context.Context, httpRoute core.Route) (core.Stack, error)
}

type LatticeServiceModelBuilder struct {
	log         gwlog.Logger
	client      client.Client
	brTgBuilder BackendRefTargetGroupModelBuilder
}

func NewLatticeServiceBuilder(
	log gwlog.Logger,
	client client.Client,
	brTgBuilder BackendRefTargetGroupModelBuilder,
) *LatticeServiceModelBuilder {
	return &LatticeServiceModelBuilder{
		log:         log,
		client:      client,
		brTgBuilder: brTgBuilder,
	}
}

func (b *LatticeServiceModelBuilder) Build(
	ctx context.Context,
	route core.Route,
) (core.Stack, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(route.K8sObject())))

	task := &latticeServiceModelBuildTask{
		log:         b.log,
		route:       route,
		stack:       stack,
		client:      b.client,
		brTgBuilder: b.brTgBuilder,
	}

	if err := task.run(ctx); err != nil {
		return task.stack, err
	}

	return task.stack, nil
}

func (t *latticeServiceModelBuildTask) run(ctx context.Context) error {
	err := t.buildModel(ctx)
	return err
}

func (t *latticeServiceModelBuildTask) buildModel(ctx context.Context) error {
	modelSvc, err := t.buildLatticeService(ctx)
	if err != nil {
		return err
	}

	err = t.buildListeners(ctx, modelSvc.ID())
	if err != nil {
		return fmt.Errorf("failed to build listener due to %w", err)
	}

	var modelListeners []*model.Listener
	err = t.stack.ListResources(&modelListeners)
	if err != nil {
		return err
	}
	t.log.Debugf("Building rules for %d listeners", len(modelListeners))
	for _, modelListener := range modelListeners {
		// building rules will also build target groups and targets as needed
		// even on delete we try to build everything we may then need to remove
		err = t.buildRules(ctx, modelListener.ID())
		if err != nil {
			return fmt.Errorf("failed to build rules due to %w", err)
		}
	}

	return nil
}

func (t *latticeServiceModelBuildTask) buildLatticeService(ctx context.Context) (*model.Service, error) {
	routeType := core.HttpRouteType
	if _, ok := t.route.(*core.GRPCRoute); ok {
		routeType = core.GrpcRouteType
	}

	spec := model.ServiceSpec{
		ServiceTagFields: model.ServiceTagFields{
			RouteName:      t.route.Name(),
			RouteNamespace: t.route.Namespace(),
			RouteType:      routeType,
		},
	}

	for _, parentRef := range t.route.Spec().ParentRefs() {
		spec.ServiceNetworkNames = append(spec.ServiceNetworkNames, string(parentRef.Name))
	}
	defaultGateway, err := config.GetClusterLocalGateway()
	if err == nil {
		spec.ServiceNetworkNames = append(spec.ServiceNetworkNames, defaultGateway)
	}

	if len(t.route.Spec().Hostnames()) > 0 {
		// The 1st hostname will be used as lattice customer-domain-name
		spec.CustomerDomainName = string(t.route.Spec().Hostnames()[0])

		t.log.Infof("Setting customer-domain-name: %s for route %s-%s",
			spec.CustomerDomainName, t.route.Name(), t.route.Namespace())
	} else {
		t.log.Infof("No custom-domain-name for route %s-%s",
			t.route.Name(), t.route.Namespace())
		spec.CustomerDomainName = ""
	}

	certArn, err := t.getACMCertArn(ctx)
	if err != nil {
		return nil, err
	}
	spec.CustomerCertARN = certArn

	svc, err := model.NewLatticeService(t.stack, spec)
	if err != nil {
		return nil, err
	}

	t.log.Debugf("Added service %s to the stack (ID %s)", svc.Spec.LatticeServiceName(), svc.ID())
	svc.IsDeleted = !t.route.DeletionTimestamp().IsZero()
	return svc, nil
}

// returns empty string if not found
func (t *latticeServiceModelBuildTask) getACMCertArn(ctx context.Context) (string, error) {
	gw, err := t.getGateway(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) && !t.route.DeletionTimestamp().IsZero() {
			return "", nil // ok if we're deleting the route
		}
		return "", err
	}

	for _, parentRef := range t.route.Spec().ParentRefs() {
		if parentRef.Name != t.route.Spec().ParentRefs()[0].Name {
			// when a service is associate to multiple service network(s), all listener config MUST be same
			// so here we are only using the 1st gateway
			t.log.Debugf("Ignore ParentRef of different gateway %s-%s", parentRef.Name, parentRef.Namespace)
			continue
		}

		if parentRef.SectionName == nil {
			continue
		}

		for _, section := range gw.Spec.Listeners {
			if section.Name == *parentRef.SectionName && section.TLS != nil {
				if section.TLS.Mode != nil && *section.TLS.Mode == gwv1beta1.TLSModeTerminate {
					curCertARN, ok := section.TLS.Options[awsCustomCertARN]
					if ok {
						t.log.Debugf("Found certification %s under section %s", curCertARN, section.Name)
						return string(curCertARN), nil
					}
				}
				break
			}
		}
	}

	return "", nil
}

type latticeServiceModelBuildTask struct {
	log         gwlog.Logger
	route       core.Route
	client      client.Client
	stack       core.Stack
	brTgBuilder BackendRefTargetGroupModelBuilder
}
