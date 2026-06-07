package logger

import "context"

type ctxKey struct{}

// IntoContext returns a copy of ctx carrying the given Logger.
func IntoContext(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the Logger stored in ctx, or a no-op Logger if none is set.
func FromContext(ctx context.Context) Logger {
	if ctx != nil {
		if l, ok := ctx.Value(ctxKey{}).(Logger); ok && l != nil {
			return l
		}
	}
	return Nop()
}
