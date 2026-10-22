---
title: Command line
description: Every effectgo subcommand and flag, exit codes, the differences between standalone, go vet and golangci-lint runs, and how go vet caches results.
---

One binary, `effectgo`, runs in these ways:

| Way | Command | What runs |
|---|---|---|
| standalone | `effectgo ./...` | effects, errset, mustuse, exhaustive, schematag, directive; test files analyzed by default; ends with a summary line |
| go vet | `go vet -vettool=$(which effectgo) ./...` | the same six analyzers, through `go vet` |
| sig | `effectgo sig ./...` | signatures only |
| subcommands | `effectgo openapi`, `ts`, `http-gin`, `http-huma`, `http` | the OpenAPI, TypeScript and HTTP checks |
| shared contract | `effectgo contract [patterns]` | the six static analyzers and Gin/Huma checks over one package load |

The golangci-lint plugin is another integration; see [Editor integration](../guide/editor.md). The shared contract command is released in `v0.2.0` with JSON `schema_version: 1`; the other command behaviors remain unchanged.

## Subcommands

| Command | Purpose | Output |
|---|---|---|
| `effectgo ./...` | diagnostics of the six analyzers | the driver's diagnostics |
| `effectgo sig ./...` | `Effect<A, E, R>` for every function | one line per function; exit 3 when it printed any (exit 3 is success here, see [Exit codes](#exit-codes)) |
| `effectgo openapi [-merge spec.json \| -check spec.json] ./...` | error responses of each route | a fragment (JSON), a merged document (JSON), or a drift report (text) |
| `effectgo ts [-o FILE] [-name NAME] ./...` | the routes of `rt/web` as a TypeScript client (effect/http-api, effect@4.0.4) | the file on stdout, or in `FILE`; a generation error exits 1 with its position |
| `effectgo http-gin [-json] [-all] [-mappers] [-fail-on=SEVERITY] [-fail-unresolved] [-coverage=json] ./...` | Gin responses against their error sets | text, or one JSON object per finding; optional machine-readable coverage on stderr |
| `effectgo http-huma [-json] [-all] [-fail-on=SEVERITY] [-fail-unresolved] [-coverage=json] ./...` | Huma routes whose errors reach HTTP 500 | text, or one JSON object per finding; optional machine-readable coverage on stderr |
| `effectgo http [-json] [-all] [-fail-on=SEVERITY] [-fail-unresolved] [-coverage=json] ./...` | both HTTP checks, one list | text, or one JSON object per finding; optional machine-readable coverage on stderr |
| `effectgo contract [-format=text\|json] [-all] [-fail-on=…] [-fail-unresolved] [-coverage=json] [patterns]` | shared six-analyzer and Gin/Huma report | text or one JSON document; see [Contract checks](../guide/contract-checks.md) |

## Flags

### Analyzer flags

These are the same in standalone and go vet runs. The multichecker prefixes each analyzer's flags with the analyzer's name.

| Flag | Default | Meaning |
|---|---|---|
| `-stubs=a.stubs,b.stubs` | empty | comma-separated stub files; replaces the zero configuration. When empty, `$EFFECTGO_STUBS` is used; when that is empty too, the zero configuration applies (see [Stub files](stubs.md#discovery)) |
| `-effects.report` | false | print `effects of <F>: {…}` for every declared function |
| `-effects.witness` | false | print one `witness of <F>: <effect> via <chain>` per effect |
| `-errset.report` | false | print `errors of <F>: …` (and `handler errors of <F>: …`) for every function with an error result |
| `-errset.witness` | false | print one `witness of <F>: <variant> via <chain>` per variant |
| `-errset.statuses` | false | print the HTTP status of each variant of each handler (`statuses of <F>: …`) |
| `-errset.ignoreopaque` | false | do not report opaque (`?`) and fresh (`new@`) errors in handler checks |
| `-errset.engine=ssa\|ast` | `ssa` | `ssa`: flow-sensitive over SSA values (default); `ast`: the older flow-insensitive engine |
| `-mustuse.allow=f,(*T).M` | empty | fully qualified functions whose results may be discarded |
| `-effects.all` | false | report every `effects` violation, including the callers whose root is another function's violation (see [Analyzers](analyzers.md#effects)) |
| `-errset.all` | false | report every `errset` contract violation, including the callers whose root is another function's violation (see [Analyzers](analyzers.md#errset)) |
| `-test` | true (standalone and `sig`) | analyze `_test.go` files too, as `go vet` does; `-test=false` skips them (see [Test files](#test-files)) |

Each `-errset.engine` value is accepted by the subcommands too.

### Driver flags

The default CLI lists its accepted flags with `effectgo -h`, `-help` or `--help`. Use `effectgo help [analyzer]` for the multichecker's analyzer documentation.

| Standalone flag | Default | Meaning |
|---|---|---|
| `-test` | true | analyze test files too; `-test=false` skips them |
| `-json` | false | print native driver diagnostics as JSON on stdout; the summary remains on stderr |
| `-c=N` | `-1` | print `N` lines of source context around diagnostics; the default prints none |
| `-cpuprofile=file` | empty | record a CPU profile, including package loading and analysis |
| `-memprofile=file` | empty | write a heap profile after analysis and a garbage collection |

The standalone run also accepts the analyzer flags above. Profiles are flushed when the run returns, including when it reports findings or encounters a load, stub or analysis error. A profile creation, start or write error goes to stderr and exits 1. Inspect the files with `go tool pprof`, for example:

```sh
effectgo -cpuprofile=cpu.pprof -memprofile=heap.pprof ./...
go tool pprof -top ./effectgo cpu.pprof
```

`sig` and the `go vet` tool path use the multichecker, which adds driver flags such as `-fix`, `-diff`, `-debug`, `-trace`, `-tags` and analyzer enable flags. These flags are not accepted by the standalone run (exit 2). effectgo does not implement fixes, so `-fix` has nothing to apply. `sig` runs only the signature analyzer; the standalone and vet modes run the six analyzers listed above.

### sig

`effectgo sig [-stubs] [-errset.engine] [-test=false] ./...`. Test files are analyzed by default, as in the standalone run.

### openapi

| Flag | Meaning |
|---|---|
| `-stubs` | stub files (falls back to `$EFFECTGO_STUBS`) |
| `-merge spec.json` | print the document with the error responses of each known operation replaced |
| `-check spec.json` | compare the error responses with the document; exit 1 on a missing code |
| `-errset.engine` | error-set engine |

With neither `-merge` nor `-check`, the output is the fragment: `paths.<path>.<method>.responses.<code>` with `description` and `x-effectgo-errors`. Routes without a documented error set go to stderr as `undocumented: <METHOD> <path> (<pos>): <reason>`. `-merge` and `-check` cannot be combined (exit 2).

### ts

| Flag | Meaning |
|---|---|
| `-o FILE` | write the file here (default: stdout) |
| `-name NAME` | the API name passed to `HttpApi.make` (default: the last element of the module path) |

The output is deterministic. A type that rt cannot describe, a recursive type, two types with one name, or two endpoints with one name in a group is a generation error: the generator writes nothing and lists each error with its position. The schema rules that have no TypeScript check become comments that say what the server checks. Exit 2 is a usage error.

### http-gin

| Flag | Meaning |
|---|---|
| `-json` | one JSON object per finding (the shared schema, see below) |
| `-all` | also print `info` findings |
| `-mappers` | list the detected sink and status mappers on stderr |
| `-fail-on=none\|high\|medium\|leak\|info` | exit 3 when a finding at or above the selected threshold exists; default `none` keeps the historical exit 0 |
| `-fail-unresolved` | exit 3 when route coverage contains an unresolved route; default false |
| `-coverage=json` | write one machine-readable coverage object to stderr; stdout keeps the existing finding stream |
| `-stubs`, `-errset.engine` | as above |

### http-huma

| Flag | Meaning |
|---|---|
| `-json` | one JSON object per finding; the coverage summary goes to stderr |
| `-all` | also print `info` findings |
| `-fail-on=none\|high\|medium\|leak\|info` | exit 3 when a finding at or above the selected threshold exists; `leak` has no Huma findings and is accepted for shared scripts |
| `-fail-unresolved` | exit 3 when route coverage contains an unresolved route; default false |
| `-coverage=json` | write one machine-readable coverage object to stderr; stdout keeps the existing finding stream |
| `-stubs`, `-errset.engine` | as above |

### http

`-json`, `-all`, `-stubs` and `-errset.engine`, with the meaning above, for both adapters over one package load. `-fail-on`, `-fail-unresolved` and `-coverage=json` apply to the union; coverage contains separate `gin` and `huma` objects.

`-fail-on` is a severity threshold in the order `high`, `medium`, `leak`, `info`; selecting `medium` also fails for `high`, and selecting `info` fails for every finding. `leak` is a Gin category rather than a numeric severity, and is included explicitly in the order so a leak can be gated without changing finding JSON. Any invalid value, including an invalid `-coverage` format, is a usage error (exit 2).

Coverage JSON is a separate object on stderr. It includes route counts (`routes`, `resolved`, `unresolved`), finding counts, and framework-specific counters; Huma includes variant status counters. When present, `unresolved_routes` carries positions and reasons. Gin only collects registrations whose handler it resolves: non-constant paths and unresolved group prefixes count as unresolved, while variable handlers and wrappers may fall outside the collected route set. A constant empty path is a resolved root route. Gin coverage includes a `limit` message stating this boundary. A zero unresolved count does not prove every registration was collected.

### contract

`effectgo contract [-format=text|json] [-all] [-fail-on=none|high|medium|leak|info] [-fail-unresolved] [-coverage=json] [analyzer flags] [patterns]` defaults to `./...` and loads each package once for the six static analyzers plus Gin/Huma. Text is the default; JSON is one document with `schema_version`, `diagnostics`, `coverage`, and `exit_code`. Each diagnostic has `rule_id`, `analyzer`, `severity`, `position` (`file`, `line`, `column`) and `message`, with optional HTTP `route`, `handler`, `variant`, `status`, and `via`. Native diagnostics use the analyzer name as a coarse rule family, severity `error`, and always cause exit 3; consumers must not parse messages. HTTP IDs are `http.gin` and `http.huma` and retain their existing severities. Coverage reuses the Gin/Huma collections: shared route stubs can expose `rt/web` registrations to Huma, where a missing `errdefault` model is unresolved. It is not a separate proof of complete native or OpenAPI route coverage. See [Contract checks](../guide/contract-checks.md) for the wire example and limits.

## Exit codes

| Run | 0 | 1 | 2 | 3 |
|---|---|---|---|---|
| standalone `effectgo ./...` | no diagnostics | a run error (package loading, malformed stubs or profiling) | a usage error: an unknown flag, or a flag after the package patterns (see [Flags after the package patterns](#flags-after-the-package-patterns)) | diagnostics were reported |
| `go vet -vettool=…` | no diagnostics | diagnostics were reported (go vet's convention) | — | — |
| `effectgo sig` | — | a run error | a usage error, as above | a signature was printed. This is a success, not a diagnostic: a script that tests for a non-zero exit must accept 3 |
| `effectgo openapi` | success | `-check` found a missing code, or the run failed | usage error | — |
| `http`, `http-gin`, `http-huma` | the run completed with no selected gate violation | a load or analysis error | usage error, including an invalid gate value | selected `-fail-on` severity or `-fail-unresolved` coverage policy was violated |

The HTTP checks report their findings and exit 0 by default. `-fail-on` and `-fail-unresolved` opt into exit 3 as a gate; `-coverage=json` only changes the coverage stream and does not itself fail the run. The `openapi -check` exit 1 is the separate OpenAPI gate.

## Flags after the package patterns

The flag package stops at the first argument that is not a flag, so a flag written after a package pattern is read as a pattern. Without a check, the run fails with `malformed import path "-effects.report": leading dash`, which does not name the cause. Before it loads any package, effectgo checks for such flags in the standalone run and in each subcommand that takes patterns (`sig`, `openapi`, `http`, `http-gin`, `http-huma`). It exits 2 and prints the command with the flags first:

```text
effectgo: flag -effects.report after the package patterns; flags must come before them: effectgo -effects.report ./...
```

A value flag is moved with its value (`-stubs x.stubs`), and the message names the flags without the value. A flag that the command does not define gets a different message. When the standalone run defines it, the message names that command:

```text
effectgo: flag -errset.statuses after the package patterns, and effectgo sig does not define -errset.statuses; the standalone run does: effectgo -errset.statuses ./internal/store/
```

`sig` takes the flags of its own usage (`effectgo sig -help`), not the report flags of the analyzers, such as `-errset.statuses`, which are the standalone run's. The check does not apply under go vet, which parses its own flags.

## Test files

- The standalone run and `sig` analyze `_test.go` files by default (`-test`), as `go vet` does. Each test variant (`p [p.test]`, `p_test [p.test]`) is one analysed package in the summary. Pass `-test=false` to skip the test files.
- The generated test main packages (`p.test`) are not analyzed: they hold no source of their own.
- `go vet` decides for itself which packages and test variants it passes to the tool, so `-test` has no effect under `go vet`. Both runs now see the same test files; the standalone run adds the summary line (see [Summary line](#summary-line)).

## Summary line

The standalone run ends with one line on stderr, after the diagnostics:

```text
effectgo: 8 packages analysed (5 test variants), 2 findings, 1 duplicate hidden (-effects.all or -errset.all to show them)
```

- `N packages analysed` counts the packages of the patterns and their dependencies that are roots of the run, without the test variants. `(M test variants)` counts the test variants (`-test` on).
- `K findings` counts the distinct diagnostics printed. A diagnostic that the printer drops as a repeat of the same position and text is counted once.
- `H duplicates hidden` appears only when the root rule suppressed some violations (see [Analyzers](analyzers.md#root-rule)). It is the number of violations that are not printed; `-effects.all` and `-errset.all` print them.
- The line starts with `effectgo: ` and has no `path:line:col:` prefix, so a parser of the diagnostics does not read it as one. It is printed in `-json` mode too, on stderr, while the JSON goes to stdout.

It is printed by the standalone run only. It is not printed under `go vet` (the same analyzers through the tool interface, with no place for a summary), by the golangci-lint plugin, or by `sig`, `openapi` and the `http*` subcommands. The exit codes do not depend on it.
- `openapi` loads without tests.

## Standalone, go vet and the plugin

| Aspect | Standalone | go vet | golangci-lint plugin |
|---|---|---|---|
| Analyzers | all six | all six | all six |
| Report flags (`-effects.report`, `-errset.witness`, …) | yes | yes | no |
| Output paths | absolute | relative to the package directory | golangci-lint's (absolute, with the linter name) |
| Test files | analyzed by default (`-test`) | decided by go vet | on with `run.tests: true` |
| Summary line | yes (stderr, last line) | no | no |
| Stub configuration | flag, env, or zero configuration | the same; `-stubs` is forwarded | `settings.stubs`, or zero configuration |
| Caching | none | go vet caches under the tool identity (below) | golangci-lint's cache, which does not see stub edits |
| Subcommands | yes | no | no |

The plugin exposes `effects`, `errset`, `mustuse`, `exhaustive` and `directive` under the linter name `effectgo`, with the settings `stubs`, `ignoreOpaque` and `mustuseAllow`. Details are in [Editor integration](../guide/editor.md).

## Environment

| Variable | Used by | Meaning |
|---|---|---|
| `EFFECTGO_STUBS` | every mode | comma-separated stub files, read when `-stubs` is empty |

Setting either `-stubs` or `EFFECTGO_STUBS` replaces the zero configuration: exactly those files are read.

## go vet and the cache

`go vet` caches results under the tool's identity, and the identity does not normally cover stub files. effectgo answers `-V=full` with an identity that includes a hash of its own executable and of every stub file that the analysis reads from the working directory. An edit to a shipped file (in the executable), to the module's `effectgo.stubs`, or to a file named in `$EFFECTGO_STUBS` changes the identity, and the next `go vet` runs the analysis again.

Limits:

- `go vet` asks for the identity without flags, so a file named only by `-stubs` is not in it. Name the file in `$EFFECTGO_STUBS` as well, or run `go vet -a` after editing it.
- A project file of a module below the working directory is not in the identity.

The standalone run has no cache and is not affected.

## Output details

- **Streams.** The analyzers' diagnostics and report lines (standalone runs, `sig`, and `go vet`, which forwards them) go to **stderr**. `openapi` writes its fragment, merged document and `-check` report to **stdout**, and the undocumented-route notes to stderr. The HTTP checks write their findings to **stdout** and their summary line to **stderr**. Redirect accordingly when you pipe the output.
- The standalone run prints absolute paths; `go vet` prints paths relative to the package directory. Both are correct for their caller.
- The HTTP text output ends with a summary line on stderr, for example `effectgo http-gin: handlers=1 mappers=0 routes=1 high=0 medium=1 info=0 leak=0`. Summary counts include the hidden `info` findings.
- The HTTP JSON schema is one object per line:

```text
{"severity":"medium","framework":"gin","route":"GET /through","handler":"ThroughLocal (app/app.go:21)","pos":"app/app.go:25","variant":"huma.Error404NotFound","status":500,"via":"carry; inline `err != nil` at app/app.go:24","reason":"huma.Error404NotFound reaches a 500 response and no other mapping in the project covers it (foreign variant)"}
```

Fields: `severity`, `framework` (`gin` or `huma`), `route`, `handler`, `pos`, `variant`, `status`, `via`, `reason`. Severities are `high`, `medium`, `leak` (Gin only) and `info`. `http` lists them sorted by severity (`high`, `medium`, `leak`, `info`), then by position.
