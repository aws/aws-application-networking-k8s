package gwlog

import (
	"context"
	"fmt"
	"log"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type TracedLogger struct {
	InnerLogger *zap.SugaredLogger
}

func (t *TracedLogger) Infow(ctx context.Context, msg string, keysAndValues ...interface{}) {
	if GetTrace(ctx) != "" {
		keysAndValues = append(keysAndValues, string(traceID), GetTrace(ctx))
	}
	t.InnerLogger.Infow(msg, keysAndValues...)
}

func (t *TracedLogger) Infof(ctx context.Context, template string, args ...interface{}) {
	if GetTrace(ctx) != "" {
		t.InnerLogger.Infow(fmt.Sprintf(template, args...), string(traceID), GetTrace(ctx))
		return
	}
	t.InnerLogger.Infof(template, args...)
}

func (t *TracedLogger) Info(ctx context.Context, msg string) {
	if GetTrace(ctx) != "" {
		t.InnerLogger.Infow(msg, string(traceID), GetTrace(ctx))
		return
	}
	t.InnerLogger.Info(msg)
}

func (t *TracedLogger) Errorw(ctx context.Context, msg string, keysAndValues ...interface{}) {
	if GetTrace(ctx) != "" {
		keysAndValues = append(keysAndValues, string(traceID), GetTrace(ctx))
	}
	t.InnerLogger.Errorw(msg, keysAndValues)
}

func (t *TracedLogger) Errorf(ctx context.Context, template string, args ...interface{}) {
	if GetTrace(ctx) != "" {
		t.InnerLogger.Errorw(fmt.Sprintf(template, args...), string(traceID), GetTrace(ctx))
		return
	}
	t.InnerLogger.Errorf(template, args...)
}

func (t *TracedLogger) Error(ctx context.Context, msg string) {
	if GetTrace(ctx) != "" {
		t.InnerLogger.Errorw(msg, string(traceID), GetTrace(ctx))
		return
	}
	t.InnerLogger.Error(msg)
}

func (t *TracedLogger) Debugw(ctx context.Context, msg string, keysAndValues ...interface{}) {
	if GetTrace(ctx) != "" {
		keysAndValues = append(keysAndValues, string(traceID), GetTrace(ctx))
	}
	t.InnerLogger.Debugw(msg, keysAndValues...)
}

func (t *TracedLogger) Debugf(ctx context.Context, template string, args ...interface{}) {
	if GetTrace(ctx) != "" {
		t.InnerLogger.Debugw(fmt.Sprintf(template, args...), string(traceID), GetTrace(ctx))
		return
	}
	t.InnerLogger.Debugf(template, args...)
}

func (t *TracedLogger) Debug(ctx context.Context, msg string) {
	if GetTrace(ctx) != "" {
		t.InnerLogger.Debugw(msg, string(traceID), GetTrace(ctx))
		return
	}
	t.InnerLogger.Debug(msg)
}

func (t *TracedLogger) Warnw(ctx context.Context, msg string, keysAndValues ...interface{}) {
	if GetTrace(ctx) != "" {
		keysAndValues = append(keysAndValues, string(traceID), GetTrace(ctx))
	}
	t.InnerLogger.Warnw(msg, keysAndValues...)
}

func (t *TracedLogger) Warnf(ctx context.Context, template string, args ...interface{}) {
	if GetTrace(ctx) != "" {
		t.InnerLogger.Warnw(fmt.Sprintf(template, args...), string(traceID), GetTrace(ctx))
		return
	}
	t.InnerLogger.Warnf(template, args...)
}

func (t *TracedLogger) Warn(ctx context.Context, msg string) {
	if GetTrace(ctx) != "" {
		t.InnerLogger.Warnw(msg, string(traceID), GetTrace(ctx))
		return
	}
	t.InnerLogger.Warn(msg)
}

func (t *TracedLogger) Named(name string) *TracedLogger {
	return &TracedLogger{InnerLogger: t.InnerLogger.Named(name)}
}

type Logger = *TracedLogger

func NewLogger(level zapcore.Level) Logger {
	var zc zap.Config

	dev := os.Getenv("DEV_MODE")
	if dev != "" {
		zc = zap.NewDevelopmentConfig()
	} else {
		zc = zap.NewProductionConfig()
		zc.DisableStacktrace = true
		zc.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	zc.Level = zap.NewAtomicLevelAt(level)

	z, err := zc.Build()
	if err != nil {
		log.Fatal("cannot initialize zapr logger", err)
	}

	return &TracedLogger{InnerLogger: z.Sugar().WithOptions(zap.AddCallerSkip(1))}
}

var FallbackLogger = NewLogger(zap.DebugLevel)
