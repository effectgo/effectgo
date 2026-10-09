---
title: effectgo
description: effectgo infers Effect-TS-style signatures for plain Go code, and enforces contracts on errors and side effects at go vet time.
---

effectgo infers Effect-TS-style signatures, `Effect<A, E, R>`, for plain Go code. It does not require a rewrite into a monad. The error set (`E`) and the side effects (`R`) of each function are inferred statically, across packages, and a contract that you write in a comment is enforced at `go vet` time.

## The 30-second example

`examples/layers/domain/todo.go` declares a function pure, but one of its callees reaches the clock two hops down:

```go
// Normalize trims a title and appends a stamp. Declared pure, but the stamp
// reaches time.Now through infra.Stamp: a two-hop leak.
//
//effectgo:pure
func Normalize(s string) string {
	return strings.TrimSpace(s) + infra.Stamp()
}
```

`infra.Stamp` calls `time.Now().Format(...)`. Run effectgo on the module with no configuration:

```sh
$ effectgo ./...
domain/todo.go:31:6: Normalize is declared //effectgo:pure but has effect time via Normalize → infra.Stamp → time.Now
```

The message is the witness chain: each step is a call that the analyzer followed, and the last step introduced the effect.

## What it checks

- **Effects.** Which side effects a function can perform: `net`, `fs`, `db`, `time`, `rand`, `env`, `exec`, `stdio`, `global`, and more. Contracts are `//effectgo:pure` and `//effectgo:allow`, on a function, a package, or an interface.
- **Errors.** Which error values a function can return, as variants: sentinels, types, opaque errors. A handler's errors are checked against the HTTP mapping that the project declares. Error contracts are `//effectgo:errors`.
- **Discarded results.** An error or a `//effectgo:mustuse` value that a statement drops.
- **Exhaustive switches.** A switch over a sealed interface or a marked enum that misses a variant.

Each diagnostic is reported only when the code is provably at risk. A rule that cannot be proved stays silent.

## Where to go next

| If you want to | Read |
|---|---|
| install it and run it on your module | [Getting started](guide/getting-started.md) |
| write new code with contracts | [Contract mode](guide/contract-mode.md) |
| run it in VS Code or Neovim | [Editor integration](guide/editor.md) |
| check HTTP handlers and OpenAPI | [HTTP checks](guide/http-checks.md) |
| look up a directive, a flag or a stub verb | [Reference](reference/directives.md) |
| understand the design | [Architecture](design/architecture.md), [Decisions](design/decisions.md) |
| see what it does not do | [Limitations](design/limitations.md), and the [backlog](design/backlog.md) of known gaps |
| see what a stub file covers | [Stub coverage](reference/stub-coverage.md) |
| read the development history | [Journal](journal/index.md) |

The reference is in six pages: [directives](reference/directives.md), [stub files](reference/stubs.md), [effects](reference/effects.md), [error sets](reference/errors.md), [analyzers](reference/analyzers.md), and the [command line](reference/cli.md).

## Positioning

effectgo does not change how Go code is written. It runs as a `go vet` tool, standalone, or in golangci-lint, and it does not replace the standard vet checks. Related tools are discussed in [Decisions](design/decisions.md#positioning-against-related-tools).
