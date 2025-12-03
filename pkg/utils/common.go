package utils

import (
	"fmt"
	"math/rand"
	"strings"

	"golang.org/x/exp/constraints"
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

func SliceMapToPtr[T any](in []T) []*T {
	return SliceMap(in, func(t T) *T { return &t })
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

func LatticeServiceName(k8sSourceRouteName string, k8sSourceRouteNamespace string, serviceNameOverride string) string {
	if serviceNameOverride != "" {
		return serviceNameOverride
	}

	return fmt.Sprintf("%s-%s", Truncate(k8sSourceRouteName, 20), Truncate(k8sSourceRouteNamespace, 18))
}

func RandomAlphaString(length int) string {
	str := make([]rune, length)
	for i := range str {
		str[i] = rune(rand.Intn(26) + 'a')
	}
	return string(str)
}

type none struct{}

type Set[T comparable] struct {
	set map[T]none
}

func NewSet[T comparable](objs ...T) Set[T] {
	s := Set[T]{
		set: make(map[T]none, len(objs)),
	}
	for _, t := range objs {
		s.set[t] = none{}
	}
	return s
}

func (s *Set[T]) Put(t T) {
	if s.set == nil {
		s.set = map[T]none{}
	}
	s.set[t] = none{}
}

func (s *Set[T]) Delete(t T) {
	delete(s.set, t)
}

func (s *Set[T]) Contains(t T) bool {
	if s.set == nil {
		s.set = map[T]none{}
	}
	_, ok := s.set[t]
	return ok
}

func (s *Set[T]) Items() []T {
	out := make([]T, len(s.set))
	i := 0
	for t := range s.set {
		out[i] = t
		i++
	}
	return out
}
