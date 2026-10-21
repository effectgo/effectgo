---
title: Getting started
description: Install effectgo, run it on a Go module with no configuration, and read what it prints.
---

This page takes you from nothing installed to a first diagnostic. It assumes a Go module that builds with Go 1.26.

## Install

The repository is private, so the module proxy cannot serve it. Build from a clone, the verified route:

```text
git clone <the repository URL> effectgo
cd effectgo
go install ./cmd/effectgo
```

`go install` puts the binary in `$GOBIN`, or in `$GOPATH/bin` when `GOBIN` is not set (verified with `GOBIN` pointed at a scratch directory). Make sure that directory is on your `PATH`. If you do not want it installed, build a local copy instead:

```text
go build -o bin/effectgo ./cmd/effectgo
```

To install from the module path without a clone, the module proxy must be bypassed for the private path. Set `GOPRIVATE` and let the go command fetch the module directly, with git credentials that can read the repository:

```text
GOPRIVATE=github.com/faintghost/* go install github.com/faintghost/effectgo/cmd/effectgo@latest
```

That route was not run from the build environment, because the repository is not reachable from there. The clone route is the verified one.

If `go build` fails with an error about version control information (the module sits below a directory that git cannot read), add `-buildvcs=false`: `go build -buildvcs=false ./...`. `go vet` and `go test` are not affected. This was seen while evaluating effectgo on a service in a nested directory.

## First run: no configuration

effectgo needs no configuration to start. With no flags and no environment variables, it reads the stub files shipped inside the binary and the `effectgo.stubs` file at the root of your module, if you have one. Run it from the module root:

```text
effectgo ./...
```

Two ways to run it, with the same six analyzers:

| Command | Exit status when there are diagnostics |
|---|---|
| `effectgo ./...` | 3 |
| `go vet -vettool=$(which effectgo) ./...` | 1 |

Diagnostics are written to stderr. Paths are absolute in the standalone run and relative to the package under `go vet`.

## A clean run is not a full run

The standalone run ends with a summary line on stderr. It says how many packages it analysed, how many test variants, and how many findings it printed:

```text
effectgo: 1 package analysed (1 test variant), 0 findings
```

A clean run therefore shows that the package was analysed. The analyzers ran without an error and found nothing they could prove. To see the signatures of a package, print them:

```text
effectgo sig ./domain     # one Effect<A, E, R> line per function; exit status 3 when it printed
```

The standalone run analyses `_test.go` files by default, as `go vet` does: a test function with `//effectgo:pure` that calls `time.Now` is reported by both commands (`-test=false` skips the test files). See [Command line](../reference/cli.md#test-files) and [the summary line](../reference/cli.md#summary-line).

Note that `effectgo sig` exits with 3 when it succeeds, because each signature is printed as a diagnostic. A script must read that as success.

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

The `effectgo contract` command released in `v0.2.0` combines the six native analyzers with the Gin and Huma checks and emits a shared text or JSON report. Native diagnostics always use exit 3; the shared report uses exit 1 for load errors and exit 2 for usage errors. See [Contract checks](contract-checks.md).

## Next steps

- Write new code with contracts: [Contract mode](contract-mode.md).
- Use it in your editor: [Editor integration](editor.md).
- Check your HTTP handlers: [HTTP checks](http-checks.md).
- Look up a directive or a flag: [Reference](../reference/directives.md).
- Configure stubs for code effectgo cannot see into: [Stub files](../reference/stubs.md).
