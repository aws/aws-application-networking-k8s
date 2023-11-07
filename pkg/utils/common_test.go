package utils

import (
	"fmt"
	"testing"
)

func TestChunks(t *testing.T) {

	type test struct {
		sliceLen         int
		chunkSize        int
		wantChunksLen    int
		wantLastChunkLen int
	}

	tests := []test{
		{0, -1, 0, 0},
		{0, 0, 0, 0},
		{0, 1, 0, 0},
		{1, 1, 1, 1},
		{2, 1, 2, 1},
		{10, 2, 5, 2},
		{102, 10, 11, 2},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("sliceLen=%d, chunkSize=%d", tt.sliceLen, tt.chunkSize), func(t *testing.T) {
			slice := makeIntSlice(tt.sliceLen)
			chunks := Chunks(slice, tt.chunkSize)
			gotChunksLen := len(chunks)
			if gotChunksLen != tt.wantChunksLen {
				t.Errorf("number of chunks does not match, want=%d, got=%d", tt.wantChunksLen, gotChunksLen)
			}
			if gotChunksLen > 0 {
				lastChunk := chunks[gotChunksLen-1]
				if len(lastChunk) != tt.wantLastChunkLen {
					t.Errorf("last chunk size does not match, want=%d, got=%d", tt.wantLastChunkLen, len(lastChunk))
				}
			}
		})
	}
}

func makeIntSlice(s int) []int {
	out := make([]int, s)
	for i := 0; i < s; i++ {
		out[i] = i
	}
	return out
}
