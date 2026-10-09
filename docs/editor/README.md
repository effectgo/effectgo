# Editor integration

effectgo's contract analyzers (`effects`, `errset`) and its must-use and exhaustive analyzers (`mustuse`, `exhaustive`) can report while you write
code, through golangci-lint v2 and its module plugin system. The editor recipes
are [VS Code](vscode.md) and [Neovim](neovim.md). Both run the same thing: a
golangci-lint binary that contains the `effectgo` plugin.

## 1. Build a golangci-lint binary with the plugin

The plugin is `github.com/faintghost/effectgo/plugin`. golangci-lint builds it
into a custom binary with `golangci-lint custom`, from `.custom-gcl.yml` in the
current directory. The shipped example is `examples/editor/.custom-gcl.yml`:

```yaml
version: v2.14.0          # the golangci-lint version to build against
name: custom-gcl
destination: ../../bin    # a checkout path; the binary lands in ./bin (git-ignored)
plugins:
  - module: github.com/faintghost/effectgo
    import: github.com/faintghost/effectgo/plugin
    path: ../..           # the effectgo checkout; replace with `version: <tag>` outside it
```

```
cd examples/editor && golangci-lint custom     # writes <checkout>/bin/custom-gcl
```

Outside the effectgo checkout, use `version: <effectgo tag>` instead of `path`.
That route needs the module to be reachable through the Go proxy (or a
`GOPRIVATE` setting), which was not verified here: the module is private and
was only built from a local path.

## 2. Enable the plugin in the project

Put a `.golangci.yml` at the module root (the example is
`examples/editor/.golangci.yml`). Only the plugin is enabled; it has no required
settings:

```yaml
version: "2"
run:
  tests: false              # effectgo ./... skips _test.go files by default
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

The plugin uses the same zero configuration as the command line
(see the README "Stub files" section): the stub files shipped inside the binary,
plus `effectgo.stubs` at the module root when the module has one. To use a
different set, pass `settings: { stubs: [...] }` (relative paths resolve against
the module root of the working directory; the plugin then reads exactly those
files). The plugin settings are:

| setting | default | meaning |
|---|---|---|
| `stubs` | absent | explicit stub files, replacing the zero configuration |
| `ignoreOpaque` | `false` | do not report opaque (`?`) and fresh (`new@`) errors in handler checks |

Report flags (`-effects.report`, `-errset.report`, witness output) are not
available through the plugin: golangci-lint reports diagnostics only.

## 4. Editor recipes

- [vscode.md](vscode.md): the Go extension's golangci-lint and go vet hooks.
- [neovim.md](neovim.md): nvim-lint with the same binary.

What each recipe was checked against is listed in NOTES.md, "Editor
integration", with the exact commands.
