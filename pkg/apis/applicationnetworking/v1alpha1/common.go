package v1alpha1

func toPtrSlice[T any](s []T) []*T {
	ps := make([]*T, len(s))
	for i, t := range s {
		ct := t
		ps[i] = &ct
	}
	return ps
}
