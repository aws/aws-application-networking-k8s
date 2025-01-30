package gwlog

import (
	"context"

	"github.com/google/uuid"
)

type key string

const metadataKey key = "metadata_key"
const traceID string = "trace_id"

type metadata struct {
	m map[string]string
}

func (mv *metadata) set(key, val string) {
	mv.m[key] = val
}

func newMetadata() *metadata {
	return &metadata{
		m: make(map[string]string),
	}
}

func NewTrace(ctx context.Context) context.Context {
	currID := uuid.New()

	newCtx := context.WithValue(ctx, metadataKey, newMetadata())
	AddMetadata(newCtx, traceID, currID.String())

	return newCtx
}

func AddMetadata(ctx context.Context, key, value string) {
	if ctx.Value(metadataKey) != nil {
		ctx.Value(metadataKey).(*metadata).set(key, value)
	}
}

func getMetadata(ctx context.Context) []interface{} {
	var fields []interface{}

	if ctx.Value(metadataKey) != nil {
		for k, v := range ctx.Value(metadataKey).(*metadata).m {
			if k == traceID {
				// skip since there's a separate method to grab the trace id
				continue
			}
			fields = append(fields, k)
			fields = append(fields, v)
		}
	}
	return fields
}

func GetTraceID(ctx context.Context) string {
	if ctx.Value(metadataKey) != nil {
		m := ctx.Value(metadataKey).(*metadata).m
		if m == nil {
			return ""
		}
		return ctx.Value(metadataKey).(*metadata).m[traceID]
	}
	return ""
}

func StartReconcileTrace(ctx context.Context, log Logger, k8sresourcetype, name, namespace string) context.Context {
	ctx = NewTrace(ctx)
	AddMetadata(ctx, "type", k8sresourcetype)
	AddMetadata(ctx, "name", name)
	AddMetadata(ctx, "namespace", namespace)

	log.Infow(ctx, ReconcileStart, getMetadata(ctx)...)

	return ctx
}

func EndReconcileTrace(ctx context.Context, log Logger) {
	log.Infow(ctx, ReconcileEnd, getMetadata(ctx)...)
}
