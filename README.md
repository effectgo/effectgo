# effectgo

effectgo infers Effect-TS-style signatures — `Effect<A, E, R>` — for plain Go code. No rewrite into a monad: errors (E) and side effects (R) are inferred statically, cross-package, and enforced by contracts at `go vet` time.

## 30-second example

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

```
$ cd examples/layers && effectgo ./...
domain/todo.go:31:6: Normalize is declared //effectgo:pure but has effect time via Normalize → infra.Stamp → time.Now
```

## Install

Build from a clone (Go 1.26):

```
go build -o bin/effectgo ./cmd/effectgo
```

The repository is private, so the module proxy route needs `GOPRIVATE`; it was not run from this environment:

```
GOPRIVATE=github.com/faintghost/* go install github.com/faintghost/effectgo/cmd/effectgo@latest
```

## Use it

- `effectgo ./...` runs the six analyzers (effects, errset, mustuse, exhaustive, schematag, directive) with no configuration.
- `go vet -vettool=$(which effectgo) ./...` runs the same analyzers through `go vet`.
- `effectgo sig ./...` prints `Effect<A, E, R>` for each function.
- `effectgo openapi`, `http-gin`, `http-huma` and `http` check HTTP error responses against the error sets.
- `effectgo ts -name NAME ./...` writes the TypeScript client of the `rt/web` routes (effect/http-api).
- The candidate `effectgo contract` command provides one shared text or JSON report for the six static analyzers and HTTP checks. See [Contract checks](docs/guide/contract-checks.md) and [the CLI reference](docs/reference/cli.md#contract).

The stable baseline is `v0.1.0`; `v0.2.0-rc.1` adds candidate contract reporting whose report schema may change before `v0.2.0`.
Release validation: install the locked example dependencies, then run `bash scripts/release-check.sh`; see [releasing](docs/guide/releasing.md). Generated-client compilation and HTTP decoding are mandatory at this gate.

To update an application's pinned source, run `node scripts/pin-source.mjs /path/to/app` from a verified source checkout. This records a release candidate; add `--release v0.1.0` only from the clean tagged release. Applications keep their own lock and thin `scripts/effectgo-source.mjs` entry point; the verifier is copied unchanged from the framework release.

## Documentation

- [Contract checks](docs/guide/contract-checks.md): the candidate shared text/JSON report and CI gates.
- [Contract mode](docs/guide/contract-mode.md): writing new code with contracts.
- [Editor integration](docs/guide/editor.md): golangci-lint, VS Code, Neovim.
- [HTTP checks](docs/guide/http-checks.md): the Huma, Gin and Fuego/OpenAPI checks.
- [Reference](docs/reference/directives.md): [directives](docs/reference/directives.md), [stub files](docs/reference/stubs.md), [effects](docs/reference/effects.md), [error sets](docs/reference/errors.md), [analyzers](docs/reference/analyzers.md), [command line](docs/reference/cli.md).
- [Design](docs/design/architecture.md): [architecture](docs/design/architecture.md), [decisions](docs/design/decisions.md), [limitations](docs/design/limitations.md).
- [Journal](docs/journal/index.md): the development history and its measurements.
- [AGENTS.md](AGENTS.md): the entry point for coding agents, both those using effectgo and those developing it.
- The documentation site built from `docs/` with Nimbus: see [site/README.md](site/README.md).

Formerly goeffect.
