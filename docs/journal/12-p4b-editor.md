---
title: P4b: editor integration
description: The golangci-lint plugin and the editor recipes: mechanism, zero-configuration stubs, verification matrix and latency.
---

## Editor and tooling

### Editor integration

**What was built.** A golangci-lint v2 module plugin, `plugin/` (`github.com/faintghost/effectgo/plugin`), that exposes the contract analyzers `effects` and `errset`. The list is one variable, `analyzers` in `plugin/plugin.go`: adding an analyzer for the editors is one line there. The report-only flags are never set, and the HTTP subcommands and `sig` are not part of the plugin.

**Mechanism (golangci-lint v2.14.0, the newest v2 on the module proxy at the time).** Read from the sources, not from the docs alone (the docs page does not give the Go-side contract):
- `.custom-gcl.yml` (`version`, `name`, `destination`, `plugins[]` with `module`, `import`, `path` or `version`), built with `golangci-lint custom` (`pkg/commands/internal/configuration.go`, `builder.go`: `go mod edit -replace` for a local `path`, then `go mod tidy`, then the build).
- The plugin package calls `register.Plugin(name, New)` from `github.com/golangci/plugin-module-register` **v0.1.2**. `New(conf any) (LinterPlugin, error)`; `LinterPlugin` has `BuildAnalyzers() ([]*analysis.Analyzer, error)` and `GetLoadMode() string` (`register.LoadModeTypesInfo`, `LoadModeSyntax`).
- `linters.settings.custom.<name>` with `type: module` and a `settings` map. golangci-lint passes that map to `New`; `register.DecodeSettings[T]` decodes it with `DisallowUnknownFields`, so an unknown key is an error.
- golangci-lint's module builder (`pkg/lint/lintersdb/builder_plugin_module.go`) creates one linter from the analyzers, named as the config key (`effectgo`), with the load mode the plugin asked for.

**Plugin settings.** `stubs` (list; absent or empty = the zero configuration; relative paths resolve against the module root of the working directory), `ignoreOpaque` (bool, errset `-ignoreopaque`). Tests: `plugin/plugin_test.go` (registration, analyzers, load mode, strict decode, path resolution, the flag mapping).

**Zero-config stub discovery.** Decided per analyzed package in `internal/stubs/discover.go`:
- `-stubs` or `$EFFECTGO_STUBS` set: exactly those files (unchanged behavior, and the explicit set replaces the zero configuration).
- otherwise: the shipped stub files, embedded with `go:embed` by the new package `stubs/` (`stubs/shipped.go`, `stubs.Files`), then `<module root>/effectgo.stubs` if the module (nearest `go.mod` at or above the analyzed file) has one. Chosen name: `effectgo.stubs` (a file, not a directory: one stub format, one file per module).
- Every analyzer reads through the same resolver: `effects`, `errset`, and the HTTP adapters (`gin`, `huma`) and `openapi`. The old global `stubs.Paths()` callers now pass the pass (`stubs.RulesForPass`).
- The no-stub-files warning is removed: the zero configuration always has stub rules. `TestNoStubsWarning` is gone; `TestIdentityCoversProjectStubs` replaces the warning's place in the identity logic.
- Vet identity: `cmd/effectgo` `identityHash(dir)` hashes the executable and `stubs.Sources(cwd)`. The `-V=full` probe is run by `go vet` from the directory it was started in (assumed from the go command's behaviour; the runs below start `go vet` at the module root), so the project file of that module is in the ID. Verified below for that case: vet picks up a project-stub edit without `-a`.
- Behaviour change to be aware of: the zero configuration loads all five shipped files (`stdlib`, `fuego`, `gin`, `huma`, `pgx`). Framework rules only apply to packages that are present, and on the disciplined corpus the result is identical to `stdlib.stubs` alone. The older eval baselines (`realworld-eval/baseline.jsonl`, SUMMARY) used `stdlib.stubs` only; zero-configuration numbers on the real-world snapshots are not comparable with them without `-stubs=stubs/stdlib.stubs`.
- Test packages: `effects` and `errset` `TestMain` now point `stubs.Flag` at `testdata/stubs/none.stubs` (an empty explicit file), because an empty `-stubs` is the zero configuration. Three tests (`TestKnownLimits`, `TestOpaqueHandlerReported`, `TestIgnoreOpaque`) failed under the first version of this change and pass with it.

**Verification matrix** (every command was run; "unverified" lines were not):

| what | how | result |
|---|---|---|
| golangci-lint v2.14.0 installed | `GOBIN=runs/p4b/bin go install …/v2/cmd/golangci-lint@v2.14.0` | `golangci-lint version` = 2.14.0 |
| custom build with the plugin | `golangci-lint custom -v` (path = this worktree) | built; plugin imported, replace directive applied |
| corpus, zero configuration | `custom-gcl run` (config outside the corpus, only `effectgo` enabled, `tests: false`, `max-issues-per-linter: 0`, `max-same-issues: 0`) vs `effectgo ./...` | 12 and 12, **identical sets** (script `runs/p4b/gcl/compare.py`) |
| corpus, `settings.stubs` (absolute `stubs/stdlib.stubs`) | same | 12, identical |
| corpus, `ignoreOpaque: true` vs `effectgo -errset.ignoreopaque` | same | 11 and 11, identical (drops the `new@` finding in `listTodos`) |
| example module, zero configuration | `examples/editor`: golangci-lint and `go vet`, no flags | 2 diagnostics each, one from the project stub (`Remember` → `db`) |
| project stub edit, golangci-lint | copy of `examples/editor`, `Save db` → `pure` | **stale**: the cached run still reports `Remember` (cache on); `custom-gcl cache clean` then gives the correct result |
| project stub edit, go vet | same copy, `go vet -vettool=…` without `-a` | correct on the next run (identity changed) |
| `go vet -vettool` on the example | `examples/run.sh` section | exit 1, the two diagnostics |
| standalone on the example | `examples/run.sh` section | exit 3, the two diagnostics |
| VS Code, workspace scope | the extension's argument list (below) run on the corpus | 12 lines, exit 0 (`--issues-exit-code=0`), parsed by the extension's regex (`extension/src/diagnostics/diagnostics.ts`) |
| VS Code, package scope | same, cwd = package directory | 4 lines for `service/`, works |
| VS Code, file scope | same, argument = one file | **broken**: `undefined: WithTx (typecheck)` (the file does not type-check alone); do not use `lintOnSave: file` |
| VS Code, vet | `go vet -vettool=… .` / `./...` | 4 lines for `service`; 12 for the module; exit 1 |
| nvim-lint args | nvim-lint's `getArgs` (v2 branch: JSON, every other output path blanked, `--show-stats=false`, `--path-mode=abs`, directory argument) | valid JSON after the arguments were complete (an earlier run without the blanked outputs printed the text summary to stdout and broke the JSON parse); 4 issues parsed |
| `examples/run.sh` | `bash examples/run.sh` | exit 0 |

The VS Code argument list (from `goLint.ts`, golangci-lint v2 branch): `run <lintFlags> --issues-exit-code=0 --output.text.print-issued-lines=false --show-stats=false --output.text.path=stdout --path-mode=abs <./... | file | nothing>`, run with cwd = workspace root or the file's directory.

**Not verified:** running VS Code or Neovim; `go.alternateTools` for the `golangci-lint-v2` name and the editor's problem matcher; nvim-lint's working directory; installing the plugin from the Go proxy (`version: <tag>`; the module is private and was only built from a local path); Windows paths.

**Latency.** Measured on this machine, with the measurement script holding the heavy lock (the other agents' locked jobs were queued behind it; unlocked work on the machine was not controlled). Each row is one run, wall-clock. Corpus: `corpus/disciplined`, six packages, 12 diagnostics. Kuzma: `kuzma-wms/src/backend`, package `./internal/service` (106 files; copied, the snapshot is read-only), no diagnostics. "Touched" is a content change (a comment appended to one file, in a copy), because the caches are keyed by content.

| case | `go vet -vettool=effectgo` | golangci-lint + plugin |
|---|---|---|
| corpus, cold (empty GOCACHE / lint cache) | 19.1 s | 21.0 s |
| corpus, warm, nothing changed | 0.14 s | 0.65 s |
| corpus, warm, one file touched | 0.16 s | 1.16 s |
| kuzma `internal/service`, cold | 107.9 s | 101.1 s |
| kuzma `internal/service`, warm, nothing changed | 0.39 s | 0.73 s |
| kuzma `internal/service`, warm, one file touched | 4.33 s | 3.78 s |

Phase probes on kuzma, warm, after one more touch (single runs; the vet number here is 6.7 s against 4.3 s in the table, which shows the noise of single runs):

| probe | time | what it is |
|---|---|---|
| `go build ./internal/service` | 2.5 s | the compile of the touched package alone |
| golangci-lint `-v` | 6.4 s | loader 2.0 s; analyzers 0.64 s (errset 0.57 s, effects 0.07 s); linter stage 3.7 s |
| `go vet -vettool` | 6.7 s | build of the package, plus the analysis |
| `effectgo ./internal/service` (no cache) | 22.8 s | loads and type-checks every dependency from source, then analyzes |

**What dominates.** Cold runs take about 100 s on kuzma with either tool; they are dominated by building the dependency graph into an empty cache (not profiled separately). On save, the analysis itself is small: errset is about 0.6 s on the 106-file package. The rest of the on-save cost is the loader: golangci-lint spends about 2 s in `packages.Load` on every run, even when nothing changed, and go vet re-builds the touched package. The standalone run is the slowest because it has no cache and loads the dependency graph from source; it is not the editor path.

**Proposed fixes** (not implemented here):
1. Stop the loader from re-listing the whole graph on each save: a persistent `go list -export` cache keyed by module state would remove most of golangci-lint's 2 s floor. This belongs in golangci-lint, not in the plugin.
2. Make errset's per-package analysis incremental: hash the SSA of each function and reuse the error sets of unchanged functions across runs (the on-save analysis is already per package; this would cut the 0.6 s on large packages).
3. Ask golangci-lint for a plugin salt (an analyzer-supplied cache key input) so that project stub edits invalidate its cache. Until then, the editor recipe must mention `golangci-lint cache clean` after a stub edit, or the user can run `go vet` for the stub-sensitive checks.
4. For the editor, prefer package scope (the default). File scope is broken for multi-file packages, see the matrix.

**Limitations.**
- golangci-lint's result cache is salted with the binary version, the lint settings and go.mod, not with stub file contents. A project `effectgo.stubs` edit (or an explicit stub edit) is stale until `cache clean`. Verified above; `go vet` is not affected.
- The vet identity covers the project file of the module containing the working directory (where `go vet` is started), the explicit `$EFFECTGO_STUBS` files, and the shipped files. A project file of another module below that directory, and a `-stubs` file (not in `$EFFECTGO_STUBS`), are not in the ID; use `-a` after such an edit.
- The plugin resolves relative `stubs` paths against the module root of the working directory, not against the config file. For a config outside the module, give absolute paths.
- The zero configuration loads all five shipped stub files. This is intended, but it changes results compared with `stdlib.stubs`-only runs (see above).
- `examples/editor/.custom-gcl.yml` builds the plugin from the checkout (`path: ../..`); the Go-proxy route is documented but unverified.
- The README's `sig` example shows `{unknown}` for `v3Update`; the current binary (and the base commit) prints `{}` for `examples/todo`. This snippet was already stale and was not changed here.
- Plugin reports are golangci-lint diagnostics: the `effects of` / `errors of` / witness lines, which are report-only, are not available in the editor.

