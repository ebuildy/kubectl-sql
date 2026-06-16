package completion

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"

	shellCompletionPort "github.com/ebuildy/kubectl-sql/internal/port/autocomplete"
	k8sport "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
	"github.com/ebuildy/kubectl-sql/internal/port/schema"
)

// maxSuggestions caps how many completion candidates are shown at once.
const maxSuggestions = 50

// sqlKeywords is the static keyword list offered by Tab completion. Only
// statements kubectl-sql actually supports are included — it is read-only, so
// UPDATE/DELETE/INSERT are intentionally absent.
// statementStarters are the only keywords offered when nothing has been typed
// yet (Tab on an empty line / fresh clause), since a statement must begin with
// one of these.
var statementStarters = []string{"SELECT", "SHOW", "DESCRIBE", "WITH"}

var sqlKeywords = []string{
	// Statement starters
	"SELECT", "SHOW", "DESCRIBE", "WITH",
	// Clauses and operators
	"FROM", "WHERE", "ORDER BY", "GROUP BY", "HAVING", "LIMIT", "OFFSET",
	"DISTINCT", "AS", "AND", "OR", "NOT", "IN", "IS NULL", "LIKE", "BETWEEN",
	"COUNT", "ASC", "DESC", "ON", "USING", "UNION", "ALL",
	// Joins
	"JOIN", "INNER JOIN", "LEFT JOIN", "RIGHT JOIN", "FULL JOIN",
	"LEFT OUTER JOIN", "RIGHT OUTER JOIN", "CROSS JOIN",
	"INNER", "LEFT", "RIGHT", "FULL", "OUTER", "CROSS",
	// SHOW / DESCRIBE targets
	"TABLES", "TABLE",
}

// wordRe matches the trailing identifier the user is currently typing.
var wordRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*$`)

// fromRe extracts the first table after FROM (optionally k8s.-qualified).
var fromRe = regexp.MustCompile(`(?i)\bfrom\s+(?:k8s\.)?([A-Za-z_][A-Za-z0-9_]*)`)

// arrowChainRe matches a struct field access chain (e.g. spec->containers->0->)
// immediately preceding the word under the cursor. Group 1 is the chain with
// the trailing "->" stripped, e.g. "spec->containers->0".
var arrowChainRe = regexp.MustCompile(`((?:[A-Za-z_][A-Za-z0-9_]*|\d+)(?:->(?:[A-Za-z_][A-Za-z0-9_]*|\d+))*)->[A-Za-z0-9_]*$`)

// cliCompletionSource implements shellCompletionPort.ShellCompletionSource over the k8s DataSource port.
type cliCompletionSource struct {
	ctx         context.Context
	ds          k8sport.DataSource
	functions   []string // sorted SQL function names (lowercase) offered in expression positions
	mu          sync.Mutex
	columnCache map[string][]schema.Field // table -> fields, including SubFields (session cache)
}

// NewCompletionSource builds a completion source, returning nil if the cluster
// connection fails (completion is then disabled). functionNames is the set of
// SQL function names (e.g. "map_get", "upper") offered alongside keywords and
// columns in expression positions.
func NewShellCompletion(ctx context.Context, dataSource k8sport.DataSource, functionNames []string) shellCompletionPort.ShellCompletionRunner {
	return &cliCompletionSource{ctx: ctx, ds: dataSource, functions: functionNames, columnCache: make(map[string][]schema.Field)}
}

// Tables returns queryable resource names via the port.
func (s *cliCompletionSource) Tables() []string {
	resources, err := s.ds.Resources(s.ctx)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(resources))
	for _, r := range resources {
		names = append(names, r.Name)
	}
	return names
}

// Columns returns the inferred fields for a table (including SubFields for
// struct/map columns), or nil if it cannot be resolved.
func (s *cliCompletionSource) Columns(table string) []schema.Field {
	resource, err := s.ds.Resolve(s.ctx, strings.ToLower(table))
	if err != nil {
		return nil
	}
	fields, err := s.ds.InferSchema(s.ctx, resource)
	if err != nil {
		return nil
	}
	return fields
}

// Do implements readline.AutoCompleter. line[:pos] is the text up to the cursor.
// It returns suffix candidates and the length of the word prefix they extend.
func (c *cliCompletionSource) Do(line []rune, pos int) ([][]rune, int) {
	if pos > len(line) {
		pos = len(line)
	}
	prefixText := string(line[:pos])
	fullLine := string(line)

	word := wordRe.FindString(prefixText)
	candidates := c.candidates(prefixText, fullLine, word)
	if len(candidates) == 0 {
		return nil, len(word)
	}

	// Order alphabetically (case-insensitive) and cap the number shown.
	sort.Slice(candidates, func(i, j int) bool {
		return strings.ToLower(candidates[i]) < strings.ToLower(candidates[j])
	})
	if len(candidates) > maxSuggestions {
		candidates = candidates[:maxSuggestions]
	}

	out := make([][]rune, 0, len(candidates))
	for _, cand := range candidates {
		// Return the part of the candidate beyond what the user already typed.
		out = append(out, []rune(cand[len(word):]))
	}
	return out, len(word)
}

// candidates picks the right candidate set based on cursor context and filters
// by the typed word. The returned strings are full candidates (including the
// typed prefix), case-adjusted to match the user's typed case for keywords.
// candidates picks the candidate set based on cursor context. prefixText is the
// line up to the cursor (used to decide what the user is typing now); fullLine
// is the entire line (used to resolve the FROM table, which may appear after the
// cursor, e.g. "SELECT sta| FROM pods").
func (c *cliCompletionSource) candidates(prefixText, fullLine, word string) []string {
	lowerWord := strings.ToLower(word)

	if expectsTableName(prefixText) {
		return matchPrefix(c.Tables(), lowerWord)
	}

	table := tableInLine(fullLine)

	// Struct/map field access (e.g. "spec->con"): suggest only the subfields of
	// the resolved parent path, not the full keyword/function/column set.
	if chain := arrowChainRe.FindStringSubmatch(prefixText); chain != nil {
		if table == "" {
			return nil
		}
		fields := schema.SubFieldsAtPath(c.columns(table), strings.Split(chain[1], "->"))
		return matchPrefix(fieldNames(fields), lowerWord)
	}

	var out []string
	// Keywords (case preserved relative to the typed word).
	out = append(out, matchKeywords(word)...)
	// Function names (always lowercase), e.g. map_get, upper. Suffixed with "("
	// since a function name is always followed by its argument list.
	for _, fn := range matchPrefix(c.functions, lowerWord) {
		out = append(out, fn+"(")
	}
	// Columns for the FROM table, if one is resolvable from the whole line.
	if table != "" {
		out = append(out, matchPrefix(fieldNames(c.columns(table)), lowerWord)...)
	}
	return out
}

// fieldNames extracts the Name of each field.
func fieldNames(fields []schema.Field) []string {
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
	}
	return names
}

// expectsTableName reports whether the word under the cursor is in a position
// where a table name is expected: immediately after FROM/JOIN, or after the
// TABLE keyword in a "DESCRIBE TABLE <here>" statement.
func expectsTableName(prefixText string) bool {
	// Strip the trailing partial word, then look at the last token(s).
	stripped := wordRe.ReplaceAllString(prefixText, "")
	fields := strings.Fields(stripped)
	if len(fields) == 0 {
		return false
	}
	last := strings.ToLower(fields[len(fields)-1])
	if last == "from" || last == "join" {
		return true
	}
	// DESCRIBE TABLE <table>
	if last == "table" && len(fields) >= 2 && strings.ToLower(fields[len(fields)-2]) == "describe" {
		return true
	}
	return false
}

// tableInLine extracts the FROM table from the line, or "" if none.
func tableInLine(line string) string {
	m := fromRe.FindStringSubmatch(line)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// matchKeywords returns keywords whose lowercase form has the typed word as a
// prefix, rendered in the case the user is typing (upper if the typed prefix is
// uppercase, lower otherwise).
func matchKeywords(word string) []string {
	lower := strings.ToLower(word)
	// With no typed letters, default to uppercase (SQL convention); otherwise
	// match the case of what the user is typing.
	upper := word == "" || word == strings.ToUpper(word)
	// With nothing typed, only statement starters make sense (a statement must
	// begin with one of them) — avoids flooding the prompt with operators.
	pool := sqlKeywords
	if word == "" {
		pool = statementStarters
	}
	var out []string
	for _, kw := range pool {
		if strings.HasPrefix(strings.ToLower(kw), lower) {
			if upper {
				out = append(out, kw)
			} else {
				out = append(out, strings.ToLower(kw))
			}
		}
	}
	return out
}

// matchPrefix returns items (lowercased identifiers) that start with word.
func matchPrefix(items []string, lowerWord string) []string {
	var out []string
	for _, it := range items {
		if strings.HasPrefix(strings.ToLower(it), lowerWord) {
			out = append(out, it)
		}
	}
	return out
}

// columns returns the cached fields for table, fetching and caching them on
// first use (eager prefetch happens via Prefetch; this is the lazy fallback).
func (c *cliCompletionSource) columns(table string) []schema.Field {
	c.mu.Lock()
	cols, ok := c.columnCache[table]
	c.mu.Unlock()
	if ok {
		return cols
	}
	cols = c.Columns(table)
	c.mu.Lock()
	c.columnCache[table] = cols
	c.mu.Unlock()
	return cols
}

// Prefetch warms the column cache for the table named in the line, if any, so
// that subsequent column completions do not block on inference. Safe to call
// from a separate goroutine.
func (c *cliCompletionSource) Prefetch(line string) {
	table := tableInLine(line)
	if table == "" {
		return
	}
	c.mu.Lock()
	_, ok := c.columnCache[table]
	c.mu.Unlock()
	if ok {
		return
	}
	_ = c.Columns(table) // populates cache
}
