# VS Code

Two hooks run effectgo through the Go extension (`golang.go`). Use one or both.

## golangci-lint v2 with the plugin (contract diagnostics on save)

Build the custom binary first (see [README.md](README.md)), then in the user or
workspace settings:

```jsonc
{
  "go.lintTool": "golangci-lint-v2",
  "go.alternateTools": {
    "golangci-lint-v2": "/abs/path/to/checkout/bin/custom-gcl"
  },
  "go.lintOnSave": "package"
}
```

- `go.lintOnSave: "package"` (the default) is the scope that works. The
  extension then runs golangci-lint in the file's directory with no pattern,
  which lints that package and its subdirectories.
- Do not use `"file"`: the extension passes the file path, and a single file
  of a multi-file package does not type-check (verified: `undefined: WithTx`).
- Put the `.golangci.yml` of the module root in the workspace; add
  `"go.lintFlags": ["--config=/abs/path/.golangci.yml"]` only if it is elsewhere.

What the extension runs (from `extension/src/diagnostics/goLint.ts`, golangci-lint v2 branch):

```
custom-gcl run [lintFlags] --issues-exit-code=0 --output.text.print-issued-lines=false \
    --show-stats=false --output.text.path=stdout --path-mode=abs [./... | <file> | (no pattern)]
```

Verified here: the same invocation on the corpus, with package scope and with
workspace scope, prints the 12 diagnostics (`file:line:col: effects: ... (effectgo)`),
which the extension's line parser accepts.

## go vet with the effectgo vettool (on save, standard go vet)

```jsonc
{
  "go.vetOnSave": "package",
  "go.vetFlags": ["-vettool=/abs/path/to/effectgo"]
}
```

`go vet -vettool=…` runs only the tool's analyzers, so the standard vet checks
are not run through this hook. No stub configuration is needed (zero
configuration). The extension runs `go vet -vettool=… .` for package scope; the
same command on the corpus prints the `effectgo` diagnostics with exit code 1.

Build `effectgo` with `go build -o /abs/path/effectgo ./cmd/effectgo` in the checkout.

## Not verified

- No VS Code was run. The settings keys (`go.lintTool`, `go.alternateTools`,
  `go.lintOnSave`, `go.vetOnSave`, `go.vetFlags`, `go.lintFlags`) and the
  behaviour of `alternateTools` for the `golangci-lint-v2` tool name are taken
  from the vscode-go sources, not observed in an editor.
- The problem matcher and the on-save timing in the editor were not observed.
