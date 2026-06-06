# ADR-003: Implementation Language — Go over Rust

**Date:** 2026-06-06  
**Status:** Accepted  
**Deciders:** Thomas Decaux

---

## Context

`kubectl-sql` is a CLI tool distributed as a `kubectl` plugin. The choice of implementation language affects development velocity, ecosystem fit, binary distribution, and long-term maintainability. Go and Rust are both credible choices for a systems-level CLI tool in the Kubernetes space.

---

## Decision

Use **Go**.

---

## Rationale

### Development velocity

Go's simplicity — a small language spec, fast compilation, and minimal boilerplate — makes it significantly faster to iterate on than Rust. Rust's ownership model and borrow checker enforce memory safety at compile time, but the cognitive overhead is substantial. For a tool where correctness is achieved primarily through tests (not memory safety invariants), Go reaches a working state faster and is easier to extend.

### Kubernetes ecosystem fit

The entire Kubernetes control plane and its tooling (`kubectl`, `client-go`, `controller-runtime`, `kubebuilder`, `helm`) is written in Go. This means:

- `k8s.io/client-go`, `k8s.io/apimachinery`, and `sigs.k8s.io/controller-runtime` are first-class Go libraries with no FFI layer
- The `kubectl` plugin contract is defined around Go conventions
- `envtest` (used for integration tests) is a Go library — running it from Rust would require cross-language bindings
- Community knowledge, examples, and operator patterns are all Go-first

Implementing the same tool in Rust would require maintaining FFI bindings or reimplementing the Kubernetes client from scratch.

### Portability

Go compiles to a statically linked binary with no runtime dependency by default (as long as CGo is avoided, which is enforced here). A single `GOARCH`/`GOOS` cross-compile produces a self-contained executable for Linux, macOS, and Windows. Rust can do the same, but the toolchain setup for cross-compilation is more involved and musl-based static linking adds friction.

### Binary size

A Go binary for `kubectl-sql` is in the 20–30 MB range (including octosql and the Kubernetes client). Rust binaries can be smaller with aggressive LTO and `opt-level = "z"`, but the difference is marginal for a developer tool installed once. The Go binary requires no shared library at runtime and is accepted by standard `kubectl` plugin distribution mechanisms without modification.

### Why not Rust

Rust's primary advantages — memory safety without a GC, zero-cost abstractions, and peak throughput — are not the binding constraints for this tool. The bottleneck is network I/O (Kubernetes API calls), not CPU. GC pauses in Go are sub-millisecond and irrelevant at the latency of a kubectl plugin. The productivity cost of Rust — particularly for Kubernetes API integration — outweighs the performance and safety benefits for this use case.

---

## Consequences

- CGo must be avoided to preserve static linking; this is why DuckDB was ruled out as a SQL engine candidate (see [ADR-002](adr-002-octosql-sql-engine.md))
- The `golangci-lint` linter is used to enforce code quality in place of Rust's stricter compile-time guarantees
- All Kubernetes client libraries are consumed directly from their canonical Go modules with no translation layer
