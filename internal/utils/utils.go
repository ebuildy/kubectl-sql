package utils

import (
	"os"

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
