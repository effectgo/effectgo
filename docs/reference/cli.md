---
title: Command line
description: Every effectgo subcommand and flag, exit codes, the differences between standalone, go vet and golangci-lint runs, and how go vet caches results.
---

One binary, `effectgo`, runs in four ways:

| Way | Command | What runs |
|---|---|---|
| standalone | `effectgo ./...` | effects, errset, mustuse, exhaustive, schematag, directive; test files analyzed by default; ends with a summary line |
| go vet | `go vet -vettool=$(which effectgo) ./...` | the same five analyzers, through `go vet` |
| sig | `effectgo sig ./...` | signatures only |
| subcommands | `effectgo openapi`, `ts`, `http-gin`, `http-huma`, `http` | the OpenAPI, TypeScript and HTTP checks |

The golangci-lint plugin is a fifth way; see [Editor integration](../guide/editor.md).

Build the binary from a clone with `go build -o bin/effectgo ./cmd/effectgo` (Go 1.26). For the install route, see [Getting started](../guide/getting-started.md).

## Subcommands

| Command | Purpose | Output |
|---|---|---|
| `effectgo ./...` | diagnostics of the five analyzers | the driver's diagnostics |
| `effectgo sig ./...` | `Effect<A, E, R>` for every function | one line per function; exit 3 when it printed any (exit 3 is success here, see [Exit codes](#exit-codes)) |
| `effectgo openapi [-merge spec.json \| -check spec.json] ./...` | error responses of each route | a fragment (JSON), a merged document (JSON), or a drift report (text) |
| `effectgo ts [-o FILE] [-name NAME] ./...` | the routes of `rt/web` as a TypeScript client (effect/http-api, effect@4.0.4) | the file on stdout, or in `FILE`; a generation error exits 1 with its position |
| `effectgo http-gin [-json] [-all] [-mappers] ./...` | Gin responses against their error sets | text, or one JSON object per finding |
| `effectgo http-huma [-json] [-all] ./...` | Huma routes whose errors reach HTTP 500 | text, or one JSON object per finding |
| `effectgo http [-json] [-all] ./...` | both HTTP checks, one list | text, or one JSON object per finding |

The HTTP checks are described in [HTTP checks](../guide/http-checks.md); the OpenAPI command there too.

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

The multichecker adds its own flags, for example `-json` (the driver's JSON diagnostics, not the HTTP schema), `-c`, `-fix` and `-diff`. effectgo does not implement fixes: `-fix` has nothing to apply.

The analyzer enable flags (`-effects`, `-errset`, `-mustuse`, `-exhaustive`, `-directive`) are the driver's. `-signature` enables the signature analyzer in the standalone binary; under `sig` it is already the only one.

The standalone run is not the multichecker, so it accepts a smaller set of driver flags: `-test`, `-json` (the diagnostics as JSON on stdout), `-c` (lines of context), and the analyzer flags. `-fix`, `-diff`, `-debug`, `-cpuprofile`, `-memprofile` and `-trace` are not accepted there (exit 2). They still work under `go vet`, which runs the multichecker.

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
| `-stubs`, `-errset.engine` | as above |

### http-huma

| Flag | Meaning |
|---|---|
| `-json` | one JSON object per finding; the coverage summary goes to stderr |
| `-all` | also print `info` findings |
| `-stubs`, `-errset.engine` | as above |

### http

`-json`, `-all`, `-stubs` and `-errset.engine`, with the meaning above, for both adapters over one package load.

## Exit codes

| Run | 0 | 1 | 2 | 3 |
|---|---|---|---|---|
| standalone `effectgo ./...` | no diagnostics | a run error (the packages do not load, a malformed stub file) | a usage error: an unknown flag, or a flag after the package patterns (see [Flags after the package patterns](#flags-after-the-package-patterns)) | diagnostics were reported |
| `go vet -vettool=…` | no diagnostics | diagnostics were reported (go vet's convention) | — | — |
| `effectgo sig` | — | a run error | a usage error, as above | a signature was printed. This is a success, not a diagnostic: a script that tests for a non-zero exit must accept 3 |
| `effectgo openapi` | success | `-check` found a missing code, or the run failed | usage error | — |
| `http`, `http-gin`, `http-huma` | the run completed (findings do not change the status) | a load or analysis error | usage error | — |

The HTTP checks report their findings and exit 0; they are for reading and for JSON consumers, not for gating a build. The `openapi -check` exit 1 is the gate.

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
| Analyzers | all four | all four | all four |
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
