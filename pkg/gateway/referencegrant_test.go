package gateway

import (
	"context"
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

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

func rgScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))
	assert.NoError(t, gwv1.Install(scheme))
	assert.NoError(t, anv1alpha1.Install(scheme))
	return scheme
}

func rgTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return testclient.NewClientBuilder().WithScheme(rgScheme(t)).WithObjects(objs...).Build()
}

func httpRouteWithBackend(routeNs string, backend gwv1.HTTPBackendRef) core.Route {
	return core.NewHTTPRoute(gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: routeNs},
		Spec: gwv1.HTTPRouteSpec{
			Rules: []gwv1.HTTPRouteRule{
				{BackendRefs: []gwv1.HTTPBackendRef{backend}},
			},
		},
	})
}

func serviceBackend(name, namespace string) gwv1.HTTPBackendRef {
	ref := gwv1.HTTPBackendRef{
		BackendRef: gwv1.BackendRef{
			BackendObjectReference: gwv1.BackendObjectReference{
				Name: gwv1.ObjectName(name),
			},
		},
	}
	if namespace != "" {
		ref.Namespace = ptr.To(gwv1.Namespace(namespace))
	}
	return ref
}

func serviceImportBackend(name, namespace string, withGroup bool) gwv1.HTTPBackendRef {
	ref := serviceBackend(name, namespace)
	ref.Kind = ptr.To(gwv1.Kind("ServiceImport"))
	if withGroup {
		ref.Group = ptr.To(gwv1.Group(anv1alpha1.GroupName))
	}
	return ref
}

func grant(namespace string, from gwv1.ReferenceGrantFrom, to gwv1.ReferenceGrantTo) *gwv1.ReferenceGrant {
	return namedGrant("grant", namespace, from, to)
}

func namedGrant(name, namespace string, from gwv1.ReferenceGrantFrom, to gwv1.ReferenceGrantTo) *gwv1.ReferenceGrant {
	return &gwv1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: gwv1.ReferenceGrantSpec{
			From: []gwv1.ReferenceGrantFrom{from},
			To:   []gwv1.ReferenceGrantTo{to},
		},
	}
}

func httpRouteFrom(namespace string) gwv1.ReferenceGrantFrom {
	return gwv1.ReferenceGrantFrom{
		Group:     gwv1.Group(gwv1.GroupName),
		Kind:      gwv1.Kind("HTTPRoute"),
		Namespace: gwv1.Namespace(namespace),
	}
}

func serviceTo(name string) gwv1.ReferenceGrantTo {
	to := gwv1.ReferenceGrantTo{Group: gwv1.Group(""), Kind: gwv1.Kind("Service")}
	if name != "" {
		to.Name = ptr.To(gwv1.ObjectName(name))
	}
	return to
}

func TestIsBackendRefAllowedByReferenceGrant(t *testing.T) {
	tests := []struct {
		name    string
		routeNs string
		backend gwv1.HTTPBackendRef
		grants  []client.Object
		want    bool
	}{
		{
			name:    "same namespace, no explicit namespace, allowed without grant",
			routeNs: "foo",
			backend: serviceBackend("svc", ""),
			want:    true,
		},
		{
			name:    "same namespace, explicit matching namespace, allowed without grant",
			routeNs: "foo",
			backend: serviceBackend("svc", "foo"),
			want:    true,
		},
		{
			name:    "cross namespace, no grant, denied",
			routeNs: "foo",
			backend: serviceBackend("svc", "bar"),
			want:    false,
		},
		{
			name:    "cross namespace, whole-namespace grant, allowed",
			routeNs: "foo",
			backend: serviceBackend("svc", "bar"),
			grants:  []client.Object{grant("bar", httpRouteFrom("foo"), serviceTo(""))},
			want:    true,
		},
		{
			name:    "cross namespace, named grant matching, allowed",
			routeNs: "foo",
			backend: serviceBackend("svc", "bar"),
			grants:  []client.Object{grant("bar", httpRouteFrom("foo"), serviceTo("svc"))},
			want:    true,
		},
		{
			name:    "cross namespace, named grant mismatched, denied",
			routeNs: "foo",
			backend: serviceBackend("svc", "bar"),
			grants:  []client.Object{grant("bar", httpRouteFrom("foo"), serviceTo("other"))},
			want:    false,
		},
		{
			name:    "cross namespace, grant to wrong kind, denied",
			routeNs: "foo",
			backend: serviceBackend("svc", "bar"),
			grants: []client.Object{grant("bar", httpRouteFrom("foo"),
				gwv1.ReferenceGrantTo{Group: gwv1.Group(anv1alpha1.GroupName), Kind: gwv1.Kind("ServiceImport")})},
			want: false,
		},
		{
			name:    "cross namespace, grant from wrong source namespace, denied",
			routeNs: "foo",
			backend: serviceBackend("svc", "bar"),
			grants:  []client.Object{grant("bar", httpRouteFrom("other"), serviceTo(""))},
			want:    false,
		},
		{
			name:    "cross namespace, grant in wrong namespace, denied",
			routeNs: "foo",
			backend: serviceBackend("svc", "bar"),
			grants:  []client.Object{grant("baz", httpRouteFrom("foo"), serviceTo(""))},
			want:    false,
		},
		{
			name:    "cross namespace, multiple grants one matches, allowed",
			routeNs: "foo",
			backend: serviceBackend("svc", "bar"),
			grants: []client.Object{
				namedGrant("g1", "bar", httpRouteFrom("other"), serviceTo("")),
				namedGrant("g2", "bar", httpRouteFrom("foo"), serviceTo("svc")),
			},
			want: true,
		},
		{
			name:    "cross namespace ServiceImport with explicit group, grant allows, allowed",
			routeNs: "foo",
			backend: serviceImportBackend("imp", "bar", true),
			grants: []client.Object{grant("bar", httpRouteFrom("foo"),
				gwv1.ReferenceGrantTo{Group: gwv1.Group(anv1alpha1.GroupName), Kind: gwv1.Kind("ServiceImport")})},
			want: true,
		},
		{
			name:    "cross namespace ServiceImport with omitted group, grant allows, allowed",
			routeNs: "foo",
			backend: serviceImportBackend("imp", "bar", false),
			grants: []client.Object{grant("bar", httpRouteFrom("foo"),
				gwv1.ReferenceGrantTo{Group: gwv1.Group(anv1alpha1.GroupName), Kind: gwv1.Kind("ServiceImport")})},
			want: true,
		},
		{
			name:    "cross namespace ServiceImport, no grant, denied",
			routeNs: "foo",
			backend: serviceImportBackend("imp", "bar", false),
			want:    false,
		},
		{
			name:    "cross namespace ServiceImport with foreign group, grant to app group, allowed",
			routeNs: "foo",
			backend: func() gwv1.HTTPBackendRef {
				ref := serviceBackend("imp", "bar")
				ref.Kind = ptr.To(gwv1.Kind("ServiceImport"))
				ref.Group = ptr.To(gwv1.Group("multicluster.x-k8s.io"))
				return ref
			}(),
			grants: []client.Object{grant("bar", httpRouteFrom("foo"),
				gwv1.ReferenceGrantTo{Group: gwv1.Group(anv1alpha1.GroupName), Kind: gwv1.Kind("ServiceImport")})},
			want: true,
		},
		{
			name:    "cross namespace, grant from wrong route kind, denied",
			routeNs: "foo",
			backend: serviceBackend("svc", "bar"),
			grants: []client.Object{grant("bar",
				gwv1.ReferenceGrantFrom{Group: gwv1.Group(gwv1.GroupName), Kind: gwv1.Kind("GRPCRoute"), Namespace: gwv1.Namespace("foo")},
				serviceTo(""))},
			want: false,
		},
		{
			name:    "cross namespace, grant from wrong group, denied",
			routeNs: "foo",
			backend: serviceBackend("svc", "bar"),
			grants: []client.Object{grant("bar",
				gwv1.ReferenceGrantFrom{Group: gwv1.Group("example.com"), Kind: gwv1.Kind("HTTPRoute"), Namespace: gwv1.Namespace("foo")},
				serviceTo(""))},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := rgTestClient(t, tt.grants...)
			route := httpRouteWithBackend(tt.routeNs, tt.backend)
			backendRef := route.Spec().Rules()[0].BackendRefs()[0]

			got, err := IsBackendRefAllowedByReferenceGrant(context.Background(), c, route, backendRef)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsBackendRefAllowedByReferenceGrant_SameNamespaceSkipsList(t *testing.T) {
	c := testclient.NewClientBuilder().
		WithScheme(rgScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				t.Fatal("List should not be called for a same-namespace reference")
				return nil
			},
		}).Build()
	route := httpRouteWithBackend("foo", serviceBackend("svc", "foo"))
	backendRef := route.Spec().Rules()[0].BackendRefs()[0]

	got, err := IsBackendRefAllowedByReferenceGrant(context.Background(), c, route, backendRef)
	assert.NoError(t, err)
	assert.True(t, got)
}

func TestIsBackendRefAllowedByReferenceGrant_ListErrorPropagates(t *testing.T) {
	c := testclient.NewClientBuilder().
		WithScheme(rgScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return assert.AnError
			},
		}).Build()
	route := httpRouteWithBackend("foo", serviceBackend("svc", "bar"))
	backendRef := route.Spec().Rules()[0].BackendRefs()[0]

	got, err := IsBackendRefAllowedByReferenceGrant(context.Background(), c, route, backendRef)
	assert.Error(t, err)
	assert.False(t, got)
}
