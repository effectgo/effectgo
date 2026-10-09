---
title: Editor integration
description: Run effectgo diagnostics in VS Code and Neovim through a golangci-lint v2 plugin, or through go vet; plugin settings, stub behaviour, verification status and known limits.
---

effectgo's four analyzers can report while you write code. They run through golangci-lint v2, which has a module plugin system, and the editor recipes call golangci-lint. `go vet` with the effectgo vettool is the other route, and needs no plugin.

The plugin exposes `effects`, `errset`, `mustuse` and `exhaustive`. It does not expose the report flags (`-effects.report`, `-errset.witness`, …) or the HTTP subcommands: golangci-lint reports diagnostics only.

## 1. Build a golangci-lint binary with the plugin

The plugin is `github.com/faintghost/effectgo/plugin`. golangci-lint builds it into a custom binary with `golangci-lint custom`, from a `.custom-gcl.yml` in the current directory. The shipped example is `examples/editor/.custom-gcl.yml`:

```yaml
version: v2.14.0          # the golangci-lint version to build against
name: custom-gcl
destination: ../../bin    # a checkout path; the binary lands in ./bin (git-ignored)
plugins:
  - module: github.com/faintghost/effectgo
    import: github.com/faintghost/effectgo/plugin
    path: ../..           # the effectgo checkout; replace with `version: <tag>` outside it
```

```text
cd examples/editor && golangci-lint custom     # writes <checkout>/bin/custom-gcl
```

Outside the effectgo checkout, use `version: <effectgo tag>` instead of `path`. That route needs the module to be reachable through the Go proxy (or `GOPRIVATE`). It was not verified: the module is private, and it was built only from a local path.

## 2. Enable the plugin in the project

Put a `.golangci.yml` at the module root. The example is `examples/editor/.golangci.yml`. Only the plugin is enabled, and it has no required settings:

```yaml
version: "2"
run:
  tests: false              # effectgo ./... skips _test.go files by default; match it here
linters:
  default: none
  enable:
    - effectgo
  settings:
    custom:
      effectgo:
        type: module
        description: effectgo analyzers (effects, errset, mustuse, exhaustive)
        settings: {}
```

## 3. Stub files: none needed

The plugin uses the same zero configuration as the command line: the stub files shipped in the binary, plus `effectgo.stubs` at the module root when the module has one. To use a different set, pass `settings: { stubs: [...] }`. With `stubs` set, exactly those files are read.

| Setting | Default | Meaning |
|---|---|---|
| `stubs` | absent | explicit stub files, replacing the zero configuration; relative paths resolve against the module root of the working directory |
| `ignoreOpaque` | `false` | do not report opaque (`?`) and fresh (`new@`) errors in handler checks |
| `mustuseAllow` | empty | fully qualified functions whose results may be discarded, the list form of `-mustuse.allow`, for example `["(*os.File).Close"]` |

The settings are decoded strictly: an unknown key is an error.

## 4. Editor recipes

### VS Code

Two hooks run effectgo through the Go extension (`golang.go`). Use one or both.

**golangci-lint v2 with the plugin (contract diagnostics on save).** Build the custom binary first, then in the user or workspace settings:

```jsonc
{
  "go.lintTool": "golangci-lint-v2",
  "go.alternateTools": {
    "golangci-lint-v2": "/abs/path/to/checkout/bin/custom-gcl"
  },
  "go.lintOnSave": "package"
}
```

- `go.lintOnSave: "package"` (the default) is the scope that works. The extension runs golangci-lint in the file's directory with no pattern, which lints that package and its subdirectories.
- Do not use `"file"`. The extension passes the file path, and a single file of a multi-file package does not type-check (verified: `undefined: WithTx`).
- Put the module's `.golangci.yml` in the workspace. Add `"go.lintFlags": ["--config=/abs/path/.golangci.yml"]` only when it is elsewhere.

**go vet with the effectgo vettool (on save).**

```jsonc
{
  "go.vetOnSave": "package",
  "go.vetFlags": ["-vettool=/abs/path/to/effectgo"]
}
```

`go vet -vettool=…` runs only effectgo's analyzers, not the standard vet checks. No stub configuration is needed. Build the tool with `go build -o /abs/path/effectgo ./cmd/effectgo` in the checkout.

### Neovim (nvim-lint)

nvim-lint runs golangci-lint on save and publishes its JSON issues as diagnostics. It has a built-in `golangcilint` linter. Point it at the custom binary:

```lua
local lint = require("lint")
lint.linters_by_ft = { go = { "golangcilint" } }
lint.linters.golangcilint.cmd = "/abs/path/to/checkout/bin/custom-gcl"

vim.api.nvim_create_autocmd({ "BufWritePost" }, {
  callback = function() lint.try_lint() end,
})
```

nvim-lint's arguments for golangci-lint v2 turn every text output off, so the JSON parses. golangci-lint v2.14.0 has no language server, so nvim-lint is the route.

## Verification status

Each row was run in the build environment unless it says otherwise.

| What | How | Result |
|---|---|---|
| golangci-lint v2.14.0 | installed with `go install` | version 2.14.0 |
| custom build with the plugin | `golangci-lint custom -v` from the checkout | built; the plugin is imported |
| corpus, zero configuration | `custom-gcl run` against `effectgo ./...` | the same 12 diagnostics |
| corpus, `settings.stubs` | absolute `stubs/stdlib.stubs` | 12, identical |
| corpus, `ignoreOpaque: true` | against `effectgo -errset.ignoreopaque` | 11 and 11, identical |
| example module, zero configuration | `examples/editor`, golangci-lint and `go vet` | two diagnostics each, one from the project stub |
| project stub edit, golangci-lint | copy of the example, `Save db` changed to `pure` | stale: the cached run still reports; `custom-gcl cache clean` gives the correct result |
| project stub edit, go vet | the same copy, without `-a` | correct on the next run |
| VS Code, workspace scope | the extension's argument list, run on the corpus | 12 lines, parsed by the extension's regex |
| VS Code, package scope | the same, from the package directory | 4 lines for the package |
| VS Code, file scope | the same, one file as argument | broken: a file of a multi-file package does not type-check alone |
| Neovim arguments | nvim-lint's argument list | valid JSON, 4 issues parsed |
| `examples/run.sh` | the script | exit 0 |

Not verified: running VS Code or Neovim; `go.alternateTools` for the `golangci-lint-v2` name and the editor's problem matcher; nvim-lint's working directory; installing the plugin from the Go proxy; Windows paths.

## Latency

Measured on one machine, one run per row, wall-clock. The corpus has six packages; kuzma-wms's `internal/service` has 106 files.

| Case | `go vet -vettool` | golangci-lint with the plugin |
|---|---|---|
| corpus, cold cache | 19.1 s | 21.0 s |
| corpus, warm, nothing changed | 0.14 s | 0.65 s |
| corpus, warm, one file touched | 0.16 s | 1.16 s |
| kuzma `internal/service`, cold | 107.9 s | 101.1 s |
| kuzma `internal/service`, warm, nothing changed | 0.39 s | 0.73 s |
| kuzma `internal/service`, warm, one file touched | 4.33 s | 3.78 s |

On save, the analysis itself is small: errset takes about 0.6 s on the 106-file package. Most of the on-save cost is the loader: golangci-lint spends about 2 s in `packages.Load` on every run, and go vet rebuilds the touched package. Cold runs are dominated by building the dependency graph. The per-phase numbers and the proposed fixes are in the [editor journal](../journal/12-p4b-editor.md).

## Known limits

- **golangci-lint's cache does not see stub contents.** Its result cache is keyed by the binary, the lint settings and `go.mod`, not by stub file contents. After editing a project `effectgo.stubs` (or a stub file named in `stubs`), run `golangci-lint cache clean` once. `go vet` is not affected.
- **The vet identity has the same gap in one case.** The `go vet` identity covers the shipped files, the module's `effectgo.stubs` and the `$EFFECTGO_STUBS` files. A file given only by `-stubs`, or a project file of a module below the working directory, is not in it; run `go vet -a` after such an edit. See [CLI](../reference/cli.md#go-vet-and-the-cache).
- **File scope is broken for multi-file packages.** Use package scope, which is the default.
- **The zero configuration loads all five shipped stub files.** Framework rules apply only to packages that are present. On the corpus the result is the same as with `stdlib.stubs` alone, but numbers from a `stdlib.stubs`-only run are not comparable with zero-configuration numbers on other code.
- **Plugin reports are golangci-lint diagnostics.** The `effects of`, `errors of` and witness lines are not available in the editor. Use the command line for them.

## Related

- [Getting started](getting-started.md): the command-line route.
- [Stub files](../reference/stubs.md): what `effectgo.stubs` can contain.
- [Command line](../reference/cli.md): the same analyzers outside the editor, with their flags.
