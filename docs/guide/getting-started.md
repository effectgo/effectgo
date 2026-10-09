---
title: Getting started
description: Install effectgo, run it on a Go module with no configuration, and read what it prints.
---

This page takes you from nothing installed to a first diagnostic. It assumes a Go module that builds with Go 1.26.

## Install

From a clone of the repository:

```text
go build -o bin/effectgo ./cmd/effectgo
```

Put `bin/effectgo` on your `PATH`, or use the absolute path in the commands below.

The repository is private, so the module proxy cannot serve it; the install route through the proxy needs `GOPRIVATE`:

```text
GOPRIVATE=github.com/faintghost/* go install github.com/faintghost/effectgo/cmd/effectgo@latest
```

That route was not run from the build environment. The clone route is the verified one.

## First run: no configuration

effectgo needs no configuration to start. With no flags and no environment variables, it reads the stub files shipped inside the binary and the `effectgo.stubs` file at the root of your module, if you have one. Run it from the module root:

```text
effectgo ./...
```

Two ways to run it, with the same four analyzers:

| Command | Exit status when there are diagnostics |
|---|---|
| `effectgo ./...` | 3 |
| `go vet -vettool=$(which effectgo) ./...` | 1 |

Standalone runs skip `_test.go` files by default. `-test=true` includes them. Under `go vet`, go vet decides which test variants it passes to the tool.

Diagnostics are written to stderr. Paths are absolute in the standalone run and relative to the package under `go vet`.

## Reading the output

Here is the zero-configuration run on `examples/layers`, which is a small module that declares a pure domain and calls an infrastructure package. The paths are shortened:

```sh
$ effectgo ./...
repo/repo.go:32:20: (*HTTPRepo).Get violates contract of (Repo).Get [allow db]: has effect net via (*HTTPRepo).Get → http.Get
domain/todo.go:31:6: Normalize is declared //effectgo:pure but has effect time via Normalize → infra.Stamp → time.Now
domain/todo.go:36:6: Save is declared //effectgo:package pure but has effect net via Save → infra.Fetch → http.Head
domain/todo.go:44:6: Load is declared //effectgo:package pure but has effect db via Load → repo.(Repo).Get [contract]
```

Each line has the same parts:

- **Position**: `file:line:col` of the function's name (or of the method that breaks a contract).
- **Claim**: the contract the function states, `//effectgo:pure` on `Normalize`, or the package default `//effectgo:package pure` on `Save` and `Load`. The `[allow db]` in the first line is the interface contract that `*HTTPRepo` violates.
- **Effect**: the effect that is not allowed, here `time`, `net` or `db`.
- **Witness**: the chain of calls from the function to the origin of the effect, with the last step being the library call that introduced it. `Normalize → infra.Stamp → time.Now` means that `Normalize` calls `infra.Stamp`, which calls `time.Now`.

A `[contract]` step means the effect comes through an interface contract, not through a body.

A diagnostic is a fact about the code, not a style hint. Either the contract is wrong (widen it with `//effectgo:allow`), or the code is wrong (move the call out of the pure function).

## Errors

The same run also reports error-set findings when a module declares error contracts or HTTP error maps. The `examples/todo` module has a handler whose error can reach an HTTP 500:

```sh
$ effectgo ./...   # in examples/todo
api/v3.go:53:6: handler v3Update may return domain.ErrConflict (via Service.Update → Store.checkTitle), which errmap httpres.ToHTTPError does not map → HTTP 500
```

The run prints three lines for this module; this is one of them. The message says what the handler can return, how the error reaches it, and which mapping does not cover it. The fix is to map the error in the errmap, or to accept the 500 on purpose. See [Error sets](../reference/errors.md) and [Directives](../reference/directives.md).

## Look at what is inferred

Signatures for every function, in the `Effect<A, E, R>` form:

```sh
$ effectgo sig ./...      # exit status 3 when it printed signatures
domain/service.go:42:19: Service.Update: Effect<Todo, ErrConflict | ErrInvalid | ErrNotFound, {}>
domain/todo.go:62:17: Store.modify:   Effect<Todo, ErrNotFound | param:1, {param:1}>
```

`A` is the result, `E` the error variants, `R` the effects. `param:1` is the effect of calling the function's own parameter 1. See [Analyzers](../reference/analyzers.md#signature).

Report flags print what the analyzers inferred for every function, not only the ones that violate a contract:

```text
effectgo -effects.report -errset.report -errset.witness ./...
```

See [Command line](../reference/cli.md#flags).

## Next steps

- Write new code with contracts: [Contract mode](contract-mode.md).
- Use it in your editor: [Editor integration](editor.md).
- Check your HTTP handlers: [HTTP checks](http-checks.md).
- Look up a directive or a flag: [Reference](../reference/directives.md).
- Configure stubs for code effectgo cannot see into: [Stub files](../reference/stubs.md).
