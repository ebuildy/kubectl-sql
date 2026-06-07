package octosql

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/cube2222/octosql/execution"
	"github.com/cube2222/octosql/octosql"
	"github.com/cube2222/octosql/physical"
	"github.com/olekukonko/tablewriter"
)

// Options controls how Render collects and formats results.
type Options struct {
	Format          string // "table" | "json" | "csv"
	Limit           *int64
	OrderBy         []execution.Expression
	OrderDirections []int
	Schema          physical.Schema
	Writer          io.Writer
}

// Render drives the octosql execution node, collects all records, applies
// ORDER BY and LIMIT, then writes results in the requested format.
// It has no dependency on terminal state or /dev/tty.
func Render(execCtx execution.ExecutionContext, node execution.Node, opts Options) error {
	var rows [][]octosql.Value

	if err := node.Run(
		execCtx,
		func(_ execution.ProduceContext, record execution.Record) error {
			if record.Retraction {
				return nil
			}
			row := make([]octosql.Value, len(record.Values))
			copy(row, record.Values)
			rows = append(rows, row)
			return nil
		},
		func(_ execution.ProduceContext, _ execution.MetadataMessage) error {
			return nil
		},
	); err != nil {
		return fmt.Errorf("output: execute query: %w", err)
	}

	if len(opts.OrderBy) > 0 {
		sort.SliceStable(rows, func(i, j int) bool {
			for k, expr := range opts.OrderBy {
				vi, err := expr.Evaluate(execCtx.WithRecord(execution.NewRecord(rows[i], false, time.Time{})))
				if err != nil {
					return false
				}
				vj, err := expr.Evaluate(execCtx.WithRecord(execution.NewRecord(rows[j], false, time.Time{})))
				if err != nil {
					return false
				}
				cmp := vi.Compare(vj)
				if cmp == 0 {
					continue
				}
				dir := 1
				if k < len(opts.OrderDirections) {
					dir = opts.OrderDirections[k]
				}
				return cmp*dir < 0
			}
			return false
		})
	}

	if opts.Limit != nil && int64(len(rows)) > *opts.Limit {
		rows = rows[:*opts.Limit]
	}

	fields := schemaFieldNames(opts.Schema)

	switch opts.Format {
	case "json":
		return renderJSON(opts.Writer, fields, opts.Schema.Fields, rows)
	case "csv":
		return renderCSV(opts.Writer, fields, rows)
	default:
		return renderTable(opts.Writer, fields, rows)
	}
}

func schemaFieldNames(schema physical.Schema) []string {
	names := make([]string, len(schema.Fields))
	for i, f := range schema.Fields {
		names[i] = f.Name
	}
	return names
}

func renderTable(w io.Writer, fields []string, rows [][]octosql.Value) error {
	table := tablewriter.NewWriter(w)
	table.SetHeader(fields)
	table.SetAutoFormatHeaders(false)
	table.SetBorder(true)
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, v := range row {
			cells[i] = valueToString(v)
		}
		table.Append(cells)
	}
	table.Render()
	return nil
}

func renderJSON(w io.Writer, fields []string, schemaFields []physical.SchemaField, rows [][]octosql.Value) error {
	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		obj := make(map[string]interface{}, len(fields))
		for i, f := range fields {
			if i >= len(row) {
				continue
			}
			if i < len(schemaFields) {
				obj[f] = valueToNativeTyped(row[i], schemaFields[i].Type)
			} else {
				obj[f] = valueToNative(row[i])
			}
		}
		out = append(out, obj)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// valueToNativeTyped converts an octosql.Value to a Go native value using the
// schema type for struct field name resolution.
func valueToNativeTyped(v octosql.Value, t octosql.Type) interface{} {
	if v.TypeID == octosql.TypeIDStruct && t.TypeID == octosql.TypeIDStruct {
		out := make(map[string]interface{}, len(v.Struct))
		for i, elem := range v.Struct {
			if i >= len(t.Struct.Fields) {
				break
			}
			out[t.Struct.Fields[i].Name] = valueToNativeTyped(elem, t.Struct.Fields[i].Type)
		}
		return out
	}
	return valueToNative(v)
}

func renderCSV(w io.Writer, fields []string, rows [][]octosql.Value) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(fields); err != nil {
		return err
	}
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, v := range row {
			cells[i] = valueToString(v)
		}
		if err := cw.Write(cells); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func valueToString(v octosql.Value) string {
	switch v.TypeID {
	case octosql.TypeIDNull:
		return "<null>"
	case octosql.TypeIDInt:
		return fmt.Sprintf("%d", v.Int)
	case octosql.TypeIDFloat:
		return fmt.Sprintf("%g", v.Float)
	case octosql.TypeIDBoolean:
		if v.Boolean {
			return "true"
		}
		return "false"
	case octosql.TypeIDString:
		return v.Str
	case octosql.TypeIDTime:
		return v.Time.Format(time.RFC3339)
	default:
		return v.String()
	}
}

func valueToNative(v octosql.Value) interface{} {
	switch v.TypeID {
	case octosql.TypeIDNull:
		return nil
	case octosql.TypeIDInt:
		return v.Int
	case octosql.TypeIDFloat:
		return v.Float
	case octosql.TypeIDBoolean:
		return v.Boolean
	case octosql.TypeIDString:
		return v.Str
	case octosql.TypeIDTime:
		return v.Time.Format(time.RFC3339)
	default:
		return v.String()
	}
}
