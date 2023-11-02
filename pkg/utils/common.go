package utils

import (
	"fmt"
	"strings"

	"golang.org/x/exp/constraints"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type MapFunc[T any, U any] func(T) U
type FilterFunc[T any] func(T) bool

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func Chunks[T any](in []T, size int) [][]T {
	out := [][]T{}
	if size <= 0 {
		return out
	}
	for low := 0; low < len(in); low += size {
		high := min(low+size, len(in))
		out = append(out, in[low:high])
	}
	return out
}

func Truncate(name string, length int) string {
	if len(name) > length {
		name = name[:length]
	}
	return strings.Trim(name, "-")
}

func SliceMap[T any, U any](in []T, f MapFunc[T, U]) []U {
	out := make([]U, len(in))
	for i, t := range in {
		out[i] = f(t)
	}
	return out
}

func SliceFilter[T any](in []T, f FilterFunc[T]) []T {
	out := []T{}
	for _, t := range in {
		if f(t) {
			out = append(out, t)
		}
	}
	return out
}

func LatticeServiceName(k8sSourceRouteName string, k8sSourceRouteNamespace string) string {
	return fmt.Sprintf("%s-%s", Truncate(k8sSourceRouteName, 20), Truncate(k8sSourceRouteNamespace, 18))
}

func TargetRefToLatticeResourceName(
	targetRef *gwv1alpha2.PolicyTargetReference,
	parentNamespace string,
) (string, error) {
	// For Service Network, the name is just the Gateway's name.
	if targetRef.Kind == "Gateway" {
		return string(targetRef.Name), nil
	}

	// For VPC Lattice Service, the name is Route's name, followed by hyphen (-), then the Route's namespace.
	// If the Route's namespace is not provided, we assume it's the parent's namespace.
	if targetRef.Kind == "HTTPRoute" || targetRef.Kind == "GRPCRoute" {
		namespace := parentNamespace
		if targetRef.Namespace != nil {
			namespace = string(*targetRef.Namespace)
		}
		return LatticeServiceName(string(targetRef.Name), namespace), nil
	}

	return "", fmt.Errorf("unsupported targetRef Kind: %s", targetRef.Kind)
}
