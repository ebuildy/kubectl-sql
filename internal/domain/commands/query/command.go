package query

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	k8sAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/datasources/k8s"
	octosqlAdapter "github.com/ebuildy/kubectl-sql/internal/adapter/sql/octosql"
	"github.com/ebuildy/kubectl-sql/internal/port/api"
	k8sPort "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/logger"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
	sqlPort "github.com/ebuildy/kubectl-sql/internal/port/sql"
	"github.com/olekukonko/tablewriter"
	"golang.org/x/term"
)

// QueryCommand encapsulates the logic for executing a single query, either from
type QueryCommand struct {
	config api.Config
	k8s    k8sPort.DataSource
}

// NewQueryCommand builds a QueryCommand from CLI flags. It is the single wiring
func NewQueryCommand(config api.Config) (*QueryCommand, error) {
	ds, err := k8sAdapter.New(config.Kubeconfig, config.KubeContext, config.Namespace)
	if err != nil {
		return nil, fmt.Errorf("kubectl-sql: connect to cluster: %w", err)
	}

	return &QueryCommand{config: config, k8s: ds}, nil
}

// NewQueryCommand builds a QueryCommand from CLI flags. It is the single wiring
func NewQueryCommandWithDataSource(config api.Config, k8s k8sPort.DataSource) (*QueryCommand, error) {
	return &QueryCommand{config: config, k8s: k8s}, nil
}

func (c *QueryCommand) Run(ctx context.Context, query string) error {

	ctx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	if c.config.Watch {
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

	config := sqlPort.Config{
		Output:    c.config.Output,
		Namespace: c.config.Namespace,
		PageSize:  c.config.PageSize,
		NoColor:   c.config.NoColor,
	}
	eng := octosqlAdapter.New(config, c.k8s)
	execErr := eng.Execute(ctx, sqlPort.Query{
		SQL: query,
	}, w)
	log.Debug("query completed", logger.Duration("elapsed", time.Since(start)))
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

	table := tablewriter.NewWriter(c.config.Out)
	table.SetHeader([]string{"COLUMN", "TYPE"})
	table.SetAutoFormatHeaders(false)
	for _, f := range fields {
		table.Append([]string{f.Name, string(f.Type)})
	}
	table.Render()
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
