package utils

import (
	"os"

	"golang.org/x/term"
)

// StdinIsTTY reports whether the process stdin is an interactive terminal.
func StdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
