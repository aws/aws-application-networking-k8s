package gwlog

import (
	"context"
	"github.com/google/uuid"
)

type key string

const traceID key = "trace_id"
const metadata key = "metadata"

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
	return context.WithValue(context.WithValue(ctx, traceID, currID.String()), metadata, newMetadata())
}

func AddMetadata(ctx context.Context, key, value string) {
	if ctx.Value(metadata) != nil {
		ctx.Value(metadata).(*metadataValue).set(key, value)
	}
}

func GetMetadata(ctx context.Context) []interface{} {
	var fields []interface{}
	/*
		if ctx.Value(traceID) != nil {
			fields = append(fields, string(traceID))
			fields = append(fields, ctx.Value(traceID))
		}
	*/
	if ctx.Value(metadata) != nil {
		for k, v := range ctx.Value(metadata).(*metadataValue).m {
			fields = append(fields, k)
			fields = append(fields, v)
		}
	}
	return fields
}

func GetTrace(ctx context.Context) string {
	t := ctx.Value(traceID)
	if t == nil {
		return ""
	}
	return t.(string)
}
