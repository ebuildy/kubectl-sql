## ADDED Requirements

### Requirement: DELETE statement grammar
The query string SHALL accept a `DELETE [FROM] <resource> [WHERE <expr>]` statement, where
`<resource>` is any resolvable resource name (plural, short, or kind) and `<expr>` is a WHERE
expression with the same syntax and semantics as a SELECT WHERE clause. The `FROM` keyword
SHALL be optional. The statement SHALL be recognised case-insensitively and intercepted before
the SELECT (octosql) pipeline. A `DELETE` with no resource SHALL be a parse error and exit 1.

#### Scenario: DELETE with WHERE filter
- **WHEN** the user runs `kubectl-sql "DELETE pod WHERE status->phase = 'Pending'"`
- **THEN** the deletion set is resolved as the pods matching `status->phase = 'Pending'`, using
  the same WHERE semantics as `SELECT name, namespace FROM pods WHERE status->phase = 'Pending'`

#### Scenario: DELETE FROM with optional keyword
- **WHEN** the user runs `kubectl-sql "DELETE FROM pods WHERE name = 'nginx'"`
- **THEN** the statement parses identically to the `FROM`-less form and targets the `nginx` pod

#### Scenario: DELETE without resource is a parse error
- **WHEN** the user runs `kubectl-sql "DELETE WHERE x = 1"`
- **THEN** the command prints a parse error to stderr and exits 1

### Requirement: Delete options via MySQL-style hint comment
A `DELETE` statement SHALL accept `kubectl delete`-style options in a MySQL-style hint comment
placed immediately after the `DELETE` keyword, of the form `DELETE /* hints */ [FROM] resource`.
The hints SHALL be a comma-separated list of tokens, each either a bare flag or a `key=value`
pair. The recognised hints SHALL be: `force` (immediate deletion, grace period 0),
`grace-period=<n>` (deletion grace period in seconds), and `cascade=background|foreground|orphan`
(propagation policy). Hint names SHALL be matched case-insensitively. An unrecognised hint or a
malformed value SHALL be a parse error and exit 1. The parsed options SHALL be applied to every
object deleted by the statement.

#### Scenario: Force and grace period hints are applied
- **WHEN** the user runs `kubectl-sql "DELETE /* force, grace-period=0 */ FROM pod WHERE status->phase = 'Pending'"`
- **THEN** each matched pod is deleted with grace period 0 (force/immediate deletion)

#### Scenario: Cascade propagation hint is applied
- **WHEN** the user runs `kubectl-sql "DELETE /* cascade=orphan */ deployment WHERE name = 'web'"`
- **THEN** the deployment is deleted with the `Orphan` propagation policy

#### Scenario: No hint comment uses cluster defaults
- **WHEN** a DELETE statement has no hint comment
- **THEN** objects are deleted with default delete options (no grace-period or propagation override)

#### Scenario: Unknown hint is a parse error
- **WHEN** the user runs `kubectl-sql "DELETE /* bogus_option */ FROM pods"`
- **THEN** the command prints a parse error naming the unrecognised hint and exits 1

### Requirement: Deletion set is previewed before any delete
Before deleting anything, the command SHALL resolve the full set of matching objects and print
each object's `namespace` and `name` (and a total count) to the user. For every matched object
the command SHALL also print the exact equivalent `kubectl delete` command that will be executed
if the user confirms, including the resource kind, object name, the `-n/--namespace` flag (for
namespaced objects), and the flags derived from the parsed delete-option hints (`--force`,
`--grace-period=<n>`, `--cascade=<policy>`). The printed command SHALL match what is actually
sent to the cluster (same target, same options). No object SHALL be deleted until after this
preview is shown and confirmation is obtained.

#### Scenario: Preview lists namespace and name
- **WHEN** a DELETE matches three pods
- **THEN** the command prints, for each of the three pods, its namespace and name and the
  equivalent `kubectl delete` command, plus the total count, before prompting

#### Scenario: Preview shows the exact kubectl delete command with options
- **WHEN** the user runs `kubectl-sql "DELETE /* force, grace-period=0 */ pod WHERE name = 'nginx'"` and one pod `nginx` in namespace `default` matches
- **THEN** the preview prints the line `kubectl delete pod nginx -n default --force --grace-period=0`, which is exactly the operation that will run on confirmation

#### Scenario: Empty deletion set is a no-op
- **WHEN** a DELETE matches no objects
- **THEN** the command prints that nothing matched, deletes nothing, does not prompt, and exits 0

### Requirement: Interactive confirmation is required before deletion
On an interactive (TTY) session the command SHALL prompt the user to confirm the deletion after
the preview. The default answer SHALL be "no": pressing Enter or answering anything other than
an explicit yes SHALL abort with no objects deleted and exit 0.

#### Scenario: User confirms
- **WHEN** the preview is shown and the user answers `yes`
- **THEN** the command proceeds to delete every previewed object

#### Scenario: User declines
- **WHEN** the preview is shown and the user answers `no` (or presses Enter)
- **THEN** the command deletes nothing, prints that the deletion was cancelled, and exits 0

### Requirement: --yes flag skips the prompt and is required when non-interactive
A `--yes` / `-y` flag SHALL skip the interactive confirmation and proceed directly to deletion
after the preview. When the session is non-interactive (stdin is not a TTY, e.g. piped or batch
mode) and `--yes` is not set, the command SHALL refuse to delete, print guidance to pass
`--yes`, and exit 1.

#### Scenario: --yes proceeds without prompting
- **WHEN** the user runs `kubectl-sql -y "DELETE pod WHERE status->phase = 'Pending'"`
- **THEN** the preview is printed and the matched pods are deleted without an interactive prompt

#### Scenario: Non-interactive without --yes is refused
- **WHEN** a DELETE is run with piped stdin and no `--yes` flag
- **THEN** the command prints that `--yes` is required for non-interactive deletion and exits 1

### Requirement: Deletions run in parallel, bounded to 10 concurrent
After confirmation, the command SHALL delete the matched objects concurrently with at most 10
deletions in flight at any time, since a single Kubernetes delete can take seconds. Every matched
object SHALL be attempted even if some fail. The command SHALL wait for all deletions to complete
before reporting.

#### Scenario: Large deletion set runs with bounded concurrency
- **WHEN** a DELETE matches 50 objects
- **THEN** the command issues the deletions concurrently with no more than 10 in flight at once and waits for all 50 to finish

### Requirement: A progress bar is shown during deletion outside the REPL
When a `DELETE` runs as a one-shot CLI invocation on an interactive terminal, the command SHALL
display a live progress bar that advances as each object is deleted, with its total set to the
number of matched objects. The progress bar SHALL be suppressed in the REPL and when the output is
not an interactive terminal; in those cases deletion behaves identically and only the bar is
omitted. The end-of-run per-object status summary SHALL be printed in all cases.

#### Scenario: One-shot DELETE shows a progress bar
- **WHEN** the user runs `kubectl-sql -y "DELETE pod WHERE status->phase = 'Pending'"` on a TTY and several pods match
- **THEN** a progress bar is displayed and advances as deletions complete, followed by the per-object status summary

#### Scenario: REPL DELETE shows no progress bar
- **WHEN** the same DELETE is run from the interactive REPL
- **THEN** no progress bar is rendered, deletions still run, and the per-object status summary is printed

### Requirement: Deletion results and exit codes
After all deletions complete, the command SHALL report a per-object result (deleted, or failed
with the reason) for every matched object, printed once at the end in the preview order. If every
delete succeeds the command SHALL exit 0. If any delete fails with a Kubernetes API error the
command SHALL exit 2. The `--namespace` flag SHALL scope which objects are matched and deleted.

#### Scenario: All deletes succeed
- **WHEN** every matched object is deleted without error
- **THEN** the command prints a per-object status table and a deleted-count summary and exits 0

#### Scenario: A delete fails
- **WHEN** at least one matched object fails to delete (e.g. forbidden by RBAC)
- **THEN** after all deletions settle the command reports which objects succeeded and which failed
  (with reasons), and exits 2

### Requirement: DELETE is rejected under --watch
A `DELETE` statement combined with `--watch` / `-w` SHALL be rejected before any object is
resolved or removed. Watch re-executes the query on a polling interval, which is incompatible
with a one-shot, confirmed mutation. The command SHALL print an error explaining the conflict and
exit 1.

#### Scenario: DELETE with --watch is refused
- **WHEN** the user runs `kubectl-sql --watch "DELETE pod WHERE status->phase = 'Pending'"`
- **THEN** the command prints that DELETE cannot be used with `--watch`, deletes nothing, and exits 1

### Requirement: --dry-run previews the deletion without mutating
When `--dry-run` is set, a `DELETE` statement SHALL resolve and print the deletion-set preview
(including the per-object `kubectl delete` command lines) and then exit without prompting for
confirmation and without deleting any object. The exit code SHALL be 0.

#### Scenario: --dry-run prints the plan and deletes nothing
- **WHEN** the user runs `kubectl-sql --dry-run "DELETE /* force */ pod WHERE status->phase = 'Pending'"`
- **THEN** the command prints the preview and the `kubectl delete ... --force` lines, prompts nothing, deletes nothing, and exits 0

### Requirement: DELETE works inside the REPL
A `DELETE` statement entered at the interactive REPL SHALL be routed to the mutator adapter and
follow the same preview + confirmation flow as a one-shot invocation. The confirmation prompt
SHALL read the user's answer from the REPL's interactive input.

#### Scenario: DELETE from the REPL prompts and deletes on confirmation
- **WHEN** the user types `DELETE pod WHERE name = 'nginx'` at the REPL prompt and answers `yes`
- **THEN** the preview is shown, the answer is read from the REPL input, and the matched pod is deleted
