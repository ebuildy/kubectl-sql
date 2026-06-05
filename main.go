// Package main is the entrypoint for the kubectl-sql binary.
package main

import (
	"os"

	"github.com/ebuildy/kubectl-sql/cmd"
)

func main() {
	cmd.Execute()
	// ristretto (used by octosql functions) spawns background goroutines that
	// never stop. os.Exit ensures the process terminates cleanly.
	os.Exit(0)
}
