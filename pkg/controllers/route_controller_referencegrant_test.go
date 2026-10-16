package controllers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
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

func gaugeFor(routeKind, routeNs, routeName, kind string) float64 {
	return testutil.ToFloat64(crossNamespaceRefsNotPermitted.WithLabelValues(routeKind, routeNs, routeName, kind))
}

func TestWarnOnUnpermittedCrossNamespaceRefs(t *testing.T) {
	tests := []struct {
		name              string
		routeNs           string
		refs              []gwv1.HTTPBackendRef
		grants            []client.Object
		wantEvents        int
		wantGauge         map[string]float64
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
			wantGauge:  map[string]float64{"Service": 1},
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
			wantGauge:  map[string]float64{"ServiceImport": 1},
		},
		{
			name:    "two cross namespace refs, single combined event",
			routeNs: "foo",
			refs: []gwv1.HTTPBackendRef{
				backendRef("svc", "bar", ""),
				backendRef("imp", "baz", "ServiceImport"),
			},
			wantEvents:        1,
			wantGauge:         map[string]float64{"Service": 1, "ServiceImport": 1},
			wantEventContains: []string{"bar/svc", "baz/imp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crossNamespaceRefsNotPermitted.Reset()
			c := rgReconcilerTestClient(t, tt.grants...)
			recorder := record.NewFakeRecorder(10)
			r := &routeReconciler{
				log:                     gwlog.FallbackLogger,
				client:                  c,
				eventRecorder:           recorder,
				referenceGrantSupported: true,
			}

			route := routeWithBackendRefs(tt.routeNs, tt.refs...)
			r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)

			events := drainEvents(recorder.Events)
			assert.Len(t, events, tt.wantEvents)
			if tt.wantEvents > 0 {
				assert.True(t, strings.HasPrefix(events[0],
					corev1.EventTypeWarning+" "+k8s.RouteEventReasonCrossNamespaceRefNotPermitted))
			}
			assert.Equal(t, len(tt.wantGauge), testutil.CollectAndCount(crossNamespaceRefsNotPermitted))
			for kind, want := range tt.wantGauge {
				assert.Equal(t, want, gaugeFor("HTTPRoute", tt.routeNs, "route", kind))
			}
			for _, want := range tt.wantEventContains {
				assert.Contains(t, events[0], want)
			}
		})
	}
}

func TestWarnOnUnpermittedCrossNamespaceRefs_NotSupported(t *testing.T) {
	crossNamespaceRefsNotPermitted.Reset()
	c := rgReconcilerTestClient(t)
	recorder := record.NewFakeRecorder(10)
	r := &routeReconciler{
		log:                     gwlog.FallbackLogger,
		client:                  c,
		eventRecorder:           recorder,
		referenceGrantSupported: false,
	}

	route := routeWithBackendRefs("foo", backendRef("svc", "bar", ""))
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)

	assert.Empty(t, drainEvents(recorder.Events))
	assert.Equal(t, 0, testutil.CollectAndCount(crossNamespaceRefsNotPermitted))
}

func TestWarnOnUnpermittedCrossNamespaceRefs_EvalErrorPreservesGauge(t *testing.T) {
	crossNamespaceRefsNotPermitted.Reset()
	c := testclient.NewClientBuilder().
		WithScheme(rgReconcilerScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errors.New("boom")
			},
		}).Build()
	recorder := record.NewFakeRecorder(10)
	r := &routeReconciler{
		log:                     gwlog.FallbackLogger,
		client:                  c,
		eventRecorder:           recorder,
		referenceGrantSupported: true,
	}

	route := routeWithBackendRefs("foo", backendRef("svc", "bar", ""))
	// Seed a prior value; a transient eval error must not drop it to zero.
	crossNamespaceRefsNotPermitted.WithLabelValues("HTTPRoute", "foo", "route", "Service").Set(1)

	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)

	assert.Empty(t, drainEvents(recorder.Events))
	assert.Equal(t, float64(1), gaugeFor("HTTPRoute", "foo", "route", "Service"))
}

func TestReconcileClearsGaugeWhenRouteGone(t *testing.T) {
	crossNamespaceRefsNotPermitted.Reset()
	c := rgReconcilerTestClient(t)
	r := &routeReconciler{
		routeType:               core.HttpRouteType,
		log:                     gwlog.FallbackLogger,
		client:                  c,
		referenceGrantSupported: true,
	}

	// Seed a stale series for a route that no longer exists.
	crossNamespaceRefsNotPermitted.WithLabelValues("HTTPRoute", "foo", "gone", "Service").Set(1)

	err := r.reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "foo", Name: "gone"},
	})
	assert.NoError(t, err)
	assert.Equal(t, 0, testutil.CollectAndCount(crossNamespaceRefsNotPermitted))
}

func TestWarnOnUnpermittedCrossNamespaceRefs_EvalErrorKeepsConfirmed(t *testing.T) {
	crossNamespaceRefsNotPermitted.Reset()
	// Fail only the List for the "bad" namespace; allow the confirmed-unpermitted ref
	// into "bar" to be evaluated.
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
	r := &routeReconciler{
		log:                     gwlog.FallbackLogger,
		client:                  c,
		eventRecorder:           recorder,
		referenceGrantSupported: true,
	}

	route := routeWithBackendRefs("foo",
		backendRef("svc", "bar", ""),
		backendRef("other", "bad", ""),
	)
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)

	// The confirmed violation into "bar" is still recorded despite the "bad" eval error.
	assert.Equal(t, float64(1), gaugeFor("HTTPRoute", "foo", "route", "Service"))
	assert.Len(t, drainEvents(recorder.Events), 1)
}

func TestWarnOnUnpermittedCrossNamespaceRefs_EvalErrorDoesNotLower(t *testing.T) {
	crossNamespaceRefsNotPermitted.Reset()
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
	r := &routeReconciler{
		log:                     gwlog.FallbackLogger,
		client:                  c,
		eventRecorder:           recorder,
		referenceGrantSupported: true,
	}

	// A prior pass established two unpermitted Service refs.
	crossNamespaceRefsNotPermitted.WithLabelValues("HTTPRoute", "foo", "route", "Service").Set(2)

	// This pass can only confirm one (the other namespace's List fails); the gauge must
	// not drop to the confirmed lower bound.
	route := routeWithBackendRefs("foo",
		backendRef("svc", "bar", ""),
		backendRef("other", "bad", ""),
	)
	r.warnOnUnpermittedCrossNamespaceRefs(context.Background(), route)

	// One warning proves the "bar" ref was confirmed (count 1 < seeded 2), so the raise
	// path genuinely ran with a lower confirmed count rather than skipping all refs.
	assert.Len(t, drainEvents(recorder.Events), 1)
	assert.Equal(t, float64(2), gaugeFor("HTTPRoute", "foo", "route", "Service"))
}

func TestSetCrossNamespaceRefGaugeIsolatesRouteKinds(t *testing.T) {
	crossNamespaceRefsNotPermitted.Reset()
	r := &routeReconciler{log: gwlog.FallbackLogger}

	httpRoute := core.NewHTTPRoute(gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "foo"},
	})
	grpcRoute := core.NewGRPCRoute(gwv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "foo"},
	})

	r.setCrossNamespaceRefGauge(httpRoute, map[string]int{"Service": 1})
	// A GRPCRoute with the same namespace/name and no offending refs must not clear the
	// HTTPRoute's series.
	r.setCrossNamespaceRefGauge(grpcRoute, map[string]int{})

	assert.Equal(t, float64(1), gaugeFor("HTTPRoute", "foo", "app", "Service"))
}

func TestSetCrossNamespaceRefGaugeClearsStale(t *testing.T) {
	crossNamespaceRefsNotPermitted.Reset()
	r := &routeReconciler{log: gwlog.FallbackLogger}
	route := routeWithBackendRefs("foo", backendRef("svc", "bar", ""))

	r.setCrossNamespaceRefGauge(route, map[string]int{"Service": 2})
	assert.Equal(t, float64(2), gaugeFor("HTTPRoute", "foo", "route", "Service"))

	r.setCrossNamespaceRefGauge(route, map[string]int{})
	assert.Equal(t, 0, testutil.CollectAndCount(crossNamespaceRefsNotPermitted))

	r.setCrossNamespaceRefGauge(route, map[string]int{"Service": 1})
	assert.Equal(t, float64(1), gaugeFor("HTTPRoute", "foo", "route", "Service"))
	r.clearCrossNamespaceRefGauge("HTTPRoute", "foo", "route")
	assert.Equal(t, 0, testutil.CollectAndCount(crossNamespaceRefsNotPermitted))
}
