package utils

import (
	"reflect"
	"testing"
)

func TestChunks(t *testing.T) {
	obj1 := &struct{}{}
	obj2 := &struct{}{}
	obj3 := &struct{}{}
	obj4 := &struct{}{}
	obj5 := &struct{}{}

	type testCase struct {
		name string
		in   []*struct{}
		size int
		want [][]*struct{}
	}
	tests := []testCase{
		{
			name: "No items, no chunks",
			in:   []*struct{}{},
			size: 2,
			want: [][]*struct{}{},
		},
		{
			name: "single item, single chunk: [obj1] -> [[obj1]]",
			in:   []*struct{}{obj1},
			size: 2,
			want: [][]*struct{}{{obj1}},
		},
		{
			name: "[obj1, obj2] -> [[obj1,obj2]]",
			in:   []*struct{}{obj1, obj2},
			size: 2,
			want: [][]*struct{}{{obj1, obj2}},
		},
		{
			name: "[obj1, obj2, obj3] -> [[obj1,obj2],[obj3]]",
			in:   []*struct{}{obj1, obj2, obj3},
			size: 2,
			want: [][]*struct{}{{obj1, obj2}, {obj3}},
		},
		{
			name: "[obj1, obj2, obj3, obj4] -> [[obj1,obj2],[obj3,obj4]]",
			in:   []*struct{}{obj1, obj2, obj3, obj4},
			size: 2,
			want: [][]*struct{}{{obj1, obj2}, {obj3, obj4}},
		},
		{
			name: "[obj1, obj2, obj3, obj4, obj5] -> [[obj1,obj2],[obj3,obj4],[obj5]]",
			in:   []*struct{}{obj1, obj2, obj3, obj4, obj5},
			size: 2,
			want: [][]*struct{}{{obj1, obj2}, {obj3, obj4}, {obj5}},
		},
		{
			name: "size 0",
			in:   []*struct{}{obj1, obj2, obj3, obj4, obj5},
			size: 0,
			want: [][]*struct{}{},
		},
		{
			name: "negative size",
			in:   []*struct{}{obj1, obj2, obj3, obj4, obj5},
			size: -1,
			want: [][]*struct{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Chunks(tt.in, tt.size); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Chunks() got %v, want %v", got, tt.want)
			}
		})
	}
}
