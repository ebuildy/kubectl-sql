package utils

import (
	"os"
	"regexp"

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
