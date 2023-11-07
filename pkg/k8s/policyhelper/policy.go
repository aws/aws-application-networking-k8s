package policyhelper

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

func GetAttachedPolicies[T core.Policy](ctx context.Context, k8sClient client.Client, refObjNamespacedName types.NamespacedName, policy T) ([]T, error) {
	var policies []T
	policyList, expectedTargetRefObjGroup, expectedTargetRefObjKind, err := policyTypeToPolicyListAndTargetRefGroupKind(policy)
	if err != nil {
		return policies, err
	}

	err = k8sClient.List(ctx, policyList.(client.ObjectList), &client.ListOptions{
		Namespace: refObjNamespacedName.Namespace,
	})
	if err != nil {
		if meta.IsNoMatchError(err) {
			// CRD does not exist
			return policies, nil
		}
		return policies, err
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
			policies = append(policies, p.(T))
		}
	}
	return policies, nil
}

func policyTypeToPolicyListAndTargetRefGroupKind(policyType core.Policy) (core.PolicyList, gwv1beta1.Group, gwv1beta1.Kind, error) {
	switch policyType.(type) {
	case *anv1alpha1.VpcAssociationPolicy:
		return &anv1alpha1.VpcAssociationPolicyList{}, gwv1beta1.GroupName, "Gateway", nil
	case *anv1alpha1.TargetGroupPolicy:
		return &anv1alpha1.TargetGroupPolicyList{}, corev1.GroupName, "Service", nil
	default:
		return nil, "", "", fmt.Errorf("unsupported policy type %T", policyType)
	}
}
