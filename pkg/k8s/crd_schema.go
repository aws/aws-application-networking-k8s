package k8s

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// openAPIV3Document is the part of an OpenAPI v3 group-version document needed to inspect a CRD schema.
type openAPIV3Document struct {
	Components struct {
		Schemas map[string]openAPIV3Schema `json:"schemas"`
	} `json:"components"`
}

type openAPIV3Schema struct {
	Properties        map[string]openAPIV3Schema `json:"properties"`
	GroupVersionKinds []struct {
		Group   string `json:"group"`
		Version string `json:"version"`
		Kind    string `json:"kind"`
	} `json:"x-kubernetes-group-version-kind"`
}

// IsCRDSpecFieldServed reports whether the schema the API server serves for gvk defines spec.<field>.
// It reads the OpenAPI v3 discovery document, readable by every authenticated client, so no extra RBAC
// is needed. An error means the schema could not be determined, not that the field is missing.
func IsCRDSpecFieldServed(cfg *rest.Config, gvk schema.GroupVersionKind, field string) (bool, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return false, err
	}
	paths, err := dc.OpenAPIV3().Paths()
	if err != nil {
		return false, err
	}
	gv, ok := paths["apis/"+gvk.Group+"/"+gvk.Version]
	if !ok {
		return false, fmt.Errorf("group version %s is not served", gvk.GroupVersion())
	}
	doc, err := gv.Schema(runtime.ContentTypeJSON)
	if err != nil {
		return false, err
	}
	return openAPIV3DefinesSpecField(doc, gvk, field)
}

func openAPIV3DefinesSpecField(doc []byte, gvk schema.GroupVersionKind, field string) (bool, error) {
	var parsed openAPIV3Document
	if err := json.Unmarshal(doc, &parsed); err != nil {
		return false, fmt.Errorf("parsing the OpenAPI v3 document for %s: %w", gvk, err)
	}
	for _, s := range parsed.Components.Schemas {
		for _, g := range s.GroupVersionKinds {
			if g.Group != gvk.Group || g.Version != gvk.Version || g.Kind != gvk.Kind {
				continue
			}
			spec, ok := s.Properties["spec"]
			if !ok {
				return false, nil
			}
			_, ok = spec.Properties[field]
			return ok, nil
		}
	}
	return false, fmt.Errorf("schema for %s not found in the served OpenAPI v3 document", gvk)
}
