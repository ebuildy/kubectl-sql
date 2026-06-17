package query

import (
	"context"
	"strings"
	"testing"

	"github.com/ebuildy/kubectl-sql/internal/port/api"
	k8sPort "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	"github.com/ebuildy/kubectl-sql/internal/utils"
	"github.com/olekukonko/tablewriter"
)

// fakeDataSource serves a fixed schema for DESCRIBE TABLE tests, regardless
// of which resource is requested.
type fakeDataSource struct {
	fields []schema.Field
}

func (f fakeDataSource) Resolve(context.Context, string) (k8sPort.Resource, error) {
	return k8sPort.Resource{Name: "pods"}, nil
}
func (f fakeDataSource) Resources(context.Context) ([]k8sPort.Resource, error) { return nil, nil }
func (f fakeDataSource) InferSchema(context.Context, k8sPort.Resource) ([]schema.Field, error) {
	return f.fields, nil
}
func (f fakeDataSource) List(context.Context, k8sPort.Resource, k8sPort.ListOptions, func([]map[string]any) error) error {
	return nil
}
func (f fakeDataSource) Delete(context.Context, k8sPort.Resource, string, string, k8sPort.DeleteOptions) error {
	return nil
}

func TestRunDescribeTable_SchemaColumn(t *testing.T) {
	fields := []schema.Field{
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "containers", Type: schema.FieldTypeList},
		}},
		{Name: "metadata", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "name", Type: schema.FieldTypeString},
			{Name: "labels", Type: schema.FieldTypeMap, SubFields: []schema.Field{
				{Name: "app", Type: schema.FieldTypeString},
			}},
		}},
		{Name: "status", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "conditions", Type: schema.FieldTypeList},
		}},
	}

	var buf strings.Builder
	cmd := NewQueryCommand(api.Config{Out: &buf}, fakeDataSource{fields: fields}, nil, nil, false)

	if err := cmd.RunWithWriter(context.Background(), "DESCRIBE TABLE pods", &buf); err != nil {
		t.Fatalf("RunWithWriter: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "SCHEMA") {
		t.Fatalf("expected SCHEMA header in output:\n%s", out)
	}

	if !strings.Contains(out, `"name": "app"`) || !strings.Contains(out, `"type": "string"`) {
		t.Errorf("metadata->labels row missing pretty nested SCHEMA JSON, got:\n%s", out)
	}

	nameLine := lineContaining(t, out, "| name")
	if strings.Contains(nameLine, "{") {
		t.Errorf("leaf field row should have empty SCHEMA cell, got: %s", nameLine)
	}

	// Object-typed fields are not listed on their own row: only their depth-2
	// "parent->child" subfields are.
	for _, want := range []string{"metadata->name", "metadata->labels", "status->conditions", "spec->containers"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected COLUMN row %q in output:\n%s", want, out)
		}
	}
	for _, notWant := range []string{"| spec ", "| metadata ", "| status "} {
		if strings.Contains(out, notWant) {
			t.Errorf("object field %q should not have its own row, got:\n%s", notWant, out)
		}
	}
}

func TestAppendDescribeRow_ColorizesSchemaKeys(t *testing.T) {
	f := schema.Field{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
		{Name: "containers", Type: schema.FieldTypeList},
	}}

	var plain strings.Builder
	plainTable := tablewriter.NewWriter(&plain)
	plainTable.SetAutoWrapText(false)
	if err := appendDescribeRow(plainTable, f.Name, f, false); err != nil {
		t.Fatalf("appendDescribeRow: %v", err)
	}
	plainTable.Render()
	if strings.Contains(plain.String(), utils.AnsiCyan) {
		t.Errorf("expected no ANSI color codes when colorKeys=false, got:\n%s", plain.String())
	}

	var colored strings.Builder
	coloredTable := tablewriter.NewWriter(&colored)
	coloredTable.SetAutoWrapText(false)
	if err := appendDescribeRow(coloredTable, f.Name, f, true); err != nil {
		t.Fatalf("appendDescribeRow: %v", err)
	}
	coloredTable.Render()
	if !strings.Contains(colored.String(), utils.AnsiCyan+`"name"`+utils.AnsiReset) {
		t.Errorf("expected colorized SCHEMA keys when colorKeys=true, got:\n%s", colored.String())
	}
}

func TestSortDescribeFields(t *testing.T) {
	fields := []schema.Field{
		{Name: "status", Type: schema.FieldTypeObject},
		{Name: "zzz_custom", Type: schema.FieldTypeString},
		{Name: "spec", Type: schema.FieldTypeObject},
		{Name: "namespace", Type: schema.FieldTypeString},
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "aaa_custom", Type: schema.FieldTypeString},
		{Name: "metadata", Type: schema.FieldTypeObject},
	}

	sorted := sortDescribeFields(fields)

	names := make([]string, len(sorted))
	for i, f := range sorted {
		names[i] = f.Name
	}

	want := "metadata,name,namespace,spec,status,zzz_custom,aaa_custom"
	if got := strings.Join(names, ","); got != want {
		t.Errorf("sortDescribeFields() order = %q, want %q", got, want)
	}
}

func TestRunDescribeTable_FieldOrder(t *testing.T) {
	fields := []schema.Field{
		{Name: "status", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "phase", Type: schema.FieldTypeString},
		}},
		{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "containers", Type: schema.FieldTypeList},
		}},
		{Name: "namespace", Type: schema.FieldTypeString},
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "metadata", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "labels", Type: schema.FieldTypeMap},
			{Name: "annotations", Type: schema.FieldTypeMap},
			{Name: "namespace", Type: schema.FieldTypeString},
			{Name: "name", Type: schema.FieldTypeString},
		}},
	}

	var buf strings.Builder
	cmd := NewQueryCommand(api.Config{Out: &buf}, fakeDataSource{fields: fields}, nil, nil, false)
	if err := cmd.RunWithWriter(context.Background(), "DESCRIBE TABLE pods", &buf); err != nil {
		t.Fatalf("RunWithWriter: %v", err)
	}

	out := buf.String()
	wantOrder := []string{
		"metadata->name", "metadata->namespace", "metadata->annotations", "metadata->labels",
		"name", "namespace", "spec->containers", "status->phase",
	}
	lastIdx := -1
	for _, col := range wantOrder {
		// "| <col> " pins to the COLUMN cell boundary so e.g. "namespace" doesn't
		// match inside "metadata->namespace".
		needle := "| " + col + " "
		idx := strings.Index(out, needle)
		if idx == -1 {
			t.Fatalf("expected COLUMN %q in output:\n%s", col, out)
		}
		if idx < lastIdx {
			t.Errorf("expected %q to appear in order, got out of order in:\n%s", col, out)
		}
		lastIdx = idx
	}
}

func lineContaining(t *testing.T, out, needle string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	t.Fatalf("no line containing %q in output:\n%s", needle, out)
	return ""
}
