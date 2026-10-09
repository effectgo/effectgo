# Neovim (nvim-lint)

nvim-lint runs golangci-lint on save and publishes the JSON `Issues` as
diagnostics. It has a built-in `golangcilint` linter (`lua/lint/linters/golangcilint.lua`).
Point it at the custom binary:

```lua
local lint = require("lint")
lint.linters_by_ft = { go = { "golangcilint" } }
lint.linters.golangcilint.cmd = "/abs/path/to/checkout/bin/custom-gcl"

vim.api.nvim_create_autocmd({ "BufWritePost" }, {
  callback = function() lint.try_lint() end,
})
```

For golangci-lint v2 nvim-lint runs (from its `getArgs`):

```
custom-gcl run --output.json.path=stdout --output.text.path= --output.tab.path= \
    --output.html.path= --output.checkstyle.path= --output.code-climate.path= \
    --output.junit-xml.path= --output.teamcity.path= --output.sarif.path= \
    --issues-exit-code=0 --show-stats=false --path-mode=abs <directory of the buffer>
```

The directory is the buffer's package directory (`:h`) when a `go.mod` exists.
The `.golangci.yml` of the module (see [README.md](README.md)) enables the plugin.

Verified here: the same argument list on the corpus, with a `--config` for the
plugin, returns valid JSON with the four `service/` diagnostics (`FromLinter`
`effectgo`, `Pos.Filename`, `Pos.Line`, `Text`). nvim-lint parses the JSON with
`vim.json.decode`, so the text output must stay off, and the arguments above
turn it off.

golangci-lint v2.14.0 has no language server (`golangci-lint langserver` does
not exist in its source), so nvim-lint is the route.

## Not verified

- Neovim was not run. The linter name, the `cmd` override and `try_lint` are
  from the nvim-lint sources; the autocmd is the usual pattern.
- nvim-lint's working directory for the golangci-lint process was not observed.
  golangci-lint looks for `.golangci.yml` upward from its working directory, so
  the file must be found from there.
