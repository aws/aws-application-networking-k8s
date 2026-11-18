package k8s

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

var vapGVK = schema.GroupVersionKind{Group: "application-networking.k8s.aws", Version: "v1alpha1", Kind: "VpcAssociationPolicy"}

// vapOpenAPIV3Doc mimics the group-version document the API server serves for the CRD; extraSpec
// is spliced into spec.properties.
func vapOpenAPIV3Doc(extraSpec string) string {
	return fmt.Sprintf(`{"openapi":"3.0.0","components":{"schemas":{
  "aws.k8s.application-networking.v1alpha1.VpcAssociationPolicy":{"type":"object","properties":{
     "apiVersion":{"type":"string"},
     "spec":{"type":"object","properties":{"associateWithVpc":{"type":"boolean"}%s,"targetRef":{"type":"object"}}},
     "status":{"type":"object"}},
     "x-kubernetes-group-version-kind":[{"group":"application-networking.k8s.aws","kind":"VpcAssociationPolicy","version":"v1alpha1"}]},
  "aws.k8s.application-networking.v1alpha1.VpcAssociationPolicyList":{"type":"object",
     "x-kubernetes-group-version-kind":[{"group":"application-networking.k8s.aws","kind":"VpcAssociationPolicyList","version":"v1alpha1"}]}
}}}`, extraSpec)
}

const privateDnsSpec = `,"privateDnsEnabled":{"type":"boolean"},"dnsOptions":{"type":"object"}`

func newOpenAPIServer(t *testing.T, groupVersionDoc string, serveGroupVersion bool) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/v3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		paths := `{}`
		if serveGroupVersion {
			paths = `{"apis/application-networking.k8s.aws/v1alpha1":{"serverRelativeURL":"/openapi/v3/apis/application-networking.k8s.aws/v1alpha1?hash=test"}}`
		}
		_, _ = w.Write([]byte(`{"paths":` + paths + `}`))
	})
	mux.HandleFunc("/openapi/v3/apis/application-networking.k8s.aws/v1alpha1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(groupVersionDoc))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func Test_IsCRDSpecFieldServed_NewCRD(t *testing.T) {
	srv := newOpenAPIServer(t, vapOpenAPIV3Doc(privateDnsSpec), true)
	served, err := IsCRDSpecFieldServed(&rest.Config{Host: srv.URL}, vapGVK, "privateDnsEnabled")
	assert.NoError(t, err)
	assert.True(t, served)
}

func Test_IsCRDSpecFieldServed_OldCRD(t *testing.T) {
	srv := newOpenAPIServer(t, vapOpenAPIV3Doc(""), true)
	served, err := IsCRDSpecFieldServed(&rest.Config{Host: srv.URL}, vapGVK, "privateDnsEnabled")
	assert.NoError(t, err)
	assert.False(t, served, "a CRD without the field must be reported as not serving it")
}

func Test_IsCRDSpecFieldServed_GroupVersionNotServed_IsAnError(t *testing.T) {
	srv := newOpenAPIServer(t, vapOpenAPIV3Doc(privateDnsSpec), false)
	_, err := IsCRDSpecFieldServed(&rest.Config{Host: srv.URL}, vapGVK, "privateDnsEnabled")
	assert.Error(t, err, "an absent group version is unknown, not a missing field")
}

func Test_openAPIV3DefinesSpecField(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		want    bool
		wantErr bool
	}{
		{name: "field present", doc: vapOpenAPIV3Doc(privateDnsSpec), want: true},
		{name: "field absent", doc: vapOpenAPIV3Doc(""), want: false},
		{name: "kind not in document", doc: `{"components":{"schemas":{"other":{"x-kubernetes-group-version-kind":[{"group":"g","version":"v1","kind":"Other"}]}}}}`, wantErr: true},
		{name: "kind without spec", doc: `{"components":{"schemas":{"vap":{"type":"object","x-kubernetes-group-version-kind":[{"group":"application-networking.k8s.aws","version":"v1alpha1","kind":"VpcAssociationPolicy"}]}}}}`, want: false},
		{name: "invalid json", doc: `{`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := openAPIV3DefinesSpecField([]byte(tt.doc), vapGVK, "privateDnsEnabled")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
