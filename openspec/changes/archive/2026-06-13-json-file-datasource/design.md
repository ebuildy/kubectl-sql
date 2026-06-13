## Context

`internal/adapter/sql/octosql/engine.go` builds the `physical.Environment` passed to
octosql's parser/typechecker/optimizer:

```go
Datasources: &physical.DatasourceRepository{
    Databases: map[string]func() (physical.Database, error){
        "k8s": func() (physical.Database, error) { return db, nil },
    },
    FileHandlers: map[string]func(context.Context, string, map[string]string) (physical.DatasourceImplementation, physical.Schema, error){},
},
```

octosql resolves every `FROM <name>` table reference via
`physical.DatasourceRepository.GetDatasource(ctx, name, options)`
(`external/octosql/physical/physical.go:65`):

```go
func (dr *DatasourceRepository) GetDatasource(ctx context.Context, name string, options map[string]string) (DatasourceImplementation, Schema, error) {
    if index := strings.Index(name, "."); index != -1 {
        dbName := name[:index]
        if dbConstructor, ok := dr.Databases[dbName]; ok {
            db, _ := dbConstructor()
            return db.GetTable(ctx, name[index+1:], options)
        }
    }
    if index := strings.LastIndex(name, "."); index != -1 {
        extension := name[index+1:]
        if handler, ok := dr.FileHandlers[extension]; ok {
            return handler(ctx, name, options)
        }
    }
    return nil, Schema{}, fmt.Errorf("no such table: %s", name)
}
```

`name` is reconstructed by octosql's logical planner
(`external/octosql/parser/parser.go:321`, `ParseAliasedTableExpression`) as
`Qualifier + "." + Name` whenever the `sqlparser.TableName` has a non-empty
`Qualifier`. octosql's lexer (`external/octosql/parser/sqlparser/token.go`) treats
`/`, `.` (when followed by `/`), letters, digits, and `_` as identifier characters,
so unquoted paths like `fixtures/objects.json`, `./data.json`, and `/abs/path.json`
all tokenize as `<qualifier>.<ext>` and round-trip back to the original path string —
this is exactly how upstream octosql's own test fixtures reference JSON files
(`external/octosql/tests/scenarios/datasources/json/*.in`).

kubectl-sql's own rewrite step
(`internal/adapter/sql/octosql/rewrite.go`, `rewriteQuery`) only qualifies bare table
names (`tbl.Qualifier.IsEmpty()`) with `k8s.`, e.g. `FROM pods` → `FROM k8s.pods`. A
reference like `FROM data.json` already has `Qualifier = "data"` (non-empty), so it
is **left untouched** by `rewriteQuery` and falls straight through to the
extension-based `FileHandlers` lookup above. **No change to `rewrite.go` is needed.**

octosql's JSON file datasource
(`github.com/cube2222/octosql/datasources/json`, vendored for reference under
`external/octosql/datasources/json/`) implements exactly the "JSON Lines" format
called for in the proposal: `Creator` samples up to 100 lines, parsing each as a
standalone JSON object with `fastjson` to infer a `physical.Schema`, and
`DatasourceExecuting.Run` streams the file through a worker-pool line parser,
producing one row per line.

`datasources/json` (via `execution/files` and `datasources/json/execution.go`)
calls `config.FromContext(ctx)` (`external/octosql/config/config.go`), which does:

```go
func FromContext(ctx context.Context) *Config {
    return ctx.Value(contextKey{}).(*Config)
}
```

This **panics** (failed type assertion on a nil interface) if no `*config.Config`
has been placed in the context via `config.ContextWithConfig`. Today nothing in
`internal/adapter/sql/octosql` imports `config`, so the context never carries one.

## Goals / Non-Goals

**Goals:**
- Make `SELECT ... FROM <path>.json` (and `.jsonl` / `.ndjson`) work end to end:
  schema inference, column selection, `WHERE`, `ORDER BY`/`LIMIT`, and all three
  output formats (table/json/csv) — for free, since these are handled generically by
  `engine.go`'s existing pipeline and `render.go` once the datasource resolves.
- Register the JSON `FileHandlers` alongside the existing `k8s` `Databases` entry in
  the same `physical.DatasourceRepository`, so a single query can reference both
  kinds of tables without a "no such table" error, and so multiple JSON file tables
  can be combined via `JOIN` in one query (e.g. `notes.json n JOIN status.json s ON
  n.pod = s.pod`).
- Keep `github.com/cube2222/octosql` imports confined to
  `internal/adapter/sql/octosql` (existing `TestOctosqlImportBoundary` boundary
  test).
- Ensure datasource-resolution errors (e.g. "no such table", or a `.json` file that
  isn't valid JSON Lines) surface as clean errors rather than crashing the process —
  see the `typecheckNode` panic-recovery decision below.

**Non-Goals:**
- Other octosql file formats (`csv`, `tsv`, `parquet`, plain `lines`) — a future
  change per AGENTS.md "one change = one responsibility".
- Tailing/streaming a growing JSON file (octosql's `Creator` supports a `tail=true`
  option via table options, e.g. `FROM log.json?tail=true`) — not wired up or tested
  here; can be a follow-up if needed.
- Making the Kubernetes client connection lazy/optional. `NewQueryCommand`
  (`internal/domain/commands/query/command.go`) still eagerly builds the k8s dynamic
  client from kubeconfig before any query — including a pure `FROM data.json`
  query — so a loadable `--kubeconfig` is still required to start kubectl-sql, even
  though it is never called for a json-only query. Changing that wiring is out of
  scope.
- Remote/HTTP JSON sources — only local filesystem paths (octosql's
  `files.OpenLocalFile`).
- Reading a single JSON document that is a top-level array (not line-delimited) —
  explicitly out of scope per the proposal ("Use JSON lines format"); such a file
  surfaces a per-line parse error from octosql (see Risks).
- **`JOIN` between a `k8s.*`-routed table and any other table (including a `*.json`
  table)**. While building this change we found that `JOIN` execution against a
  `k8s.*` table currently returns no/incorrect rows independent of the other side —
  even `SELECT p.name AS a, p2.name AS b FROM pods p JOIN pods p2 ON p.name = p2.name`
  (k8s ⋈ k8s) returns rows with `a`/`b` both `null`, and `pods p JOIN notes.jsonl n ON
  p.name = n.pod` (k8s ⋈ json, either plain `JOIN` or `LOOKUP JOIN`) returns zero
  rows. By contrast, `JOIN` between two `*.jsonl` files returns correctly matched
  rows. This points to a pre-existing bug in
  `internal/adapter/sql/octosql/database.go`'s `KubernetesDatabase`
  execution/schema-mapping (likely watermark/end-of-stream signaling or
  field-qualification on join), unrelated to JSON files and out of scope for this
  change (different package, different responsibility). Tracked as a follow-up
  change once `database.go`'s join execution is fixed.

## Decisions

### Use octosql's `datasources/json.Creator` unmodified, via direct import
`github.com/cube2222/octosql/datasources/json` is part of the
`github.com/cube2222/octosql v0.13.0` module already required by `go.mod` — no
version bump. Importing and registering `json.Creator` (and `json.DatasourceExecuting`
indirectly through it) requires no copying/vendoring; `external/octosql/` remains
read-only reference source for the AI assistant per AGENTS.md.

**Alternative considered**: write a kubectl-sql-native JSON Lines reader in
`internal/adapter/sql/octosql`. Rejected — it would duplicate
`datasources/json`'s schema-inference and worker-pool execution logic for no
behavioral gain, directly contradicting the proposal's "octosql has native support
for it" framing.

### Register `json`, `jsonl`, and `ndjson` extensions, all mapped to the same `json.Creator`
The proposal's trigger is `SELECT file.json`, but the format itself ("JSON Lines",
one object per line) is conventionally also written with `.jsonl` and `.ndjson`
extensions. `json.Creator` does not branch on the extension string — it only uses
`name` to open the file — so mapping all three extensions to the same function is a
3-line addition with no extra code paths to maintain:

```go
FileHandlers: map[string]func(ctx context.Context, name string, options map[string]string) (physical.DatasourceImplementation, physical.Schema, error){
    "json":   json.Creator,
    "jsonl":  json.Creator,
    "ndjson": json.Creator,
},
```

**Alternative considered**: register only `"json"` (literal proposal wording).
Rejected — `.jsonl`/`.ndjson` are the same format and same handler; omitting them
would be an arbitrary restriction a user would hit immediately when naming files by
convention.

### Inject a hardcoded default `config.Config` via `config.ContextWithConfig`, not `config.Read()`
At the top of `Execute`, before `rewriteQuery`/`sqlparser.Parse`/`typecheckNode`
(which is where `GetDatasource` → `json.Creator` → `config.FromContext` first runs
during schema inference), do:

```go
ctx = config.ContextWithConfig(ctx, &config.Config{
    Files: config.FilesConfig{
        BufferSizeBytes: 4 * 1024 * 1024,
        JSON:            config.JSONConfig{MaxLineSizeBytes: 1024 * 1024},
    },
})
```

These values match the defaults `config.Read()` falls back to when
`~/.octosql/octosql.yml` doesn't exist or doesn't set them.

**Alternative considered**: call `octosqlconfig.Read()`. Rejected — `Read()` reads
(and the `config` package's `var` initializers can create) `~/.octosql/`, coupling
kubectl-sql's behavior to a stray octosql config file a user might have from
unrelated `octosql` CLI use. A hardcoded struct keeps kubectl-sql self-contained and
the contract explicit.

### Enable `typecheckNode`'s panic recovery
`engine.go`'s `typecheckNode` wraps `node.Typecheck(ctx, env, logicalEnv)`, but its
`defer/recover` block was commented out:

```go
func typecheckNode(ctx context.Context, node logical.Node, env physical.Environment, logicalEnv logical.Environment) (_ physical.Node, _ map[string]string, outErr error) {
	// defer func() {
	// 	if r := recover(); r != nil {
	// 		outErr = fmt.Errorf("typecheck error: %v", r)
	// 	}
	// }()
	physicalNode, mapping := node.Typecheck(ctx, env, logicalEnv)
	return physicalNode, mapping, nil
}
```

`logical.(*DataSource).Typecheck` (`external/octosql/logical/logical.go:126`) calls
`env.Datasources.GetDatasource(ctx, ds.name, ds.options)` and **panics** if it returns
an error:

```go
datasource, schema, err := env.Datasources.GetDatasource(ctx, ds.name, ds.options)
if err != nil {
	panic(fmt.Errorf("couldn't create datasource: %v", err))
}
```

With the recovery commented out, this panic was unhandled and crashed the whole
`kubectl-sql` process — for "no such table" today, and for our new
`json.Creator` returning an error on a non-JSON-Lines file (the "JSON Lines format
is required" spec requirement). The sibling function `typecheckExpr` (used for
`ORDER BY`/`LIMIT` typechecking, a few lines below) already has the identical
`defer/recover` **enabled** — uncommenting the same block in `typecheckNode` makes
the two consistent and turns both cases into ordinary `error` returns
(`octosql: typecheck: typecheck error: couldn't create datasource: ...`), which
`Execute` already propagates as a non-zero exit via `cmd`'s error handling.

**Alternative considered**: leave it commented out and only handle the
non-JSON-Lines case some other way (e.g. pre-validating the file before handing it to
octosql). Rejected — duplicates `json.Creator`'s validation, and doesn't fix the
pre-existing "no such table" crash, which the new spec's "non-conforming files error
clearly" requirement would otherwise also be undermined by (a crash is not "an error
message", it's a stack trace and non-zero exit from a panic).

### Place the config-context injection at the start of `Execute`, applied unconditionally
Simpler and cheaper than detecting whether any table in the query is file-routed.
`context.WithValue` is O(1); the value is only ever read on the file-handler code
path (`json.Creator` / `DatasourceExecuting.Run`), so pure-`k8s.*` queries pay a
negligible cost and behave identically.

## Risks / Trade-offs

- **[Risk]** New transitive dependencies. Importing
  `github.com/cube2222/octosql/datasources/json` pulls in
  `github.com/cube2222/octosql/execution/files` and
  `github.com/cube2222/octosql/config`, which are not imported by any package
  currently used from `internal/adapter/sql/octosql` (`aggregates`, `functions`,
  `logical`, `optimizer`, `parser`, `physical`, `table_valued_functions`). This adds,
  via `go mod tidy`:
  - `github.com/nxadm/tail` (used by `execution/files` for `tail=true` support)
  - `github.com/adrg/xdg`, `github.com/mitchellh/go-homedir`,
    `github.com/Masterminds/semver` (used by `config` for its config-dir resolution)
  - promotes `github.com/valyala/fastjson` from `// indirect` to a direct
    requirement
  - `gopkg.in/yaml.v3` is already an indirect dependency (used by `config` for
    `octosql.yml` parsing), no new entry needed.
  → Mitigation: all are small, widely-used, permissively-licensed libraries already
  reachable from the existing `github.com/cube2222/octosql v0.13.0` requirement (no
  new module versions to audit). `go mod tidy` records them; `make lint build` and
  `go test ./...` validate nothing else breaks.

- **[Risk]** Side effect of importing `config`: its package-level `var` initializers
  (`OctosqlConfigDir`, `OctosqlCacheDir`, `OctosqlDataDir` in
  `external/octosql/config/config.go`) run `xdg.SearchConfigFile`/`homedir.Dir` and,
  on some systems, create `~/.octosql/` if no XDG config dir is found — for *every*
  kubectl-sql invocation, even pure `k8s.*` queries, regardless of the hardcoded
  config we inject (the injected config only short-circuits `FromContext`, not the
  package's own `var` initializers).
  → Mitigation: this is a one-time, idempotent, low-cost filesystem check/mkdir
  performed by a library already required by go.mod; it does not read or write any
  query-affecting state. Documented here so it isn't mistaken for an unrelated bug
  if observed (e.g. via `strace`) later.

- **[Risk]** Table-name/database-name collision: a file literally named `k8s.json`
  (or `k8s.jsonl`/`k8s.ndjson`) in the working directory would be parsed as
  `Qualifier="k8s", Name="json"` → `GetDatasource` matches the registered `"k8s"`
  Database first and calls `KubernetesDatabase.GetTable(ctx, "json", ...)`, which
  fails to resolve `"json"` as a Kubernetes resource — the file is never reached.
  → Mitigation: documented escape hatch — reference the file as `./k8s.json`
  (`Qualifier="./k8s"`, no longer matching the `"k8s"` Database name). This mirrors
  upstream octosql's own namespacing behavior (any `Databases` key collides with a
  same-named file's basename-without-extension) and is called out in README/AGENTS.md.

- **[Risk]** JSON Lines requirement: if a referenced `.json`/`.jsonl`/`.ndjson` file
  is a single pretty-printed JSON document (e.g. a top-level array or an object
  spanning multiple lines) rather than one-object-per-line, `json.Creator`'s schema
  sampling or `DatasourceExecuting.Run`'s per-line parser will return an error
  (`"expected JSON object, got '...'"` / `"couldn't parse line N"`).
  → Mitigation: this is octosql's existing, documented behavior for this datasource
  and matches the proposal's explicit "Use JSON lines format" requirement; the new
  spec and docs state the constraint plainly so it's a clear user-facing error, not a
  silent misread.

## Open Questions

None.
