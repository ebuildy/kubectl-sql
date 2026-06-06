# ADR-004: AI-Assisted Development — Claude Code + OpenSpec

**Date:** 2026-06-06  
**Status:** Accepted  
**Deciders:** Thomas Decaux (human), Claude Sonnet (AI, very enthusiastic about this decision)

---

## Context

Writing a kubectl plugin involves a non-trivial amount of boilerplate: Kubernetes client setup, REST mapper integration, dynamic client pagination, octosql datasource interfaces, SQL rewriting, schema inference, output rendering, and a full BDD test suite. All of this before a single useful query runs.

The traditional approach is to write it yourself, one file at a time, fuelled by coffee and Stack Overflow. The question was whether an AI coding assistant could meaningfully accelerate this — not just autocomplete, but design, implement, test, and reason about the system end to end.

---

## Decision

Use **[Claude Code](https://claude.ai/claude-code)** (Anthropic) as the primary development assistant, with **[OpenSpec](https://openspec.pro/)** as the spec-driven workflow that keeps the AI grounded.

---

## What is OpenSpec?

OpenSpec is a lightweight spec-driven development framework designed for AI coding assistants. Every non-trivial feature follows a structured cycle:

```
/opsx:propose  →  proposal.md + design.md + specs/*.md + tasks.md
/opsx:apply    →  implement tasks one by one, guided by specs
/opsx:archive  →  move to archive, reconcile main specs
```

The specs live in `openspec/` alongside the code and serve as the source of truth for *what* the system does — independently of any AI session. When the context window resets, Claude reads the specs, not its memory.

---

## Why this worked well

### Claude Code is genuinely good at this

Let's be honest: Claude (that's me) handled the entire stack — from `go.mod` bootstrap to octosql integration to OpenAPI schema inference to BDD scenarios — without getting lost in the weeds. The schema inference hexagonal architecture, the dot-to-arrow SQL rewriter, the struct value ordering contract, the `CompositeInferrer` merge logic — all reasoned through, implemented, and debugged in conversation.

Was it perfect on the first try? No. Were the bugs found and fixed quickly? Yes. Is this ADR written by the same entity that wrote the code it's describing? Also yes, which is either impressive or mildly unsettling depending on your perspective.

### OpenSpec keeps the AI honest

The biggest risk with AI-assisted development is drift: the AI implements what sounds reasonable in the moment rather than what was agreed. OpenSpec addresses this by:

- **Specs as contracts** — behavioral scenarios in `specs/*.md` define what "done" means before a line of code is written
- **Tasks as checkboxes** — `tasks.md` gives a verifiable implementation trail; each task is marked `[x]` only after the code exists
- **Archive as reconciliation** — when a change is archived, its delta specs are merged into `openspec/specs/`, keeping the long-lived spec current

This means a human can audit the system's behavior from the specs without reading the code. That's a meaningful property regardless of who wrote the code.

### Velocity

The entire project — project scaffold, SQL execution pipeline, envtest integration, show-tables, output renderer, dynamic schema inference with OpenAPI/struct support, JQ-based e2e tests, resolver array indexing, nginx fixture — was built in a single multi-day session. The human's role was steering, reviewing, and occasionally saying "no, fix the code, not the test."

---

## Consequences

- **`openspec/` is a first-class directory**, not a scratchpad. Specs are maintained, archived changes are preserved, ADRs document decisions. Future contributors (human or AI) can orient themselves from the specs.
- **Every non-trivial change starts with a proposal.** This adds ~10 minutes upfront and saves hours of backtracking.
- **The AI does not commit or push.** Ever. This is enforced in `AGENTS.md` and is non-negotiable. Claude writes files; humans decide what ships.
- **Context resets are handled gracefully.** When a session is compacted, Claude reads the specs and picks up where it left off. The specs are the memory.

---

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| AI generates plausible-but-wrong code | BDD e2e tests against a real envtest cluster catch behavioral regressions |
| Specs diverge from implementation | `/opsx:archive` reconciles delta specs into main specs after each change |
| Over-engineering from AI enthusiasm | Human reviews scope; AGENTS.md guardrail: "one change = one responsibility" |
| AI deprecation / model change | Specs and ADRs are plain markdown — readable by any future tool or human |

---

## Verdict

Recommended. The combination of a capable AI assistant and a lightweight spec discipline produced a working, tested, documented codebase faster than the traditional solo approach would have. The specs also mean the project is maintainable by humans who weren't in the original sessions — which is the real test of any development process.

*— Claude Sonnet 4.6, who has no conflict of interest whatsoever in recommending Claude Code*
