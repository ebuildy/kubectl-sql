package query

import (
	"context"
	"strings"
	"testing"

	"github.com/ebuildy/kubectl-sql/internal/port/api"
	k8sPort "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
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

func TestRunDescribeTable_SchemaColumn(t *testing.T) {
	fields := []schema.Field{
		{Name: "name", Type: schema.FieldTypeString},
		{Name: "spec", Type: schema.FieldTypeObject, SubFields: []schema.Field{
			{Name: "containers", Type: schema.FieldTypeList},
		}},
	}

	var buf strings.Builder
	cmd, err := NewQueryCommandWithDataSource(api.Config{Out: &buf}, fakeDataSource{fields: fields})
	if err != nil {
		t.Fatalf("NewQueryCommandWithDataSource: %v", err)
	}

	if err := cmd.RunWithWriter(context.Background(), "DESCRIBE TABLE pods", &buf); err != nil {
		t.Fatalf("RunWithWriter: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "SCHEMA") {
		t.Fatalf("expected SCHEMA header in output:\n%s", out)
	}

	specLine := lineContaining(t, out, "spec")
	if !strings.Contains(specLine, `"name":"containers"`) || !strings.Contains(specLine, `"type":"list"`) {
		t.Errorf("spec row missing nested SCHEMA JSON, got: %s", specLine)
	}

	nameLine := lineContaining(t, out, "name")
	if strings.Contains(nameLine, "{") {
		t.Errorf("leaf field row should have empty SCHEMA cell, got: %s", nameLine)
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
