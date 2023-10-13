package gateway

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type AccessLogSubscriptionModelBuilder interface {
	Build(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) (core.Stack, *model.AccessLogSubscription, error)
}

type accessLogSubscriptionModelBuilder struct {
	log    gwlog.Logger
	client client.Client
}

func NewAccessLogSubscriptionModelBuilder(log gwlog.Logger, client client.Client) *accessLogSubscriptionModelBuilder {
	return &accessLogSubscriptionModelBuilder{
		client: client,
		log:    log,
	}
}

func (b *accessLogSubscriptionModelBuilder) Build(
	ctx context.Context,
	accessLogPolicy *anv1alpha1.AccessLogPolicy,
) (core.Stack, *model.AccessLogSubscription, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(accessLogPolicy)))

	task := accessLogSubscriptionModelBuildTask{
		log:             b.log,
		stack:           stack,
		accessLogPolicy: accessLogPolicy,
	}

	if err := task.run(ctx); err != nil {
		return nil, nil, err
	}

	return task.stack, task.accessLogSubscription, nil
}

type accessLogSubscriptionModelBuildTask struct {
	log                   gwlog.Logger
	stack                 core.Stack
	accessLogPolicy       *anv1alpha1.AccessLogPolicy
	accessLogSubscription *model.AccessLogSubscription
}

func (t *accessLogSubscriptionModelBuildTask) run(ctx context.Context) error {
	sourceType := model.ServiceSourceType
	if t.accessLogPolicy.Spec.TargetRef.Kind == "Gateway" {
		sourceType = model.ServiceNetworkSourceType
	}

	sourceName, err := utils.TargetRefToLatticeResourceName(t.accessLogPolicy.Spec.TargetRef, t.accessLogPolicy.Namespace)
	if err != nil {
		return err
	}

	destinationArn := t.accessLogPolicy.Spec.DestinationArn
	if destinationArn == nil {
		return fmt.Errorf("access log policy's destinationArn cannot be nil")
	}

	isDeleted := t.accessLogPolicy.DeletionTimestamp != nil

	t.accessLogSubscription = model.NewAccessLogSubscription(t.stack, sourceType, sourceName, *destinationArn, isDeleted)
	err = t.stack.AddResource(t.accessLogSubscription)
	if err != nil {
		return err
	}

	return nil
}
