package policyhelper

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

var (
	ErrGroupKind         = errors.New("group/kind error")
	ErrTargetRefNotFound = errors.New("targetRef not found")
	ErrTargetRefConflict = errors.New("targetRef has conflict")
)

type (
	TargetRef       = gwv1alpha2.PolicyTargetReference
	ConditionType   = gwv1alpha2.PolicyConditionType
	ConditionReason = gwv1alpha2.PolicyConditionReason
)

const (
	// GEP

	ConditionTypeAccepted = gwv1alpha2.PolicyConditionAccepted

	ReasonAccepted       = gwv1alpha2.PolicyReasonAccepted
	ReasonInvalid        = gwv1alpha2.PolicyReasonInvalid
	ReasonTargetNotFound = gwv1alpha2.PolicyReasonTargetNotFound
	ReasonConflicted     = gwv1alpha2.PolicyReasonConflicted

	// Non-GEP

	ReasonUnknown = ConditionReason("Unknown")
)

// TODO: remove code below after migration to generic policy handler
type policyInfo struct {
	policyList core.PolicyList
	group      gwv1beta1.Group
	kind       gwv1beta1.Kind
}

func GetValidPolicy[T core.Policy](ctx context.Context, k8sClient k8sclient.Client, searchTargetRef types.NamespacedName, policy T) (T, error) {
	var empty T
	policies, err := GetAttachedPolicies(ctx, k8sClient, searchTargetRef, policy)
	conflictResolutionSort(policies)
	if err != nil {
		return empty, err
	}
	if len(policies) == 0 {
		return empty, nil
	}
	return policies[0], nil
}

func GetAttachedPolicies[T core.Policy](ctx context.Context, k8sClient k8sclient.Client, searchTargetRef types.NamespacedName, policy T) ([]T, error) {
	var policies []T
	info, err := getPolicyInfo(policy)
	if err != nil {
		return policies, err
	}

	pl := info.policyList
	err = k8sClient.List(ctx, pl.(k8sclient.ObjectList), &k8sclient.ListOptions{
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

// sort in-place for policy conflict resolution
// 1. older policy (CreationTimeStamp) has precedence
// 2. alphabetical order namespace, then name
func conflictResolutionSort[T core.Policy](policies []T) {
	slices.SortFunc(policies, func(a, b T) int {
		tsA := a.GetCreationTimestamp().Time
		tsB := b.GetCreationTimestamp().Time
		switch {
		case tsA.Before(tsB):
			return -1
		case tsA.After(tsB):
			return 1
		default:
			nsnA := a.GetNamespacedName()
			nsnB := b.GetNamespacedName()
			nsA := nsnA.Namespace
			nsB := nsnB.Namespace
			nsCmp := strings.Compare(nsA, nsB)
			if nsCmp != 0 {
				return nsCmp
			}
			nA := nsnA.Name
			nB := nsnB.Name
			return strings.Compare(nA, nB)
		}
	})
}

// TODO: remove code above after migration to generic policy handler

// Policy with PolicyTargetReference
type Policy interface {
	k8sclient.Object
	GetStatusConditions() *[]metav1.Condition
	GetTargetRef() *TargetRef
}

type PolicyList[P Policy] interface {
	k8sclient.ObjectList
	GetItems() []P
}

type GroupKindSet = utils.Set[GroupKind]

func NewGroupKindSet(l []k8sclient.Object) *GroupKindSet {
	gks := &GroupKindSet{}
	for _, obj := range l {
		gk := ObjGroupKind(obj)
		gks.Put(gk)
	}
	return gks
}

// A generic handler for common operations on particular policy type
type PolicyHandler[P Policy] struct {
	log    gwlog.Logger
	kinds  *GroupKindSet
	client PolicyClient[P]
}

type PolicyHandlerConfig struct {
	Log            gwlog.Logger
	Client         k8sclient.Client
	TargetRefKinds *GroupKindSet
}

// Creates policy handler for specific policy. T and TL are type and list-type for Policy (struct type, not reference).
// P and PL are reference types and should derive from T and TL. P and PL do not require explicit declaration. For example:
//
//	ph := NewPolicyHandler[IAMAuthPolicy, IAMAuthPolicyList](cfg)
func NewPolicyHandler[T, TL any, P policyPtr[T], PL policyListPtr[TL, P]](cfg PolicyHandlerConfig) *PolicyHandler[P] {
	ph := &PolicyHandler[P]{
		log:    cfg.Log,
		client: newK8sPolicyClient[T, TL, P, PL](cfg.Client),
		kinds:  cfg.TargetRefKinds,
	}
	return ph
}

// Strong-typed interface to work with k8s client
type PolicyClient[P Policy] interface {
	List(ctx context.Context, namespace string) ([]P, error)
	Get(ctx context.Context, nsname types.NamespacedName) (P, error)
	TargetRefObj(ctx context.Context, policy P) (k8sclient.Object, error)
	UpdateStatus(ctx context.Context, policy P) error
}

type policyPtr[T any] interface {
	Policy
	*T
}

type policyListPtr[T any, P Policy] interface {
	PolicyList[P]
	*T
}

// k8s client based implementation of PolicyClient
type k8sPolicyClient[T, U any, P policyPtr[T], PL policyListPtr[U, P]] struct {
	client k8sclient.Client
}

func newK8sPolicyClient[T, U any, P policyPtr[T], PL policyListPtr[U, P]](c k8sclient.Client) *k8sPolicyClient[T, U, P, PL] {
	return &k8sPolicyClient[T, U, P, PL]{client: c}
}

func (pc *k8sPolicyClient[T, U, P, PL]) newList() PL {
	var u U
	return &u
}

func (pc *k8sPolicyClient[T, U, P, PL]) newPolicy() P {
	var t T
	return &t
}

func (pc *k8sPolicyClient[T, U, P, PL]) List(ctx context.Context, namespace string) ([]P, error) {
	l := pc.newList()
	opts := &k8sclient.ListOptions{}
	if namespace != "" { // policies suppose to be namespaced
		opts.Namespace = namespace
	}
	err := pc.client.List(ctx, l, opts)
	if err != nil {
		return nil, err
	}
	return l.GetItems(), nil
}

func (pc *k8sPolicyClient[T, U, P, PL]) Get(ctx context.Context, nsname types.NamespacedName) (P, error) {
	p := pc.newPolicy()
	err := pc.client.Get(ctx, nsname, p)
	return p, err
}

func (pc *k8sPolicyClient[T, U, P, PL]) TargetRefObj(ctx context.Context, p P) (k8sclient.Object, error) {
	tr := p.GetTargetRef()
	obj := GroupKindObj(TargetRefGroupKind(tr))
	key := types.NamespacedName{
		Namespace: p.GetNamespace(),
		Name:      string(tr.Name),
	}
	err := pc.client.Get(ctx, key, obj)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (pc *k8sPolicyClient[T, U, P, PL]) UpdateStatus(ctx context.Context, policy P) error {
	return pc.client.Status().Update(ctx, policy)
}

// Get all policies for given object, filtered by targetRef match.
// List might include invalid and conflicting policies, for non-conflicting policy use ObjResolvedPolicy
func (h *PolicyHandler[P]) ObjPolicies(ctx context.Context, obj k8sclient.Object) ([]P, error) {
	allPolicies, err := h.client.List(ctx, obj.GetNamespace())
	if err != nil {
		return nil, err
	}
	out := []P{}
	for _, policy := range allPolicies {
		tr := policy.GetTargetRef()
		if h.targetRefMatch(obj, tr) {
			out = append(out, policy)
		}
	}
	return out, nil
}

// Get resolved policy. Returns policy with conflict resolution. Will return at most single policy.
// Every policy is implicitly Direct Policy Attachment and having more that 1 will result in conflict.
func (h *PolicyHandler[P]) ObjResolvedPolicy(ctx context.Context, obj k8sclient.Object) (P, error) {
	var empty P
	objPolicies, err := h.ObjPolicies(ctx, obj)
	if err != nil {
		return empty, err
	}
	if len(objPolicies) == 0 {
		return empty, nil
	}
	h.conflictResolutionSort(objPolicies)
	return objPolicies[0], nil
}

// Add Watchers for configured Kinds to controller builder
func (h *PolicyHandler[P]) AddWatchers(b *builder.Builder, objs ...k8sclient.Object) {
	h.log.Debugf("add watchers for types: %v", NewGroupKindSet(objs).Items())
	for _, watchObj := range objs {
		b.Watches(watchObj, handler.EnqueueRequestsFromMapFunc(h.watchMapFn))
	}
}

func (h *PolicyHandler[P]) watchMapFn(ctx context.Context, obj k8sclient.Object) []reconcile.Request {
	out := []reconcile.Request{}
	policies, err := h.client.List(ctx, obj.GetNamespace())
	if err != nil {
		h.log.Errorf("watch mapfn error: for obj=%s/%s: %w",
			obj.GetName(), obj.GetNamespace(), err)
		return nil
	}
	for _, policy := range policies {
		if h.targetRefMatch(obj, policy.GetTargetRef()) {
			out = append(out, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      policy.GetName(),
					Namespace: policy.GetNamespace(),
				},
			})
		}
	}
	return out
}

// Checks if objects matches targetReference, returns true if they match
// targetRef might not have namespace set, it should be inferred from policy itself.
// In this case we assume namespace already checked
func (h *PolicyHandler[P]) targetRefMatch(obj k8sclient.Object, tr *gwv1alpha2.PolicyTargetReference) bool {
	objGk := ObjGroupKind(obj)
	trGk := TargetRefGroupKind(tr)
	return objGk == trGk && obj.GetName() == string(tr.Name)
}

func (h *PolicyHandler[P]) ValidateAndUpdateCondition(ctx context.Context, policy P) (ConditionReason, error) {
	validationErr := h.ValidateTargetRef(ctx, policy)
	reason := errToReason(validationErr)
	msg := ""
	if validationErr != nil {
		msg = validationErr.Error()
	}
	err := h.UpdateAcceptedCondition(ctx, policy, reason, msg)
	if err != nil {
		return ReasonUnknown, err
	}
	return reason, nil
}

func (h *PolicyHandler[P]) ValidateTargetRef(ctx context.Context, policy P) error {
	tr := policy.GetTargetRef()
	trGk := TargetRefGroupKind(tr)
	if !h.kinds.Contains(trGk) {
		return fmt.Errorf("%w: not supported GroupKind=%s/%s",
			ErrGroupKind, tr.Group, tr.Kind)
	}
	targetRefObj, err := h.client.TargetRefObj(ctx, policy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w, target=%s/%s",
				ErrTargetRefNotFound, policy.GetNamespace(), tr.Name)
		}
		return err
	}
	resolvedPolicy, err := h.ObjResolvedPolicy(ctx, targetRefObj)
	if err != nil {
		return err
	}
	if resolvedPolicy.GetName() != policy.GetName() {
		return fmt.Errorf("%w, policy=%s",
			ErrTargetRefConflict, resolvedPolicy.GetName())
	}
	return nil
}

func errToReason(err error) ConditionReason {
	switch {
	case err == nil:
		return ReasonAccepted
	case errors.Is(err, ErrGroupKind):
		return ReasonInvalid
	case errors.Is(err, ErrTargetRefNotFound):
		return ReasonTargetNotFound
	case errors.Is(err, ErrTargetRefConflict):
		return ReasonConflicted
	default:
		return ReasonUnknown
	}
}

func (h *PolicyHandler[P]) UpdateAcceptedCondition(ctx context.Context, policy P, reason ConditionReason, msg string) error {
	status := metav1.ConditionTrue
	if reason != ReasonAccepted {
		status = metav1.ConditionFalse
	}
	cnd := metav1.Condition{
		Type:               string(ConditionTypeAccepted),
		Status:             status,
		ObservedGeneration: policy.GetGeneration(),
		Reason:             string(reason),
		Message:            msg,
	}
	meta.SetStatusCondition(policy.GetStatusConditions(), cnd)
	err := h.client.UpdateStatus(ctx, policy)
	return err
}

// sort in-place for policy conflict resolution
// 1. older policy (CreationTimeStamp) has precedence
// 2. alphabetical order namespace, then name
func (h *PolicyHandler[P]) conflictResolutionSort(policies []P) {
	slices.SortFunc(policies, func(a, b P) int {
		tsA := a.GetCreationTimestamp().Time
		tsB := b.GetCreationTimestamp().Time
		switch {
		case tsA.Before(tsB):
			return -1
		case tsA.After(tsB):
			return 1
		default:
			nA := a.GetName()
			nB := b.GetName()
			return strings.Compare(nA, nB)
		}
	})
}
