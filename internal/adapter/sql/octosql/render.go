package octosql

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/cube2222/octosql/execution"
	"github.com/cube2222/octosql/octosql"
	"github.com/cube2222/octosql/physical"
	"github.com/ebuildy/kubectl-sql/internal/utils"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v3"
)

// beautifyFormat selects how pretty-printed (beautify) struct/list/tuple/map
// cells are rendered in table output.
type beautifyFormat string

const (
	beautifyFormatJSON beautifyFormat = "json"
	beautifyFormatYAML beautifyFormat = "yaml"
)

// beautifyFormatActive is the active beautify cell format for pretty
// (pretty=true) struct/list/tuple/map cells in table output. YAML is the
// default: multi-line string values render as literal block scalars instead
// of JSON's escaped "\n", and with colorKeys enabled, top-level (root)
// mapping keys are colored via ColorizeYAMLTopLevelKeys. Switch to
// beautifyFormatJSON to fall back to pretty-printed JSON (with the same
// real-newline fix via UnescapeJSONNewlines and full-depth key coloring via
// ColorizeJSONKeys). This has no effect on --output json, --output csv, or
// --disable-beauty, which always use compact JSON. It is a var (not const)
// only so tests can override it; switching the default is still a one-line
// code change.
var beautifyFormatActive = beautifyFormatYAML

// Options controls how Render collects and formats results.
type Options struct {
	Format          string // "table" | "json" | "csv"
	Limit           *int64
	OrderBy         []execution.Expression
	OrderDirections []int
	Schema          physical.Schema
	Writer          io.Writer
	Pretty          bool // indent struct cells in table output
	ColorKeys       bool // ANSI-color JSON keys in pretty struct cells (table only)
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
		return renderCSV(opts.Writer, fields, opts.Schema.Fields, rows)
	default:
		return renderTable(opts.Writer, fields, opts.Schema.Fields, rows, opts.Pretty, opts.ColorKeys)
	}
}

func schemaFieldNames(schema physical.Schema) []string {
	names := make([]string, len(schema.Fields))
	for i, f := range schema.Fields {
		names[i] = f.Name
	}
	return names
}

func renderTable(w io.Writer, fields []string, schemaFields []physical.SchemaField, rows [][]octosql.Value, pretty, colorKeys bool) error {
	table := tablewriter.NewWriter(w)
	table.SetHeader(fields)
	table.SetAutoFormatHeaders(false)
	table.SetBorder(true)
	// Auto-wrap reflows cell text and collapses the newlines of pretty-printed
	// JSON; disable it so multi-line struct cells render verbatim.
	table.SetAutoWrapText(false)
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, v := range row {
			if i < len(schemaFields) {
				cell := valueToStringTyped(v, schemaFields[i].Type, pretty)
				if pretty && rendersAsJSON(v, schemaFields[i].Type) {
					switch beautifyFormatActive {
					case beautifyFormatJSON:
						if colorKeys {
							cell = utils.ColorizeJSONKeys(cell)
						}
						cell = utils.UnescapeJSONNewlines(cell)
					case beautifyFormatYAML:
						if colorKeys {
							cell = utils.ColorizeYAMLTopLevelKeys(cell)
						}
					}
				}
				cells[i] = cell
			} else {
				cells[i] = valueToString(v)
			}
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
	// Indexing a typed list (e.g. spec->containers[0]) yields a nullable element
	// type (Null | Struct). Strip the nullability so the struct/list branches below
	// resolve field names instead of falling back to octosql's positional form.
	t = octosql.NonNullable(t)
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
	// Map columns are carried as a flat List<Any> of alternating key/value elements
	// ([k1, v1, k2, v2, ...]). In JSON output, decode them into a real object
	// (e.g. labels -> {"app":"nginx"}) with each value in its native JSON type.
	if v.TypeID == octosql.TypeIDList && isMapListType(t) {
		out := make(map[string]interface{}, len(v.List)/2)
		for i := 0; i+1 < len(v.List); i += 2 {
			out[v.List[i].Str] = valueToNative(v.List[i+1])
		}
		return out
	}
	// A list whose element type is a struct (List<Struct>, e.g. spec->containers)
	// carries real struct element values. Decode each element with the element
	// struct type so its field names resolve, producing an array of named-key
	// objects instead of opaque JSON strings.
	if v.TypeID == octosql.TypeIDList && t.TypeID == octosql.TypeIDList &&
		t.List.Element != nil && t.List.Element.TypeID == octosql.TypeIDStruct {
		out := make([]interface{}, len(v.List))
		for i, elem := range v.List {
			out[i] = valueToNativeTyped(elem, *t.List.Element)
		}
		return out
	}
	// array_get()/list indexing extracts a single list element and types it
	// Any|Null (see arrayGetFunction): like other list elements, its string form
	// is JSON-encoded and must be decoded rather than emitted as an escaped string.
	if v.TypeID == octosql.TypeIDString && t.TypeID == octosql.TypeIDAny {
		return decodeListElement(v)
	}
	return valueToNative(v)
}

// decodeListElement decodes a JSON-encoded list element string into its native
// form (object, array, number, etc.), falling back to the raw string if it
// isn't valid JSON.
func decodeListElement(v octosql.Value) interface{} {
	if v.TypeID == octosql.TypeIDString {
		var decoded interface{}
		if json.Unmarshal([]byte(v.Str), &decoded) == nil {
			return decoded
		}
	}
	return valueToNative(v)
}

func renderCSV(w io.Writer, fields []string, schemaFields []physical.SchemaField, rows [][]octosql.Value) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(fields); err != nil {
		return err
	}
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, v := range row {
			if i < len(schemaFields) {
				cells[i] = valueToStringTyped(v, schemaFields[i].Type, false)
			} else {
				cells[i] = valueToString(v)
			}
		}
		if err := cw.Write(cells); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// rendersAsJSON reports whether a cell value is composite and should be
// rendered as JSON: structs (when the schema type carries the field names),
// lists (including map columns carried as flat key/value lists), and tuples.
func rendersAsJSON(v octosql.Value, t octosql.Type) bool {
	// A list-element access (e.g. spec->containers[0]) carries a nullable struct
	// type (Null | Struct); strip nullability so the struct cell still renders as
	// a pretty object rather than octosql's positional { v1, v2 } form.
	t = octosql.NonNullable(t)
	switch v.TypeID {
	case octosql.TypeIDStruct:
		return t.TypeID == octosql.TypeIDStruct
	case octosql.TypeIDList, octosql.TypeIDTuple:
		return true
	default:
		return false
	}
}

// valueToStringTyped renders a cell value using the schema type for struct
// field name resolution. Composite values (struct, list, tuple, map column)
// become JSON (indented when pretty); all other types keep the valueToString
// form. Marshal failures fall back to the octosql string form — rendering
// never fails an executed query.
func valueToStringTyped(v octosql.Value, t octosql.Type, pretty bool) string {
	if rendersAsJSON(v, t) {
		native := valueToNativeTyped(v, t)
		if pretty && beautifyFormatActive == beautifyFormatYAML {
			b, err := yaml.Marshal(native)
			if err != nil {
				return v.String()
			}
			return strings.TrimRight(string(b), "\n")
		}
		var b []byte
		var err error
		if pretty {
			b, err = json.MarshalIndent(native, "", "  ")
		} else {
			b, err = json.Marshal(native)
		}
		if err != nil {
			return v.String()
		}
		return string(b)
	}
	return valueToString(v)
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
	case octosql.TypeIDList:
		out := make([]interface{}, len(v.List))
		for i, e := range v.List {
			// List elements are JSON-encoded strings; decode so nested objects
			// render as real JSON rather than escaped strings.
			out[i] = decodeListElement(e)
		}
		return out
	case octosql.TypeIDTuple:
		out := make([]interface{}, len(v.Tuple))
		for i, e := range v.Tuple {
			out[i] = valueToNative(e)
		}
		return out
	default:
		return v.String()
	}
}
