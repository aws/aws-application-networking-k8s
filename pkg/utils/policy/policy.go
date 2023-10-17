package policy

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	apimachineryv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func AccessLogPolicyTargetRefExists(ctx context.Context, client client.Client, logger gwlog.Logger, policy core.Policy) (bool, error) {
	logger.Debugf("Checking if AccessLogPolicy targetRef exists")
	return checkGatewayOrRouteTargetRefExistsForPolicy(ctx, client, policy)
}

func AuthPolicyTargetRefExists(ctx context.Context, client client.Client, logger gwlog.Logger, policy core.Policy) (bool, error) {
	logger.Debugf("Checking if AuthPolicy targetRef exists")
	return checkGatewayOrRouteTargetRefExistsForPolicy(ctx, client, policy)
}

func checkGatewayOrRouteTargetRefExistsForPolicy(ctx context.Context, client client.Client, policy core.Policy) (bool, error) {
	targetRefNamespace := policy.GetNamespacedName().Namespace
	targetRef := policy.GetTargetRef()
	if targetRef.Namespace != nil {
		targetRefNamespace = string(*targetRef.Namespace)
	}
	targetRefNamespacedName := types.NamespacedName{
		Name:      string(targetRef.Name),
		Namespace: targetRefNamespace,
	}

	var err error
	switch targetRef.Kind {
	case "Gateway":
		gw := &gwv1beta1.Gateway{}
		err = client.Get(ctx, targetRefNamespacedName, gw)
	case "HTTPRoute":
		httpRoute := &gwv1beta1.HTTPRoute{}
		err = client.Get(ctx, targetRefNamespacedName, httpRoute)
	case "GRPCRoute":
		grpcRoute := &gwv1alpha2.GRPCRoute{}
		err = client.Get(ctx, targetRefNamespacedName, grpcRoute)
	default:
		return false, fmt.Errorf("policy targetRef is for an unsupported Kind: %s",
			targetRef.Kind)
	}
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}
	return err == nil, nil
}

func UpdatePolicyStatus(
	ctx context.Context,
	k8sClient client.Client,
	policy core.Policy,
	reason gwv1alpha2.PolicyConditionReason,
	message string,
) error {
	status := apimachineryv1.ConditionTrue
	if reason != gwv1alpha2.PolicyReasonAccepted {
		status = apimachineryv1.ConditionFalse
	}
	newConditions := utils.GetNewConditions(policy.GetStatusConditions(), apimachineryv1.Condition{
		Type:               string(gwv1alpha2.PolicyConditionAccepted),
		ObservedGeneration: policy.(client.Object).GetGeneration(),
		Message:            message,
		Status:             status,
		Reason:             string(reason),
	})
	policy.SetStatusConditions(newConditions)
	if err := k8sClient.Status().Update(ctx, policy.(client.Object)); err != nil {
		return fmt.Errorf("failed to set Policy Accepted status to %s and reason to %s, %w",
			status, reason, err)
	}
	return nil
}
