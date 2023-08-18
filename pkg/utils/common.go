package utils

import "strings"

type MapFunc[T any, U any] func(T) U

// TODO: should be check by API call (Mingxi)
func ArntoId(arn string) string {
	if len(arn) == 0 {
		return ""
	}
	return arn[len(arn)-22:]
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
