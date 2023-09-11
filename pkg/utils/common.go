package utils

import (
	"strings"
)

type MapFunc[T any, U any] func(T) U
type FilterFunc[T any] func(T) bool

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
