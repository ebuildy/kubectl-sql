package query

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ebuildy/kubectl-sql/internal/port/api"
	k8sPort "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	sqlPort "github.com/ebuildy/kubectl-sql/internal/port/sql"
)

// fakeMutator is an injectable Mutator for QueryCommand delete tests.
type fakeMutator struct {
	plan       sqlPort.DeletePlan
	planErr    error
	result     sqlPort.DeleteResult
	applyErr   error
	applied    bool
	progressN  int
	appliedSQL string
}

func (m *fakeMutator) Plan(_ context.Context, sql string) (sqlPort.DeletePlan, error) {
	m.appliedSQL = sql
	return m.plan, m.planErr
}

func (m *fakeMutator) Apply(_ context.Context, plan sqlPort.DeletePlan, onProgress func()) (sqlPort.DeleteResult, error) {
	m.applied = true
	if onProgress != nil {
		for range plan.Targets {
			onProgress()
			m.progressN++
		}
	}
	return m.result, m.applyErr
}

// twoPodPlan builds a 2-target plan with matching kubectl command lines.
func twoPodPlan() sqlPort.DeletePlan {
	return sqlPort.DeletePlan{
		Targets: []sqlPort.ObjectRef{
			{Namespace: "default", Name: "nginx"},
			{Namespace: "kube-system", Name: "coredns"},
		},
		Resource: k8sPort.Resource{Name: "pods", Namespaced: true},
		KubectlCommands: []string{
			"kubectl delete pods nginx -n default",
			"kubectl delete pods coredns -n kube-system",
		},
	}
}

func allDeleted(plan sqlPort.DeletePlan) sqlPort.DeleteResult {
	out := make([]sqlPort.ObjectOutcome, len(plan.Targets))
	for i, t := range plan.Targets {
		out[i] = sqlPort.ObjectOutcome{Ref: t}
	}
	return sqlPort.DeleteResult{Outcomes: out}
}

func newDeleteCmd(t *testing.T, mut sqlPort.Mutator, cfg api.Config, stdinTTY bool, in string) (*QueryCommand, *strings.Builder) {
	t.Helper()
	var buf strings.Builder
	cfg.Out = &buf
	cmd, err := NewQueryCommandWithDataSource(cfg, fakeDataSource{})
	if err != nil {
		t.Fatalf("NewQueryCommandWithDataSource: %v", err)
	}
	cmd.mut = mut
	cmd.stdinIsTTY = stdinTTY
	cmd.in = strings.NewReader(in)
	return cmd, &buf
}

func TestRunDelete_PreviewAndDecline(t *testing.T) {
	plan := twoPodPlan()
	mut := &fakeMutator{plan: plan, result: allDeleted(plan)}
	cmd, buf := newDeleteCmd(t, mut, api.Config{}, true, "n\n")

	if err := cmd.RunWithWriter(context.Background(), "DELETE pods WHERE x = 1", buf); err != nil {
		t.Fatalf("RunWithWriter: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"nginx", "default", "kubectl delete pods nginx -n default", "2 object(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview missing %q in:\n%s", want, out)
		}
	}
	if mut.applied {
		t.Error("declined delete must not call Apply")
	}
	if !strings.Contains(out, "cancelled") {
		t.Errorf("expected cancellation message, got:\n%s", out)
	}
}

func TestRunDelete_YesDeletesAll(t *testing.T) {
	plan := twoPodPlan()
	mut := &fakeMutator{plan: plan, result: allDeleted(plan)}
	cmd, buf := newDeleteCmd(t, mut, api.Config{Yes: true}, false, "")

	if err := cmd.RunWithWriter(context.Background(), "DELETE pods", buf); err != nil {
		t.Fatalf("RunWithWriter: %v", err)
	}
	if !mut.applied {
		t.Error("--yes should proceed to Apply without prompting")
	}
	if !strings.Contains(buf.String(), "deleted: 2, failed: 0") {
		t.Errorf("expected success summary, got:\n%s", buf.String())
	}
}

func TestRunDelete_EmptyMatchIsNoop(t *testing.T) {
	mut := &fakeMutator{plan: sqlPort.DeletePlan{}}
	cmd, buf := newDeleteCmd(t, mut, api.Config{}, true, "y\n")

	if err := cmd.RunWithWriter(context.Background(), "DELETE pods WHERE x = 1", buf); err != nil {
		t.Fatalf("RunWithWriter: %v", err)
	}
	if mut.applied {
		t.Error("empty match must not call Apply")
	}
	if !strings.Contains(buf.String(), "nothing matched") {
		t.Errorf("expected nothing-matched message, got:\n%s", buf.String())
	}
}

func TestRunDelete_NonInteractiveWithoutYesRefuses(t *testing.T) {
	plan := twoPodPlan()
	mut := &fakeMutator{plan: plan, result: allDeleted(plan)}
	cmd, buf := newDeleteCmd(t, mut, api.Config{}, false, "")

	err := cmd.RunWithWriter(context.Background(), "DELETE pods", buf)
	if err == nil {
		t.Fatal("expected refusal error")
	}
	var ec api.ExitError
	if !errors.As(err, &ec) || ec.Code != 1 {
		t.Errorf("expected ExitError code 1, got %v", err)
	}
	if mut.applied {
		t.Error("non-interactive refusal must not delete")
	}
}

func TestRunDelete_FailureMapsToExit2(t *testing.T) {
	plan := twoPodPlan()
	result := sqlPort.DeleteResult{Outcomes: []sqlPort.ObjectOutcome{
		{Ref: plan.Targets[0]},
		{Ref: plan.Targets[1], Err: fmt.Errorf("forbidden")},
	}}
	mut := &fakeMutator{plan: plan, result: result}
	cmd, buf := newDeleteCmd(t, mut, api.Config{Yes: true}, false, "")

	err := cmd.RunWithWriter(context.Background(), "DELETE pods", buf)
	if err == nil {
		t.Fatal("expected exit-2 error on delete failure")
	}
	var ec api.ExitError
	if !errors.As(err, &ec) || ec.Code != 2 {
		t.Errorf("expected ExitError code 2, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "failed: forbidden") || !strings.Contains(out, "deleted: 1, failed: 1") {
		t.Errorf("expected failure summary, got:\n%s", out)
	}
}

func TestRunDelete_DryRunPreviewsOnly(t *testing.T) {
	plan := twoPodPlan()
	mut := &fakeMutator{plan: plan, result: allDeleted(plan)}
	cmd, buf := newDeleteCmd(t, mut, api.Config{DryRun: true}, true, "y\n")

	if err := cmd.RunWithWriter(context.Background(), "DELETE /* force */ pods", buf); err != nil {
		t.Fatalf("RunWithWriter: %v", err)
	}
	if mut.applied {
		t.Error("--dry-run must not delete")
	}
	out := buf.String()
	if !strings.Contains(out, "kubectl delete pods nginx") || !strings.Contains(out, "dry-run") {
		t.Errorf("expected preview + dry-run note, got:\n%s", out)
	}
}

func TestRunDelete_WatchIsRejected(t *testing.T) {
	plan := twoPodPlan()
	mut := &fakeMutator{plan: plan, result: allDeleted(plan)}
	var buf strings.Builder
	cmd, err := NewQueryCommandWithDataSource(api.Config{Watch: true, Out: &buf}, fakeDataSource{})
	if err != nil {
		t.Fatalf("NewQueryCommandWithDataSource: %v", err)
	}
	cmd.mut = mut

	if err := cmd.Run(context.Background(), "DELETE pods WHERE x = 1"); err == nil {
		t.Fatal("expected DELETE + --watch to be rejected")
	}
	if mut.applied {
		t.Error("rejected DELETE + watch must not delete")
	}
}
