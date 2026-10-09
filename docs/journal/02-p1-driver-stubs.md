---
title: P1: driver stubs and closure attribution
description: Why driver stubs, closure attribution and named func types were added, with the stub files, the measurements and the spot check.
---

### Driver stubs & closure attribution

**Why.** Real services were dominated by `unknown` and `?`. Three causes were measured in the real-world evaluation: driver interfaces (pgx v5, database/sql `Row`/`Rows`/`Result`), framework methods that reach gin's guarded panic and an interface render call, and closures whose effects were charged to the function that registers them (route handlers, struct fields, returned constructors).

**Stub files** (checked against module-cache sources; each rule cites file:line in a comment):

| file | covers | use it when |
|---|---|---|
| `stubs/stdlib.stubs` | database/sql `Row`/`Rows`/`Result`/`Tx` errors, `context.Context` (`Deadline`, `Done`, `Err`, `Value`; `Err` errors), `context.CancelFunc` (named func type), `io/fs` `FileInfo` and `DirEntry` getters, `http.ResponseWriter.Header` | always |
| `stubs/pgx.stubs` | `pgx.Row`, `pgx.Rows`, `pgx.Tx`, `pgx.BatchResults`, `*pgx.Conn`, `*pgxpool.Pool`: `db` and the errors (`pgx.ErrNoRows`, `pgx.ErrTxClosed`, `pgx.ErrTxCommitRollback`) | the service uses pgx v5 |
| `stubs/gin.stubs` | `*gin.Context` response writers (`net`), body readers (`net`), query/param/header readers and in-memory state (`pure`), `Error` (`pure`, nil panic not charged) | the service uses gin |
| `stubs/huma.stubs` | `huma.NewError`, a package-level func variable (`pure`, the default implementation only) | the service uses Huma v2 |

Load them with `EFFECTGO_STUBS` or `-stubs`, comma-separated and absolute. The order does not matter, but a rule that names the same target twice in one run is a duplicate error. Judgment calls, with the reason in the stub comments: `sql.Result` methods are pure (the driver contract describes values of a finished query); `sql.Rows.Scan` and `pgx.Rows.Scan` are `db` (they read the open cursor); `gin.Context.ShouldBind*` and `PostForm` are `net` (they read the body), while `BindQuery`, `BindUri`, `BindHeader` are pure (they read parsed values); `io.Closer.Close`, `io.ReadAll` and `DirEntry.Info` stay `unknown` or `fs` because the effect depends on the dynamic type.

**Named func types and package-level func variables (Feature A).** `effects (context.CancelFunc) pure` applies to every call through a value whose type is `context.CancelFunc`, so `cancel()` is no longer `unknown`. `effects pkg.Var pure` applies to calls through a package-level variable of func type (`huma.NewError`). The parser (`internal/stubs`) gives the named-type form `TypeOnly`; `errset` takes the same two forms (`errors (pkg.T) ...`, `errors pkg.Var ...`). **[test]** `effects/testdata/src/funcvals` (named type, variable, and an unstubbed func value that stays `param:0`); `errset/testdata/src/funcvals`; `internal/stubs` parser tests.

**Closure attribution (Feature B).** A literal is charged to its declared function only when the function runs it:
- called in place, `func(){...}()`, including under `defer` and `go` (a `go` still adds `spawn`);
- assigned once to a local variable, and that variable is called by code that runs: the function itself, or a literal the function runs. Any other assignment to the variable, or none, disqualifies it. A call made inside a literal that is only passed or stored does not make the local literal run.

Any other literal (passed, stored in a field, returned, sent) gets a state of its own, with the enclosing function's parameters. A literal argument then contributes its own effects through parameter polymorphism, when the callee calls that parameter; a callee that only stores it contributes nothing. A callee with no body information (stdlib without a stub, a dependency without facts, or a call through a func value) may run the literal, so the literal is charged there, as before. Interface methods are "known" too (their effects are the contract or `unknown`), so a literal passed to one is not charged separately. **[test]** `effects/testdata/src/closure` (immediate, deferred, go, local, reassigned local, passed, registered, stored, returned, stdlib `sort.Slice`, func-value parameter). The witness of a passed literal names it: `CallsLiteralClock → polylib.Modify → func literal (poly.go:39) → time.Now`.

**errset.** The return sets were already the function's own returns (`walkOwn`), so errset did not charge literal bodies to the encloser. The one fix: a literal called in place, `func() error {...}()`, now has its own returns as its error set; it was `?` before.

**Effect on `//effectgo:pure` and the tests.** A function that returns or stores a literal is now pure when the literal is not run. The tests that encoded the old attribution were updated: `effs.Closure` (returns a clock literal; now `map[]`), and the witnesses of `poly.CallsLiteralClock` and `poly.CallsRepeatClock` (they now name the literal).

**Measurement** (real-world snapshots, warm GOCACHE, `metrics.py` with GOCACHE testmain files counted as test). Baseline is `realworld-eval/baseline.jsonl` (stdlib.stubs config). "features" is the new binary with the baseline stub file, so it isolates the two features; "stdlib" is the new `stdlib.stubs`; "all" adds pgx, gin and huma. Every run set `EFFECTGO_STUBS` explicitly.

| project | config | unknown % | `?` % | db % | panic % | funcs |
|---|---|---|---|---|---|---|
| order-mgmt | baseline | 54.3 | 95.7 | 0.0 | 31.1 | 2201 |
| | features | 51.0 | 95.7 | 0.0 | 29.7 | 2201 |
| | stdlib | 50.5 | 95.2 | 0.0 | 29.7 | 2201 |
| | all | 30.3 | 95.2 | 30.2 | 2.2 | 2201 |
| kuzma-wms | baseline | 48.1 | 92.7 | 41.0 | 10.6 | 1090 |
| | features | 46.6 | 92.7 | 35.2 | 9.9 | 1090 |
| | stdlib | 42.5 | 92.7 | 40.0 | 9.9 | 1090 |
| | all | 40.8 | 92.7 | 40.8 | 9.6 | 1090 |
| arcteryx-console | baseline | 41.4 | 94.4 | 24.1 | 20.0 | 606 |
| | features | 30.9 | 94.4 | 23.3 | 7.8 | 606 |
| | stdlib | 27.2 | 93.3 | 24.4 | 7.8 | 606 |
| | all | 26.7 | 93.3 | 24.4 | 6.8 | 606 |

What did not move: the `?` share of error sets is nearly unchanged (92.7 to 95.2 on order-mgmt, where `?` is the default for interface calls and, at the time of this measurement, for `fmt.Errorf` without `%w`; that case is a fresh `new@pkg.F` variant now, see "Report tooling & error variants"). What did move is the set composition: the share of functions whose error set is only `?` fell on kuzma-wms (50.3 to 26.3) and arcteryx-console (69.1 to 38.7), and the average number of named variants fell on order-mgmt (4.06 to 1.48) because dependency-internal error types are now `?` under the pgx stubs. db on kuzma-wms drops with the features alone (41.0 to 35.2) because route-registration functions no longer inherit the database calls of their handler literals; that is the intended correction.

Spot check (seed 42, 5 changed effect sets per project, read against the source): 15 of 15 consistent with the code. The changes are: a pgxpool or database/sql method now `db`; an `unknown` removed because the call went through `context.Context.Value`, `os.FileInfo.Mode`, `sql.Result.RowsAffected` or a local closure; route-registration and handler-constructor functions losing the effects of literals they return or hand to a registrar. One case needs a caveat: a constructor that returns a struct of default implementations (`defaultRunnerDeps`) lost its `net`/`rand`/`spawn` effects, which now show as `unknown` at the field calls. That follows from the rule that a stored literal is not charged to its constructor.

