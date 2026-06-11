package logger

import (
	"context"
	"testing"
)

func TestNopIsSafe(t *testing.T) {
	l := Nop()
	if l == nil {
		t.Fatal("Nop() returned nil")
	}
	// Must not panic.
	l.Trace("t", String("k", "v"))
	if l.TraceEnabled() {
		t.Error("Nop TraceEnabled() = true, want false")
	}
	l.Debug("d", String("k", "v"))
	l.Info("i")
	l.Error("e", Err(context.Canceled))
	if l.With(Int("n", 1)) == nil {
		t.Error("With returned nil")
	}
	if err := l.Sync(); err != nil {
		t.Errorf("Nop Sync should be nil, got %v", err)
	}
}

func TestFieldConstructors(t *testing.T) {
	if f := String("host", "x"); f.Key != "host" || f.Value != "x" {
		t.Errorf("String: %+v", f)
	}
	if f := Int("n", 3); f.Key != "n" || f.Value != 3 {
		t.Errorf("Int: %+v", f)
	}
	if f := Err(context.Canceled); f.Key != "error" || f.Value != context.Canceled {
		t.Errorf("Err: %+v", f)
	}
	if f := Any("k", 1.5); f.Key != "k" || f.Value != 1.5 {
		t.Errorf("Any: %+v", f)
	}
}

func TestFromContext_RoundTrip(t *testing.T) {
	l := Nop()
	ctx := IntoContext(context.Background(), l)
	got := FromContext(ctx)
	if got == nil {
		t.Fatal("FromContext returned nil")
	}
}

func TestFromContext_EmptyReturnsNop(t *testing.T) {
	if FromContext(context.Background()) == nil {
		t.Error("FromContext on empty context returned nil, want non-nil Nop")
	}
	if FromContext(nil) == nil { //nolint:staticcheck // intentionally testing nil ctx
		t.Error("FromContext(nil) returned nil, want non-nil Nop")
	}
}
