package readline

import (
	shellPort "github.com/ebuildy/kubectl-sql/internal/port/shell"
)

// factory is the readline-backed shell.Factory. It builds a NewReadlineShell
// per session from the port's Spec, so the composition root can inject it
// without the domain importing this adapter.
type factory struct{}

// NewFactory returns a shell.Factory that builds readline-backed shell sessions.
func NewFactory() shellPort.Factory { return factory{} }

// New builds a Runner for spec, mapping the port's session description onto the
// concrete readline shell.
func (factory) New(spec shellPort.Spec) shellPort.Runner {
	return &NewReadlineShell{
		RunQuery:   RunQueryFunc(spec.RunQuery),
		IOIn:       spec.In,
		IOOut:      spec.Out,
		IsTTY:      spec.Interactive,
		Completion: spec.Completion,
		Version:    spec.Version,
		ProjectURL: spec.ProjectURL,
	}
}
