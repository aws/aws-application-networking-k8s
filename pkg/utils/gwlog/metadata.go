package gwlog

import (
	"context"
	"github.com/google/uuid"
)

type key string

const metadata key = "metadata"

const traceID string = "trace_id"

type metadataValue struct {
	m map[string]string
}

func (mv *metadataValue) get(key string) string {
	return mv.m[key]
}

func (mv *metadataValue) set(key, val string) {
	mv.m[key] = val
}

func newMetadata() *metadataValue {
	return &metadataValue{
		m: make(map[string]string),
	}
}

func NewTrace(ctx context.Context) context.Context {
	currID := uuid.New()

	newCtx := context.WithValue(ctx, metadata, newMetadata())
	AddMetadata(newCtx, traceID, currID.String())

	return newCtx
}

func AddMetadata(ctx context.Context, key, value string) {
	if ctx.Value(metadata) != nil {
		ctx.Value(metadata).(*metadataValue).set(key, value)
	}
}

func GetMetadata(ctx context.Context) []interface{} {
	var fields []interface{}

	if ctx.Value(metadata) != nil {
		for k, v := range ctx.Value(metadata).(*metadataValue).m {
			fields = append(fields, k)
			fields = append(fields, v)
		}
	}
	return fields
}

func GetTrace(ctx context.Context) string {
	if ctx.Value(metadata) != nil {
		m := ctx.Value(metadata).(*metadataValue).m
		if m == nil {
			return ""
		}
		return ctx.Value(metadata).(*metadataValue).m[traceID]
	}
	return ""
}
