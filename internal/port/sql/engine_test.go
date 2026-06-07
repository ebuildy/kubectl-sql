package sql

import (
	"context"
	"io"
	"testing"
)

type fakeEngine struct{}

func (fakeEngine) Execute(context.Context, Query, io.Writer) error { return nil }

func TestPortIsSatisfiable(t *testing.T) {
	var _ Engine = fakeEngine{}
}
