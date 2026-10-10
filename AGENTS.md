# AGENTS.md

Entry point for coding agents. Two readers are covered: an agent using effectgo in a Go project, and an agent changing effectgo itself. The human-readable documentation is in [docs/](docs/index.md); this file points into it and does not repeat it.

## Part 1: using effectgo in a Go project

effectgo infers the errors (E) and side effects (R) of each function and checks them against contracts written as `//effectgo:` comments. Full reference: [docs/reference/](docs/reference/directives.md). Workflow for new code: [docs/guide/contract-mode.md](docs/guide/contract-mode.md).

### Directive cheat-sheet

| Directive | Where | Meaning |
|---|---|---|
| `//effectgo:pure` | function, interface method or type | no effects |
| `//effectgo:allow net,db` | function, interface method or type | only these effects (`unknown` only if listed) |
| `//effectgo:package pure` / `allow …` | package doc comment | default contract for the package |
| `//effectgo:errors A, pkg.B, new, ?, iface:pkg.I` | function, interface method or type | only these error variants (`new` = fresh, `?` = opaque) |
| `//effectgo:errmap` | `func(error) error` | declares an error-to-HTTP map |
| `//effectgo:handler M` | HTTP handler or handler constructor | every error must be mapped by errmap `M` |
| `//effectgo:mustuse` | named type | discarding a value of this type is reported |
| `//effectgo:sealed` | interface with an unexported method, or named int/string type | switches must cover every variant |

Effect names: `net fs exec env time rand stdio db spawn panic exit global unknown`.

### How to run it

- `effectgo ./...`: the five analyzers, zero configuration. Exit 3 means diagnostics.
- `go vet -vettool=$(which effectgo) ./...`: the same through go vet. Exit 1 means diagnostics.
- `effectgo sig ./...`: one `Effect<A, E, R>` line per function (exit 3 when printed; the lines go to stderr).
- Report flags: `-effects.report`, `-effects.witness`, `-errset.report`, `-errset.witness`, `-errset.statuses`. They print information, not diagnostics. `-effects.all` and `-errset.all` print the violations that the root rule hides.
- HTTP checks: `effectgo http ./...`, `http-gin`, `http-huma`, and `openapi -check spec.json ./...` (exit 1 on a missing code). See [docs/guide/http-checks.md](docs/guide/http-checks.md).
- Stub files: the zero configuration reads the shipped stubs and the module's `effectgo.stubs`. `-stubs=…` or `$EFFECTGO_STUBS` replace it entirely. Stub syntax: [docs/reference/stubs.md](docs/reference/stubs.md). What the shipped stubs cover, target by target: [docs/reference/stub-coverage.md](docs/reference/stub-coverage.md).
- Test files are analyzed by default, as under `go vet`; `-test=false` skips them. The standalone run ends with a summary line on stderr (`effectgo: N packages analysed (M test variants), K findings`).

### How to read a diagnostic

- `F is declared //effectgo:pure but has effect time via F → g → time.Now`: `F` breaks its contract; the chain is the call path to the source of the effect.
- `F may return X (via …), not in its //effectgo:errors contract`: an error variant the contract does not allow.
- A function whose violation is only the consequence of a callee's violation is not reported: the callee is, and fixing it fixes the caller. The summary line counts the hidden ones; `-effects.all` and `-errset.all` print them. See [the root rule](docs/reference/analyzers.md#root-rule).
- `handler H may return X (via …), which errmap M does not map → HTTP 500`: the errmap has no case for `X`, so the response is 500.
- `//effectgo:K is not attached to a function, interface or type declaration and is not read`, `effectgo:K on F checks nothing: …`: the directive is where no analyzer reads it, so it has no effect. Move it to the declaration it describes. See [Placement rules](docs/reference/directives.md#placement-rules-that-are-easy-to-miss).
- effectgo does not check data races, goroutine leaks, deadlocks, cancellation or timing; use `go test -race` and tests for those. See [What effectgo does not check](docs/guide/what-it-does-not-check.md).
- In error sets: `?` is an error of unknown origin; `new@pkg.F` is a fresh error created in `F` (not matchable by `errors.Is`); `param:i` is the error of the function's parameter `i`; `unknown` in effects is a call the analysis cannot resolve.
- `(none)` in an error set means no error can be returned.

### What to do when a contract fails

Decide which side is wrong, and change that side:

1. **The code is wrong.** Move the call out of the pure function, wrap the error with `%w` so it can be matched, add the variant to the errmap, or stop discarding the error. This is the usual case.
2. **The contract is wrong.** Widen it deliberately (`//effectgo:allow db`, `//effectgo:errors …`) and say why in the comment next to it.
3. **The callee is opaque.** It has no body the analysis can see. Add a rule to the module's `effectgo.stubs`, with a source reference, for example `effects example.com/x.Save db`. Do not add a stub just to hide a finding you have not read.

Do not add a directive only to silence output. A contract that is never checked against a real finding is noise.

### Lessons from the dogfood run

Each lesson is a mistake a first-time user made with effectgo, and the page that answers it.

- **Put the contract on the interface you call.** An uncontracted interface call is `unknown`, and `unknown` breaks `pure` and any `allow` list that does not name it. Declare the contract at package level, next to the interface. See [Directives](docs/reference/directives.md#unknown-and-the-contracts).
- **In net/http, check an inner method that returns an error.** A `func(w, r)` handler has no error result, so a directive on it checks nothing, and effectgo reports it. Put `//effectgo:handler` on the method that returns the error, and let a thin adapter write the response. See [HTTP checks](docs/guide/http-checks.md#nethttp-handlers).
- **Mark enums and sealed sets explicitly.** A named integer is not an enum unless it has `//effectgo:sealed`, and an interface is sealed by an unexported marker method, not by the directive alone. See [Directives](docs/reference/directives.md#when-an-interface-needs-effectgosealed).
- **Read the chain, then the origin.** The first words name the function that breaks the contract; the last witness step is the call that introduced the effect. A run of `cannot resolve` lines is one bad name repeated, and a `?` or `new@` in an error set is an error that `errors.Is` cannot match. See [Analyzers](docs/reference/analyzers.md#errset) and [Error sets](docs/reference/errors.md#variants).
- **Confirm the package was analysed.** A clean standalone run prints only the summary line, which counts the packages it analysed. Use `effectgo sig` to see the signatures of a package. See [Getting started](docs/guide/getting-started.md#a-clean-run-is-not-a-full-run).

### Caching gotchas

- After editing a stub file given with `-stubs`, run `go vet -a`. Stubs named with `$EFFECTGO_STUBS`, shipped stubs and the module's `effectgo.stubs` invalidate the vet cache by themselves.
- golangci-lint does not see stub edits: run `golangci-lint cache clean` after changing `effectgo.stubs`.

## Part 2: developing effectgo

### Build and test

From the repository root (Go 1.26):

```sh
go build ./...
go vet ./...
go test ./... -count=1
bash examples/run.sh          # builds bin/effectgo and runs both examples; must exit 0
```

`examples/todo`, `examples/layers` and `examples/editor` are separate modules; run their tests from their own directories. `examples/run.sh` is the acceptance script: it prints the expected diagnostics, so compare its output after a change.

In this workspace, run `go test` and other heavy commands under the shared lock, with the memory cap:

```sh
flock /root/pp/go-rust/realworld-eval/heavy.lock systemd-run --scope -q -p MemoryMax=6G -p MemorySwapMax=0 timeout 2400 \
  bash -c 'cd DIR && env -u GOROOT GOFLAGS=-buildvcs=false <cmd>'
```

### Repository layout

| Path | What it is |
|---|---|
| `effects/`, `errset/` | the two contract analyzers; errset has the SSA engine (default) and the AST engine |
| `mustuse/`, `exhaustive/` | must-use results and sealed switches |
| `signature/` | `effectgo sig` |
| `openapi/`, `adapters/gin/`, `adapters/huma/` | the OpenAPI and HTTP checks |
| `internal/stubs/`, `internal/goroot/` | stub parser and discovery; standard-library detection |
| `internal/cmd/stubdoc/` | generates `docs/reference/stub-coverage.md` from the shipped stubs; its test fails when the page is stale |
| `stubs/` | shipped stub files, embedded; `go generate ./stubs` regenerates the coverage page |
| `plugin/` | golangci-lint module plugin |
| `cmd/effectgo/` | the binary and its subcommands |
| `examples/` | `layers` (effects), `todo` (errset, OpenAPI, HTTP), `editor` (plugin and project stubs) |
| `docs/` | the documentation (reference, guide, design, journal) |

Architecture: [docs/design/architecture.md](docs/design/architecture.md).

### Invariants

- **Zero false positives on the disciplined corpus.** The corpus lives outside the repository (`realworld-eval/corpus/disciplined`, read-only). A change that adds a finding there is a regression unless the corpus marks it as intended. Do not edit the corpus to make a check pass.
- **Silence when unproved.** A new rule reports only when the risk is provable. When in doubt, be silent and document the limit.
- **Determinism.** Output must not depend on scheduling or on cache state. The determinism guard is `cmd/effectgo/determinism_test.go`. Comparisons between runs need the same stub configuration; a difference in stubs is not nondeterminism.
- **Stdlib detection by layout.** Standard library is decided from the source path under a Go installation root (`internal/goroot`), never from `$GOROOT` or the toolchain. Do not reintroduce environment checks.
- **Stub rules cite their source.** Each shipped rule has a `file:line` comment and the module version it was checked against.
- **Facts are gob-encodable and registered** in the analyzer's `FactTypes`. A result type must be returned as a typed value, not nil, when `ResultType` is set.
- **Keep the vet identity honest.** `cmd/effectgo` hashes the executable and the stub files; a new stub source must be added to `internal/stubs.Sources`.

### Adding a stub rule

1. Put the rule in the right file under `stubs/` (`stdlib.stubs` for standard library, or the framework's file). Use the verb with the right arity (see [docs/reference/stubs.md](docs/reference/stubs.md)).
2. Add a comment with the source path and line, and the module version checked.
3. If the rule changes an analyzer's output, add or update a test in that analyzer's `testdata/` and run `bash examples/run.sh`.
4. Check that the file still parses: a malformed line is a hard error that names the file and line.
5. Regenerate the coverage page with `go generate ./stubs` and commit it with the rule. `go test ./internal/cmd/stubdoc` fails until the page matches the files ([docs/reference/stub-coverage.md](docs/reference/stub-coverage.md)).

### Adding an analyzer

1. Create a package with an `analysis.Analyzer`; declare `Requires`, `FactTypes` and `ResultType` as needed; register flags on `Analyzer.Flags`.
2. Add it to the multichecker in `cmd/effectgo/main.go`. For editors, add it to the `analyzers` list in `plugin/plugin.go` (one line).
3. Test with `analysistest` and `testdata/src/`.
4. Document it in [docs/reference/analyzers.md](docs/reference/analyzers.md) with its exact message formats, and its flags in [docs/reference/cli.md](docs/reference/cli.md).

### Adding a directive

Read it in the analyzer's directive parser. An analyzer must skip keywords it does not own, and report unknown keywords. A keyword is read only at the placements its analyzer uses. The placement table is `placed` in `directive/directive.go`, and it mirrors the readers: a new reader, or a new placement for a keyword, changes that table. Update [docs/reference/directives.md](docs/reference/directives.md) and add a `testdata/` case for the diagnostic. `cmd/effectgo/inert_test.go` pins one diagnostic per misplaced probe line, so a change that adds or repeats one fails there.

### Documentation rules

- Every `docs/**/*.md` has YAML frontmatter with `title` and `description`.
- Plain CommonMark and GFM tables, fenced code; no MDX, no raw HTML, no absolute site URLs.
- Internal links are relative (`../reference/directives.md`). Check that every link and anchor resolves.
- Regenerate example outputs by running the binary; do not copy them from older text.
- Journal files (`docs/journal/`) record history; do not put current rules there.
