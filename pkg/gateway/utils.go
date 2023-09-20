package gateway

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

func GetAttachedPolicy[T core.Policy](ctx context.Context, k8sClient client.Client, refObjNamespacedName types.NamespacedName, policy T) (T, error) {
	null := *new(T)
	policyList, expectedTargetRefObjGroup, expectedTargetRefObjKind, err := policyTypeToPolicyListAndTargetRefGroupKind(policy)
	if err != nil {
		return null, err
	}

	err = k8sClient.List(ctx, policyList.(client.ObjectList), &client.ListOptions{
		Namespace: refObjNamespacedName.Namespace,
	})
	if err != nil {
		if meta.IsNoMatchError(err) {
			// CRD does not exist
			return null, nil
		}
		return null, err
	}
	for _, p := range policyList.GetItems() {
		targetRef := p.GetTargetRef()
		if targetRef == nil {
			continue
		}
		groupKindMatch := targetRef.Group == expectedTargetRefObjGroup && targetRef.Kind == expectedTargetRefObjKind
		nameMatch := string(targetRef.Name) == refObjNamespacedName.Name

		retrievedNamespace := p.GetNamespacedName().Namespace
		if targetRef.Namespace != nil {
			retrievedNamespace = string(*targetRef.Namespace)
		}
		namespaceMatch := retrievedNamespace == refObjNamespacedName.Namespace
		if groupKindMatch && nameMatch && namespaceMatch {
			return p.(T), nil
		}
	}
	return null, nil
}

func policyTypeToPolicyListAndTargetRefGroupKind(policyType core.Policy) (core.PolicyList, gateway_api.Group, gateway_api.Kind, error) {
	switch policyType.(type) {
	case *v1alpha1.VpcAssociationPolicy:
		return &v1alpha1.VpcAssociationPolicyList{}, gateway_api.GroupName, "Gateway", nil
	case *v1alpha1.TargetGroupPolicy:
		return &v1alpha1.TargetGroupPolicyList{}, corev1.GroupName, "Service", nil
	default:
		return nil, "", "", fmt.Errorf("unsupported policy type %T", policyType)
	}
}
