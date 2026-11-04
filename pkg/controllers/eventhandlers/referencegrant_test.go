package eventhandlers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func httpRoute(name, namespace, backendNamespace string) *gwv1.HTTPRoute {
	ref := gwv1.HTTPBackendRef{
		BackendRef: gwv1.BackendRef{
			BackendObjectReference: gwv1.BackendObjectReference{Name: "svc"},
		},
	}
	if backendNamespace != "" {
		ref.Namespace = ptr.To(gwv1.Namespace(backendNamespace))
	}
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: gwv1.HTTPRouteSpec{
			Rules: []gwv1.HTTPRouteRule{{BackendRefs: []gwv1.HTTPBackendRef{ref}}},
		},
	}
}

func referenceGrant(namespace, fromNamespace, fromKind string) *gwv1.ReferenceGrant {
	return &gwv1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "grant", Namespace: namespace},
		Spec: gwv1.ReferenceGrantSpec{
			From: []gwv1.ReferenceGrantFrom{{
				Group:     gwv1.Group(gwv1.GroupName),
				Kind:      gwv1.Kind(fromKind),
				Namespace: gwv1.Namespace(fromNamespace),
			}},
			To: []gwv1.ReferenceGrantTo{{Group: gwv1.Group(""), Kind: gwv1.Kind("Service")}},
		},
	}
}

func TestReferenceGrantToRoutes(t *testing.T) {
	tests := []struct {
		name       string
		routes     []client.Object
		grant      *gwv1.ReferenceGrant
		wantRoutes []string
	}{
		{
			name:       "route in from namespace referencing grant namespace is selected",
			routes:     []client.Object{httpRoute("r1", "foo", "bar")},
			grant:      referenceGrant("bar", "foo", "HTTPRoute"),
			wantRoutes: []string{"r1"},
		},
		{
			name:       "route not referencing the grant namespace is not selected",
			routes:     []client.Object{httpRoute("r1", "foo", "other")},
			grant:      referenceGrant("bar", "foo", "HTTPRoute"),
			wantRoutes: nil,
		},
		{
			name:       "route in a different namespace than from is not selected",
			routes:     []client.Object{httpRoute("r1", "elsewhere", "bar")},
			grant:      referenceGrant("bar", "foo", "HTTPRoute"),
			wantRoutes: nil,
		},
		{
			name: "only routes referencing the grant namespace are selected",
			routes: []client.Object{
				httpRoute("r1", "foo", "bar"),
				httpRoute("r2", "foo", "other"),
			},
			grant:      referenceGrant("bar", "foo", "HTTPRoute"),
			wantRoutes: []string{"r1"},
		},
		{
			name:       "nil grant selects nothing",
			routes:     []client.Object{httpRoute("r1", "foo", "bar")},
			grant:      nil,
			wantRoutes: nil,
		},
		{
			name:       "same-namespace backendRef (nil namespace) is not selected and does not panic",
			routes:     []client.Object{httpRoute("r1", "foo", "")},
			grant:      referenceGrant("bar", "foo", "HTTPRoute"),
			wantRoutes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			assert.NoError(t, clientgoscheme.AddToScheme(scheme))
			assert.NoError(t, gwv1.Install(scheme))
			c := testclient.NewClientBuilder().WithScheme(scheme).WithObjects(tt.routes...).Build()

			mapper := &resourceMapper{log: gwlog.FallbackLogger, client: c}
			got := mapper.ReferenceGrantToRoutes(context.Background(), tt.grant, core.HttpRouteType)

			var names []string
			for _, r := range got {
				names = append(names, r.Name())
			}
			assert.ElementsMatch(t, tt.wantRoutes, names)
		})
	}
}

func TestReferenceGrantToRoutes_MultipleFromNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))
	assert.NoError(t, gwv1.Install(scheme))
	c := testclient.NewClientBuilder().WithScheme(scheme).WithObjects(
		httpRoute("r1", "foo", "bar"),
		httpRoute("r2", "qux", "bar"),
		httpRoute("r3", "foo", "other"),
	).Build()

	grant := &gwv1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "grant", Namespace: "bar"},
		Spec: gwv1.ReferenceGrantSpec{
			From: []gwv1.ReferenceGrantFrom{
				{Group: gwv1.Group(gwv1.GroupName), Kind: gwv1.Kind("HTTPRoute"), Namespace: gwv1.Namespace("foo")},
				{Group: gwv1.Group(gwv1.GroupName), Kind: gwv1.Kind("HTTPRoute"), Namespace: gwv1.Namespace("qux")},
			},
			To: []gwv1.ReferenceGrantTo{{Group: gwv1.Group(""), Kind: gwv1.Kind("Service")}},
		},
	}

	mapper := &resourceMapper{log: gwlog.FallbackLogger, client: c}
	got := mapper.ReferenceGrantToRoutes(context.Background(), grant, core.HttpRouteType)

	var names []string
	for _, r := range got {
		names = append(names, r.Name())
	}
	assert.ElementsMatch(t, []string{"r1", "r2"}, names)
}

func grpcRoute(name, namespace, backendNamespace string) *gwv1.GRPCRoute {
	ref := gwv1.GRPCBackendRef{
		BackendRef: gwv1.BackendRef{
			BackendObjectReference: gwv1.BackendObjectReference{Name: "svc"},
		},
	}
	if backendNamespace != "" {
		ref.Namespace = ptr.To(gwv1.Namespace(backendNamespace))
	}
	return &gwv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: gwv1.GRPCRouteSpec{
			Rules: []gwv1.GRPCRouteRule{{BackendRefs: []gwv1.GRPCBackendRef{ref}}},
		},
	}
}

func tlsRoute(name, namespace, backendNamespace string) *gwv1.TLSRoute {
	ref := gwv1.BackendRef{
		BackendObjectReference: gwv1.BackendObjectReference{Name: "svc"},
	}
	if backendNamespace != "" {
		ref.Namespace = ptr.To(gwv1.Namespace(backendNamespace))
	}
	return &gwv1.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: gwv1.TLSRouteSpec{
			Rules: []gwv1.TLSRouteRule{{BackendRefs: []gwv1.BackendRef{ref}}},
		},
	}
}

func TestReferenceGrantToRoutes_RouteKinds(t *testing.T) {
	tests := []struct {
		name      string
		routeType core.RouteType
		routes    []client.Object
		grant     *gwv1.ReferenceGrant
		want      []string
	}{
		{
			name:      "GRPCRoute selected by matching grant",
			routeType: core.GrpcRouteType,
			routes:    []client.Object{grpcRoute("g1", "foo", "bar")},
			grant:     referenceGrant("bar", "foo", "GRPCRoute"),
			want:      []string{"g1"},
		},
		{
			name:      "GRPCRoute not selected by HTTPRoute grant",
			routeType: core.GrpcRouteType,
			routes:    []client.Object{grpcRoute("g1", "foo", "bar")},
			grant:     referenceGrant("bar", "foo", "HTTPRoute"),
			want:      nil,
		},
		{
			name:      "TLSRoute not selected by HTTPRoute grant",
			routeType: core.TlsRouteType,
			routes:    []client.Object{tlsRoute("t1", "foo", "bar")},
			grant:     referenceGrant("bar", "foo", "HTTPRoute"),
			want:      nil,
		},
		{
			name:      "GRPCRoute referencing another namespace not selected",
			routeType: core.GrpcRouteType,
			routes:    []client.Object{grpcRoute("g1", "foo", "other")},
			grant:     referenceGrant("bar", "foo", "GRPCRoute"),
			want:      nil,
		},
		{
			name:      "TLSRoute selected by matching grant",
			routeType: core.TlsRouteType,
			routes:    []client.Object{tlsRoute("t1", "foo", "bar")},
			grant:     referenceGrant("bar", "foo", "TLSRoute"),
			want:      []string{"t1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			assert.NoError(t, clientgoscheme.AddToScheme(scheme))
			assert.NoError(t, gwv1.Install(scheme))
			c := testclient.NewClientBuilder().WithScheme(scheme).WithObjects(tt.routes...).Build()

			mapper := &resourceMapper{log: gwlog.FallbackLogger, client: c}
			got := mapper.ReferenceGrantToRoutes(context.Background(), tt.grant, tt.routeType)

			var names []string
			for _, r := range got {
				names = append(names, r.Name())
			}
			assert.ElementsMatch(t, tt.want, names)
		})
	}
}

func TestReferenceGrantToRoutes_ListErrorReturnsNil(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))
	assert.NoError(t, gwv1.Install(scheme))
	c := testclient.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return errors.New("boom")
			},
		}).Build()

	mapper := &resourceMapper{log: gwlog.FallbackLogger, client: c}
	got := mapper.ReferenceGrantToRoutes(context.Background(), referenceGrant("bar", "foo", "HTTPRoute"), core.HttpRouteType)
	assert.Nil(t, got)
}

func TestReferenceGrantEventHandler_MapToRoute(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))
	assert.NoError(t, gwv1.Install(scheme))
	c := testclient.NewClientBuilder().WithScheme(scheme).
		WithObjects(httpRoute("r1", "foo", "bar")).Build()

	h := NewReferenceGrantEventHandler(gwlog.FallbackLogger, c)

	// A non-ReferenceGrant object yields no requests.
	assert.Nil(t, h.mapToRoute(context.Background(), &gwv1.HTTPRoute{}, core.HttpRouteType))

	reqs := h.mapToRoute(context.Background(), referenceGrant("bar", "foo", "HTTPRoute"), core.HttpRouteType)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "r1", reqs[0].Name)
	assert.Equal(t, "foo", reqs[0].Namespace)
}
