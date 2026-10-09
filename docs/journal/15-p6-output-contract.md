---
title: "P6: the output contract (root-only reporting, test files, the summary line)"
description: "Three changes to what effectgo prints: a contract violation is reported at its root, the standalone run analyzes test files by default, and the standalone run ends with a summary line. Design, tests, corpus, dogfood, oracle and the example outputs."
---

### Output contract (P6)

Branch `feat/output-contract`, from `feat/docs-public`. Three items from the backlog ([B1 to B3](../design/backlog.md), now removed from it). The evidence for each is in the backlog entry as it was: one uncontracted interface call printed five diagnostics, the standalone run skipped the test files that `go vet` analyzes, and a clean run gave no sign that it had analyzed anything.

**B1. Root-only reporting.** A function whose violation is only the consequence of a callee's violation is not reported. The callee is, and fixing it fixes the caller.

The effects analyzer. A violation of `F` for effect `e` is suppressed when every source of `e` in `F` is a call to a declared function that itself violates its own contract for `e` (`suppressed` and `violates` in `effects/effects.go`). A source is a direct effect of the body, a stub, an interface or dynamic call, a call with a literal argument to a callee without body information, or an argument-supplied effect (a placeholder). Any of those reports `F`. The fact `EffectFact` carries the list of effects that the function violates (`Violated`), so the rule works across packages.

The errset analyzer. A violation of `F` for variant `X` is suppressed when `X` reaches `F` through a declared callee that violates its own contract for `X` (the new `Variant.Via`, set when `bind` takes the variant from a declared callee), and every other call in `F`'s body that may return `X` is such a callee too (`suppressedVariant` in `errset/errset.go`). `ErrFact.Violated` carries the keys across packages. A handler function is not suppressed.

The rule is stricter than the decision in the orchestrator's brief, which said "the next hop of the witness is a violating function". The witness shows one path. A second source of the same effect that does not violate would keep `F` a real root after the callee is fixed, and the report would be lost until the next run. So every source must violate. Silence stays on the side of proof: a stub, a dependency without facts, the standard library and a func value are never suppressors.

The flag is `-effects.all` and `-errset.all` (standalone and under `go vet`); it turns off the rule for its analyzer. The hidden violations are recorded in `internal/hidden` by position and text, as the printer keys its de-duplication, so a file that belongs to a package and to its test variant, or a repeated analysis of one package, is counted once.

**B2. Test files by default.** The standalone run and `sig` analyze `_test.go` files (`-test`, default true), as `go vet` does. The package-level default still does not apply to a function in a test file (P5). The generated test main packages (`p.test`) are skipped: they hold no source.

**B3. The summary line.** The standalone run ends with one line on stderr, `effectgo: N packages analysed (M test variants), K findings`, with `, H duplicates hidden (...)` when the root rule hid some violations. It has no `path:line:col:` prefix, so a parser of the diagnostics does not read it as one. It is not printed under `go vet`, by the golangci-lint plugin, or by `sig`, `openapi` and the `http*` subcommands. The exit codes do not change.

The standalone run is no longer the multichecker. The multichecker exits after it prints, so it cannot print a line after the diagnostics. The new runner (`cmd/effectgo/standalone.go`) uses the same checker API as the subcommands and prints the diagnostics the same way. Its driver flags are `-test`, `-json` and `-c`, plus the analyzer flags; `-fix`, `-diff`, `-debug`, the profiling flags and `-trace` are not accepted by the standalone run (exit 2). Under `go vet` and for `-help`, the multichecker runs as before.

**Bugs found while verifying.** Three, all fixed before the numbers below. The package loads did not request `NeedForTest`, so the summary said `0 test variants` while the test files were analyzed. The count of findings counted diagnostics that the printer then drops as repeats, so it was too high on a package that is both a package and its test variant. The count of hidden violations was added per analysis run, which counted a package twice. Review also found that `namedFuncSet` kept a callee's `Via` on a variant it re-labelled; the first hop of a re-labelled variant is not the caller's, so `Via` is now cleared there.

**Tests.** `effects/testdata/src/suppress` (same-package and cross-package suppression, with `suppressdep`; the cases with no contract on the callee, an allowing contract, an interface call, a stub, a direct source, a second callee that does not violate), and `suppressall` for `-all` (`TestSuppressed`, `TestSuppressedAll`). `errset/testdata/src/root` and `rootall` (`TestRootSuppressed`, `TestRootSuppressedAll`), which also run under the AST engine. `chain` changed: its `Deep` violation is now the consequence of `Normalize`'s. `cmd/effectgo/standalone_test.go` builds the binary and runs it on a scratch module with a test file that has its own directive: `TestStandaloneAnalyzesTestFiles` (B2, and `-test=false`), `TestSummaryOnStandalone` (B3 clean and with a finding, the summary not matching the diagnostic shape, and no summary under `go vet`). `cmd/effectgo/main_test.go` covers the summary text and the test-main detection.

**Corpus and oracle.** The disciplined corpus gives TP 17, FN 0, FP 0 with the new binary; `check.py` ignores the summary line (it is one of the unparsed lines). No corpus EXPECT line was suppressed.

| run | before | after |
|---|---|---|
| disciplined corpus, zero config, `--future` | TP 17, FN 0, FP 0 | TP 17, FN 0, FP 0 |
| kuzma-wms backend, `http -json -all` (oracle) | absent 18/18, present 2/2 | absent 18/18, present 2/2 |
| kuzma-wms backend, standalone, default | no output (test files skipped) | 1 finding in a test file: an exhaustive report on a type switch over `go/ast.Expr` with no default, which `go vet` also reports |

**Dogfood and mutants** (`dogfood/bin/effectgo` against the new binary, default run on each directory; the diagnostic lines are the lines the run printed on stderr):

| case | old | new | note |
|---|---|---|---|
| reservations | 0 | 0 | |
| mutant 1 (`time.Now` two frames below `Hold`) | 3 | 1 | `Hold` and `stamp` are hidden; `roundToNow` is the root |
| mutant 2 | 1 | 1 | |
| mutant 3 | 1 | 1 | |
| mutant 4 | 1 | 1 | |
| mutant 5 | 1 | 1 | |
| mutant 6 (`log` in `service`) | 3 | 2 | `httpapi`'s two callers are hidden; the test file adds `TestSweepOnce`, which has its own directive |
| mutant 7 (goroutine in a contracted function) | 2 | 2 | `run` is hidden; `TestSweepOnce` is new (its own directive) |
| mutant 8 (control, silent) | 0 | 0 | |

**Examples.** `examples/run.sh` exits 0. Compared with the output before this change, the only difference is the summary line at the end of each standalone run (`layers`, `todo`, `editor`). No diagnostic changed, and the `go vet` and `sig` outputs are the same. The run also lists `test variants` for `todo`, which has test files and no new findings.

**Docs.** `docs/reference/cli.md` (the test default, the summary line and its scope, the two show-all flags, the standalone flag set), `docs/reference/analyzers.md` (the root rule with an example from a scratch module, and the flags), `AGENTS.md`, `docs/guide/getting-started.md`, `docs/guide/contract-mode.md`, `docs/guide/editor.md` (the `tests` setting of the example), and this entry. The backlog keeps B4 and B5.
