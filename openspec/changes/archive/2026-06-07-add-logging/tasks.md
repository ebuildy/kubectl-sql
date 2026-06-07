## 1. Dependency

- [x] 1.1 Add `go.uber.org/zap` via `go get go.uber.org/zap` (pulls in `go.uber.org/multierr`)

## 2. Logging Port + Adapter (hexagonal)

- [x] 2.1 Create the port package `internal/port/logger/logger.go`: `Logger` interface (`Debug/Info/Error(msg string, fields ...Field)`, `With(...Field) Logger`, `Sync() error`), the `Field` type, field constructors (`String`, `Int`, `Err`, `Any`), `Options{Verbosity int; NoColor bool}`, and `Nop() Logger`. MUST NOT import any logging library.
- [x] 2.2 Add context helpers in the port package (`internal/port/logger/context.go`): `IntoContext(ctx, Logger) context.Context` and `FromContext(ctx) Logger` (returns `Nop()` when absent).
- [x] 2.3 Create the adapter package `internal/adapter/logger/zap/zap.go`: the ONLY package importing `go.uber.org/zap`. Implements `New(logger.Options) logger.Logger` with a console encoder to `zapcore.Lock(os.Stderr)`, level mapped `0→Error, 1→Info, >=2→Debug`, `NoColor` disabling level coloring, and `logger.Field → zap.Field` mapping. Returns a type implementing the port (including `Sync`).
- [x] 2.4 Unit test the port (`Nop()` is non-nil and safe, field constructors produce expected keys, `FromContext` round-trips and falls back to `Nop()`) and the adapter (level mapping 0/1/2/3).
- [x] 2.5 Guard test (`internal/adapter/logger/zap/boundary_test.go` or repo-root): scan the source tree and assert `go.uber.org/zap` is imported only by `internal/adapter/logger/zap` and the `cmd` composition root.

## 3. CLI Integration

- [x] 3.1 Register `rootCmd.PersistentFlags().CountP("verbose", "v", "Increase log verbosity: -v=info, -vv=debug (default error)")`
- [x] 3.2 Add `PersistentPreRunE` to the root command (composition root): read `verbose` and `--no-color`, build the logger via `zap.New(logger.Options{...})`, and store it with `cmd.SetContext(logger.IntoContext(cmd.Context(), l))`. This is the only `cmd` import of the zap adapter.
- [x] 3.3 Flush via `defer l.Sync()` (port `Sync()`, no-op for `Nop()`) so callers never touch the library's `Sync`

## 4. Instrument the Pipeline

All components retrieve the logger via `logger.FromContext(ctx)` (the port) and call only port methods — no zap imports.

- [x] 4.1 `internal/k8s`: info log when the dynamic client/cluster connection is established (context, host)
- [x] 4.2 `internal/schema`: debug log the resolved GVR and which inferrer (OpenAPI vs sample) supplied fields
- [x] 4.3 `cmd` query pipeline: info log query accepted (+ target resource); debug logs for parse, typecheck, optimize, materialize steps
- [x] 4.4 `internal/executor`: debug log LIST pagination (page index + item count per page)
- [x] 4.5 `internal/repl`: info log REPL/batch start; debug log each executed query
- [x] 4.6 `cmd` watch: debug log each poll tick refresh
- [x] 4.7 Add a `Duration(key, time.Duration) Field` port constructor (records `<key>_ms`); log elapsed time at debug for total query, schema inference, and each LIST page

## 5. Tests

- [x] 5.1 Integration/e2e scenario: `kubectl-sql -vv "SELECT name FROM pods"` exits 0 and stdout is still valid (stderr carries logs)
- [x] 5.2 Integration/e2e scenario: default run (no `-v`) produces no info/debug log lines on stderr

## 6. Documentation

- [x] 6.1 Update `README.md`: add `-v` / `--verbose` to the flag table and a short note on `-v`/`-vv` levels and stderr output
