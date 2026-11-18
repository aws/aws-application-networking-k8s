package controllers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	latticetypes "github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
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

func newWarnReconciler(t *testing.T, c client.Client, recorder record.EventRecorder) *routeReconciler {
	t.Helper()
	setReferenceGrantServed(t, true)
	return &routeReconciler{
		routeType:                 core.HttpRouteType,
		log:                       gwlog.FallbackLogger,
		client:                    c,
		eventRecorder:             recorder,
		lastCrossNamespaceWarning: &sync.Map{},
	}
}

func setReferenceGrantServed(t *testing.T, served bool) {
	t.Helper()
	gateway.SetReferenceGrantServed(served)
	t.Cleanup(func() { gateway.SetReferenceGrantServed(false) })
}

func TestReportUnpermittedCrossNamespaceRefs(t *testing.T) {
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
			r := newWarnReconciler(t, rgReconcilerTestClient(t, tt.grants...), recorder)

			route := routeWithBackendRefs(tt.routeNs, tt.refs...)
			r.reportUnpermittedCrossNamespaceRefs(context.Background(), route)

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

func TestReconcileClearsWarningWhenRouteGone(t *testing.T) {
	c := rgReconcilerTestClient(t)
	r := newWarnReconciler(t, c, record.NewFakeRecorder(10))

	// Seed a dedup entry for a route that no longer exists.
	r.lastCrossNamespaceWarning.Store("HTTPRoute/foo/gone", "bar/svc (Service)")

	err := r.reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "foo", Name: "gone"},
	})
	assert.NoError(t, err)
	_, stillThere := r.lastCrossNamespaceWarning.Load("HTTPRoute/foo/gone")
	assert.False(t, stillThere)
}

func TestReportUnpermittedCrossNamespaceRefs_EventDedupedAcrossReconciles(t *testing.T) {
	c := rgReconcilerTestClient(t)
	recorder := record.NewFakeRecorder(10)
	r := newWarnReconciler(t, c, recorder)
	route := routeWithBackendRefs("foo", backendRef("svc", "bar", ""))

	// Three reconciles with an unchanged unpermitted-ref set emit exactly one event.
	r.reportUnpermittedCrossNamespaceRefs(context.Background(), route)
	r.reportUnpermittedCrossNamespaceRefs(context.Background(), route)
	r.reportUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Len(t, drainEvents(recorder.Events), 1)

	// A grant permits the reference: targets become empty, which clears the dedup entry
	// and emits no event.
	g := crossNsGrant("bar", "foo")
	assert.NoError(t, c.Create(context.Background(), g))
	r.reportUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Empty(t, drainEvents(recorder.Events))

	// Removing the grant makes the reference unpermitted again and re-emits a fresh event.
	assert.NoError(t, c.Delete(context.Background(), g))
	r.reportUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Len(t, drainEvents(recorder.Events), 1)
}

func TestReportUnpermittedCrossNamespaceRefs_PersistentEvalErrorEmitsOnce(t *testing.T) {
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
	r := newWarnReconciler(t, c, recorder)
	route := routeWithBackendRefs("foo",
		backendRef("svc", "bar", ""),
		backendRef("other", "bad", ""),
	)

	// With no prior entry the first errored pass emits once and records the confirmed set.
	r.reportUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Len(t, drainEvents(recorder.Events), 1)
	stored, ok := r.lastCrossNamespaceWarning.Load("HTTPRoute/foo/route")
	assert.True(t, ok)
	assert.Equal(t, "bar/svc (Service)", stored)

	// Further errored passes with the same set must not re-emit.
	r.reportUnpermittedCrossNamespaceRefs(context.Background(), route)
	r.reportUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Empty(t, drainEvents(recorder.Events))
}

func TestReportUnpermittedCrossNamespaceRefs_EvalErrorDoesNotRewriteDedup(t *testing.T) {
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
	r := newWarnReconciler(t, c, recorder)
	route := routeWithBackendRefs("foo",
		backendRef("svc", "bar", ""),
		backendRef("other", "bad", ""),
	)

	// Clean pass records both targets and emits one event.
	r.reportUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Len(t, drainEvents(recorder.Events), 1)
	prev, _ := r.lastCrossNamespaceWarning.Load("HTTPRoute/foo/route")

	// A pass where "bad" cannot be evaluated must not re-emit or rewrite the stored set.
	failBad = true
	r.reportUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Empty(t, drainEvents(recorder.Events))
	now, _ := r.lastCrossNamespaceWarning.Load("HTTPRoute/foo/route")
	assert.Equal(t, prev, now)

	// A clean pass with the unchanged set still emits nothing.
	failBad = false
	r.reportUnpermittedCrossNamespaceRefs(context.Background(), route)
	assert.Empty(t, drainEvents(recorder.Events))
}

func TestCheckReferenceGrantAccess(t *testing.T) {
	gr := schema.GroupResource{Group: gwv1.GroupName, Resource: "referencegrants"}
	tests := []struct {
		name    string
		listErr error
		wantErr bool
	}{
		{name: "permitted"},
		{name: "forbidden is reported", listErr: apierrors.NewForbidden(gr, "", errors.New("no rbac")), wantErr: true},
		{name: "other errors are left to the watch", listErr: errors.New("connection refused")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testclient.NewClientBuilder().
				WithScheme(rgReconcilerScheme(t)).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						if tt.listErr != nil {
							return tt.listErr
						}
						return cl.List(ctx, list, opts...)
					},
				}).Build()

			err := checkReferenceGrantAccess(context.Background(), c)
			if tt.wantErr {
				assert.ErrorContains(t, err, "apply the controller ClusterRole")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateBackedRefs_ReferenceGrant(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "bar"}}
	tests := []struct {
		name       string
		objs       []client.Object
		wantReason gwv1.RouteConditionReason
	}{
		{
			name:       "no grant, target exists",
			objs:       []client.Object{svc},
			wantReason: gwv1.RouteReasonRefNotPermitted,
		},
		{
			// Same result as when the target exists, so status does not reveal whether
			// it exists.
			name:       "no grant, target missing",
			wantReason: gwv1.RouteReasonRefNotPermitted,
		},
		{
			name:       "grant, target exists",
			objs:       []client.Object{svc, crossNsGrant("bar", "foo")},
			wantReason: gwv1.RouteReasonResolvedRefs,
		},
		{
			name:       "grant, target missing",
			objs:       []client.Object{crossNsGrant("bar", "foo")},
			wantReason: gwv1.RouteReasonBackendNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newWarnReconciler(t, rgReconcilerTestClient(t, tt.objs...), record.NewFakeRecorder(10))
			route := routeWithBackendRefs("foo", backendRef("svc", "bar", "Service"))

			cnd, err := r.validateBackedRefs(context.Background(), route)
			assert.NoError(t, err)
			assert.Equal(t, string(tt.wantReason), cnd.Reason)
		})
	}
}

func TestValidateBackedRefs_GrantListErrorReturnsError(t *testing.T) {
	c := testclient.NewClientBuilder().
		WithScheme(rgReconcilerScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				return errors.New("boom")
			},
		}).Build()
	r := newWarnReconciler(t, c, record.NewFakeRecorder(10))
	route := routeWithBackendRefs("foo", backendRef("svc", "bar", "Service"))

	_, err := r.validateBackedRefs(context.Background(), route)
	assert.Error(t, err)
}

func TestValidateBackendRefsIpFamilies_ReferenceGrant(t *testing.T) {
	dualStack := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "bar"},
		Spec:       corev1.ServiceSpec{IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}},
	}
	route := routeWithBackendRefs("foo", backendRef("svc", "bar", "Service"))

	// Rejecting the route here would tell its owner the blocked target exists.
	r := newWarnReconciler(t, rgReconcilerTestClient(t, dualStack), record.NewFakeRecorder(10))
	assert.NoError(t, r.validateBackendRefsIpFamilies(context.Background(), route))

	r = newWarnReconciler(t, rgReconcilerTestClient(t, dualStack, crossNsGrant("bar", "foo")), record.NewFakeRecorder(10))
	assert.Error(t, r.validateBackendRefsIpFamilies(context.Background(), route))
}

func TestValidateBackedRefs_ManagedExternalTargetGroup(t *testing.T) {
	const arn = "arn:aws:vpc-lattice:us-west-2:123456789012:targetgroup/tg-0df85aff983932f06"
	const ownManagedBy = "999988887777/this-cluster/vpc-1"
	tests := []struct {
		name       string
		tags       map[string]string
		tagErr     error
		wantErr    bool
		wantReason gwv1.RouteConditionReason
		wantMsg    string
	}{
		{
			name: "target group of this cluster's route points to a backendRef",
			tags: map[string]string{
				pkg_aws.TagManagedBy:                                 ownManagedBy,
				latticemodel.K8SSourceTypeKey:                        string(latticemodel.SourceTypeHTTPRoute),
				"application-networking.k8s.aws/K8SServiceNamespace": "secret-ns",
			},
			wantReason: gwv1.RouteReasonRefNotPermitted,
			wantMsg:    "Reference that Service with a backendRef",
		},
		{
			name: "target group of another cluster's route points to exporting it",
			tags: map[string]string{
				pkg_aws.TagManagedBy:                                 "111122223333/other/vpc-0",
				latticemodel.K8SSourceTypeKey:                        string(latticemodel.SourceTypeHTTPRoute),
				"application-networking.k8s.aws/K8SServiceNamespace": "secret-ns",
			},
			wantReason: gwv1.RouteReasonRefNotPermitted,
			wantMsg:    "Export that Service with a ServiceExport in its cluster",
		},
		{
			name: "target group of a ServiceExport points to export discovery",
			tags: map[string]string{
				pkg_aws.TagManagedBy:                                 "111122223333/other/vpc-0",
				latticemodel.K8SSourceTypeKey:                        string(latticemodel.SourceTypeSvcExport),
				"application-networking.k8s.aws/K8SServiceNamespace": "secret-ns",
			},
			wantReason: gwv1.RouteReasonRefNotPermitted,
			wantMsg:    "named after the exported Service",
		},
		{name: "unmanaged target group resolves", wantReason: gwv1.RouteReasonResolvedRefs},
		{
			// Returning the error keeps the previous status; the model build fails on the same
			// error and keeps the previous traffic, so the two stay consistent.
			name:    "tag read failure is returned",
			tagErr:  errors.New("throttled"),
			wantErr: true,
		},
		{
			// The model build reports these as ExternalTargetGroupNotFound, so status must
			// still be written instead of failing the whole validation.
			name:       "missing target group is left to the model build",
			tagErr:     &latticetypes.ResourceNotFoundException{Message: aws.String("nope")},
			wantReason: gwv1.RouteReasonResolvedRefs,
		},
		{
			name:       "unreadable target group is left to the model build",
			tagErr:     &latticetypes.AccessDeniedException{Message: aws.String("denied")},
			wantReason: gwv1.RouteReasonResolvedRefs,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			mockLattice := services.NewMockLattice(c)
			mockLattice.EXPECT().ListTagsForResource(gomock.Any(), gomock.Any()).
				Return(&vpclattice.ListTagsForResourceOutput{Tags: tt.tags}, tt.tagErr)
			mockCloud := pkg_aws.NewMockCloud(c)
			mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()
			mockCloud.EXPECT().DefaultTags().Return(services.Tags{pkg_aws.TagManagedBy: ownManagedBy}).AnyTimes()

			svcImport := &anv1alpha1.ServiceImport{ObjectMeta: metav1.ObjectMeta{
				Name: "imp", Namespace: "foo",
				Annotations: map[string]string{k8s.TargetGroupArnAnnotation: arn},
			}}
			r := newWarnReconciler(t, rgReconcilerTestClient(t, svcImport), record.NewFakeRecorder(10))
			r.cloud = mockCloud

			cnd, err := r.validateBackedRefs(context.Background(),
				routeWithBackendRefs("foo", backendRef("imp", "", "ServiceImport")))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, string(tt.wantReason), cnd.Reason)
			assert.Contains(t, cnd.Message, tt.wantMsg)
			assert.NotContains(t, cnd.Message, "111122223333")
			assert.NotContains(t, cnd.Message, "secret-ns")
			assert.NotContains(t, cnd.Message, "999988887777")
		})
	}
}
