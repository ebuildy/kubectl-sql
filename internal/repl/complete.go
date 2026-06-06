package repl

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// maxSuggestions caps how many completion candidates are shown at once.
const maxSuggestions = 50

// CompletionSource supplies the dynamic data the completer needs. It is
// implemented in cmd/ over discovery + the schema inferrer, and injected via
// Config so the repl package stays independent of cmd/.
type CompletionSource interface {
	// Tables returns the set of queryable resource names (same set as SHOW TABLES).
	Tables() []string
	// Columns returns the column names for a given table, or nil if it cannot
	// be resolved. Implementations should cache; the completer also caches.
	Columns(table string) []string
}

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

// completer implements readline.AutoCompleter. It offers, for the word under
// the cursor, SQL keywords, table names (after FROM/JOIN), and column names for
// the table named in the query's FROM clause.
type completer struct {
	src CompletionSource

	mu          sync.Mutex
	columnCache map[string][]string // table -> columns (session cache)
}

func newCompleter(src CompletionSource) *completer {
	return &completer{src: src, columnCache: map[string][]string{}}
}

// wordRe matches the trailing identifier the user is currently typing.
var wordRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*$`)

// fromRe extracts the first table after FROM (optionally k8s.-qualified).
var fromRe = regexp.MustCompile(`(?i)\bfrom\s+(?:k8s\.)?([A-Za-z_][A-Za-z0-9_]*)`)

// Do implements readline.AutoCompleter. line[:pos] is the text up to the cursor.
// It returns suffix candidates and the length of the word prefix they extend.
func (c *completer) Do(line []rune, pos int) ([][]rune, int) {
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
func (c *completer) candidates(prefixText, fullLine, word string) []string {
	lowerWord := strings.ToLower(word)

	if expectsTableName(prefixText) {
		return matchPrefix(c.tables(), lowerWord)
	}

	var out []string
	// Keywords (case preserved relative to the typed word).
	out = append(out, matchKeywords(word)...)
	// Columns for the FROM table, if one is resolvable from the whole line.
	if table := tableInLine(fullLine); table != "" {
		out = append(out, matchPrefix(c.columns(table), lowerWord)...)
	}
	return out
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

func (c *completer) tables() []string {
	if c.src == nil {
		return nil
	}
	return c.src.Tables()
}

// columns returns the cached column list for table, fetching and caching it on
// first use (eager prefetch happens via Prefetch; this is the lazy fallback).
func (c *completer) columns(table string) []string {
	if c.src == nil {
		return nil
	}
	c.mu.Lock()
	cols, ok := c.columnCache[table]
	c.mu.Unlock()
	if ok {
		return cols
	}
	cols = c.src.Columns(table)
	c.mu.Lock()
	c.columnCache[table] = cols
	c.mu.Unlock()
	return cols
}

// Prefetch warms the column cache for the table named in the line, if any, so
// that subsequent column completions do not block on inference. Safe to call
// from a separate goroutine.
func (c *completer) Prefetch(line string) {
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
	_ = c.columns(table) // populates cache
}
