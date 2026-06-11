package octosql

import (
	"bytes"
	"context"
	"encoding/csv"
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

// structTestSchema has a scalar column and a struct column with a nested struct.
var structTestSchema = physical.Schema{
	Fields: []physical.SchemaField{
		{Name: "name", Type: octosql.String},
		{Name: "status", Type: octosql.Type{
			TypeID: octosql.TypeIDStruct,
			Struct: struct{ Fields []octosql.StructField }{Fields: []octosql.StructField{
				{Name: "phase", Type: octosql.String},
				{Name: "conditions", Type: octosql.Type{
					TypeID: octosql.TypeIDStruct,
					Struct: struct{ Fields []octosql.StructField }{Fields: []octosql.StructField{
						{Name: "ready", Type: octosql.Boolean},
					}},
				}},
			}},
		}},
	},
}

var structTestRows = [][]octosql.Value{
	{
		octosql.NewString("pod-a"),
		octosql.NewStruct([]octosql.Value{
			octosql.NewString("Running"),
			octosql.NewStruct([]octosql.Value{octosql.NewBoolean(true)}),
		}),
	},
}

func TestRenderTableStructPretty(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: structTestRows}, Options{
		Format: "table",
		Schema: structTestSchema,
		Writer: &buf,
		Pretty: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"phase": "Running"`) {
		t.Errorf("struct cell missing named field: %s", out)
	}
	if !strings.Contains(out, `"ready": true`) {
		t.Errorf("nested struct field not resolved: %s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("output must not contain ANSI codes when ColorKeys is false: %q", out)
	}
}

func TestRenderTableStructCompact(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: structTestRows}, Options{
		Format: "table",
		Schema: structTestSchema,
		Writer: &buf,
		Pretty: false, // --disable-beauty
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `{"conditions":{"ready":true},"phase":"Running"}`) {
		t.Errorf("struct cell should be compact single-line JSON: %s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("output must not contain ANSI codes with beauty disabled: %q", out)
	}
}

func TestRenderTableStructColorKeys(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: structTestRows}, Options{
		Format:    "table",
		Schema:    structTestSchema,
		Writer:    &buf,
		Pretty:    true,
		ColorKeys: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, ansiCyan+`"phase"`+ansiReset+":") {
		t.Errorf("keys should be ANSI cyan: %q", out)
	}
	if strings.Contains(out, ansiCyan+`"Running"`) {
		t.Errorf("values must not be colored: %q", out)
	}
}

func TestRenderCSVStructCompact(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: structTestRows}, Options{
		Format: "csv",
		Schema: structTestSchema,
		Writer: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("CSV output must never contain ANSI codes: %q", buf.String())
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("CSV output does not round-trip: %v", err)
	}
	if len(records) != 2 { // header + 1 row
		t.Fatalf("expected 2 CSV records, got %d", len(records))
	}
	cell := records[1][1]
	if strings.Contains(cell, "\n") {
		t.Errorf("CSV struct cell must be single-line, got %q", cell)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(cell), &decoded); err != nil {
		t.Fatalf("CSV struct cell is not valid JSON: %v (%q)", err, cell)
	}
	if decoded["phase"] != "Running" {
		t.Errorf("unexpected struct cell content: %v", decoded)
	}
}

func TestRenderScalarsUnchanged(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: testRows}, Options{
		Format: "csv",
		Schema: testSchema,
		Writer: &buf,
		Pretty: true, // pretty must not affect scalar cells
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "name,count\npod-b,2\npod-a,1\n"
	if buf.String() != want {
		t.Errorf("scalar CSV output changed:\nwant %q\ngot  %q", want, buf.String())
	}
}

func TestRenderTableListPretty(t *testing.T) {
	elem := octosql.String
	listSchema := physical.Schema{
		Fields: []physical.SchemaField{
			{Name: "containers", Type: octosql.Type{
				TypeID: octosql.TypeIDList,
				List:   struct{ Element *octosql.Type }{Element: &elem},
			}},
		},
	}
	// List columns carry JSON-string elements (see fieldToOctoType).
	rows := [][]octosql.Value{
		{octosql.NewList([]octosql.Value{
			octosql.NewString(`{"name":"c1"}`),
			octosql.NewString(`{"name":"c2"}`),
		})},
	}
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: rows}, Options{
		Format:    "table",
		Schema:    listSchema,
		Writer:    &buf,
		Pretty:    true,
		ColorKeys: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, ansiCyan+`"name"`+ansiReset+`: "c1"`) {
		t.Errorf("list cell should be pretty JSON array with decoded, key-colored elements: %q", out)
	}
}

func TestRenderCSVMapCompact(t *testing.T) {
	mapSchema := physical.Schema{
		Fields: []physical.SchemaField{
			{Name: "labels", Type: octosql.Type{
				TypeID: octosql.TypeIDList,
				List:   struct{ Element *octosql.Type }{Element: &octosql.Any},
			}},
		},
	}
	// Map columns are flat alternating key/value lists.
	rows := [][]octosql.Value{
		{octosql.NewList([]octosql.Value{
			octosql.NewString("app"), octosql.NewString("nginx"),
		})},
	}
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: rows}, Options{
		Format: "csv",
		Schema: mapSchema,
		Writer: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("CSV output does not round-trip: %v", err)
	}
	if got := records[1][0]; got != `{"app":"nginx"}` {
		t.Errorf("map cell should decode to a compact JSON object, got %q", got)
	}
}

func TestRenderTableTuple(t *testing.T) {
	tupleSchema := physical.Schema{
		Fields: []physical.SchemaField{{Name: "pair", Type: octosql.Any}},
	}
	rows := [][]octosql.Value{
		{octosql.NewTuple([]octosql.Value{octosql.NewString("a"), octosql.NewInt(1)})},
	}
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: rows}, Options{
		Format: "table",
		Schema: tupleSchema,
		Writer: &buf,
		Pretty: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `["a",1]`) {
		t.Errorf("tuple cell should be a JSON array: %q", buf.String())
	}
}

func TestColorizeJSONKeys(t *testing.T) {
	in := "{\n  \"phase\": \"Running\",\n  \"message\": \"weird \\\" : value\"\n}"
	out := colorizeJSONKeys(in)
	if got := strings.Count(out, ansiCyan); got != 2 {
		t.Errorf("expected exactly 2 colored keys, got %d: %q", got, out)
	}
	if !strings.Contains(out, ansiCyan+`"phase"`+ansiReset+":") {
		t.Errorf("key not colored: %q", out)
	}
	if !strings.Contains(out, `"weird \" : value"`) {
		t.Errorf("value must stay uncolored and unmodified: %q", out)
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
