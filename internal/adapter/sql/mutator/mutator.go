// Package mutator is the SQL adapter for mutating statements (DELETE now;
// UPDATE later), a sibling of the octosql adapter under internal/adapter/sql.
// octosql is SELECT-only, so DELETE/UPDATE logic lives here and nowhere else.
//
// The mutator resolves the set of objects a statement affects by delegating a
// `SELECT namespace, name FROM <resource> [WHERE ...]` to the injected SQL
// engine port, and performs the mutation through the injected k8s DataSource
// port. It therefore imports neither octosql internals nor client-go — both
// stay behind their ports.
package mutator

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"sync"

	k8sport "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	portsql "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// maxConcurrentDeletes bounds how many deletions Apply runs in flight at once.
const maxConcurrentDeletes = 10

// nullCell is the string octosql renders for a NULL value; it appears in the
// namespace column for cluster-scoped resources, which have no namespace.
const nullCell = "<null>"

// mutator implements the portsql.Mutator port. eng MUST be configured to render
// a fixed structured (CSV) format so the resolved rows parse robustly,
// independent of the user's chosen --output.
type mutator struct {
	eng portsql.Engine
	ds  k8sport.DataSource
}

// New builds a mutator from the SQL engine port (used to resolve the deletion
// set via SELECT) and the k8s DataSource port (used to delete). The engine is
// expected to render CSV. The returned value is port-typed.
func New(eng portsql.Engine, ds k8sport.DataSource) portsql.Mutator {
	return &mutator{eng: eng, ds: ds}
}

// Plan parses the DELETE, resolves the resource, delegates the row resolution
// to the SELECT engine, and builds the deletion plan. It mutates nothing.
func (m *mutator) Plan(ctx context.Context, sql string) (portsql.DeletePlan, error) {
	pd, err := parseDelete(sql)
	if err != nil {
		return portsql.DeletePlan{}, err
	}

	resource, err := m.ds.Resolve(ctx, pd.resource)
	if err != nil {
		return portsql.DeletePlan{}, fmt.Errorf("mutator: resolve resource %q: %w", pd.resource, err)
	}

	// Alias the projected columns so the CSV headers are deterministic and
	// unqualified: octosql otherwise prefixes them with the table name
	// (e.g. "pods.name"), which would not match the column lookup below.
	selectSQL := "SELECT namespace AS namespace, name AS name FROM " + pd.resource
	if pd.tail != "" {
		selectSQL += " " + pd.tail
	}

	var buf bytes.Buffer
	if err := m.eng.Execute(ctx, portsql.Query{SQL: selectSQL}, &buf); err != nil {
		return portsql.DeletePlan{}, fmt.Errorf("mutator: resolve deletion set: %w", err)
	}

	targets, err := parseCSVTargets(buf.Bytes())
	if err != nil {
		return portsql.DeletePlan{}, fmt.Errorf("mutator: parse deletion set: %w", err)
	}

	flags := deleteOptionsToFlags(pd.options)
	cmds := make([]string, len(targets))
	for i, t := range targets {
		cmds[i] = kubectlDeleteLine(resource, t, flags)
	}

	return portsql.DeletePlan{
		Targets:         targets,
		Resource:        resource,
		Options:         pd.options,
		KubectlCommands: cmds,
	}, nil
}

// Apply deletes the plan's targets concurrently (bounded to maxConcurrentDeletes
// in flight), recording each outcome race-free into a position-indexed slice so
// results stay in the plan's order. onProgress, when non-nil, fires once per
// completed delete. Apply prints nothing.
func (m *mutator) Apply(ctx context.Context, plan portsql.DeletePlan, onProgress func()) (portsql.DeleteResult, error) {
	outcomes := make([]portsql.ObjectOutcome, len(plan.Targets))
	sem := make(chan struct{}, maxConcurrentDeletes)
	var wg sync.WaitGroup

	for i, t := range plan.Targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, t portsql.ObjectRef) {
			defer wg.Done()
			defer func() { <-sem }()

			err := m.ds.Delete(ctx, plan.Resource, t.Namespace, t.Name, plan.Options)
			outcomes[i] = portsql.ObjectOutcome{Ref: t, Err: err}
			if onProgress != nil {
				onProgress()
			}
		}(i, t)
	}
	wg.Wait()

	return portsql.DeleteResult{Outcomes: outcomes}, nil
}

// parseCSVTargets reads the CSV the SELECT engine produced and extracts the
// (namespace, name) of each row by column name, skipping rows without a name.
func parseCSVTargets(data []byte) ([]portsql.ObjectRef, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	nsIdx, nameIdx := -1, -1
	for i, h := range records[0] {
		// Match the trailing path segment so a table-qualified header
		// (e.g. "pods.name") still resolves to "name".
		col := strings.ToLower(strings.TrimSpace(h))
		if dot := strings.LastIndex(col, "."); dot >= 0 {
			col = col[dot+1:]
		}
		switch col {
		case "namespace":
			nsIdx = i
		case "name":
			nameIdx = i
		}
	}
	if nameIdx < 0 {
		return nil, fmt.Errorf("resolved rows missing a 'name' column")
	}

	var out []portsql.ObjectRef
	for _, rec := range records[1:] {
		var ref portsql.ObjectRef
		if nameIdx < len(rec) {
			ref.Name = rec[nameIdx]
		}
		if nsIdx >= 0 && nsIdx < len(rec) && rec[nsIdx] != nullCell {
			ref.Namespace = rec[nsIdx]
		}
		if ref.Name == "" || ref.Name == nullCell {
			continue
		}
		out = append(out, ref)
	}
	return out, nil
}

// kubectlDeleteLine renders the equivalent `kubectl delete` command for one
// target. The namespace flag is omitted for cluster-scoped resources and when
// the object has no namespace.
func kubectlDeleteLine(r k8sport.Resource, t portsql.ObjectRef, flags []string) string {
	parts := []string{"kubectl", "delete", r.Name, t.Name}
	if r.Namespaced && t.Namespace != "" {
		parts = append(parts, "-n", t.Namespace)
	}
	parts = append(parts, flags...)
	return strings.Join(parts, " ")
}
