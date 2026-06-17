## 1. Driving ports for the web adapter

- [x] 1.1 Add a `QueryRunner` port with `RunJSON(ctx, sql string) (QueryResult, error)` and a `QueryResult` type (`Columns []string`, `Rows []map[string]any`) in a new `internal/port/web` package
- [x] 1.2 Add a `Completer` port with `Complete(line string, pos int) []string` in `internal/port/web`
- [x] 1.3 Define a `web.Error`/structured error shape carrying message + optional `Suggestion`/`CorrectedSQL` so handlers can serialize typo corrections

## 2. Web HTTP adapter (primary/driving)

- [x] 2.1 Create `internal/adapter/web/server.go`: an HTTP server type built from a `QueryRunner`, a `Completer`, and a bind address; registers routes and supports `Start`/`Shutdown`
- [x] 2.2 Implement `GET /` handler serving the embedded HTML page
- [x] 2.3 Implement `POST /api/query`: decode `{sql}`, reject malformed body with 400, run via `QueryRunner`, return `{columns, rows}` JSON
- [x] 2.4 In `POST /api/query`, reject DELETE/mutating statements with 403 (reuse the existing delete-statement classifier) before any execution
- [x] 2.5 In `POST /api/query`, map parse/exec errors to 400 with a JSON error body; include suggestion + corrected SQL when a `*sqlPort.SuggestionError` is returned
- [x] 2.6 Implement `GET /api/complete`: parse `line` + `pos`, call `Completer`, return `{candidates: [...]}` JSON
- [x] 2.7 Ensure all `/api/*` responses set `Content-Type: application/json` for success and error paths

## 3. Embedded front-end assets

- [x] 3.1 Create `internal/adapter/web/assets/index.html` with the two-section layout (SQL editor + results) and a Run control
- [x] 3.2 Add small CSS (`assets/app.css`) — minimal styling, no framework
- [x] 3.3 Implement `assets/app.js`: textarea-over-`<pre>` syntax highlight overlay (keywords, strings, `->`)
- [x] 3.4 Implement debounced autocomplete in `app.js` calling `GET /api/complete`, rendering a candidate list, inserting on Enter/Tab
- [x] 3.5 Implement query submit (Ctrl/Cmd+Enter and Run button) calling `POST /api/query` and rendering `{columns, rows}` into an HTML `<table>`, with empty-state and error/suggestion rendering
- [x] 3.6 Wire `//go:embed assets/*` in the adapter and serve assets

## 4. Domain command + wiring

- [x] 4.1 Create `internal/domain/commands/ui/command.go` (`UICommand`): build data source via `k8sAdapter.New`, implement `QueryRunner` (octosql engine in JSON mode → `{columns, rows}`) and `Completer` (wrap `ShellCompletionRunner.Do` into full tokens, best-effort `Prefetch`)
- [x] 4.2 Implement `UICommand.Run`: start the web adapter, print listen URL to stderr, and shut down gracefully on SIGINT/SIGTERM via `signal.NotifyContext`
- [x] 4.3 Return a non-zero `api.ExitError` on bind failure or cluster-connection failure

## 5. CLI flags

- [x] 5.1 Register `--ui` (bool) and `--ui-address` (string, default `127.0.0.1:8080`) flags in `cmd/root.go`
- [x] 5.2 Validate `--ui-address` as a `host:port` (`net.SplitHostPort` + numeric/in-range port) before wiring anything; return a non-zero `api.ExitError` with the bad value on failure, without contacting the cluster or starting a server
- [x] 5.3 In `rootCmd.RunE`, branch to `UICommand` when `--ui` is set, before the query/REPL branch; do not require a positional query
- [x] 5.4 Print a warning when `--ui-address` binds to a non-loopback host

## 6. Tests

- [x] 6.1 Handler tests for `POST /api/query` (success, empty result, parse error→400, suggestion passthrough, malformed body→400, DELETE→403) using a fake `QueryRunner`
- [x] 6.2 Handler test for `GET /api/complete` (candidates + empty) using a fake `Completer`
- [x] 6.3 Test that `GET /` serves the embedded page and `/api/*` responses are `application/json`
- [x] 6.4 Unit test the `Completer` wrapper that turns `Do` offsets into full candidate tokens
- [x] 6.5 Test `--ui-address` validation: malformed values (no port, non-numeric/out-of-range port) fail fast with a non-zero exit and no server started

## 7. Docs & verification

- [x] 7.1 Add a `--ui` usage section to README
- [x] 7.2 Run `make lint build` and `make test` — must pass clean

## 8. UX enhancements

- [x] 8.1 Render composite (object/array) result cells as colored YAML (keys + scalar types) in `app.js`/`app.css`, mirroring the CLI table renderer; keep scalars as plain text; build `innerHTML` only from escaped content + known span tags
- [x] 8.2 Open the UI in the default browser on startup (best-effort, non-fatal) via a platform launcher (`open`/`xdg-open`/`rundll32`) in `internal/domain/commands/ui/browser.go`; rewrite unspecified bind hosts to `127.0.0.1` for the opened URL
- [x] 8.3 Forward an optional positional query through the page URL's `?sql=` parameter; pass it from `cmd/root.go` into `UICommand.Run`
- [x] 8.4 Pre-fill the editor from `?sql=` on load and run immediately (`app.js`)
- [x] 8.5 Reflect the submitted query in the URL via `history.pushState` (only when changed) and restore + re-run on `popstate` so Back/Forward work
- [x] 8.6 Make result columns resizable by dragging the header edge (`table-layout: fixed`, drag handle, minimum width) in `app.js`/`app.css`
- [x] 8.7 Add unit test for `browserURL` (encoding + unspecified-host rewrite); re-run `make lint build` and `make test`
- [x] 8.8 Minify embedded assets at build time: keep editable sources under `assets/`, minify into committed `dist/` via a `make web-assets` target using `github.com/tdewolff/minify` as a `go tool` (build-time-only) dependency; embed `dist/*`
