---
title: Contract checks
description: The unreleased shared report for static analyzers and HTTP route checks.
---

# Contract checks

`effectgo contract` is the shared report command under development after the `v0.1.0` release. It loads each package once, runs the six static analyzers (`effects`, `errset`, `mustuse`, `exhaustive`, `schematag`, and `directive`), and runs the Gin and Huma checks over the same load. Its default package pattern is `./...`.

The released commands remain available and keep their existing output and exit behavior. Use `effectgo ./...` or `go vet -vettool=$(which effectgo) ./...` for the released native analyzer paths; use `http-gin`, `http-huma`, and `http` when you need their existing standalone reports. Contract mode is unreleased and is documented here so CI integrations can converge on one wire format.

## Command

```text
effectgo contract [-format=text|json] [-all] [-fail-on=none|high|medium|leak|info] [-fail-unresolved] [-coverage=json] [analyzer flags] [patterns]
```

`-format=text` is the default. Both formats write their report to stdout. JSON is one document, not the legacy finding stream. The command analyzes production packages only. `-all` adds HTTP `info` findings; use `-effects.all` and `-errset.all` separately to show native root-rule duplicates. The default `-fail-on=leak` fails on HTTP `high`, `medium`, and `leak` findings. `-fail-on=none` disables only the HTTP severity gate. `-fail-unresolved` additionally gates detected unresolved HTTP routes. `-coverage=json` writes coverage to stderr; JSON format already embeds it. The [native analyzer flags](../reference/cli.md#analyzer-flags) keep their existing meanings; `-stubs` selects the existing stub configuration.

HTTP coverage reuses the Gin and Huma adapters' route collections. The Huma adapter also reads shared route stubs, so an `rt/web` registration can appear there; without an applicable `errdefault` model it is explicitly unresolved. These counters are not a separate proof of complete native `rt/web` or OpenAPI coverage. Native analyzer diagnostics do not contribute route counts. Gin counts registrations whose handler resolves; variable handlers and wrappers can be outside the collected set. Unresolved paths, group prefixes, and missing default mappings are recorded when detected. A zero unresolved count does not prove every registration was collected. Huma includes its resolved routes and variant status counters.

## JSON report

The top-level object is:

```json
{
  "schema_version": 1,
  "diagnostics": [],
  "coverage": {
    "packages": 1,
    "hidden_diagnostics": 0,
    "gin": {"framework": "gin", "routes": 0, "resolved": 0, "unresolved": 0, "findings": {}, "limit": "Gin route counts include only registrations with resolved handlers; variable handlers and wrappers can be outside coverage"},
    "huma": {"framework": "huma", "routes": 0, "resolved": 0, "unresolved": 0, "findings": {}}
  },
  "exit_code": 0
}
```

The exact counters are framework-specific. `packages` counts analyzed production packages. `hidden_diagnostics` counts native effects/errset violations suppressed by the root rule, not hidden HTTP `info` findings. `gin` and `huma` contain their existing coverage objects directly, including route, resolved, unresolved, finding and framework-specific counters. `exit_code` matches the process result after a successfully written report; usage, load, analysis or output failures do not promise a completed report.

Each diagnostic has a stable coarse `rule_id`, an `analyzer`, a `severity`, a source `position` (`file`, `line`, and `column`), and a human `message`. Native analyzer diagnostics use the analyzer name as their coarse rule family and severity `error`; consumers must not parse the message to classify them. HTTP diagnostics use `http.gin` or `http.huma` as their rule family and retain their existing severities (`high`, `medium`, `leak`, or `info`). HTTP records may also include `route`, `handler`, `variant`, `status`, and `via`.

Positions are relative to the working directory. A column of `0` means the adapter supplied only a line number, as the Gin adapter does; the report does not invent a column. Diagnostics are deduplicated and sorted by location and rule family with deterministic tie-breakers. Messages and HTTP witness text remain human-readable evidence, not a machine classification API. Adding a rule within a family does not create a new per-message identifier.

## Exit status

The shared command uses exit 0 when there are no native findings and no selected HTTP policy is violated, exit 1 for load, analysis or output errors, exit 2 for usage errors such as an invalid format, threshold or flag, and exit 3 when native diagnostics exist or an HTTP gate is violated. Native diagnostics, including explicitly requested analyzer report output, always produce exit 3; `-fail-on` does not control them. A report can contain advisory HTTP findings and still exit 0 when its policy permits them. `-coverage=json` alone never changes the exit status. `-h` prints usage and exits 0.

For real generator and migration commands, see the [framework example](../../examples/web/README.md). It shows `effectgo ts`, `go test` for the migration contract, and the `rt/web` mounting API. Gin mounting is documented only where a project provides an actual Gin registration; this example uses `rt/web` and does not establish a Gin mounting command.

## Local validation

The real combined fixture produced 11 diagnostics spanning all six native rule families
and both HTTP adapters, with exit 3 even under `-fail-on=none`. The HTTP-only fixture
exited 0 under `-fail-on=high` and 3 under `-fail-on=none -fail-unresolved`.
The web example exited 0 with no diagnostics and two explicitly unresolved routes.

Root build/vet, 16 Node checks, runtime race tests, all four example modules,
generated-file consistency, and TypeScript typechecking passed. The OpenAPI, CLI,
generator, and error-set packages passed all 119 top-level tests through the existing
per-test race isolation runner, with strict cross-language dependencies required.
The original `release-check.sh` run stopped at the OpenAPI package's default ten-minute
timeout; the isolated runs and remaining phases completed separately. Its original
whole-script result remains a timeout.

