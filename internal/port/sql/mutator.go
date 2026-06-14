package sql

import (
	"context"

	k8s "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
)

// ObjectRef identifies a single object a mutating statement will act on.
type ObjectRef struct {
	Namespace string
	Name      string
}

// DeletePlan is the resolved, not-yet-applied set of objects a DELETE will
// remove, plus the options and the human-readable equivalent kubectl commands.
// It carries only domain types so it can cross the port boundary freely.
type DeletePlan struct {
	// Targets are the objects to delete, in the order the resolving SELECT
	// returned them. Apply preserves this order in its result.
	Targets []ObjectRef
	// Resource is the resolved kind the targets belong to.
	Resource k8s.Resource
	// Options are the delete options parsed from the statement's hint comment,
	// applied uniformly to every target.
	Options k8s.DeleteOptions
	// KubectlCommands holds, per target (same order as Targets), the equivalent
	// `kubectl delete ...` command line for preview.
	KubectlCommands []string
}

// ObjectOutcome is the result of attempting to delete one object.
type ObjectOutcome struct {
	Ref ObjectRef
	// Err is nil on success, or the delete error otherwise.
	Err error
}

// DeleteResult aggregates per-object outcomes in the plan's original order.
type DeleteResult struct {
	Outcomes []ObjectOutcome
}

// Failed reports how many outcomes carry an error.
func (r DeleteResult) Failed() int {
	n := 0
	for _, o := range r.Outcomes {
		if o.Err != nil {
			n++
		}
	}
	return n
}

// Deleted reports how many outcomes succeeded.
func (r DeleteResult) Deleted() int {
	return len(r.Outcomes) - r.Failed()
}

// Mutator handles mutating SQL statements (DELETE now; UPDATE later). It is the
// port behind the mutator adapter; consumers depend only on this interface. It
// is library-free: no octosql, no k8s.io, no progress-bar types appear here.
type Mutator interface {
	// Plan parses a mutating statement, resolves the affected objects (by
	// delegating row resolution to the SELECT engine), and returns the plan
	// without mutating anything.
	Plan(ctx context.Context, sql string) (DeletePlan, error)
	// Apply executes the plan, deleting each target through the DataSource port
	// with bounded concurrency, and returns every per-object outcome in the
	// plan's order. onProgress, when non-nil, is invoked exactly once per
	// completed delete and must be safe to call concurrently. Apply does not
	// print to the user.
	Apply(ctx context.Context, plan DeletePlan, onProgress func()) (DeleteResult, error)
}
