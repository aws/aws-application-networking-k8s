package gwlog

import (
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger = *zap.SugaredLogger

func NewLogger(debug bool) Logger {
	var zc zap.Config
	if debug {
		zc = zap.NewDevelopmentConfig()
		zc.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	} else {
		zc = zap.NewProductionConfig()
		zc.DisableStacktrace = true
	}
	z, err := zc.Build()
	if err != nil {
		log.Fatal("cannot initialize zapr logger", err)
	}
	return z.Sugar()
}

var FallbackLogger = NewLogger(true)
