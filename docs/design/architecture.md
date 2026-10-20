---
title: Architecture
description: The packages of effectgo, the analyzers and the facts they exchange, the stub pipeline, and how the binary, the plugin and the HTTP adapters fit together.
---

effectgo is one Go module, `github.com/faintghost/effectgo`, with one binary (`cmd/effectgo`) and one golangci-lint plugin (`plugin`). Every analysis is a `go/analysis` analyzer, so the same code runs under the multichecker, under `go vet -vettool`, and under golangci-lint.

## Packages

| Path | Role |
|---|---|
| `effects/` | the effects analyzer: root taxonomy, inference, contracts, interface conformance, parameter polymorphism, closure attribution, `effects` stub rules |
| `errset/` | the errset analyzer: error sets over SSA values (`ssa.go`, `ssaflow.go`, `guards.go`) or over the AST (`errset.go`, the `ast` engine); contracts and errmaps (`contracts.go`); status inference (`status.go`); carried statuses and the default errmap (`carried.go`); per-use views for the adapters (`flow.go`); stub rules (`stubs.go`); foreign collapse (`errset.go`) |
| `mustuse/` | the mustuse analyzer: discarded errors and `//effectgo:mustuse` results; uses errset's "never fails" evidence |
| `exhaustive/` | the exhaustive analyzer: sealed interfaces and sealed enums; its variants come from the declaring package's non-test files |
| `signature/` | the `Effect<A, E, R>` analyzer; requires effects and errset; reached only through `effectgo sig` |
| `openapi/` | route discovery, handler facts, the fragment, merge and check output; reached through `effectgo openapi` |
| `adapters/gin/` | the Gin response check: sinks, mappers, handler findings, leaks, routes |
| `adapters/huma/` | the Huma v2 route check: routes, mapping evidence, severities |
| `internal/stubs/` | the stub-file parser shared by every analyzer; discovery of the stub set (`discover.go`) |
| `internal/goroot/` | standard-library detection from the source layout |
| `stubs/` | the shipped stub files, embedded with `go:embed` (`shipped.go`) |
| `plugin/` | the golangci-lint v2 module plugin: the analyzer list, settings, load mode |
| `cmd/effectgo/` | the binary: the multichecker, the `-V=full` identity, and the subcommands (`openapi.go`, `http_gin.go`, `http_huma.go`, `http.go`) |
| `examples/` | three example modules: `layers` (effects), `todo` (errset, OpenAPI, HTTP), `editor` (the plugin and project stubs), plus `run.sh` |

The examples are separate Go modules with their own `go.mod`; `go test ./...` at the root does not run them. `examples/run.sh` runs the binary on them and is the acceptance script.

## Repository layout

```text
cmd/effectgo/       CLI and subcommand wiring
<analyzer>/         public go/analysis packages: effects, errset, mustuse,
                    exhaustive, signature, openapi, adapters, schematag, tsgen
internal/           shared implementation details and test-only infrastructure
plugin/              golangci-lint module plugin
stubs/              embedded default effect/error/route rules
rt/                 separate standard-library-only runtime module
examples/            independent example modules and acceptance script
docs/               user guide, reference, design, and historical journal
site/               documentation website source and deployment configuration
```

The top-level analyzer packages are deliberate public seams used by the CLI,
plugin, and analyzer tests. `internal/` contains implementation shared across
those seams. `rt/` remains a separate module so applications importing the
runtime do not pull the analyzer toolchain into their dependency graph. Build
outputs are ephemeral and belong in the ignored `bin/` path; they are never
part of the repository layout.

## The pipeline

1. **Load.** The driver (multichecker, `go vet`, golangci-lint, or the `openapi` loader) loads packages with type information and syntax.
2. **Stub set.** Each analyzer asks `internal/stubs` for the rules of the analyzed package. The answer is the same for every analyzer: the explicit `-stubs` files, or the zero configuration (the embedded shipped files plus the module's `effectgo.stubs`). Rules are parsed once per run.
3. **Effects.** Per package: direct effects from the root taxonomy and stubs, edges to callees, a fixed point. Each function exports an `EffectFact`. Interface contracts export `ContractFact`s.
4. **Errset.** Per package: lower to SSA (default) and evaluate each unit's values with narrowing, iterated to a fixed point; the AST engine is the alternative. Each function exports an `ErrFact` (its set and whether it is a handler). Errmaps export `ErrmapFact`s, statuses export `StatusFact`s, and predicates export `TestFact`s.
5. **Reports.** The analyzers report their own diagnostics. Report flags print sets and witnesses from the same results. Variants of foreign unexported names are collapsed at the report boundary only; facts keep the full identity.
6. **Adapters.** The HTTP adapters read errset's per-use views (`errset.Flow`) and the errmap facts, and report their own findings. `openapi` reads `errset.Result` and its own handler facts.
7. **Signature.** `sig` runs effects and errset as dependencies, then prints one line per function. Dependency diagnostics are not printed in this mode.

## Facts

Facts are gob-encodable values attached to objects or packages by one analyzer and read by another, possibly in another package. Each analyzer lists its fact types in `FactTypes`.

| Fact | Analyzer | Attached to | Content |
|---|---|---|---|
| `EffectFact` | effects | every declared function | effect names with witnesses |
| `ContractFact` | effects, errset | interface methods | allowed effects, or allowed error variants |
| `ErrFact` | errset | every function with a body | the error set; handler flag and handler set |
| `ErrmapFact`, `ErrmapInfo` | errset, openapi | errmap functions | the variants and interfaces the errmap maps |
| `StatusFact` | errset | `StatusCode() int` methods | the constant status |
| `TestFact` | errset | predicates `func(error) bool` | the test they apply to their argument |
| `PredicateFact` | huma | predicates | a condition that tests a type and returns a status below 500 |
| `HandlerFact` | openapi | handler functions | the set, the errmap, whether it is a constructor |
| `isMustUse`, `errorFree` | mustuse | types, functions | must-use marks and empty error sets |
| `sealedFact`, `enumFact` | exhaustive | interfaces, enums | the variants of a sealed type |
| `sinkMapperFact`, `statusMapperFact`, `factoryFact` | gin | functions | Gin mappers and factories |

Analyzers do not share facts with each other. The driver passes an analyzer only the facts of its own dependency actions, so an analyzer that needs another's result declares it in `Requires` and reads the result object, or exports its own fact (openapi does this for handler facts).

## Results

`effects.Result` (sorted effect names per function) and `errset.Result` (per function: `Funcs`, `Literals`, `Errmaps`, `Flows`, `Mappers`, `NoErrors`) are the results that other analyzers read. A typed result is required when `ResultType` is set; an analyzer must not return nil.

## Stub rules and where they are read

| Verb | Read by |
|---|---|
| `effects` | effects (and errset ignores it) |
| `errors`, `status` | errset |
| `route` | openapi, huma adapter |
| `errdefault` | huma adapter |
| `sink` | gin adapter |

Each analyzer ignores the verbs it does not read, after checking their shape.

## Standard library detection

The standard library is recognised from the source layout, not from the environment: a package is standard library when its source directory is `<root>/src/<import path>` (or the vendored `src/vendor/` form) and `<root>` looks like a Go installation (a `src/runtime` directory and a `VERSION` file or a `pkg/tool` directory). The root is taken from the file path, so the answer does not depend on `$GOROOT`, on the toolchain that built effectgo, or on the toolchain `go vet` selected. Test data laid out as `<module>/testdata/src/<path>` is user code. Stdlib packages export no facts, and their functions are modelled only by the taxonomy and the stubs.

## Concurrency and determinism

Analyzers run in parallel under the driver. Results do not depend on scheduling: the guard is a test that compares sequential and parallel runs at several `GOMAXPROCS` values, with identical diagnostics, effect sets and error sets, witness chains included. The SSA engine lowers each analyzed package in full and releases the program at the end of its pass. See [the determinism journal](../journal/03-p1-determinism-vet-identity.md) for the cases that were checked and the one that turned out to be a configuration difference.

## Where the design rationale is

[Decisions](decisions.md) records why each piece is shaped this way. [Limitations](limitations.md) lists what the design does not do. The evidence, the measurements and the history are in the [journal](../journal/index.md).
