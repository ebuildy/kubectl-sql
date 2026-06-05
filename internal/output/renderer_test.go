package output

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cube2222/octosql/execution"
	"github.com/cube2222/octosql/octosql"
	"github.com/cube2222/octosql/physical"
)

// mockNode is a test execution.Node that emits a fixed set of rows.
type mockNode struct {
	rows [][]octosql.Value
}

func (m *mockNode) Run(_ execution.ExecutionContext, produce execution.ProduceFn, _ execution.MetaSendFn) error {
	for _, row := range m.rows {
		if err := produce(execution.ProduceContext{}, execution.NewRecord(row, false, time.Time{})); err != nil {
			return err
		}
	}
	return nil
}

var testSchema = physical.Schema{
	Fields: []physical.SchemaField{
		{Name: "name", Type: octosql.String},
		{Name: "count", Type: octosql.Int},
	},
}

var testRows = [][]octosql.Value{
	{octosql.NewString("pod-b"), octosql.NewInt(2)},
	{octosql.NewString("pod-a"), octosql.NewInt(1)},
}

func execCtx() execution.ExecutionContext {
	return execution.ExecutionContext{Context: context.Background()}
}

func TestRenderTable(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: testRows}, Options{
		Format: "table",
		Schema: testSchema,
		Writer: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "pod-a") || !strings.Contains(out, "pod-b") {
		t.Errorf("table output missing rows: %s", out)
	}
	if !strings.Contains(out, "name") || !strings.Contains(out, "count") {
		t.Errorf("table output missing headers: %s", out)
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: testRows}, Options{
		Format: "json",
		Schema: testSchema,
		Writer: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(result) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result))
	}
	if result[0]["name"] != "pod-b" {
		t.Errorf("unexpected first row: %v", result[0])
	}
}

func TestRenderCSV(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: testRows}, Options{
		Format: "csv",
		Schema: testSchema,
		Writer: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 { // header + 2 rows
		t.Errorf("expected 3 lines, got %d:\n%s", len(lines), buf.String())
	}
	if lines[0] != "name,count" {
		t.Errorf("unexpected header: %s", lines[0])
	}
}

func TestRenderLimit(t *testing.T) {
	var buf bytes.Buffer
	limit := int64(1)
	err := Render(execCtx(), &mockNode{rows: testRows}, Options{
		Format: "csv",
		Schema: testSchema,
		Writer: &buf,
		Limit:  &limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 { // header + 1 row
		t.Errorf("expected 2 lines after LIMIT 1, got %d:\n%s", len(lines), buf.String())
	}
}
