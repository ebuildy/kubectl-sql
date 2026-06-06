## Context

`kubectl-sql` currently requires every query to be passed as a positional argument, making iterative debugging workflows awkward. A REPL removes the shell quoting friction and adds session continuity. The existing `cmd/root.go` entry point already detects missing arguments (showing help); the REPL replaces that path on a TTY.

The watch mode polling loop (`internal/repl` doesn't exist yet) and the main query runner (`runQueryWithWriter`) are the closest analogues in the codebase. The REPL will reuse `runQueryWithWriter` unchanged.

## Goals / Non-Goals

**Goals:**
- Interactive prompt loop (`sql> `) with Enter-to-execute, up-arrow history
- Clean exit on `\q`, `quit`, `exit`, Ctrl-C
- `\help` / `?` listing available commands
- Inherits all CLI flags from the parent invocation
- Non-TTY stdin: batch line-by-line mode (no prompt)
- TTY detection: fall into REPL when `kubectl-sql` is invoked with no positional arg

**Non-Goals:**
- Persistent history across sessions (in-memory only, v1)
- Multi-line SQL input (query ends on Enter; `;` is optional/stripped)
- Tab-completion of SQL keywords or resource names
- `\output`, `\timing`, or other psql-style meta-commands beyond `\help` / `\q`

## Decisions

### 1. Line-editing library: `golang.org/x/term` raw mode vs `github.com/chzyer/readline`

**Decision**: Use `github.com/chzyer/readline`.

`readline` provides prompt display, in-memory history, and arrow-key navigation out of the box. Implementing the same with `golang.org/x/term` raw mode requires manual cursor handling, ANSI sequence parsing, and history ring management — significant incidental complexity for no benefit. `readline` is a single dependency, pure Go, maintained, and already used in tools like `hugo` and `kubectl`-ecosystem projects. It handles TTY detection internally.

If `readline` proves problematic (e.g. build issues on Windows CI), `bufio.Scanner` is the fallback for the non-TTY batch path, which is already needed anyway.

### 2. Package location: `internal/repl/` new package vs inline in `cmd/`

**Decision**: New package `internal/repl/`.

The REPL loop is ~100 lines of logic (prompt, read, execute, handle slash commands). Keeping it in `cmd/root.go` would grow that file and tangle routing logic with I/O. `internal/repl/` is testable in isolation by injecting a fake `io.Reader` / `io.Writer` pair.

### 3. Entry-point detection: `--repl` flag vs no-arg detection

**Decision**: Both — TTY no-arg detection is primary; `--repl` flag is an explicit override.

No-arg + TTY is the zero-friction path (just run `kubectl sql`). The `--repl` flag allows forcing REPL mode in scripts or when debugging from a non-TTY environment. Detection uses `golang.org/x/term.IsTerminal(int(os.Stdin.Fd()))`, consistent with how watch mode detects TTY on stdout.

### 4. Query termination: Enter vs `;`

**Decision**: Enter terminates a query. A trailing `;` is stripped if present.

Multi-line SQL is not in scope for v1. Users coming from psql may habitually type `;` — stripping it silently is more ergonomic than an error. The dot-to-arrow rewriter already runs inside `runQueryWithWriter`, so REPL queries get the same preprocessing as CLI queries.

## Risks / Trade-offs

- **`readline` dependency** → added to `go.mod`. Justified by eliminating ~300 lines of raw-mode handling. Noted in proposal Impact section.
- **`readline` and non-TTY** → `readline` handles this by falling back to buffered read; the REPL will also do an explicit `term.IsTerminal` check to switch to batch mode before creating the readline instance.
- **Ctrl-C inside a running query** → the query runs synchronously; Ctrl-C during execution will interrupt the current query context (SIGINT propagates to the context cancel). The REPL should return to the prompt, not exit. This requires wrapping each query in a per-tick context with cancel, separate from the session context. Same pattern as watch mode.
- **Error display** → query errors are printed to stderr; the prompt continues. This matches the "invalid SQL prints an error and returns to prompt" requirement.

## Open Questions

- Should `\history` be a v1 command? Deferred — in-memory history is accessible via arrow keys; a printed list is a v2 nicety.
- Windows TTY support: `readline` uses `golang.org/x/sys` under the hood and supports Windows consoles. No special handling needed in v1, but should be smoke-tested.
