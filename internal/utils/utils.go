package utils

import (
	"os"
	"regexp"
	"strings"

	"golang.org/x/term"
)

// StdinIsTTY reports whether the process stdin is an interactive terminal.
func StdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// MapKeys return keys slice of a map
func MapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

const (
	AnsiCyan  = "\x1b[36m"
	AnsiReset = "\x1b[0m"
)

// jsonKeyPattern matches an object key at the start of a line of indented
// JSON. Anchoring on the line start means quote-colon sequences inside string
// values can never match.
var jsonKeyPattern = regexp.MustCompile(`(?m)^(\s*)("(?:[^"\\]|\\.)*")(\s*:)`)

// ColorizeJSONKeys wraps the object keys of indented JSON in ANSI cyan,
// leaving values, braces, and punctuation uncolored.
func ColorizeJSONKeys(s string) string {
	return jsonKeyPattern.ReplaceAllString(s, "${1}"+AnsiCyan+"${2}"+AnsiReset+"${3}")
}

// yamlTopLevelKeyPattern matches a top-level (column 0) mapping key in
// indented YAML: a double- or single-quoted key, or a plain key that doesn't
// start with whitespace, '#', or '-' (which would be a comment or sequence
// item), followed by ':' and a space or end of line. Anchoring on column 0
// means nested map keys, sequence-item keys, and the indented content of
// literal block scalars (always indented relative to their key) can never
// match.
var yamlTopLevelKeyPattern = regexp.MustCompile(`(?m)^("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|[^\s:#-][^:\n]*?)(:)(\s|$)`)

// ColorizeYAMLTopLevelKeys wraps the top-level (root) mapping keys of
// indented YAML in ANSI cyan, leaving nested keys, sequence-item keys,
// values, and block scalar content uncolored.
func ColorizeYAMLTopLevelKeys(s string) string {
	return yamlTopLevelKeyPattern.ReplaceAllString(s, AnsiCyan+"${1}"+AnsiReset+"${2}${3}")
}

// UnescapeJSONNewlines converts the JSON escape sequence \n inside string
// literals of an already-marshaled JSON document into a real newline byte,
// leaving every other escape sequence (\", \\, \t, \uXXXX, ...) untouched.
// This makes multi-line string values (e.g. a ConfigMap data entry holding a
// shell script) readable as real lines in pretty-printed table cells.
func UnescapeJSONNewlines(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	inString := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString && c == '\\' && i+1 < len(s) {
			next := s[i+1]
			if next == 'n' {
				out.WriteByte('\n')
			} else {
				out.WriteByte(c)
				out.WriteByte(next)
			}
			i++
			continue
		}
		if c == '"' {
			inString = !inString
		}
		out.WriteByte(c)
	}
	return out.String()
}
