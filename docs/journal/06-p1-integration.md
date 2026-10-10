---
title: "P1: integration and merge"
description: The merge of the P0 and P1 branches, the commands and results, and the vet cache proof.
---

## Merge

Everything below was produced from the final tree by `examples/run.sh` and by the commands shown. The prototypes in `proto-c/` were only read, and their binaries and test caches were not modified.

The module is `github.com/faintghost/effectgo`. Formerly goeffect: the two prototypes' module path, now retired.

### Commands and results

Build, vet, tests, gofmt and the grep check (`go build ./... && go vet ./... && go test ./... -count=1` at the root, plus `examples/todo` and `examples/layers`):

```text
== go build ./... && go vet ./... && go test ./... -count=1
?   	github.com/faintghost/effectgo/cmd/effectgo	[no test files]
ok  	github.com/faintghost/effectgo/effects	40.214s
ok  	github.com/faintghost/effectgo/errset	48.888s
?   	github.com/faintghost/effectgo/internal/stubs	[no test files]
ok  	github.com/faintghost/effectgo/signature	64.556s
ACCEPT_RC=0
== gofmt -l .
GOFMT_RC=0
== grep -rn goeffect .
GREP_RC=0
== examples/todo go test ./...
ok  	github.com/faintghost/effectgo/examples/todo/api	0.159s
?   	github.com/faintghost/effectgo/examples/todo/cmd/server	[no test files]
ok  	github.com/faintghost/effectgo/examples/todo/domain	0.070s
?   	github.com/faintghost/effectgo/examples/todo/experiments/ignored	[no test files]
?   	github.com/faintghost/effectgo/examples/todo/httpres	[no test files]
ok  	github.com/faintghost/effectgo/examples/todo/result	0.056s
TODO_TEST_RC=0
== examples/layers go vet ./...
LAYERS_VET_RC=0
== bin/effectgo -V=full
```

Full `go test` output for the root shows 29 tests: 14 in `effects` (the prototype's 14), 14 in `errset` (the prototype's 12 plus 2 in `stubs_test.go`), and 1 in `signature`. `go test ./...` in `examples/todo` passes all six packages.

`grep -rn goeffect .` finds only the two explicit "formerly goeffect" notes (README and NOTES). `gofmt -l .` prints nothing.

### examples/run.sh

```text
== layers, standalone: effectgo -stubs=stubs/stdlib.stubs ./...
/root/pp/go-rust/effectgo/examples/layers/repo/repo.go:32:20: (*HTTPRepo).Get violates contract of (Repo).Get [allow db]: has effect net via (*HTTPRepo).Get → http.Get
/root/pp/go-rust/effectgo/examples/layers/domain/todo.go:31:6: Normalize is declared //effectgo:pure but has effect time via Normalize → infra.Stamp → time.Now
/root/pp/go-rust/effectgo/examples/layers/domain/todo.go:36:6: Save is declared //effectgo:package pure but has effect net via Save → infra.Fetch → http.Head
/root/pp/go-rust/effectgo/examples/layers/domain/todo.go:44:6: Load is declared //effectgo:package pure but has effect db via Load → repo.(Repo).Get [contract]
exit 3

== layers, go vet: go vet -vettool=bin/effectgo -stubs=stubs/stdlib.stubs ./...
repo/repo.go:32:20: (*HTTPRepo).Get violates contract of (Repo).Get [allow db]: has effect net via (*HTTPRepo).Get → http.Get
domain/todo.go:31:6: Normalize is declared //effectgo:pure but has effect time via Normalize → infra.Stamp → time.Now
domain/todo.go:36:6: Save is declared //effectgo:package pure but has effect net via Save → infra.Fetch → http.Head
domain/todo.go:44:6: Load is declared //effectgo:package pure but has effect db via Load → repo.(Repo).Get [contract]
exit 1

== layers, standalone with -effects.report (first 6 lines)
/root/pp/go-rust/effectgo/examples/layers/infra/infra.go:10:6: effects of Fetch: {net}
/root/pp/go-rust/effectgo/examples/layers/infra/infra.go:19:6: effects of Stamp: {time}
/root/pp/go-rust/effectgo/examples/layers/repo/repo.go:21:19: effects of (*SQLRepo).Get: {db}
/root/pp/go-rust/effectgo/examples/layers/repo/repo.go:32:20: effects of (*HTTPRepo).Get: {net}
/root/pp/go-rust/effectgo/examples/layers/repo/repo.go:32:20: (*HTTPRepo).Get violates contract of (Repo).Get [allow db]: has effect net via (*HTTPRepo).Get → http.Get
/root/pp/go-rust/effectgo/examples/layers/domain/todo.go:17:6: effects of Sum: {}

== todo, standalone: EFFECTGO_STUBS=stdlib,fuego effectgo ./...
/root/pp/go-rust/effectgo/examples/todo/api/v3.go:24:6: handler v3Create may return an unclassified error (via (Context).Body [stub]), which errmap httpres.ToHTTPError does not map → HTTP 500
/root/pp/go-rust/effectgo/examples/todo/api/v3.go:53:6: handler v3Update may return domain.ErrConflict (via Service.Update → Store.checkTitle), which errmap httpres.ToHTTPError does not map → HTTP 500
/root/pp/go-rust/effectgo/examples/todo/api/v3.go:53:6: handler v3Update may return an unclassified error (via (Context).Body [stub]), which errmap httpres.ToHTTPError does not map → HTTP 500
exit 3

== todo, go vet: go vet -vettool=bin/effectgo -stubs=stdlib,fuego ./...
api/v3.go:24:6: handler v3Create may return an unclassified error (via (Context).Body [stub]), which errmap httpres.ToHTTPError does not map → HTTP 500
api/v3.go:53:6: handler v3Update may return domain.ErrConflict (via Service.Update → Store.checkTitle), which errmap httpres.ToHTTPError does not map → HTTP 500
api/v3.go:53:6: handler v3Update may return an unclassified error (via (Context).Body [stub]), which errmap httpres.ToHTTPError does not map → HTTP 500
exit 1

== todo, go vet with -errset.ignoreopaque (drops the opaque body findings)
api/v3.go:53:6: handler v3Update may return domain.ErrConflict (via Service.Update → Store.checkTitle), which errmap httpres.ToHTTPError does not map → HTTP 500
exit 1

== todo, signatures: effectgo sig ./... (EFFECTGO_STUBS)
/root/pp/go-rust/effectgo/examples/todo/domain/service.go:42:19: Service.Update: Effect<Todo, ErrConflict | ErrInvalid | ErrNotFound, {}>
/root/pp/go-rust/effectgo/examples/todo/domain/todo.go:62:17: Store.modify:   Effect<Todo, ErrNotFound | param:1, {param:1}>
/root/pp/go-rust/effectgo/examples/todo/api/v3.go:53:6: v3Update:       Effect<domain.Todo, domain.ErrConflict | domain.ErrInvalid | domain.ErrNotFound | fuego.BadRequestError | fuego.HTTPError | ?, {unknown}>

== check: the normal run prints no signatures
lines with Effect<: 0
RUN_RC=0
```

### effectgo sig on examples/todo (with the shipped stubs)

The full output is 89 lines, all `Effect<` lines, with no contract or HTTP lines. The lines for the functions named in the task:

```text
/root/pp/go-rust/effectgo/examples/todo/domain/service.go:42:19: Service.Update: Effect<Todo, ErrConflict | ErrInvalid | ErrNotFound, {}>
/root/pp/go-rust/effectgo/examples/todo/domain/todo.go:62:17: Store.modify:   Effect<Todo, ErrNotFound | param:1, {param:1}>
/root/pp/go-rust/effectgo/examples/todo/api/v3.go:53:6: v3Update:       Effect<domain.Todo, domain.ErrConflict | domain.ErrInvalid | domain.ErrNotFound | fuego.BadRequestError | fuego.HTTPError | ?, {unknown}>
```

### Vet cache identity: the proof

Run order: the same `GOCACHE` is used throughout, and it is not cleared between the runs. The stub edit removes `,?` from the Fuego `Body` rule in a copy of `fuego.stubs` (the copy is in a scratch directory; the shipped file is not changed).

```text
== cache A (EFFECTGO_STUBS)
A_RC=1
== cache B (same GOCACHE, stub edited, no clearing)
B_RC=1
-- diff A B
1d0
< api/v3.go:24:6: handler v3Create may return an unclassified error (via (Context).Body [stub]), which errmap httpres.ToHTTPError does not map → HTTP 500
3d1
< api/v3.go:53:6: handler v3Update may return an unclassified error (via (Context).Body [stub]), which errmap httpres.ToHTTPError does not map → HTTP 500
DIFF_RC=1
== cache C (-stubs flag only)
C_RC=1
D_RC=1
-- diff C D (expected: no difference, a limitation)
DIFF_RC=0
E_RC=1
-- diff C E (go vet -a re-runs the analysis)
1d0
< api/v3.go:24:6: handler v3Create may return an unclassified error (via (Context).Body [stub]), which errmap httpres.ToHTTPError does not map → HTTP 500
3d1
< api/v3.go:53:6: handler v3Update may return an unclassified error (via (Context).Body [stub]), which errmap httpres.ToHTTPError does not map → HTTP 500
DIFF_RC=1
```

Baseline, the prototype `errsetvet` with `GOEFFECT_STUBS`, same sequence: the output does not change after the stub edit (the stale result the fix addresses):

```text
== baseline: prototype errsetvet with GOEFFECT_STUBS, same sequence
BA_RC=1
BB_RC=1
-- diff A B (baseline; empty means stale)
DIFF_RC=0
api/v3.go:24:6: handler v3Create may return an unclassified error (via (Context).Body [stub]), which errmap httpres.ToHTTPError does not map → HTTP 500
api/v3.go:53:6: handler v3Update may return domain.ErrConflict (via Service.Update → Store.checkTitle), which errmap httpres.ToHTTPError does not map → HTTP 500
api/v3.go:53:6: handler v3Update may return an unclassified error (via (Context).Body [stub]), which errmap httpres.ToHTTPError does not map → HTTP 500
```

The version line that `go vet` reads (`effectgo -V=full`):

```text
effectgo version devel buildID=c425bdfce6f5e74ddaec0c778478683210a37a6732a60d4f65758af9c5b3ab1a
```

The hash changes whenever the binary or a stub file named in `EFFECTGO_STUBS` changes.

### Git

Two commits: the analyzer import, and the signature analyzer with the cache fix and the docs. Each was made with the existing git user configuration. Nothing was pushed, and no remote was added.


### Integration (P0+P1a+P1b)

The branches `fix/determinism`, `feat/driver-stubs` and `feat/report-tooling` are merged into `integrate/p1` with `--no-ff`. Conflicts: one textual conflict, in `errset/errset_test.go` (both sides add tests; both kept). `effects/effects.go`, `errset/errset.go`, `README.md` and `NOTES.md` merged without conflict markers; they were then reconciled by hand (stub file table, `fmt.Errorf` limits, the no-stub warning).

**Interaction tests** (each one passes and is listed with its expectation):
- **(a) witness through literals and stubbed func values**, `effects/testdata/src/interact` with `TestInteractWitness`. A literal passed to a callee that calls it reaches the effect through parameter polymorphism: `witness of Caller: time via Run → func literal (interact.go:18) → time.Now`, and `Outer` extends it with `Caller →`. A named func type stub gives `witness of ViaClock: time via (Clock) call [stub]`. A package-level func variable with a `pure` stub gives no witness, and a stored literal is not charged (`map[]`).
- **(b) immediately-invoked literal and fresh variants**, `errset/testdata/src/interact` with `TestInteractImmediateLiteral`. `func() error { return errors.New("x") }()` inside `Immediate` gives `new@interact.Immediate`: the literal's returns are its own set (driver-stubs rule), and the fresh variant is keyed by the declared function that runs the literal (report-tooling rule). The witness is `via (direct)`. A literal wrapped by `fmt.Errorf("%w", …)` gives the sentinel, not a fresh variant. A stored literal is a handler set of its enclosing function (`handler=new@interact.Stored`).
- **(c) TestMain clears EFFECTGO_STUBS vs. explicit stubs**: `effects` and `errset` `TestMain` clear the variable; tests that load shipped files set them explicitly (`TestShippedStubs`, `TestFuncValueStubs`, `TestStubsEnvFallback`), and `TestAllShippedStubFilesViaEnv` sets the four shipped files through the variable after `TestMain` cleared it. Load succeeds: no malformed line, no duplicate target across the files.
- **(d) foreign collapse vs. pgx stubs**, `errset/collapse_test.go` `TestShippedPgxStubsSurviveCollapse`. The variants named by the shipped `pgx.stubs` (`pgx.ErrNoRows`, `pgx.ErrTxClosed`, `pgx.ErrTxCommitRollback`) survive `collapseForeign`; an unexported driver internal collapses to `?`. This test found a naming bug: an unresolved `github.com/jackc/pgx/v5` was named `v5.ErrNoRows`; `shortPkg` now skips a major-version element.

**Stub-file warning** (`cmd/effectgo`): a standalone, `sig` or `openapi` run with neither `-stubs` nor `EFFECTGO_STUBS` prints one stderr line. Go vet probes and `.cfg` invocations are quiet. `TestNoStubsWarning`.

**Stdlib detection bug (resolved).** The earlier open problem (integrated `pct_panic` 59 to 70 against 2 to 10 for P1a) had one cause. `isStdlib` (`effects/effects.go`) and `isStd` (`errset/errset.go`) compared file paths with `build.Default.GOROOT`. That value comes from `$GOROOT`, or else from the analyzer binary's own toolchain. The shell exported `GOROOT=…/go/1.26.8` while `go list` resolved the standard library under `…/go/1.26.9`, so the check failed and the stdlib bodies were analyzed as user code: `strconv.Atoi → … → formatBits → panic`, `fmt.Errorf → … → reflect.Type.String`. The rows that were measured under that mismatch are superseded below.

**Fix.** `internal/goroot` decides stdlib from the source layout only. A package is stdlib iff its source directory is `<root>/src/<import path>` (or `<root>/src/vendor/<import path>`) and `<root>` has `src/runtime` plus a `VERSION` file or `pkg/tool` directory. The root comes from the file path, so the answer does not depend on `$GOROOT`, on the analyzer's build toolchain, or on `GOTOOLCHAIN`. Results are cached per root. `effects` checks the files of the package under analysis; `errset` checks the source files that declare the callee's objects (positions from the shared FileSet). No `go env` is run inside an analyzer. analysistest GOPATH testdata (`…/testdata/src/a`) has no such root and stays user code. Tests: `internal/goroot/goroot_test.go` (stdlib recognized with GOROOT unset, stale to another or a missing directory, or correct; testdata and the repository's own testdata non-stdlib; `unsafe` stdlib). The one-time "analyzed as user code" warning was not implemented: a dot-less module path such as `backend/...` would trigger it falsely, and nothing in the measurements needed it.

**Robustness proof** (kuzma-wms, config B, `-effects.report -errset.report`, env `GOFLAGS=-buildvcs=false GOPRIVATE=github.com/VectorSprint/*`, fresh GOCACHE per run). Sorted outputs of the fixed binary are identical for GOROOT unset, GOROOT=…/1.26.8 and GOROOT=…/1.26.9 (1690 lines each, md5 `98b1ea254657c0c69ecdbcb1143c5bb4`). The old integrated binary with the stale GOROOT reproduces the bug (pct_panic 67.0, pct_unknown 67.4, against 9.6 and 40.8 for the fixed binary; its sorted output has md5 `25523379…`, the value recorded in the earlier determinism smoke test, which therefore measured the bug and not determinism).

**Fair comparison.** Same environment for all rows: `env -u GOROOT GOFLAGS=-buildvcs=false GOPRIVATE=github.com/VectorSprint/*` and `EFFECTGO_STUBS`. The OLD baseline, P1a and P1b binaries run with `GOROOT=…/go/1.26.9` explicitly, because they still have the path bug. Baseline uses the `stdlib.stubs` of `feat/openapi`; P1a the four P1a stub files; P1b `effectgo-p1-tooling/stubs/stdlib.stubs`; integrated A `stdlib.stubs`; integrated B `stdlib,pgx,gin,huma`. Raw outputs and `metrics.jsonl` are in `runs/fix-stdlib/`. Percentages from `python3 -I metrics.py`; `avg_named_variants` = mean named variants per error function with a non-empty set.

order-mgmt (2201 functions, 854 with error sets)

| row | pct_unknown | pct_opaque | pct_only_opaque | avg_named_variants | pct_db | pct_panic |
|---|---|---|---|---|---|---|
| baseline | 54.3 | 95.7 | 38.6 | 4.06 | 0.0 | 31.1 |
| P1a | 30.3 | 95.2 | 41.5 | 1.48 | 30.2 | 2.2 |
| P1b | 54.3 | 93.1 | 30.2 | 2.62 | 0.0 | 31.1 |
| integrated A | 50.5 | 92.6 | 29.0 | 3.09 | 0.0 | 29.7 |
| integrated B | 30.3 | 92.6 | 33.8 | 1.87 | 30.2 | 2.2 |

kuzma-wms (1090 functions, 600 with error sets)

| row | pct_unknown | pct_opaque | pct_only_opaque | avg_named_variants | pct_db | pct_panic |
|---|---|---|---|---|---|---|
| baseline | 48.1 | 92.7 | 50.3 | 0.77 | 41.0 | 10.6 |
| P1a | 40.8 | 92.7 | 26.2 | 2.03 | 40.8 | 9.6 |
| P1b | 48.1 | 87.5 | 34.2 | 1.30 | 41.0 | 10.6 |
| integrated A | 42.5 | 87.5 | 16.5 | 2.59 | 40.0 | 9.9 |
| integrated B | 40.8 | 87.5 | 16.3 | 2.59 | 40.8 | 9.6 |

arcteryx-console (606 functions, 269 with error sets)

| row | pct_unknown | pct_opaque | pct_only_opaque | avg_named_variants | pct_db | pct_panic |
|---|---|---|---|---|---|---|
| baseline | 41.4 | 94.4 | 69.1 | 0.86 | 24.1 | 20.0 |
| P1a | 26.7 | 93.3 | 38.7 | 1.76 | 24.4 | 6.8 |
| P1b | 41.4 | 89.2 | 48.7 | 1.34 | 24.1 | 20.0 |
| integrated A | 27.2 | 88.1 | 24.2 | 2.24 | 24.4 | 7.8 |
| integrated B | 26.7 | 88.1 | 24.2 | 2.24 | 24.4 | 6.8 |

Reading the tables: integrated B now matches P1a on the panic and unknown shares for all three projects, and keeps P1a's `db` share on order-mgmt (30.2). Integrated A (stdlib stubs only) has a higher panic share on order-mgmt (29.7), which is not a stdlib effect: the pgx and gin bodies are then analyzed as user code, since their stubs are absent. This was not separately verified. P1b and the baseline are unchanged by the fix, as expected: their numbers were measured with the correct GOROOT. The `pct_only_opaque` direction and the `db` gains in P1a and B are unchanged from the earlier analysis.

**Witness top-10** (config B, fixed binary, `-errset.witness -effects.witness`; script `runs/fix-stdlib/witness_top10.py`, output `runs/fix-stdlib/witness_top10.txt`). Plain `?` last hops (1545 witnesses, all three projects): `(Row).Scan [stub]` 22.8%, `(Pool).Query [stub]` 6.6%, `(Pool).Begin [stub]` 5.5%, `(Pool).Exec [stub]` 4.2%, `sql.DB.BeginTx` 4.2%, `json.Marshal` 3.9%, `sql.DB.QueryContext` 3.6%, `sql.DB.ExecContext` 3.3%, `row.Scan` 2.7%, `sql.Tx.QueryContext` 2.1%. `unknown` last hops (1275 witnesses): `io.Closer.Close` 10.9%, `queryer.QueryRowContext` 5.3%, `scanner.Scan` 4.9%, `dynamic call GenerateOperationID` 3.8%, `io.ReadAll` 2.7%, `cipher.AEAD.NonceSize` 1.9%, `io.Writer.Write` 1.9%, `giftCardQuerier.Query` 1.6%, `huma.API.OpenAPI` 1.3%, `io.Copy` 1.3%. The old `reflect.Type.String`, `reflectlite.Type.Comparable` and `regexp.input.step` hops are gone, and no stdlib body (`formatBits`, `fmt.(*pp)`, `reflect.Type.String`) appears in any witness file. The remaining hops are project-level or driver-level (`queryer`, `scanner`, `giftCardQuerier`, dynamic calls) or stdlib API boundaries (`sql.DB.*`, `json.Marshal`, `io.Closer.Close`, `io.ReadAll`), which the analyzer treats as unknown or opaque by design. Per project: order-mgmt and arcteryx-console lead with `(Row).Scan`/`(Pool).*` and `io.Closer.Close`; kuzma-wms leads with `(Row).Scan` and `sql.DB.BeginTx`, and its `unknown` list is `io.Closer.Close`, `queryer.QueryRowContext`, `scanner.Scan`, `dynamic call GenerateOperationID`.

**Determinism.** Config B on kuzma-wms, three runs with fresh caches under three GOROOT settings: identical sorted outputs (md5 `98b1ea254657c0c69ecdbcb1143c5bb4`).

**Limitations.** Three projects, one run each per configuration, except order-mgmt A (three runs). P1a and P1b rows were not re-measured with the integrated binary's environment, so their panic numbers may not be comparable with the integrated rows. The stub warning is not emitted for go vet runs. Each row is one run; the fixed binary's results are deterministic across the GOROOT settings tested, but other toolchains were not tested.

