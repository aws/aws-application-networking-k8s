package controllers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func rgReconcilerScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))
	assert.NoError(t, gwv1.Install(scheme))
	assert.NoError(t, anv1alpha1.Install(scheme))
	return scheme
}

func rgReconcilerTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return testclient.NewClientBuilder().WithScheme(rgReconcilerScheme(t)).WithObjects(objs...).Build()
}

func routeWithBackendRefs(routeNs string, refs ...gwv1.HTTPBackendRef) core.Route {
	return core.NewHTTPRoute(gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: routeNs},
		Spec: gwv1.HTTPRouteSpec{
			Rules: []gwv1.HTTPRouteRule{{BackendRefs: refs}},
		},
	})
}

func backendRef(name, namespace, kind string) gwv1.HTTPBackendRef {
	ref := gwv1.HTTPBackendRef{
		BackendRef: gwv1.BackendRef{
			BackendObjectReference: gwv1.BackendObjectReference{Name: gwv1.ObjectName(name)},
		},
	}
	if namespace != "" {
		ref.Namespace = ptr.To(gwv1.Namespace(namespace))
	}
	if kind != "" {
		ref.Kind = ptr.To(gwv1.Kind(kind))
	}
	return ref
}

func crossNsGrant(namespace, fromNs string) *gwv1.ReferenceGrant {
	return &gwv1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "grant", Namespace: namespace},
		Spec: gwv1.ReferenceGrantSpec{
			From: []gwv1.ReferenceGrantFrom{{
				Group:     gwv1.Group(gwv1.GroupName),
				Kind:      gwv1.Kind("HTTPRoute"),
				Namespace: gwv1.Namespace(fromNs),
			}},
			To: []gwv1.ReferenceGrantTo{{Group: gwv1.Group(""), Kind: gwv1.Kind("Service")}},
		},
	}
}

func drainEvents(ch chan string) []string {
	var out []string
	for {
		select {
		case e := <-ch:
			out = append(out, e)
		default:
			return out
		}
	}
}

func newWarnReconciler(c client.Client, recorder record.EventRecorder) *routeReconciler {
	return &routeReconciler{
		routeType:                 core.HttpRouteType,
		log:                       gwlog.FallbackLogger,
		client:                    c,
		eventRecorder:             recorder,
		lastCrossNamespaceWarning: &sync.Map{},
		referenceGrantSupported:   true,
	}
}

func TestWarnOnUnpermittedCrossNamespaceRefs(t *testing.T) {
	tests := []struct {
		name              string
		routeNs           string
		refs              []gwv1.HTTPBackendRef
		grants            []client.Object
		wantEvents        int
		wantEventContains []string
	}{
		{
			name:       "same namespace service, no warning",
			routeNs:    "foo",
			refs:       []gwv1.HTTPBackendRef{backendRef("svc", "", "")},
			wantEvents: 0,
		},
		{
			name:       "same namespace explicit, no warning",
			routeNs:    "foo",
			refs:       []gwv1.HTTPBackendRef{backendRef("svc", "foo", "")},
			wantEvents: 0,
		},
		{
			name:       "cross namespace service without grant, warns",
			routeNs:    "foo",
			refs:       []gwv1.HTTPBackendRef{backendRef("svc", "bar", "")},
			wantEvents: 1,
		},
		{
			name:       "cross namespace service with grant, no warning",
			routeNs:    "foo",
			refs:       []gwv1.HTTPBackendRef{backendRef("svc", "bar", "")},
			grants:     []client.Object{crossNsGrant("bar", "foo")},
			wantEvents: 0,
		},
		{
			name:       "cross namespace service import without grant, warns",
			routeNs:    "foo",
			refs:       []gwv1.HTTPBackendRef{backendRef("imp", "bar", "ServiceImport")},
			wantEvents: 1,
		},
		{
			name:    "two cross namespace refs, single combined event",
			routeNs: "foo",
			refs: []gwv1.HTTPBackendRef{
				backendRef("svc", "bar", ""),
				backendRef("imp", "baz", "ServiceImport"),
			},
			wantEvents:        1,
			wantEventContains: []string{"bar/svc", "baz/imp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := record.NewFakeRecorder(10)
			r := newWarnReconciler(rgReconcilerTestClient(t, tt.grants...), recorder)

			route := routeWithBackendRefs(tt.routeNs, tt.refs...)
			r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)

			events := drainEvents(recorder.Events)
			assert.Len(t, events, tt.wantEvents)
			if tt.wantEvents > 0 {
				assert.True(t, strings.HasPrefix(events[0],
					corev1.EventTypeWarning+" "+k8s.RouteEventReasonCrossNamespaceRefNotPermitted))
			}
			for _, want := range tt.wantEventContains {
				assert.Contains(t, events[0], want)
			}
		})
	}
}

func TestWarnOnUnpermittedCrossNamespaceRefs_NotSupported(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	r := newWarnReconciler(rgReconcilerTestClient(t), recorder)
	r.referenceGrantSupported = false

	route := routeWithBackendRefs("foo", backendRef("svc", "bar", ""))
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)

	assert.Empty(t, drainEvents(recorder.Events))
}

func TestReconcileClearsWarningWhenRouteGone(t *testing.T) {
	c := rgReconcilerTestClient(t)
	r := newWarnReconciler(c, record.NewFakeRecorder(10))

	// Seed a dedup entry for a route that no longer exists.
	r.lastCrossNamespaceWarning.Store("HTTPRoute/foo/gone", "bar/svc (Service)")

	err := r.reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "foo", Name: "gone"},
	})
	assert.NoError(t, err)
	_, stillThere := r.lastCrossNamespaceWarning.Load("HTTPRoute/foo/gone")
	assert.False(t, stillThere)
}

func TestWarnOnUnpermittedCrossNamespaceRefs_EventDedupedAcrossReconciles(t *testing.T) {
	c := rgReconcilerTestClient(t)
	recorder := record.NewFakeRecorder(10)
	r := newWarnReconciler(c, recorder)
	route := routeWithBackendRefs("foo", backendRef("svc", "bar", ""))

	// Three reconciles with an unchanged unpermitted-ref set emit exactly one event.
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Len(t, drainEvents(recorder.Events), 1)

	// A grant permits the reference: targets become empty, which clears the dedup entry
	// and emits no event.
	g := crossNsGrant("bar", "foo")
	assert.NoError(t, c.Create(context.Background(), g))
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Empty(t, drainEvents(recorder.Events))

	// Removing the grant makes the reference unpermitted again and re-emits a fresh event.
	assert.NoError(t, c.Delete(context.Background(), g))
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Len(t, drainEvents(recorder.Events), 1)
}

func TestWarnOnUnpermittedCrossNamespaceRefs_PersistentEvalErrorEmitsOnce(t *testing.T) {
	// Fail only the List for the "bad" namespace; the ref into "bar" is still confirmed.
	c := testclient.NewClientBuilder().
		WithScheme(rgReconcilerScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				lo := &client.ListOptions{}
				for _, o := range opts {
					o.ApplyToList(lo)
				}
				if lo.Namespace == "bad" {
					return errors.New("boom")
				}
				return cl.List(ctx, list, opts...)
			},
		}).Build()
	recorder := record.NewFakeRecorder(10)
	r := newWarnReconciler(c, recorder)
	route := routeWithBackendRefs("foo",
		backendRef("svc", "bar", ""),
		backendRef("other", "bad", ""),
	)

	// With no prior entry the first errored pass emits once and records the confirmed set.
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Len(t, drainEvents(recorder.Events), 1)
	stored, ok := r.lastCrossNamespaceWarning.Load("HTTPRoute/foo/route")
	assert.True(t, ok)
	assert.Equal(t, "bar/svc (Service)", stored)

	// Further errored passes with the same set must not re-emit.
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Empty(t, drainEvents(recorder.Events))
}

func TestWarnOnUnpermittedCrossNamespaceRefs_EvalErrorDoesNotRewriteDedup(t *testing.T) {
	failBad := false
	c := testclient.NewClientBuilder().
		WithScheme(rgReconcilerScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				lo := &client.ListOptions{}
				for _, o := range opts {
					o.ApplyToList(lo)
				}
				if failBad && lo.Namespace == "bad" {
					return errors.New("boom")
				}
				return cl.List(ctx, list, opts...)
			},
		}).Build()
	recorder := record.NewFakeRecorder(10)
	r := newWarnReconciler(c, recorder)
	route := routeWithBackendRefs("foo",
		backendRef("svc", "bar", ""),
		backendRef("other", "bad", ""),
	)

	// Clean pass records both targets and emits one event.
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Len(t, drainEvents(recorder.Events), 1)
	prev, _ := r.lastCrossNamespaceWarning.Load("HTTPRoute/foo/route")

	// A pass where "bad" cannot be evaluated must not re-emit or rewrite the stored set.
	failBad = true
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Empty(t, drainEvents(recorder.Events))
	now, _ := r.lastCrossNamespaceWarning.Load("HTTPRoute/foo/route")
	assert.Equal(t, prev, now)

	// A clean pass with the unchanged set still emits nothing.
	failBad = false
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Empty(t, drainEvents(recorder.Events))
}
