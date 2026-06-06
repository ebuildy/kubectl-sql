# ADR-006: REPL Line-Editing Library — `chzyer/readline` vs. Rolling Our Own

**Date:** 2026-06-06  
**Status:** Accepted  
**Deciders:** Thomas Decaux

---

## Context

The `sql-repl` change adds an interactive Read-Eval-Print-Loop: a `kuery> ` prompt that
reads SQL, executes it, prints results, and loops until the user quits. A usable REPL needs
more than `bufio.Scanner`:

- A printed, editable prompt
- Cursor movement and in-line editing (left/right, backspace, kill-line)
- Command history navigable with the up/down arrows
- Correct raw-mode terminal handling and clean restore on exit / Ctrl-C
- TTY detection so piped input falls back to plain line reads

Implementing this from scratch means raw-mode `termios` handling, ANSI escape-sequence
parsing for arrow keys, a history ring buffer, and platform-specific terminal restore logic
— several hundred lines of fiddly, well-trodden code that every shell has already solved.

Before writing our own, the policy is: **search for an existing library with real adoption
(≥ 500 GitHub stars) and recent activity (commits after 2024). If one exists, use it. If
not, build our own.**

---

## Decision

Use **[`github.com/chzyer/readline`](https://github.com/chzyer/readline)** for the
interactive REPL line editing and history.

We do **not** build our own raw-mode shell.

---

## Survey of Candidates

Evaluated June 2026. Bar: ≥ 500 stars **and** commits after 2024, pure Go, not archived.

| Library | Stars | Last push | Meets bar? | Notes |
|---|---|---|---|---|
| `chzyer/readline` | **2,293** | **2025-06** | ✅ Yes | Pure Go GNU-readline reimplementation. Used by 25k+ repos. The de-facto standard. |
| `reeflective/readline` | 142 | 2026-06 | ❌ stars | Modern, `.inputrc` support, very active — but well under 500 stars. |
| `ergochat/readline` | 52 | 2025-01 | ❌ stars | Maintained fork of chzyer. Too niche. |
| `lmorg/readline` | ~39 | 2025-02 | ❌ stars | "Batteries included" CLI input, low adoption. |
| roll our own (`x/term` raw mode) | — | — | — | Fallback only if no library qualified. |

`chzyer/readline` is the only candidate that clears **both** thresholds: 2,293 stars
(well above 500) and a most-recent push of **June 2025** (after the 2024 floor). It is not
archived. Therefore, per policy, we adopt it rather than writing our own.

---

## Rationale

### A qualifying library exists, so we use it

The policy is explicit: build our own only if no project with ≥ 500 stars and post-2024
commits exists. `chzyer/readline` clears both bars comfortably, so the "roll our own"
branch does not apply.

### It is the ecosystem default

`chzyer/readline` is used by 25k+ downstream repositories and is the line-editing layer
behind many Go CLIs. Picking it means battle-tested edge-case handling (multi-byte input,
window resize, Ctrl-C/Ctrl-D semantics, Windows console support) that we would otherwise
reimplement and under-test.

### Maintenance cadence is acceptable for our use

The library is mature and stable rather than rapidly evolving — releases are infrequent
because the problem (GNU-readline behavior) is essentially solved. The June 2025 push
confirms it is not abandoned. For a REPL that needs prompt + history + editing and nothing
exotic, a stable dependency is a feature, not a risk.

### Pure Go, single dependency

It pulls in only `golang.org/x/sys`, which is already in our module graph via client-go.
No cgo, consistent with [ADR-003](adr-003-go-versus-rust.md)'s portability goal — the
single static binary story is preserved.

---

## Consequences

- **New dependency** `github.com/chzyer/readline` added to `go.mod`, justified here and in
  the `sql-repl` change's `design.md`.
- **In-memory history only in v1** — `readline` supports a history file, but we deliberately
  scope persistent history out of v1 (see `sql-repl` design Non-Goals).
- **Non-TTY path is separate** — `readline` targets interactive terminals; piped stdin is
  handled by an explicit `term.IsTerminal` check that routes to a plain `bufio.Scanner`
  batch loop, bypassing readline entirely.
- **If `chzyer/readline` is later abandoned**, `reeflective/readline` and
  `ergochat/readline` are drop-in-ish migration targets; the REPL isolates the dependency
  behind `internal/repl/` so a swap touches one package.

---

## If No Library Had Qualified

Had no project met the ≥ 500 stars + post-2024 bar, we would have built our own REPL shell
in `internal/repl/` on top of `golang.org/x/term`:

- `term.MakeRaw` / `term.Restore` around the session
- A hand-written read loop parsing ANSI sequences for arrows and editing keys
- An in-memory history ring buffer
- Explicit Ctrl-C / Ctrl-D handling and terminal restore on every exit path

This was avoided precisely because a well-adopted, actively-pushed library exists.
