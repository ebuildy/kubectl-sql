// Package zap is the zap-backed adapter for the logging port. It is the ONLY
// package in the repository that imports go.uber.org/zap; all other code depends
// on internal/port/logger. Replacing the logging library means adding a sibling
// adapter implementing logger.Logger, not editing call sites.
package zap

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/ebuildy/kubectl-sql/internal/port/logger"
)

// zapLogger adapts *zap.Logger to the logger.Logger port.
type zapLogger struct {
	z *zap.Logger
}

// New constructs a console logger writing to stderr at the level derived from
// opts.Verbosity (0 -> error, 1 -> info, >=2 -> debug). Color is enabled unless
// opts.NoColor is set.
func New(opts logger.Options) logger.Logger {
	encCfg := zap.NewDevelopmentEncoderConfig()
	if opts.NoColor {
		encCfg.EncodeLevel = zapcore.CapitalLevelEncoder
	} else {
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encCfg),
		zapcore.Lock(os.Stderr),
		levelFor(opts.Verbosity),
	)
	return &zapLogger{z: zap.New(core)}
}

// levelFor maps verbosity to a zap level.
func levelFor(verbosity int) zapcore.Level {
	switch {
	case verbosity <= 0:
		return zapcore.ErrorLevel
	case verbosity == 1:
		return zapcore.InfoLevel
	default:
		return zapcore.DebugLevel
	}
}

func (l *zapLogger) Debug(msg string, fields ...logger.Field) {
	l.z.Debug(msg, toZapFields(fields)...)
}

func (l *zapLogger) Info(msg string, fields ...logger.Field) {
	l.z.Info(msg, toZapFields(fields)...)
}

func (l *zapLogger) Error(msg string, fields ...logger.Field) {
	l.z.Error(msg, toZapFields(fields)...)
}

func (l *zapLogger) With(fields ...logger.Field) logger.Logger {
	return &zapLogger{z: l.z.With(toZapFields(fields)...)}
}

func (l *zapLogger) Sync() error { return l.z.Sync() }

// toZapFields maps the port's library-agnostic fields to zap fields.
func toZapFields(fields []logger.Field) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	out := make([]zap.Field, len(fields))
	for i, f := range fields {
		out[i] = zap.Any(f.Key, f.Value)
	}
	return out
}
