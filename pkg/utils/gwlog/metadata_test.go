package gwlog

import (
	"context"
	"fmt"
	"testing"
)

func TestGetTrace(t *testing.T) {
	if GetTrace(context.TODO()) != "" {
		t.Errorf("expected context with no trace_id to return empty string")
	}

	if GetTrace(NewTrace(context.TODO())) == "" {
		t.Errorf("expected context with trace_id to return non-empty string")
	}
}

func TestMetadata(t *testing.T) {
	ctx := NewTrace(context.TODO())
	AddMetadata(ctx, "foo", "bar")

	md := GetMetadata(ctx)
	mdMap := map[string]bool{}
	for _, m := range md {
		mdMap[fmt.Sprint(m)] = true
	}

	if !mdMap["foo"] || !mdMap["bar"] {
		t.Errorf("expected context to have metadata with key foo and val bar, got %s", md)
	}
}
