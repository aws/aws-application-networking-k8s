package gateway

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"sigs.k8s.io/controller-runtime/pkg/client"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type AccessLogSubscriptionModelBuilder interface {
	Build(ctx context.Context, alp *anv1alpha1.AccessLogPolicy, targetRefName string) (core.Stack, *model.AccessLogSubscription, error)
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
	targetRefName string,
) (core.Stack, *model.AccessLogSubscription, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(accessLogPolicy)))

	task := accessLogSubscriptionModelBuildTask{
		log:             b.log,
		stack:           stack,
		accessLogPolicy: accessLogPolicy,
		targetRefName:   targetRefName,
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
	targetRefName         string
}

func (t *accessLogSubscriptionModelBuildTask) run(ctx context.Context) error {
	var eventType = core.CreateEvent
	if t.accessLogPolicy.DeletionTimestamp != nil {
		eventType = core.DeleteEvent
	} else if _, ok := t.accessLogPolicy.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]; ok {
		eventType = core.UpdateEvent
	}

	sourceType := model.ServiceSourceType
	if t.accessLogPolicy.Spec.TargetRef.Kind == "Gateway" {
		sourceType = model.ServiceNetworkSourceType
	}

	sourceName := t.targetRefName

	destinationArn := t.accessLogPolicy.Spec.DestinationArn
	if destinationArn == nil {
		if eventType != core.DeleteEvent {
			return fmt.Errorf("access log policy's destinationArn cannot be nil")
		}
		destinationArn = aws.String("")
	}

	var status *model.AccessLogSubscriptionStatus
	if eventType != core.CreateEvent {
		value, exists := t.accessLogPolicy.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]
		if exists {
			status = &model.AccessLogSubscriptionStatus{
				Arn: value,
			}
		} else {
			t.log.Debugf(ctx, "access log policy is missing %s annotation during %s event",
				anv1alpha1.AccessLogSubscriptionAnnotationKey, eventType)
		}
	}

	alsSpec := model.AccessLogSubscriptionSpec{
		SourceType:        sourceType,
		SourceName:        sourceName,
		DestinationArn:    *destinationArn,
		ALPNamespacedName: t.accessLogPolicy.GetNamespacedName(),
		EventType:         eventType,
	}

	alsSpec.AdditionalTags = k8s.GetAdditionalTagsFromAnnotations(ctx, t.accessLogPolicy)

	t.accessLogSubscription = model.NewAccessLogSubscription(t.stack, alsSpec, status)
	err := t.stack.AddResource(t.accessLogSubscription)
	if err != nil {
		return err
	}

	return nil
}
