//go:build windows

package log

import (
	"os"

	"github.com/wwicak/go-utils/sharedutils"
	"go.uber.org/zap/zapcore"
)

func newEncoder(format string) zapcore.Encoder {
	cfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		MessageKey:     "msg",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}
	if format == "json" {
		return zapcore.NewJSONEncoder(cfg)
	}
	cfg.EncodeLevel = zapcore.CapitalLevelEncoder
	return zapcore.NewConsoleEncoder(cfg)
}

func getLogBackend() zapcore.Core {
	format := sharedutils.EnvOrDefault("LOG_FORMAT", "text")
	encoder := newEncoder(format)
	return zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), zapcore.DebugLevel)
}
