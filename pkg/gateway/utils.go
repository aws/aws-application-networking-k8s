package gateway

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

func GetAttachedPolicies[T core.Policy](ctx context.Context, k8sClient client.Client, refObjNamespacedName types.NamespacedName, policy T) ([]T, error) {
	var matchedPolicies []T
	policyList, validTargetRefObjGroupKinds, err := policyTypeToPolicyListAndValidTargetRefObjGKs(policy)
	if err != nil {
		return matchedPolicies, err
	}

	err = k8sClient.List(ctx, policyList.(client.ObjectList), &client.ListOptions{
		Namespace: refObjNamespacedName.Namespace,
	})
	if err != nil {
		if meta.IsNoMatchError(err) {
			// CRD does not exist
			return matchedPolicies, nil
		}
		return matchedPolicies, err
	}
	for _, p := range policyList.GetItems() {
		targetRef := p.GetTargetRef()
		if targetRef == nil {
			continue
		}
		_, groupKindMatch := validTargetRefObjGroupKinds[schema.GroupKind{
			Group: string(targetRef.Group),
			Kind:  string(targetRef.Kind),
		}]
		nameMatch := string(targetRef.Name) == refObjNamespacedName.Name

		retrievedNamespace := p.GetNamespacedName().Namespace
		if targetRef.Namespace != nil {
			retrievedNamespace = string(*targetRef.Namespace)
		}
		namespaceMatch := retrievedNamespace == refObjNamespacedName.Namespace
		if groupKindMatch && nameMatch && namespaceMatch {
			matchedPolicies = append(matchedPolicies, p.(T))
		}
	}
	return matchedPolicies, nil
}

func policyTypeToPolicyListAndValidTargetRefObjGKs(policyType core.Policy) (core.PolicyList, map[schema.GroupKind]interface{}, error) {
	switch policyType.(type) {
	case *anv1alpha1.VpcAssociationPolicy:
		return &anv1alpha1.VpcAssociationPolicyList{}, map[schema.GroupKind]interface{}{
			{Group: gwv1beta1.GroupName, Kind: "Gateway"}: struct{}{},
		}, nil
	case *anv1alpha1.TargetGroupPolicy:
		return &anv1alpha1.TargetGroupPolicyList{}, map[schema.GroupKind]interface{}{
			{Group: corev1.GroupName, Kind: "Service"}: struct{}{},
		}, nil
	case *anv1alpha1.IAMAuthPolicy:
		return &anv1alpha1.IAMAuthPolicyList{}, map[schema.GroupKind]interface{}{
			{Group: gwv1beta1.GroupName, Kind: "Gateway"}:   struct{}{},
			{Group: gwv1beta1.GroupName, Kind: "HttpRoute"}: struct{}{},
			{Group: gwv1beta1.GroupName, Kind: "GRPCRoute"}: struct{}{},
		}, nil
	case *anv1alpha1.AccessLogPolicy:
		return &anv1alpha1.AccessLogPolicyList{}, map[schema.GroupKind]interface{}{
			{Group: gwv1beta1.GroupName, Kind: "Gateway"}:   struct{}{},
			{Group: gwv1beta1.GroupName, Kind: "HttpRoute"}: struct{}{},
			{Group: gwv1beta1.GroupName, Kind: "GRPCRoute"}: struct{}{},
		}, nil
	default:
		return nil, nil, fmt.Errorf("unknown policy type %T", policyType)
	}
}
