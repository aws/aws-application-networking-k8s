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

type policyInfo struct {
	policyList core.PolicyList
	group      gwv1beta1.Group
	kind       gwv1beta1.Kind
}

func GetAttachedPolicies[T core.Policy](ctx context.Context, k8sClient client.Client, searchTargetRef types.NamespacedName, policy T) ([]T, error) {
	var policies []T
	info, err := getPolicyInfo(policy)
	if err != nil {
		return policies, err
	}

	pl := info.policyList
	err = k8sClient.List(ctx, pl.(client.ObjectList), &client.ListOptions{
		Namespace: searchTargetRef.Namespace,
	})
	if err != nil {
		if meta.IsNoMatchError(err) {
			// CRD does not exist
			return policies, nil
		}
		return policies, err
	}
	for _, p := range pl.GetItems() {
		targetRef := p.GetTargetRef()
		if targetRef == nil {
			continue
		}
		groupKindMatch := targetRef.Group == info.group && targetRef.Kind == info.kind
		nameMatch := string(targetRef.Name) == searchTargetRef.Name

		retrievedNamespace := p.GetNamespacedName().Namespace
		if targetRef.Namespace != nil {
			retrievedNamespace = string(*targetRef.Namespace)
		}
		namespaceMatch := retrievedNamespace == searchTargetRef.Namespace
		if groupKindMatch && nameMatch && namespaceMatch {
			policies = append(policies, p.(T))
		}
	}
	return policies, nil
}

func getPolicyInfo(policyType core.Policy) (policyInfo, error) {
	switch policyType.(type) {
	case *anv1alpha1.VpcAssociationPolicy:
		return policyInfo{
			policyList: &anv1alpha1.VpcAssociationPolicyList{},
			group:      gwv1beta1.GroupName,
			kind:       "Gateway",
		}, nil
	case *anv1alpha1.TargetGroupPolicy:
		return policyInfo{
			policyList: &anv1alpha1.TargetGroupPolicyList{},
			group:      corev1.GroupName,
			kind:       "Service",
		}, nil
	default:
		return policyInfo{}, fmt.Errorf("unsupported policy type %T", policyType)
	}
}
