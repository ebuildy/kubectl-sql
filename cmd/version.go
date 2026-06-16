package cmd

// version is the build version string. It defaults to "dev" and is overridden
// at build time via -ldflags "-X github.com/ebuildy/kubectl-sql/cmd.version=<value>"
// (see the Makefile build target). Plain `go build` leaves it as "dev".
var version = "dev"

// projectURL is the canonical project home, surfaced by the REPL /version command.
const projectURL = "https://github.com/ebuildy/kubectl-sql"
