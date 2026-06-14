package mutator

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	octosqlAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/sql/octosql"
	k8sport "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	portsql "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// fakeEngine is a SELECT engine that records the SQL it was asked to run and
// renders canned CSV rows, standing in for the octosql adapter.
type fakeEngine struct {
	gotSQL string
	csv    string
	err    error
}

func (e *fakeEngine) Execute(_ context.Context, q portsql.Query, w io.Writer) error {
	e.gotSQL = q.SQL
	if e.err != nil {
		return e.err
	}
	_, err := io.WriteString(w, e.csv)
	return err
}

// recordingDS records each Delete call and resolves to a fixed resource.
type recordingDS struct {
	resource k8sport.Resource

	mu      sync.Mutex
	deletes []deleteCall
}

type deleteCall struct {
	namespace string
	name      string
	opts      k8sport.DeleteOptions
}

func (d *recordingDS) Resolve(context.Context, string) (k8sport.Resource, error) {
	return d.resource, nil
}
func (d *recordingDS) Resources(context.Context) ([]k8sport.Resource, error) { return nil, nil }
func (d *recordingDS) InferSchema(context.Context, k8sport.Resource) ([]schema.Field, error) {
	return nil, nil
}
func (d *recordingDS) List(context.Context, k8sport.Resource, k8sport.ListOptions, func([]map[string]any) error) error {
	return nil
}
func (d *recordingDS) Delete(_ context.Context, _ k8sport.Resource, ns, name string, opts k8sport.DeleteOptions) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deletes = append(d.deletes, deleteCall{namespace: ns, name: name, opts: opts})
	return nil
}

func TestPlanDelegatesSelectAndBuildsCommands(t *testing.T) {
	eng := &fakeEngine{csv: "namespace,name\ndefault,nginx\nkube-system,coredns\n"}
	ds := &recordingDS{resource: k8sport.Resource{Name: "pods", Namespaced: true}}
	m := New(eng, ds)

	plan, err := m.Plan(context.Background(), "DELETE /* force, grace-period=0 */ pod WHERE status->phase = 'Pending'")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	wantSQL := "SELECT namespace AS namespace, name AS name FROM pod WHERE status->phase = 'Pending'"
	if eng.gotSQL != wantSQL {
		t.Errorf("delegated SQL = %q, want %q", eng.gotSQL, wantSQL)
	}

	if len(plan.Targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(plan.Targets))
	}
	if plan.Targets[0] != (portsql.ObjectRef{Namespace: "default", Name: "nginx"}) {
		t.Errorf("target[0] = %+v", plan.Targets[0])
	}

	wantCmds := []string{
		"kubectl delete pods nginx -n default --force --grace-period=0",
		"kubectl delete pods coredns -n kube-system --force --grace-period=0",
	}
	for i, want := range wantCmds {
		if plan.KubectlCommands[i] != want {
			t.Errorf("command[%d] = %q, want %q", i, plan.KubectlCommands[i], want)
		}
	}

	if plan.Options.GracePeriodSeconds == nil || *plan.Options.GracePeriodSeconds != 0 {
		t.Errorf("options grace period = %v, want 0", plan.Options.GracePeriodSeconds)
	}

	// Apply deletes every target with the parsed options.
	result, err := m.Apply(context.Background(), plan, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(result.Outcomes) != 2 || result.Failed() != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(ds.deletes) != 2 {
		t.Fatalf("recorded deletes = %d, want 2", len(ds.deletes))
	}
	for _, c := range ds.deletes {
		if c.opts.GracePeriodSeconds == nil || *c.opts.GracePeriodSeconds != 0 {
			t.Errorf("delete %s used opts %+v, want grace 0", c.name, c.opts)
		}
	}
}

func TestPlanEmptyMatchHasNoTargets(t *testing.T) {
	eng := &fakeEngine{csv: "namespace,name\n"}
	ds := &recordingDS{resource: k8sport.Resource{Name: "pods", Namespaced: true}}
	m := New(eng, ds)

	plan, err := m.Plan(context.Background(), "DELETE pods WHERE name = 'nope'")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Targets) != 0 {
		t.Errorf("targets = %d, want 0", len(plan.Targets))
	}
}

// podsFakeDS mirrors a real pods data source: a schema with top-level
// name/namespace virtual columns and rows carrying metadata. It is used to
// drive the actual octosql engine so the CSV header contract (octosql qualifies
// columns as "pods.name") is exercised end-to-end.
type podsFakeDS struct{}

func (podsFakeDS) Resolve(context.Context, string) (k8sport.Resource, error) {
	return k8sport.Resource{Name: "pods", Version: "v1", Namespaced: true}, nil
}
func (podsFakeDS) Resources(context.Context) ([]k8sport.Resource, error) { return nil, nil }
func (podsFakeDS) InferSchema(context.Context, k8sport.Resource) ([]schema.Field, error) {
	return []schema.Field{
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "namespace", Type: schema.FieldTypeString},
		{Name: "metadata", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "name", Type: schema.FieldTypeString},
			{Name: "namespace", Type: schema.FieldTypeString},
		}},
	}, nil
}
func (podsFakeDS) Delete(context.Context, k8sport.Resource, string, string, k8sport.DeleteOptions) error {
	return nil
}
func (podsFakeDS) List(_ context.Context, _ k8sport.Resource, _ k8sport.ListOptions, fn func([]map[string]any) error) error {
	return fn([]map[string]any{
		{"metadata": map[string]any{"name": "nginx", "namespace": "default"}},
		{"metadata": map[string]any{"name": "coredns", "namespace": "kube-system"}},
	})
}

// TestPlanWithRealOctosqlEngine drives Plan through the actual octosql adapter
// (not a fake), so the qualified-CSV-header contract is covered. Regression for
// "resolved rows missing a 'name' column".
func TestPlanWithRealOctosqlEngine(t *testing.T) {
	ds := podsFakeDS{}
	eng := octosqlAdapter.New(portsql.Config{Output: "csv"}, ds)
	m := New(eng, ds)

	plan, err := m.Plan(context.Background(), "DELETE FROM pods LIMIT 1")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Targets) != 1 {
		t.Fatalf("targets = %d, want 1 (LIMIT 1)", len(plan.Targets))
	}
	if plan.Targets[0] != (portsql.ObjectRef{Namespace: "default", Name: "nginx"}) {
		t.Errorf("target[0] = %+v, want {default nginx}", plan.Targets[0])
	}
	if plan.KubectlCommands[0] != "kubectl delete pods nginx -n default" {
		t.Errorf("command[0] = %q", plan.KubectlCommands[0])
	}
}

// concurrencyDS blocks briefly in Delete and tracks the maximum number of
// concurrent in-flight calls.
type concurrencyDS struct {
	cur   int32
	max   int32
	calls int32
}

func (d *concurrencyDS) Resolve(context.Context, string) (k8sport.Resource, error) {
	return k8sport.Resource{Name: "pods", Namespaced: true}, nil
}
func (d *concurrencyDS) Resources(context.Context) ([]k8sport.Resource, error) { return nil, nil }
func (d *concurrencyDS) InferSchema(context.Context, k8sport.Resource) ([]schema.Field, error) {
	return nil, nil
}
func (d *concurrencyDS) List(context.Context, k8sport.Resource, k8sport.ListOptions, func([]map[string]any) error) error {
	return nil
}
func (d *concurrencyDS) Delete(context.Context, k8sport.Resource, string, string, k8sport.DeleteOptions) error {
	cur := atomic.AddInt32(&d.cur, 1)
	for {
		m := atomic.LoadInt32(&d.max)
		if cur <= m || atomic.CompareAndSwapInt32(&d.max, m, cur) {
			break
		}
	}
	atomic.AddInt32(&d.calls, 1)
	time.Sleep(2 * time.Millisecond)
	atomic.AddInt32(&d.cur, -1)
	return nil
}

func TestApplyBoundedConcurrencyAndProgress(t *testing.T) {
	const n = 30
	targets := make([]portsql.ObjectRef, n)
	for i := range targets {
		targets[i] = portsql.ObjectRef{Namespace: "default", Name: fmt.Sprintf("pod-%02d", i)}
	}
	plan := portsql.DeletePlan{
		Targets:  targets,
		Resource: k8sport.Resource{Name: "pods", Namespaced: true},
	}

	ds := &concurrencyDS{}
	m := New(&fakeEngine{}, ds)

	var progress int64
	result, err := m.Apply(context.Background(), plan, func() { atomic.AddInt64(&progress, 1) })
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if got := atomic.LoadInt32(&ds.max); got > maxConcurrentDeletes {
		t.Errorf("max concurrent deletes = %d, want <= %d", got, maxConcurrentDeletes)
	}
	if got := atomic.LoadInt32(&ds.calls); got != n {
		t.Errorf("delete calls = %d, want %d", got, n)
	}
	if progress != n {
		t.Errorf("onProgress fired %d times, want %d", progress, n)
	}
	if len(result.Outcomes) != n {
		t.Fatalf("outcomes = %d, want %d", len(result.Outcomes), n)
	}
	for i, o := range result.Outcomes {
		if o.Ref != targets[i] {
			t.Errorf("outcome[%d].Ref = %+v, want %+v (order not preserved)", i, o.Ref, targets[i])
		}
		if o.Err != nil {
			t.Errorf("outcome[%d] unexpected error: %v", i, o.Err)
		}
	}
}
