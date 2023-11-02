package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunks(t *testing.T) {
	size := 2
	obj1 := &struct{}{}
	obj2 := &struct{}{}
	obj3 := &struct{}{}

	t.Run("[obj1] -> [[obj1]]", func(t *testing.T) {

		in := []*struct{}{obj1}
		out := Chunks(in, size)
		assert.Equal(t, [][]*struct{}{{obj1}}, out)

	})

	t.Run("[obj1,obj2] -> [[obj1,obj2]]", func(t *testing.T) {

		in := []*struct{}{obj1, obj2}
		got := Chunks(in, size)
		assert.Equal(t, [][]*struct{}{{obj1, obj2}}, got)

	})

	t.Run("[obj1, obj2, obj3] -> [[obj1,obj2],[obj3]]", func(t *testing.T) {
		in := []*struct{}{obj1, obj2, obj3}
		got := Chunks(in, size)
		assert.Equal(t, [][]*struct{}{{obj1, obj2}, {obj3}}, got)
	})
}
