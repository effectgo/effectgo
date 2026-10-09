---
title: P1: stub configuration, determinism and vet cache identity
description: The determinism report, the zero-configuration stub discovery, and the fix that makes go vet notice stub edits.
---

## Stub flags and environment

- `-stubs` is registered in `cmd/effectgo` on the global flag set, so `go vet` advertises it (it appears in `-flags`) and forwards it. **[run]** `go vet -vettool=… -stubs=…` on both examples.
- `$EFFECTGO_STUBS` is used when `-stubs` is empty. Both forms are accepted for both analyzers.
- Analyzer flags stay prefixed by the multichecker: `-effects.report`, `-errset.report`, `-errset.ignoreopaque`. The standalone prototypes had unprefixed names (`-report`); the single multichecker binary uses the prefixed names everywhere.
- **Zero configuration (replaces the no-stub-files warning).** With neither `-stubs` nor `$EFFECTGO_STUBS` set, the stub set of an analysis is the shipped stub files (embedded in the `stubs` package with `go:embed`) plus `effectgo.stubs` at the module root of the analyzed file (`internal/stubs/discover.go`: `Sources`, `RulesFor`, `RulesForPass`, `ModuleRoot`). The set is decided per package from its file path, so every analyzer (effects, errset and the HTTP adapters) reads the same set. With either variable set, exactly those files are read. The warning is removed, since the zero configuration always has stub rules; it was added after an eval run silently used no stub files (see "Determinism"). **[test]** `internal/stubs` `TestZeroConfigShippedAndProjectFile`, `TestExplicitStubsDisableDiscovery`, `TestMalformedProjectFileNamesFile`; `cmd/effectgo` `TestIdentityCoversProjectStubs`; **[run]** `examples/editor` (standalone and go vet, no flags, project stub reports the `db` effect).

### Determinism

**Reported symptom.** On order-mgmt, a cold-cache run and a warm run differed in 78 report entries (67 effects, 11 errset). **Root cause: a configuration difference, not nondeterminism in the analyzer.** The saved cold report equals a run with `EFFECTGO_STUBS=stubs/stdlib.stubs`, and the saved warm report equals a run without it (every non-cache-path line is identical). Stubs change results: `io.ReadFull` becomes `unknown`, `(error).Error` becomes pure, `strconv.Atoi` gets an error set. The 78 entries are exactly the stub-dependent ones (42 gained `unknown`, 25 lost it, 11 errset). Reports are comparable only when they were produced with the same stub files.

**Checked, no difference.** Same configuration with fresh and warm caches, with `GOMAXPROCS=1`, and with the sequential action graph (`-debug=p`) against the parallel one. `go test -race` is clean for `effects`, `errset` and `cmd/effectgo`.

**Guard.** `cmd/effectgo/determinism_test.go` (`TestOutputIndependentOfScheduling`) runs both analyzers over `examples/layers` in the sequential graph and in parallel at `GOMAXPROCS` 1, 4 and 8, and requires identical diagnostics, effect sets and error sets, witness chains included. It guards scheduling and fact passing. It does not cover stub configuration, and it has not been shown to fail on a seeded ordering bug.

**Known path dependence.** Test packages include generated test main files that `go/packages` writes into GOCACHE. Their report lines carry the cache path (`/root/.cache/go-build/…-d:81:6: effects of main: {exit}`), so compare runs with the same cache location or filter those lines.

## Vet cache identity

**The bug.** go vet caches results under the vet tool's identity and the vet flags, not under the contents of stub files. An edited stub gave the same output with the prototype. **[run]** (baseline below.)

**The fix.** `cmd/effectgo` answers `-V=full` before `multichecker.Main`. It prints `effectgo version devel buildID=<sha256>`, where the hash covers the executable's own bytes and the contents of every file named by `$EFFECTGO_STUBS`. The format is the one that `cmd/go` parses (`internal/work/buildid.go`, `toolID`): the `devel` form whose last field is `buildID=`, whose content ID is the part after the last slash. `go vet` computes the tool ID for each run, so an edited stub gives a different action ID and the analysis runs again.

**Zero configuration.** The identity hash covers the files of `internal/stubs.Sources(<working directory>)`: the shipped files (their content is also in the executable) and the `effectgo.stubs` of the module containing the working directory, or the explicit `$EFFECTGO_STUBS` files. `go vet` runs `-V=full` from the directory it was started in, so a project file of that module is in the ID. **[run]** `examples/editor`: an edit to `effectgo.stubs` changes the vet result on the next `go vet` without `-a` (see "Editor integration").

**Limitation.** `-stubs` values reach the tool only after the tool ID is computed (`-V=full` has no flags), so a file named only by `-stubs` is not in the ID. **[run]** Either name the file in `$EFFECTGO_STUBS`, or run `go vet -a`, which re-runs the analysis. Both are shown in the Merge section. A project file of a module below the working directory is not in the ID.

