package gwlog

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestErrorwPreservesStructuredFields(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := &TracedLogger{InnerLogger: zap.New(core).Sugar()}
	ctx := NewTrace(context.Background())

	logger.Errorw(ctx, "reconcile error", "name", "route", "message", "boom")

	if logs.Len() != 1 {
		t.Fatalf("expected one error log, got %d", logs.Len())
	}

	entry := logs.All()[0]
	if entry.Level != zap.ErrorLevel {
		t.Fatalf("expected error level, got %s", entry.Level)
	}
	if entry.Message != "reconcile error" {
		t.Fatalf("expected reconcile error message, got %q", entry.Message)
	}

	fields := entry.ContextMap()
	if fields["name"] != "route" {
		t.Fatalf("expected name field to be preserved, got %#v", fields["name"])
	}
	if fields["message"] != "boom" {
		t.Fatalf("expected message field to be preserved, got %#v", fields["message"])
	}
	if fields[traceID] == "" {
		t.Fatalf("expected trace id field to be preserved")
	}
}
