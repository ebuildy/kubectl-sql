package zap

import (
	"testing"

	"go.uber.org/zap/zapcore"

	"github.com/ebuildy/kubectl-sql/internal/port/logger"
)

func TestLevelFor(t *testing.T) {
	cases := []struct {
		verbosity int
		want      zapcore.Level
	}{
		{0, zapcore.ErrorLevel},
		{-1, zapcore.ErrorLevel},
		{1, zapcore.InfoLevel},
		{2, zapcore.DebugLevel},
		{3, zapcore.DebugLevel},
	}
	for _, tc := range cases {
		if got := levelFor(tc.verbosity); got != tc.want {
			t.Errorf("levelFor(%d) = %v, want %v", tc.verbosity, got, tc.want)
		}
	}
}

func TestNewImplementsPortAndIsSafe(t *testing.T) {
	var l = New(logger.Options{Verbosity: 2})
	// Exercise every method; must not panic.
	l.Debug("debug", logger.String("k", "v"))
	l.Info("info", logger.Int("n", 1))
	l.Error("error", logger.Err(nil))
	child := l.With(logger.Any("ctx", "x"))
	if child == nil {
		t.Fatal("With returned nil")
	}
	child.Info("from child")
	_ = l.Sync() // stderr sync may error benignly; just ensure no panic
}
