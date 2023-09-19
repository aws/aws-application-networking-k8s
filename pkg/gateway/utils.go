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

func getAttachedPolicy[T core.Policy](ctx context.Context, k8sClient client.Client, refObjNamespacedName types.NamespacedName, policy T) (T, error) {
	null := *new(T)
	var policyList interface{}
	var expectedTargetRefObjGroup gateway_api.Group
	var expectedTargetRefObjKind gateway_api.Kind
	switch core.Policy(policy).(type) {
	case *v1alpha1.VpcAssociationPolicy:
		expectedTargetRefObjGroup = gateway_api.GroupName
		expectedTargetRefObjKind = "Gateway"
		policyList = &v1alpha1.VpcAssociationPolicyList{}
	case *v1alpha1.TargetGroupPolicy:
		expectedTargetRefObjGroup = corev1.GroupName
		expectedTargetRefObjKind = "Service"
		policyList = &v1alpha1.TargetGroupPolicyList{}
	default:
		return null, fmt.Errorf("unsupported policy type %T", policy)

	}
	err := k8sClient.List(ctx, policyList.(client.ObjectList), &client.ListOptions{
		Namespace: refObjNamespacedName.Namespace,
	})
	if err != nil {
		if meta.IsNoMatchError(err) {
			// CRD does not exist
			return null, nil
		}
		return null, err
	}
	for _, p := range policyList.(core.PolicyList).GetItems() {
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
