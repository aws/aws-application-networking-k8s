package test

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

type loggerKey struct{}

func NewContext(t *testing.T) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, loggerKey{}, zaptest.NewLogger(t, zaptest.WrapOptions(
		zap.AddCaller(),
		zap.Development(),
	)).Sugar())
	return ctx
}

func Logger(ctx context.Context) *zap.SugaredLogger {
	return ctx.Value(loggerKey{}).(*zap.SugaredLogger)
}
