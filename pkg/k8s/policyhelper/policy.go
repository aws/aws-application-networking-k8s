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

type (
	TGP  = anv1alpha1.TargetGroupPolicy
	TGPL = anv1alpha1.TargetGroupPolicyList
	IAP  = anv1alpha1.IAMAuthPolicy
	IAPL = anv1alpha1.IAMAuthPolicyList
	VAP  = anv1alpha1.VpcAssociationPolicy
	VAPL = anv1alpha1.VpcAssociationPolicyList
)

func NewVpcAssociationPolicyHandler(log gwlog.Logger, c k8sclient.Client) *PolicyHandler[*VAP] {
	phcfg := PolicyHandlerConfig{
		Log:            log,
		Client:         c,
		TargetRefKinds: NewGroupKindSet(&gwv1beta1.Gateway{}),
	}
	return NewPolicyHandler[VAP, VAPL](phcfg)
}

func NewTargetGroupPolicyHandler(log gwlog.Logger, c k8sclient.Client) *PolicyHandler[*TGP] {
	phcfg := PolicyHandlerConfig{
		Log:            log,
		Client:         c,
		TargetRefKinds: NewGroupKindSet(&corev1.Service{}, &anv1alpha1.ServiceExport{}),
	}
	return NewPolicyHandler[TGP, TGPL](phcfg)
}

func NewIAMAuthPolicyHandler(log gwlog.Logger, c k8sclient.Client) *PolicyHandler[*IAP] {
	phcfg := PolicyHandlerConfig{
		Log:            log,
		Client:         c,
		TargetRefKinds: NewGroupKindSet(&gwv1beta1.Gateway{}, &gwv1beta1.HTTPRoute{}, &gwv1alpha2.GRPCRoute{}),
	}
	return NewPolicyHandler[IAP, IAPL](phcfg)
}

// Policy with PolicyTargetReference
type Policy interface {
	k8sclient.Object
	GetTargetRef() *TargetRef
	GetStatusConditions() *[]metav1.Condition
}

type PolicyList[P Policy] interface {
	k8sclient.ObjectList
	GetItems() []P
}

type GroupKindSet = utils.Set[GroupKind]

func NewGroupKindSet(objs ...k8sclient.Object) *GroupKindSet {
	gks := utils.SliceMap(objs, func(o k8sclient.Object) GroupKind {
		return ObjToGroupKind(o)
	})
	set := utils.NewSet(gks...)
	return &set
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
	err := pc.client.List(ctx, l, &k8sclient.ListOptions{Namespace: namespace})
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
	obj, ok := GroupKindToObj(TargetRefGroupKind(tr))
	if !ok {
		return nil, fmt.Errorf("not supported GroupKind of targetRef, group/kind=%s/%s",
			tr.Group, tr.Kind)
	}
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

// Get all policies for given object, filtered by targetRef match and sorted by conflict resolution
// rules. First policy in the list is not-conflicting policy, but it might be in Accepted or Invalid
// state. Conflict resolution order uses CreationTimestamp and Name.
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
	h.conflictResolutionSort(out)
	return out, nil
}

// Get Accepted policy for given object. Returns policy with conflict resolution and status
// Accepted.  Will return at most single policy.
func (h *PolicyHandler[P]) ObjResolvedPolicy(ctx context.Context, obj k8sclient.Object) (P, error) {
	var empty P
	objPolicies, err := h.ObjPolicies(ctx, obj)
	if err != nil {
		return empty, err
	}
	if len(objPolicies) == 0 {
		return empty, nil
	}
	policy := objPolicies[0]
	cnd := meta.FindStatusCondition(*policy.GetStatusConditions(), string(ConditionTypeAccepted))
	if cnd != nil && cnd.Reason != string(ReasonAccepted) {
		return empty, nil
	}
	return objPolicies[0], nil
}

// Add Watchers for configured Kinds to controller builder
func (h *PolicyHandler[P]) AddWatchers(b *builder.Builder, objs ...k8sclient.Object) {
	h.log.Debugf("add watchers for types: %v", NewGroupKindSet(objs...).Items())
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
	objGk := ObjToGroupKind(obj)
	trGk := TargetRefGroupKind(tr)
	return objGk == trGk && obj.GetName() == string(tr.Name)
}

// Validate Policy and update Accepted status condition.
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

	// invalid
	trGk := TargetRefGroupKind(tr)
	if !h.kinds.Contains(trGk) {
		return fmt.Errorf("%w: not supported GroupKind=%s/%s",
			ErrGroupKind, tr.Group, tr.Kind)
	}

	// not found
	targetRefObj, err := h.client.TargetRefObj(ctx, policy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w, target=%s/%s",
				ErrTargetRefNotFound, policy.GetNamespace(), tr.Name)
		}
		return err
	}

	// conflicted
	objPolicies, err := h.ObjPolicies(ctx, targetRefObj)
	if err != nil {
		return err
	}
	if len(objPolicies) > 0 {
		resolvedPolicy := objPolicies[0]
		if resolvedPolicy.GetName() != policy.GetName() {
			return fmt.Errorf("%w, policy=%s",
				ErrTargetRefConflict, resolvedPolicy.GetName())
		}
	}

	// valid
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
