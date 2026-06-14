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
	"github.com/ebuildy/kubectl-sql/internal/utils"
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
	if !strings.Contains(out, "phase: Running") {
		t.Errorf("struct cell missing named field: %s", out)
	}
	if !strings.Contains(out, "ready: true") {
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
		Format: "table",
		Schema: listSchema,
		Writer: &buf,
		Pretty: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// List columns render as a YAML sequence of decoded objects.
	if !strings.Contains(out, "- name: c1") || !strings.Contains(out, "- name: c2") {
		t.Errorf("list cell should be a pretty YAML sequence of decoded elements: %q", out)
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

// dataMapSchema mirrors a ConfigMap-like `data` map column: a flat
// alternating key/value list whose element type is Any.
var dataMapSchema = physical.Schema{
	Fields: []physical.SchemaField{
		{Name: "data", Type: octosql.Type{
			TypeID: octosql.TypeIDList,
			List:   struct{ Element *octosql.Type }{Element: &octosql.Any},
		}},
	},
}

const teardownScript = "#!/bin/sh\nset -eu\nrm -rf \"$VOL_DIR\""

var dataMapRows = [][]octosql.Value{
	{octosql.NewList([]octosql.Value{
		octosql.NewString("teardown"), octosql.NewString(teardownScript),
	})},
}

func TestRenderJSONMapMultilineStringStaysEscaped(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: dataMapRows}, Options{
		Format: "json",
		Schema: dataMapSchema,
		Writer: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	data := result[0]["data"].(map[string]interface{})
	if data["teardown"] != teardownScript {
		t.Errorf("unexpected teardown value: %v", data["teardown"])
	}
}

func TestRenderCSVMapMultilineStringStaysEscaped(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: dataMapRows}, Options{
		Format: "csv",
		Schema: dataMapSchema,
		Writer: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("CSV output does not round-trip: %v", err)
	}
	cell := records[1][0]
	if strings.Contains(cell, "\n") {
		t.Errorf("CSV cell must stay single-line, got %q", cell)
	}
	if !strings.Contains(cell, `\n`) {
		t.Errorf("CSV cell should keep the JSON \\n escape sequence, got %q", cell)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(cell), &decoded); err != nil {
		t.Fatalf("CSV cell is not valid JSON: %v (%q)", err, cell)
	}
	if decoded["teardown"] != teardownScript {
		t.Errorf("unexpected teardown value: %v", decoded["teardown"])
	}
}

func TestRenderTableDisableBeautyMapMultilineStringStaysEscaped(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: dataMapRows}, Options{
		Format: "table",
		Schema: dataMapSchema,
		Writer: &buf,
		Pretty: false, // --disable-beauty
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `\n`) {
		t.Errorf("--disable-beauty cell should keep the JSON \\n escape sequence: %q", out)
	}
	if !strings.Contains(out, `{"teardown":"#!/bin/sh\nset -eu\nrm -rf \"$VOL_DIR\""}`) {
		t.Errorf("--disable-beauty cell should be compact single-line JSON: %q", out)
	}
}

func TestRenderTableYAMLBeautifyFormat(t *testing.T) {
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
	if !strings.Contains(out, "phase: Running") {
		t.Errorf("struct cell should render as YAML: %q", out)
	}
	if strings.Contains(out, `"phase"`) {
		t.Errorf("YAML cell should not contain JSON-quoted keys: %q", out)
	}
}

func TestRenderTableYAMLBeautifyFormatMultilineString(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: dataMapRows}, Options{
		Format: "table",
		Schema: dataMapSchema,
		Writer: &buf,
		Pretty: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "teardown: |") {
		t.Errorf("multi-line string should render as a YAML literal block scalar: %q", out)
	}
	for _, line := range []string{"#!/bin/sh", "set -eu", `rm -rf "$VOL_DIR"`} {
		if !strings.Contains(out, line) {
			t.Errorf("expected script line %q to appear verbatim: %q", line, out)
		}
	}
}

func TestRenderTableYAMLColorKeys(t *testing.T) {
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
	if !strings.Contains(out, utils.AnsiCyan+"phase"+utils.AnsiReset+":") {
		t.Errorf("top-level YAML key should be ANSI cyan: %q", out)
	}
	if strings.Contains(out, utils.AnsiCyan+"ready") {
		t.Errorf("nested YAML key must not be colored: %q", out)
	}
	if strings.Contains(out, utils.AnsiCyan+"Running") {
		t.Errorf("values must not be colored: %q", out)
	}
}

func TestRenderTableYAMLColorKeysWithMultilineString(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: dataMapRows}, Options{
		Format:    "table",
		Schema:    dataMapSchema,
		Writer:    &buf,
		Pretty:    true,
		ColorKeys: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, utils.AnsiCyan+"teardown"+utils.AnsiReset+":") {
		t.Errorf("top-level YAML key should be ANSI cyan: %q", out)
	}
	if got := strings.Count(out, utils.AnsiCyan); got != 1 {
		t.Errorf("expected exactly 1 colored key, got %d: %q", got, out)
	}
	for _, line := range []string{"#!/bin/sh", "set -eu", `rm -rf "$VOL_DIR"`} {
		if !strings.Contains(out, line) {
			t.Errorf("expected script line %q to appear verbatim: %q", line, out)
		}
	}
}

// nullFieldSchema has a struct column whose `reason` field is null and whose
// `message` field is an empty (but non-null) string.
var nullFieldSchema = physical.Schema{
	Fields: []physical.SchemaField{
		{Name: "status", Type: octosql.Type{
			TypeID: octosql.TypeIDStruct,
			Struct: struct{ Fields []octosql.StructField }{Fields: []octosql.StructField{
				{Name: "phase", Type: octosql.String},
				{Name: "reason", Type: octosql.Type{TypeID: octosql.TypeIDNull}},
				{Name: "message", Type: octosql.String},
			}},
		}},
	},
}

var nullFieldRows = [][]octosql.Value{
	{octosql.NewStruct([]octosql.Value{
		octosql.NewString("Running"),
		octosql.NewNull(),
		octosql.NewString(""),
	})},
}

func TestRenderTableYAMLOmitsNullFields(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: nullFieldRows}, Options{
		Format: "table",
		Schema: nullFieldSchema,
		Writer: &buf,
		Pretty: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "reason") {
		t.Errorf("null-valued field should be omitted from YAML beautify cell: %q", out)
	}
	if !strings.Contains(out, "phase: Running") {
		t.Errorf("non-null field should remain: %q", out)
	}
	if !strings.Contains(out, `message: ""`) {
		t.Errorf("empty-but-non-null field should be preserved: %q", out)
	}
}

func TestRenderJSONKeepsNullFields(t *testing.T) {
	var buf bytes.Buffer
	err := Render(execCtx(), &mockNode{rows: nullFieldRows}, Options{
		Format: "json",
		Schema: nullFieldSchema,
		Writer: &buf,
	})
	if err != nil {
		t.Fatal(err)
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	status, ok := result[0]["status"].(map[string]interface{})
	if !ok {
		t.Fatalf("status is not an object: %v", result[0]["status"])
	}
	if v, present := status["reason"]; !present || v != nil {
		t.Errorf("JSON output should keep null field as null, got present=%v value=%v", present, v)
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
