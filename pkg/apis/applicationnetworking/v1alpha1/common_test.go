package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToPtrSlice(t *testing.T) {

	type A struct {
		x int
	}

	type test struct {
		name string
		in   []A
		want []*A
	}

	tests := []test{
		{"empty", []A{}, []*A{}},
		{"single item", []A{{1}}, []*A{{1}}},
		{"multiple items", []A{{1}, {2}, {3}}, []*A{{1}, {2}, {3}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, toPtrSlice(tt.in), tt.want)
		})
	}
}
