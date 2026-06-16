package query

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	k8sAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/datasources/k8s"
	spellcheckerAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/spellchecker"
	octosqlAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/sql/octosql"
	"github.com/ebuildy/kubectl-sql/internal/port/api"
	k8sPort "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/logger"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	sqlPort "github.com/ebuildy/kubectl-sql/internal/port/sql"
	"github.com/ebuildy/kubectl-sql/internal/utils"
	"github.com/olekukonko/tablewriter"
	"golang.org/x/term"
)

// QueryCommand encapsulates the logic for executing a single query, either from
type QueryCommand struct {
	config api.Config
	k8s    k8sPort.DataSource
	// inREPL is true when the command runs inside the interactive REPL. It
	// suppresses the DELETE progress bar (whose redraws would fight the line
	// editor) and selects the REPL input reader for the confirmation prompt.
	inREPL bool
	// in is the reader the DELETE confirmation prompt reads from (os.Stdin in
	// production; overridable in tests).
	in io.Reader
	// stdinIsTTY reports whether stdin is an interactive terminal, deciding
	// whether DELETE may prompt for confirmation.
	stdinIsTTY bool
	// mut, when non-nil, overrides the default mutator built by newMutator.
	// Production leaves it nil; tests inject a fake.
	mut sqlPort.Mutator
}

// NewQueryCommand builds a QueryCommand from CLI flags. It is the single wiring
func NewQueryCommand(ctx context.Context, config api.Config) (*QueryCommand, error) {
	ds, err := k8sAdapter.New(ctx, config.Kubeconfig, config.KubeContext, config.Namespace)
	if err != nil {
		return nil, fmt.Errorf("kubectl-sql: connect to cluster: %w", err)
	}

	return &QueryCommand{config: config, k8s: ds, in: os.Stdin, stdinIsTTY: utils.StdinIsTTY()}, nil
}

// NewQueryCommandWithDataSource builds a QueryCommand from an already-wired
// DataSource. It is the REPL path, so inREPL is set.
func NewQueryCommandWithDataSource(config api.Config, k8s k8sPort.DataSource) (*QueryCommand, error) {
	return &QueryCommand{config: config, k8s: k8s, inREPL: true, in: os.Stdin, stdinIsTTY: utils.StdinIsTTY()}, nil
}

func (c *QueryCommand) Run(ctx context.Context, query string) error {

	ctx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	if c.config.Watch {
		// DELETE is a one-shot, confirmed mutation; re-running it on a poll
		// interval is nonsensical, so reject the combination before resolving
		// anything.
		if isDeleteStatement(query) {
			fmt.Fprintln(os.Stderr, "error: DELETE cannot be used with --watch")
			return fmt.Errorf("kubectl-sql: DELETE cannot be combined with --watch")
		}
		return c.runWatch(ctx, query)
	}

	return c.RunWithWriter(ctx, query, c.config.Out)
}

func (c *QueryCommand) RunWithWriter(ctx context.Context, query string, w io.Writer) error {
	log := logger.FromContext(ctx)
	start := time.Now()
	log.Info("query accepted", logger.String("query", query), logger.String("namespace", c.config.Namespace))

	if strings.EqualFold(strings.TrimSpace(query), "show tables") {

		log.Debug("cluster connection established", logger.String("mode", "show tables"))
		return c.runShowTables(ctx)
	}

	if tok := strings.Fields(strings.ToLower(strings.TrimSpace(query))); len(tok) >= 2 && tok[0] == "describe" && tok[1] == "table" {
		parts := strings.Fields(strings.TrimSpace(query))
		if len(parts) < 3 {
			fmt.Fprintln(os.Stderr, "usage: DESCRIBE TABLE <resource>")
			return fmt.Errorf("kubectl-sql: missing resource name")
		}
		resource := strings.ToLower(parts[2])
		return c.runDescribeTable(ctx, resource)
	}

	if isDeleteStatement(query) {
		log.Debug("cluster connection established", logger.String("mode", "delete"))
		return c.runDelete(ctx, query, w)
	}

	config := sqlPort.Config{
		Output:        c.config.Output,
		Namespace:     c.config.Namespace,
		PageSize:      c.config.PageSize,
		NoColor:       c.config.NoColor,
		DisableBeauty: c.config.DisableBeauty,
	}
	eng := octosqlAdapter.New(config, c.k8s, spellcheckerAdapter.New())
	execErr := eng.Execute(ctx, sqlPort.Query{
		SQL: query,
	}, w)
	log.Debug("query completed", logger.Duration("elapsed", time.Since(start)))

	// A single mistyped keyword/table/field with a close valid match surfaces as
	// a SuggestionError. In the REPL we surface it unchanged so the loop can
	// pre-fill the corrected query into the input line for editing; on the
	// one-shot path we present the correction and prompt to run it.
	var se *sqlPort.SuggestionError
	if errors.As(execErr, &se) {
		if c.inREPL {
			return execErr
		}
		return c.handleSuggestion(ctx, se, w)
	}
	return execErr
}

func (c *QueryCommand) runWatch(ctx context.Context, query string) error {
	const pollInterval = 5 * time.Second

	// Handle SIGINT/SIGTERM: cancel context for a clean exit.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	go func() {
		select {
		case <-sigCh:
			watchCancel()
		case <-watchCtx.Done():
		}
	}()

	isTTY := isTerminal(os.Stdout)

	log := logger.FromContext(ctx)
	tick := func() error {
		log.Debug("watch tick: refreshing query")
		var buf strings.Builder
		if err := c.RunWithWriter(watchCtx, query, &buf); err != nil {
			return err
		}
		if isTTY {
			_, _ = fmt.Fprint(c.config.Out, "\033[H\033[2J")
		}
		_, _ = fmt.Fprint(c.config.Out, buf.String())
		return nil
	}

	// First tick immediately.
	if err := tick(); err != nil {
		return err
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-watchCtx.Done():
			return nil
		case <-ticker.C:
			if err := tick(); err != nil {
				return err
			}
		}
	}
}

// describeTableSchemaDepth bounds how many levels of nested fields are shown
// in the SCHEMA column of DESCRIBE TABLE output.
const describeTableSchemaDepth = 4

// describeTableFieldOrder lists well-known Kubernetes field names in the
// order DESCRIBE TABLE should display them. Every Kubernetes object shares
// this same top-level (and metadata) shape, so hardcoding the order keeps
// output stable and predictable across resource kinds. The same order is
// applied to a field's own SubFields, since metadata's subfields (name,
// namespace, labels, annotations) reuse these names. Fields not listed here
// keep their existing (inferred) order, appended after listed fields.
var describeTableFieldOrder = []string{
	"apiVersion", "kind", "metadata", "name", "namespace",
	"annotations", "labels", "spec", "data", "stringData", "status",
}

// fieldOrderIndex returns name's position in describeTableFieldOrder, or
// len(describeTableFieldOrder) if it is not a well-known field.
func fieldOrderIndex(name string) int {
	for i, n := range describeTableFieldOrder {
		if n == name {
			return i
		}
	}
	return len(describeTableFieldOrder)
}

// sortDescribeFields returns a copy of fields ordered by describeTableFieldOrder,
// preserving the relative order of fields not in that list.
func sortDescribeFields(fields []schema.Field) []schema.Field {
	sorted := make([]schema.Field, len(fields))
	copy(sorted, fields)
	sort.SliceStable(sorted, func(i, j int) bool {
		return fieldOrderIndex(sorted[i].Name) < fieldOrderIndex(sorted[j].Name)
	})
	return sorted
}

func (c *QueryCommand) runDescribeTable(ctx context.Context, resource string) error {
	r, err := c.k8s.Resolve(ctx, resource)
	if err != nil {
		return fmt.Errorf("kubectl-sql: unknown resource %q: %w", resource, err)
	}

	fields, _ := c.k8s.InferSchema(ctx, r)
	if len(fields) == 0 {
		fields = []schema.Field{
			{Name: "name", Type: schema.FieldTypeString},
			{Name: "namespace", Type: schema.FieldTypeString},
		}
	}

	colorKeys := isTerminal(os.Stdout) && !c.config.NoColor && !c.config.DisableBeauty

	table := tablewriter.NewWriter(c.config.Out)
	table.SetHeader([]string{"COLUMN", "TYPE", "SCHEMA"})
	table.SetAutoFormatHeaders(false)
	table.SetAutoWrapText(false)
	for _, f := range sortDescribeFields(fields) {
		if f.Type.IsObjectLike() && len(f.SubFields) > 0 {
			for _, sf := range sortDescribeFields(f.SubFields) {
				if err := appendDescribeRow(table, f.Name+"->"+sf.Name, sf, colorKeys); err != nil {
					return err
				}
			}
			continue
		}
		if err := appendDescribeRow(table, f.Name, f, colorKeys); err != nil {
			return err
		}
	}
	table.Render()
	return nil
}

// appendDescribeRow renders a single DESCRIBE TABLE row for field f under the
// given column name (which may be a "parent->child" path for depth-2 fields).
// When colorKeys is set, JSON object keys in the SCHEMA cell are ANSI-colored
// to match the "beauty" rendering used for query result struct cells.
func appendDescribeRow(table *tablewriter.Table, name string, f schema.Field, colorKeys bool) error {
	var fieldSchema string
	if f.Type.IsObjectLike() && len(f.SubFields) > 0 {
		s, err := schema.MarshalSubFieldsJSON(schema.LimitDepth(f.SubFields, describeTableSchemaDepth))
		if err != nil {
			return fmt.Errorf("kubectl-sql: encode schema for %q: %w", name, err)
		}
		if colorKeys {
			s = utils.ColorizeJSONKeys(s)
		}
		fieldSchema = s
	}
	table.Append([]string{name, string(f.Type), fieldSchema})
	return nil
}

func (c *QueryCommand) runShowTables(ctx context.Context) error {
	resources, err := c.k8s.Resources(ctx)
	if err != nil {
		return fmt.Errorf("kubectl-sql: list API resources: %w", err)
	}

	type row struct{ name, aliases, group, version string }
	rows := make([]row, 0, len(resources))
	for _, r := range resources {
		rows = append(rows, row{r.Name, strings.Join(r.Aliases, ","), r.Group, r.Version})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].name < rows[j].name
	})

	table := tablewriter.NewWriter(c.config.Out)
	table.SetHeader([]string{"NAME", "ALIASES", "GROUP", "VERSION"})
	table.SetAutoFormatHeaders(false)
	for _, r := range rows {
		table.Append([]string{r.name, r.aliases, r.group, r.version})
	}
	table.Render()
	return nil
}

// runWatch polls the query every 5 seconds, clearing the terminal and reprinting
// the full result table on each tick — identical output to a normal batch query.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
