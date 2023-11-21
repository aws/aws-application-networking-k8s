package gwlog

import (
	"log"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger = *zap.SugaredLogger

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
	return z.Sugar()
}

var FallbackLogger = NewLogger(zap.DebugLevel)
