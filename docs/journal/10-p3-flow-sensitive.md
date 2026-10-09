---
title: "P3: flow-sensitive error sets"
description: "The SSA-based flow-sensitive error-set engine: design, guard narrowing, corpus, oracle, output changes, metrics and performance."
---

### Flow-sensitive error sets (P3)

Branch `feat/flow-sensitive`, from `integrate/p2` (25aa60c). Commits: M1 `9a0797e` (SSA values), M2 `ea09f03` (guard narrowing), M3 `84fadd6` (per-use adapters, flag, performance), M4 (default engine, documentation).

**Design.** The error set of a value is computed from its definition, not from the union of the variable's assignments.

1. *Lowering.* Each package is lowered to `go/ssa` inside its pass (`errset/ssa.go`, `buildSSA`): `ssa.NewProgram(fset, BareInits)`; the direct imports are created as synthetic packages (members without bodies, so calls and globals resolve); the analyzed package is created, put in debug mode (`SetDebugMode`, which records a `DebugRef` per evaluated expression), and built with `Package.Build`. x/tools v0.51 offers no per-function build, so the whole analyzed package is lowered; the program is dropped at the end of the pass (`p.ssaProg = nil`). Generic function bodies are not lowered by `ssa`; their units keep the AST engine's result (the fallback is per unit, with the same shared fact and set tables).
2. *Values.* Per SSA value of a type that is an error or an interface (`carriesError`): a `Call` or `Extract` takes the callee's set (with `param:i` bound to the arguments, as in the AST engine); a load of a sentinel global is the sentinel; `nil` is empty; `MakeInterface` of a concrete error type T (or *T) is T; `ChangeInterface`/`ChangeType` pass the set through; `TypeAssert` is the asserted type; a `Phi` is the union of its incoming values, each taken at the end of its predecessor on that edge; `fmt.Errorf` with a constant format takes the `%w` arguments (read from the variadic `Alloc` and its `IndexAddr` stores), without `%w` it is a fresh `new@`; `errors.Join` is the union of its arguments; a parameter is `?` (or `param:i` for a func value that is called). Anything unresolved is `?`.
3. *Captured variables.* A variable that is captured or address-taken (an `Alloc`, or a `FreeVar` of a literal, resolved to its `Alloc` through the `MakeClosure` binding) has the union of every value stored into it, in the declared function and in its literals. This covers the named result captured by a deferred closure (`CreateBatch`): a load of the result after `RunDefers` is the union of the returns and the closure's stores (conservative).
4. *Environments and narrowing* (`errset/ssaflow.go`, `errset/guards.go`). Each block has an entry environment: the sets of the values that every path to the block narrowed. The environment of an edge `p → b` is the entry environment of `p`, without the values that `p` defines, plus the narrowing of `p`'s branch on that edge. A join keeps a narrowing only when every evaluated predecessor narrows the value. A use reads the environment of its block (the narrowing happens only at block ends), so a `Phi` takes each incoming value with its edge environment. The environment is recomputed in each round; the sets only grow, and the environments are monotone in them.
5. *Returns.* A function's set is the union, over its `Return` instructions, of the effective set of the error result. A literal's set is its own returns; a returned literal (`MakeClosure`, or a bare `*ssa.Function` when it captures nothing) gives the handler set.
6. *Fixed point.* Within a unit (the declared function and its literals, outer before inner), rounds repeat until no set changes (at most 200 rounds; a unit that does not converge falls back to the AST engine). Across the package, a unit is re-evaluated only when a same-package callee's set changed since the unit last read it: `work.ver` counts the changes of each function's sets, and `ssaUnit.deps` records the versions read. Sets only grow, so a unit's state persists across the package rounds.
7. *Per-use answers.* After the fixed point, each unit's expression uses (its `DebugRef`s with an error-typed value) are turned into a `flowTable` of visible variants: `byExpr` for each use, and `byObj`, the union over the uses of each variable (the fallback for an identifier that has no `DebugRef`). `Flow.Of(e)` reads the table; the SSA program is not kept after the pass, so `Result` holds only sets (order-mgmt: see "Performance").

**Narrowing rules.** On the true (and, by `!`, the false) edge of a branch whose condition is one of these forms, the subject value `v` is narrowed; a "wrapper" is a concrete type or sentinel with an `Unwrap`, `Is` or `As` method, and an "unknown origin" is an opaque (`?`) or param variant or an `iface:` variant:

| condition | true edge | false edge |
|---|---|---|
| `errors.Is(v, S)` | keep `S`, wrappers, unknown origins | remove `S` |
| `v == S` | keep `S`, unknown origins (no wrappers: `==` does not unwrap) | remove `S` |
| `v == nil` | empty (the nil edge carries nothing) | no change |
| `errors.As(v, &x)`, `x` of concrete type `T` | keep `T`, wrappers, unknown origins | remove `T` |
| `errors.As(v, &x)`, `x` of interface `I` | keep the variants whose methods implement `I` (a fresh error only if `I` is `error`), wrappers, unknown origins | remove the known variants that implement `I` |
| `w, ok := v.(T)` (comma-ok) | as `errors.As` for `T`, without wrappers | as `errors.As` for `T` |
| `P(v)`, `P` a predicate with a `TestFact` | the test that `P` names, on `v` | the same, negated |

A *predicate* is a `func(error) bool` whose every return is a constant false, a call of `errors.Is`/`errors.As` on its parameter (or of another predicate), or the constant true reached only through the true edge of such a test. Its `TestFact` (`Is`, `Key`, `Iface`, `Methods`) is exported to the importing packages (`errset.TestFact` is registered as a fact type of the analyzer). `IsValidation` in the corpus and `service.IsValidationError` in kuzma-wms are predicates of this kind. `switch` statements are If chains and need nothing else.

**Gin and Huma (adapters).** The gin rule engine reads each rule and default at its own use, through `Flow.Of`: a variant rule answers the variants that its branch matches at its own identifier; a catch-all default (`if err != nil { sink }`, an `else`, a `switch` default, or the sink after an if-return) answers the variants at its own use. A sink after an if-return that does not name the variable reads the guard's identifier (variants that the guard maps keep their status). The previous engine read every variant of the variable at its first use, so a default could apply to a variant that another call had produced; `registerUser` in order-mgmt showed it (see "Output changes"). Huma's routes and handler-literal sets come from `Result.Funcs` and `Result.Literals`, which are the SSA sets under this engine; no Huma code changed. Huma's own `PredicateFact` (the mapping evidence of `adapters/huma/evidence.go`) is kept: it records "a condition tests T and returns a status below 500", which is not a narrowing fact. The errset `TestFact` does the narrowing; the two are separate facts of separate analyzers (a deviation from the guidance, which suggested reusing it).

**Selection and defaults.** `-errset.engine=ssa|ast` on the standalone run (`effectgo -errset.engine=...`) and on `sig`, `openapi`, `http`, `http-gin`, `http-huma` (`cmd/effectgo`: a flag value that sets the analyzer's engine; `sig` reads it before the multichecker because the multichecker prefixes its analyzer flags). The default is `ssa`, switched after this acceptance. `ast` is the P2 engine, unchanged; `TestASTFlowInsensitiveLimit` pins it on its one known false positive.

**Tests.** The errset suite passes under both engines: the default run, and `EFFECTGO_TEST_ENGINE=ssa go test ./errset` (the hook is in `TestMain` of errset, adapters/gin and adapters/huma). New fixtures: `errset/testdata/src/ssaflow/flow.go` (reassignment, `err = nil`, phi merge, `reuse`) and `guards.go` (each narrowing form, the wrapper rules, the predicate fact, the switch, the negation, the guarded reset and a callee guard), run by `TestSSAFlowSensitive`.

**Tests changed, and why.** One expectation encoded the flow-insensitive over-approximation: `limits.overwritten` (`err := ErrNotFound; if errors.Is(err, ErrNotFound) { err = nil }; return err`) reported `ErrNotFound`. It moved to `testdata/src/limitsast` with the AST expectation, and its test (`TestASTFlowInsensitiveLimit`) selects the AST engine; under the SSA engine the same function is `(none)` (`ssaflow.guardReset`). No other existing expectation changed: the whole errset suite, the gin suite and the huma suite pass unchanged under both engines.

**Disciplined corpus** (`corpus/disciplined`, `EFFECTGO_STUBS=stdlib.stubs`, `check.py`):

| engine | TP | FN | FP |
|---|---|---|---|
| ast (P2, baseline) | 12 | 0 | 8 |
| ssa | 12 | 0 | 0 |

The eight baseline false positives were `Update` and `Upsert` (`ErrNotFound` after `errors.Is`/`err = nil`), `Retitle` and `Relabel` (`ErrNotFound` and `ValidationError` after `errors.As` and the predicate), and `Remove` (after a `switch`). No EXPECT marker was wrong; none was changed.

**Oracle** (`runs/p3/p3-oracle.jsonl`, `runs/p3/oracle_check.py`). The checker matches a row by framework, by position (the finding's `pos` (route call site) or the handler position in its `handler` field, with a path-suffix test), and by variant (exact, or equal after a package qualifier). Calibration on the pre-P3 output (the P2 binary, built from 25aa60c, `runs/p3/pre/`): every row of both projects is found there, the 26 kuzma-wms rows and the 15 order-mgmt rows, including the `absent` and `present` ones (one row, `kuzma-wms` `entities.go:237 ErrDuplicate`, is written with the handler line of `POST /api/v1/operators`; the route call is at line 231, and the match uses the handler position). With the SSA engine (`runs/p3/post/ssa/`): all 24 `absent` rows are gone (18/18 kuzma-wms, 6/6 order-mgmt); all 3 `present` rows are still reported (2/2, 1/1); the 14 `either` rows are informational and all still reported.

| # | project | framework | position | variant | expect | cause (verdict) | pre (matched) | P3 SSA |
|---|---|---|---|---|---|---|---|---|
| 1 | order-mgmt | gin | giftinquiries/handlers.go:66 | pgx.ErrNoRows | absent | flow-reuse (false-positive) | reported (exact+pos) | gone → PASS |
| 2 | order-mgmt | gin | giftinquiries/order_completion.go:122 | pgx.ErrNoRows | either | aggregate-row (false-positive) | reported (exact+pos) | reported (exact+pos, high) → info |
| 3 | order-mgmt | gin | mall/cart.go:177 | pgx.ErrNoRows | absent | flow-reuse (false-positive) | reported (exact+pos) | gone → PASS |
| 4 | order-mgmt | gin | mall/cart.go:207 | pgx.ErrNoRows | either | aggregate-row (false-positive) | reported (exact+pos) | reported (exact+pos, high) → info |
| 5 | order-mgmt | gin | mall/cart.go:89 | pgx.ErrNoRows | absent | flow-reuse (false-positive) | reported (exact+pos) | gone → PASS |
| 6 | order-mgmt | gin | order_import/handlers.go:468 | pgx.ErrNoRows | absent | callee-guard (false-positive) | reported (exact+pos) | gone → PASS |
| 7 | order-mgmt | gin | order_import/handlers.go:468 | orderimport.ErrMailboxNotFound | present | narrow-race (likely-bug) | reported (exact+pos) | reported (exact+pos, high) → PASS |
| 8 | order-mgmt | gin | order_import/handlers.go:624 | pgx.ErrNoRows | either | aggregate-row (false-positive) | reported (exact+pos) | reported (exact+pos, high) → info |
| 9 | order-mgmt | gin | orders/writes.go:1331 | pgx.ErrNoRows | absent | flow-reuse-after-guard (false-positive) | reported (exact+pos) | gone → PASS |
| 10 | order-mgmt | gin | orders/writes.go:1540 | pgx.ErrNoRows | either | aggregate-row (false-positive) | reported (exact+pos) | reported (exact+pos, high) → info |
| 11 | order-mgmt | gin | orders/writes.go:285 | pgx.ErrNoRows | either | aggregate-row (false-positive) | reported (exact+pos) | reported (exact+pos, high) → info |
| 12 | order-mgmt | gin | readonly/handlers.go:101 | pgx.ErrNoRows | either | aggregate-row (false-positive) | reported (exact+pos) | reported (exact+pos, high) → info |
| 13 | order-mgmt | gin | readonly/handlers.go:191 | pgx.ErrNoRows | either | aggregate-row (false-positive) | reported (exact+pos) | reported (exact+pos, high) → info |
| 14 | order-mgmt | gin | readonly/order_completion_store.go:347 | pgx.ErrNoRows | absent | callee-guard (false-positive) | reported (exact+pos) | gone → PASS |
| 15 | order-mgmt | gin | wms/inventory_status.go:35 | pgx.ErrNoRows | either | seeded-row (false-positive) | reported (exact+pos) | reported (exact+pos, high) → info |
| 16 | kuzma-wms | huma | httpapi/outbound.go:60 | service.ErrInvalidTimezone | either | server-config (intended) | reported (exact+pos) | reported (exact+pos, high) → info |
| 17 | kuzma-wms | huma | httpapi/entities.go:415 | service.ErrNotFound | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 18 | kuzma-wms | huma | httpapi/shipment.go:98 | service.ErrNotFound | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 19 | kuzma-wms | huma | httpapi/entities.go:251 | service.ErrInvalidOperatorName | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 20 | kuzma-wms | huma | httpapi/entities.go:237 | service.ErrNotFound | either | infeasible-readback (false-positive) | reported (exact+handler) | reported (exact+handler, high) → info |
| 21 | kuzma-wms | huma | httpapi/inbound.go:362 | service.ErrNotFound | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 22 | kuzma-wms | huma | httpapi/inventory.go:92 | service.ErrNotFound | absent | flow-reuse (false-positive) | reported (exact+pos) | gone → PASS |
| 23 | kuzma-wms | huma | httpapi/entities.go:268 | service.ErrDeleteBlocked | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 24 | kuzma-wms | huma | httpapi/entities.go:166 | service.ErrNotFound | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 25 | kuzma-wms | huma | httpapi/entities.go:194 | service.ErrNotFound | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 26 | kuzma-wms | huma | httpapi/backups.go:199 | service.ErrNotFound | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 27 | kuzma-wms | huma | httpapi/backups.go:150 | service.ErrNotFound | either | aggregate-row (false-positive) | reported (exact+pos) | reported (exact+pos, high) → info |
| 28 | kuzma-wms | huma | httpapi/backups.go:210 | service.ErrNotFound | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 29 | kuzma-wms | huma | httpapi/entities.go:237 | service.ErrDuplicate | absent | guard (false-positive) | reported (exact+handler) | gone → PASS |
| 30 | kuzma-wms | huma | httpapi/entities.go:154 | service.ErrNotFound | either | aggregate-row (false-positive) | reported (exact+pos) | reported (exact+pos, high) → info |
| 31 | kuzma-wms | huma | httpapi/entities.go:251 | service.ErrNotFound | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 32 | kuzma-wms | huma | httpapi/entities.go:268 | service.ErrNotFound | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 33 | kuzma-wms | huma | httpapi/shipment.go:75 | service.ErrNotFound | either | aggregate-row (false-positive) | reported (exact+pos) | reported (exact+pos, high) → info |
| 34 | kuzma-wms | huma | httpapi/entities.go:251 | service.ErrDuplicate | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 35 | kuzma-wms | huma | httpapi/inbound.go:398 | service.ErrNotFound | absent | guard (false-positive) | reported (exact+pos) | gone → PASS |
| 36 | kuzma-wms | huma | httpapi/inbound.go:362 | service.errForecastFallbackUnavailable | absent | callee-guard (false-positive) | reported (exact+pos) | gone → PASS |
| 37 | kuzma-wms | huma | httpapi/auth_admin.go:217 | service.ErrNotFound | present | no-guard (likely-bug) | reported (exact+pos) | reported (exact+pos, high) → PASS |
| 38 | kuzma-wms | huma | httpapi/inbound.go:362 | service.validationError | present | no-guard (likely-bug) | reported (exact+pos) | reported (exact+pos, high) → PASS |
| 39 | kuzma-wms | huma | httpapi/auth_admin.go:294 | service.ErrNotFound | either | race (false-positive) | reported (exact+pos) | reported (exact+pos, high) → info |
| 40 | kuzma-wms | huma | httpapi/settings.go:82 | service.validationError | absent | predicate-guard (false-positive) | reported (exact+pos) | gone → PASS |
| 41 | kuzma-wms | huma | httpapi/auth.go:292 | service.ErrNotFound | absent | flow-reuse (false-positive) | reported (exact+pos) | gone → PASS |

Row 20 is listed in the oracle as the handler position; the matched finding's route call is at `entities.go:231`. Row 2's `either` finding is reported at `high` by the tool; the oracle's verdict is `false-positive` (aggregate row), so it stays a known false positive, reported as information in the table only.

**Output changes** (`runs/p3/post`, `effectgo http -json -all`; pre = the P2 binary). AST engine: byte-identical to pre on both projects (`runs/p3/post/ast`). SSA engine:

| project | pre (AST) high / medium / info | SSA high / medium / info | removed (SSA) | added (SSA) |
|---|---|---|---|---|
| kuzma-wms (huma) | 65 / 1 / 289 | 23 / 0 / 251 | 83 | 2 |
| order-mgmt (gin) | 83 / 136 / 300 | 48 / 78 / 274 | 119 | 0 |
| arcteryx-console (gin) | 0 / 5 / 49 | 0 / 3 / 49 | 4 | 2 |

Removed findings are the guarded and reassigned cases: 24 `service.ErrNotFound` and 5 `ErrDuplicate` (kuzma-wms), 19 `pgx.ErrNoRows` and 10 `strconv.NumError` (order-mgmt), and the 4 `strconv.NumError` of arcteryx-console, where `req, err := http.NewRequest…` replaces the `ParseInt` error that `if err != nil || id <= 0 { return 404 }` had already answered (`registration_invites.go:72-82`). Added findings, both from more precise reads:

- kuzma-wms `GET /api/v1/dashboard/trends` (info): `countByBusinessDate` is a closure bound to a local variable; the SSA engine resolves the call to the literal, whose `rows.Err()` is `context.Canceled`/`DeadlineExceeded`. The AST engine treats a call through a local variable as opaque.
- arcteryx-console `POST /api/v1/auth/register` (medium, `bcrypt.InvalidCostError` and `blowfish.KeySizeError`): `models.HashPassword` returns `InvalidCostError(cost)`, a conversion of the error type; SSA sees the type, the AST engine sees an opaque conversion. The findings are real under the model (a foreign variant reaching a 500 with no mapping), though a constant cost makes them infeasible in practice.

An intermediate version of the gin finalize applied a default to the variants at every use of its variable, and order-mgmt `registerUser` gained two medium `bcrypt` findings from another `err` value of the same handler (Go reuses one `err` object across `:=` statements). The final version reads each default at its own use, and the two are gone (order-mgmt `added 0` above).

`examples/run.sh` gives identical output under both engines (`runs/p3/examples-ast.txt`, `runs/p3/examples-ssa.txt`, and `effectgo sig` lines included).

**Metrics** (`metrics.py` on `effectgo -effects.report -errset.report -errset.engine=… ./...` with the four shipped stubs; the AST outputs are byte-identical to `runs/integrate2/*.report`):

| project | engine | non-test functions | pct_unknown | pct_pure | pct_db | pct_panic | error functions | pct_opaque | pct_only_opaque | avg_named_variants |
|---|---|---|---|---|---|---|---|---|---|---|
| arcteryx-console | ast | 606 | 26.7 | 46.4 | 24.4 | 6.8 | 269 | 88.1 | 24.2 | 2.24 |
| arcteryx-console | ssa | 606 | 26.7 | 46.4 | 24.4 | 6.8 | 269 | 88.1 | 27.1 | 2.10 |
| kuzma-wms | ast | 1090 | 40.8 | 40.6 | 40.8 | 9.6 | 600 | 86.3 | 14.7 | 2.61 |
| kuzma-wms | ssa | 1090 | 40.8 | 40.6 | 40.8 | 9.6 | 600 | 86.3 | 18.5 | 2.37 |
| order-mgmt | ast | 2201 | 30.3 | 47.2 | 30.2 | 2.2 | 854 | 92.6 | 33.8 | 1.87 |
| order-mgmt | ssa | 2201 | 30.3 | 47.2 | 30.2 | 2.2 | 854 | 92.6 | 35.2 | 2.05 |

The effects columns do not change (the effects analyzer is out of scope). The error-set columns change where the flow changes: `pct_only_opaque` rises because a set narrowed by a guard can be left with only its opaque part, and `avg_named_variants` moves with the removed reassigned variants.

**Performance** (`runs/p3/perf`; `/usr/bin/time` is not installed, so the wall time and the peak RSS come from a sampler on `/proc/<pid>/status` VmRSS, 0.2 s period, around `effectgo http -json -all ./...`, under `heavy.lock` and the 6G scope; the machine has 4 CPUs and the runs are noisy):

| project | engine | wall (s), runs | peak RSS (GB) |
|---|---|---|---|
| order-mgmt (148k lines) | ast | 11.9, 10.4, 15.1, 10.7, 16.7 (median 11.9) | 2.4–2.8 |
| order-mgmt | ssa | 14.7, 21.5, 20.8, 16.2 (median 18.5) | 3.35–3.55 |
| kuzma-wms (71k) | ast | 15.6, 12.6, 19.9 (median 15.6) | 2.7–3.0 |
| kuzma-wms | ssa | 27.0, 21.4, 20.5 (median 21.4) | 3.8–4.1 |

Target: wall ≤ 2× and within the 6G cap. The median wall ratio is 1.55 on order-mgmt (18.5 s against 11.9 s) and 1.37 on kuzma-wms (21.4 s against 15.6 s); the worst paired run is 1.94 (order-mgmt, 20.8 s against 10.7 s), so the 2× target holds on the medians and is close on the worst pair, on a noisy machine. Every SSA run stayed below the 6G cap; the largest peak is 4.06 GB. An earlier SSA version, before the flow table, peaked at 5.8 GB: the flow table, which releases the SSA program at the end of each pass, brought it to 4.4 GB, and the later changes to 3.4–4.1 GB.

Hot spots (`pprof` CPU profile, 15 s window on order-mgmt, before the last changes): the SSA build of the analyzed package was about half of the errset CPU time (5.5 s of 10.6 s: `Package.Build`, its initializer and `DebugRef` emission); `implementsError` (`types.Implements` on every value) was 11% of the SSA time, and is memoized now; the per-round environment copies were fixed by sharing the environment unless the edge narrows; and the re-evaluation of every unit in every package round was the largest factor: the wall ratio on order-mgmt fell from 2.5× (every unit each round) to 1.4× (dependency-driven rounds). Debug mode costs under 1 s of the 15 s; it is not the bottleneck. The remaining cost is the SSA build itself, which x/tools does not let us limit to some functions.

**Determinism.** Two SSA runs on kuzma-wms (`runs/p3/determinism/`): the raw outputs are byte-identical (280 lines), and the sorted outputs are identical.

**Limitations.**
- Generic function bodies are not lowered by `ssa` (x/tools v0.51 builds instances only); they keep the AST result.
- Lowering is per package, not per function, and it is paid in full by every analyzed package (about half of the errset CPU time in the profile above).
- A captured or address-taken error variable is flow-insensitive: the union of its stores, not its value at each point.
- A field, a map or slice element, a channel receive and a range value are opaque, as before; so are the calls through struct fields and through interface values with no contract or stub.
- Narrowing is per value and per edge; it does not relate two values (`if n > 0` and a later `if n <= 0` are not correlated), and it does not reason about `len`, `nil` slices or other non-error tests.
- A predicate's `TestFact` is read from a direct return or from the true edge of an `if` only; a predicate that calls a function outside the package or that tests through a field is not a predicate.
- A deferred closure that changes a named result contributes its stored values to the result (conservative); the value is not refined by the deferred code's own guards.
- Calls through a local variable holding a literal are resolved (an AST difference); calls through an interface value are contract, stub or opaque, as before.
- Only `errors.Is`, `errors.As`, `==` and type assertions are narrowing tests; `errors.Unwrap` chains, `reflect`, and `Is`/`As` methods on user types are not evaluated (their types are treated as wrappers when they declare the method).
- The per-use answers exist for error-typed expressions that `ssa` evaluates through a `DebugRef`; an expression without one falls back to its variable's union, then to `?`.
- The gin rule engine is still a rule engine: a sink that does not name the variable reads the guard's identifier, and rules and defaults are not composed across functions.

