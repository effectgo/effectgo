---
title: P4: integration of must-use, exhaustive and the editor plugin
description: The integration of the P4a and P4b branches: conflicts, wiring, corpus and sanity results.
---

### Integration (P4: mustuse/exhaustive + editor)

Branch `integrate/p4`, from `feat/flow-sensitive` (4763d2f). Merged with `--no-ff`: `feat/mustuse-exhaustive` (b775219, P4a), then `feat/editor` (91d09d3, P4b). The fixes and wiring are in d0d4b03.

**Conflicts.** Neither merge had a textual conflict. Git merged `cmd/effectgo/main.go`, `errset/stubs.go`, `effects/effects_test.go`, `errset/errset_test.go`, `stubs/stdlib.stubs`, `README.md` and `NOTES.md` on its own. The semantic checks that replaced reading the conflict markers:

- errset stub lookup: P4a's `stubSetVia` and `stubInInterfaces` sit on top of P4b's `loadStubs`, which now calls `stubs.RulesForPass`. The interface lookup takes the stub set from the same resolved rules, so `hash.Hash32` finds `(hash.Hash).Write` in zero-config mode too.
- `cmd/effectgo`: both analyzers are registered (`multichecker.Main(effects, errset, mustuse, exhaustive)`), `sig` runs only `signature.Analyzer`, and the `-stubs` help is P4b's text.
- `stubs/stdlib.stubs`: the writer rules (`bytes.Buffer`, `strings.Builder`, `hash.Hash`) appear once, and `stubs/shipped.go` embeds that file.
- `README.md`, `NOTES.md`: each heading appears once. The P4a "Must-use" text said "without the stub file the writer calls are reported", which zero configuration makes false; it was reworded in the README (see below).

**Test fallout (one fix).** P4b pins `stubs.Flag` to an empty file in the TestMain of `effects` and `errset`, but not in `mustuse`. An empty `-stubs` is the zero configuration now, so `TestMustUse` (c) and `TestWriterStubsAbsent` (wn) saw the writer rules and went silent. Fix: `mustuse/mustuse_test.go` TestMain pins `mustuse/testdata/stubs/none.stubs` (an empty file, as the other two packages do), `TestWriterStubsAbsent` uses that file, and `TestWriterStubsZeroConfig` (new) runs `w` with no `-stubs` and no `$EFFECTGO_STUBS` and requires silence, so the writer rules are tested in zero-config mode. Before this fix, 2 of the mustuse tests failed; after it, all 5 pass.

**Wiring.**
- `plugin/plugin.go`: `analyzers` is `effects, errset, mustuse, exhaustive`. A new setting `mustuseAllow` (list) is joined and passed to `mustuse.Analyzer`'s `allow` flag, with blank entries dropped. `plugin_test.go` checks the analyzer list and the setting.
- Zero-config path: mustuse and exhaustive have no stub reader of their own. mustuse reads writer rules through `errset.Result.NoErrors`, and exhaustive does not read stubs. Both therefore use `stubs.RulesForPass` through errset, the same path as `effects`, `errset`, `gin`, `huma` and `openapi`.
- `examples/todo/cmd/openapispec/main.go:25`: the discarded `os.Stdout.Write` error is now checked (`log.Fatal`). The other example findings are unchanged; `examples/run.sh` still exits 0 and its exit codes do not change.
- README "Editor integration" and the former editor recipe page (now `docs/guide/editor.md`) list all four analyzers and `mustuseAllow`. README "Must-use results" now says the writer rules come from the zero configuration or the explicit stub file.

**Corpus** (`corpus/disciplined`, `domain/domain.go:37` carries the orchestrator's `//effectgo:sealed`; the corpus is unchanged).

| run | command | result |
|---|---|---|
| a1 zero-config, `--future` | `env -u EFFECTGO_STUBS effectgo ./...`, `check.py --future` | TP 17, FN 0, FP 0 |
| a2 zero-config, no `--future` | same output, `check.py` | TP 12, FP 0; 5 future markers listed as "diagnostics on EXPECT-FUTURE lines" |
| b explicit stdlib stubs | `EFFECTGO_STUBS=stubs/stdlib.stubs effectgo ./...` | TP 17, FN 0, FP 0; the same 17 diagnostic lines as a1 |
| c vet | `env -u EFFECTGO_STUBS go vet -vettool=effectgo ./...` | 17 diagnostics, the same locations as a1 (exit 1) |
| d plugin | `custom-gcl run --config golangci.yml ./...` (only `effectgo`, `tests: false`, no caps) | 17 issues, the same locations; `check.py --future` on the plugin output: TP 17, FN 0, FP 0 |

Location comparison, plugin against standalone, by corpus-relative path and line (`runs/integrate4/compare-locations.py`, outside the repository): both lists have the same 17 entries, and nothing is only on one side. The vet list is also identical. The 17 locations: `clock/clock.go:48`, `domain/domain.go:159`, `httpapi/errmap.go:61`, `httpapi/errmap.go:74`, `httpapi/handlers.go:47`, `httpapi/handlers.go:94`, `ids/ids.go:43`, `ids/ids.go:57`, `service/audit.go:9`, `service/audit.go:23`, `service/report.go:46`, `service/service.go:157`, `service/service.go:242`, `service/service.go:257`, `service/service.go:273`, `store/store.go:110`, `store/store.go:135`.

The plugin reports `mustuse:` and `exhaustive:` findings and the `effects:` and `errset:` findings under one linter name `effectgo`. Its `(effectgo)` suffix is golangci-lint's linter tag.

**Build and tests** (`GOFLAGS=-buildvcs=false`, under the heavy lock): `go build ./...` ok, `go vet ./...` ok, `go test ./... -count=1` ok for all 12 packages that have tests. `gofmt -l .` prints nothing. `bash examples/run.sh` exits 0.

**Real-world sanity** (zero configuration, `effectgo -mustuse -exhaustive ./...` on each `src/backend`; outputs in `runs/integrate4/sanity-*.err`):

| snapshot | mustuse | exhaustive | P4a (explicit four stubs) |
|---|---|---|---|
| order-mgmt | 13 | 0 | 13 / 0 |
| kuzma-wms | 0 | 0 | 0 / 0 |
| arcteryx-console | 25 | 0 | 25 / 0 |

The findings are identical to P4a's file for order-mgmt and arcteryx-console, after the path prefix is stripped. The zero configuration loads all five shipped stub files, P4a loaded four (stdlib, gin, huma, pgx), and the extra `fuego.stubs` changed none of these findings: fuego's rules apply only to fuego packages, which these snapshots do not import.

**Known limits, unchanged.** golangci-lint's result cache does not see stub edits (see "Editor integration"). The plugin runs only the four analyzers listed above. The corpus `EXPECT` count in `check.py --future` mode is 17 and in the default mode 12.
