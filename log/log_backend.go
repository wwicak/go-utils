//go:build !windows

package log

import (
	"log/syslog"
	"os"

	"github.com/wwicak/go-utils/sharedutils"
	"go.uber.org/zap/zapcore"
)

// syslogSyncer adapts log/syslog.Writer to zapcore.WriteSyncer.
type syslogSyncer struct {
	writer *syslog.Writer
}

func (s *syslogSyncer) Write(p []byte) (int, error) {
	return len(p), s.writer.Info(string(p))
}

func (s *syslogSyncer) Sync() error {
	return nil
}

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
	output := sharedutils.EnvOrDefault("LOG_OUTPUT", "syslog")
	format := sharedutils.EnvOrDefault("LOG_FORMAT", "text")

	encoder := newEncoder(format)

	// Level filtering is done by levelFilterCore, so accept all here (DebugLevel passes everything >= Debug)
	if output == "syslog" {
		w, err := syslog.New(syslog.LOG_INFO, ProcessName)
		if err == nil {
			return zapcore.NewCore(encoder, &syslogSyncer{writer: w}, zapcore.DebugLevel)
		}
		// Fall back to stdout if syslog is unavailable
	}

	return zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), zapcore.DebugLevel)
}
