# effectgo: design notes

effectgo infers Effect-TS-style signatures — Effect<A, E, R> — for plain Go code. This file merges the design notes of the two prototypes, the effects analyzer (side effects, R) and the errset analyzer (error sets, E), and adds the signature analyzer that prints both. Formerly goeffect.

Tags used in the limitation lists:

- **[test]**: pinned by a test in `testdata/` or a `*_test.go` file.
- **[run]**: checked by running a command that is shown in the Merge section below (`examples/run.sh`, `go vet`, or the cache proof). It is not a Go test.
- **[reading]**: follows from the code and was not run.

## Layout

| Path | Purpose |
|---|---|
| `effects/` | the effects analyzer: taxonomy, inference, contracts, polymorphism, facts |
| `errset/` | the errset analyzer: error sets, contracts, errmap and handler check, interface variants |
| `mustuse/` | the mustuse analyzer: discarded errors and `//effectgo:mustuse` results (see "mustuse & exhaustive") |
| `exhaustive/` | the exhaustive analyzer: type switches on sealed interfaces and switches on sealed enums |
| `errset/status.go` | HTTP status inference for errmap branches (see "OpenAPI") |
| `errset/carried.go` | status-carrying errors (status-function rules), the default errmap, and the mapper branches (see "Huma adapter") |
| `errset/ssa.go`, `errset/ssaflow.go`, `errset/guards.go` | the SSA engine (default): lowering, per-value sets, the fixed point, guard narrowing, predicate facts (see "Flow-sensitive error sets (P3)") |
| `errset/flow.go` | the per-use views of the adapters (`Flow.Of`) |
| `adapters/huma/` | the Huma v2 route analyzer, its mapping evidence and the severity report (`effectgo http-huma`) |
| `openapi/` | the openapi analyzer (routes and their error docs) and the fragment, merge and check output |
| `signature/` | the `Effect<A, E, R>` analyzer, reached through `effectgo sig` |
| `internal/stubs/` | the stub-file parser shared by the analyzers (effects, errors, status, route verbs) |
| `cmd/effectgo/` | the binary: a multichecker, plus the `-V=full` identity and the `sig` subcommand |
| `stubs/stdlib.stubs` | stdlib rules: `effects` lines (effects) and `errors` lines (errset) in one file |
| `stubs/fuego.stubs` | Fuego `Body()` rule (errset), with source references |
| `stubs/pgx.stubs` | pgx v5 driver rules: `db` for `Row`/`Rows`/`Tx`/`Conn`/`Pool`, and their sentinel errors |
| `stubs/gin.stubs` | gin `*Context` rules: response writers and body readers (`net`), query/header readers (`pure`) |
| `stubs/huma.stubs` | Huma v2 `huma.NewError`, a package-level func variable (`pure`) |
| `examples/layers/` | the effects demo (own module) |
| `examples/todo/` | the errset realworld (own module): ErrConflict unmapped, and the form-body `?` path |
| `examples/run.sh` | builds the binary and runs it on both examples, standalone and under go vet |

Module: `github.com/faintghost/effectgo`, `go 1.26`, `golang.org/x/tools v0.51.0`.

## Shared pieces

### One directive prefix, two readers

Both analyzers read the `//effectgo:` prefix. Effects owns `pure`, `allow` and `package`. Errset owns `errors`, `errmap` and `handler`. Each analyzer skips the other's keywords silently. An unknown keyword is reported by both analyzers, so in the combined binary a typo can appear twice. **[run]** (`unknown effectgo directive "purr"` from effects and `unknown directive //effectgo:purr` from errset, one each.) Each analyzer's own unknown-keyword tests are **[test]** (`effects/testdata/src/allow`, `errset/testdata/src/contracts`).

### One stub format, one parser

`internal/stubs` parses the file format: the verb, the field count, the target and the shape of the value. Four verbs exist: `effects` and `errors` (three fields), `status <(pkg.Type)> <code>` (three fields) and `route <pkg.Func> <METHOD> <pathArg> <handlerArg>` (five fields). It does not interpret the value beyond its shape. Effects interprets `effects` rules (effect names, `pure`). Errset interprets `errors` and `status` rules. Openapi interprets `route` rules. A rule of another verb is validated for shape and then ignored. Loading is in `stubs.Load`, from the `-stubs` flag or from `$EFFECTGO_STUBS`.

Target keys: effects keys a stub by its canonical target text (`pkg.Func`, `(pkg.T).M`, `(*pkg.T).M`, `(error).Error`), so a pointer receiver is a different key. Errset keys by `pkg.Type.Method` and ignores the star, as it did in the errset prototype. Both are **[reading]** for the pointer case.

### Result types

Both analyzers declare a `ResultType`, which the signature analyzer reads:

- `effects.Result`: `map[*types.Func][]string`, the sorted effect names of every declared function (placeholders included).
- `errset.Result`: `map[*types.Func]FuncErrors`, the error variants of every function with a body. For a function that returns handler literals, `Variants` is the handler set and `Handler` is true.

An analyzer must return a typed value, not nil, when `ResultType` is set. The first version of effects returned nil on stdlib packages, and the driver rejected it. The fix is in `effects.run`.

## Effects (R)

### Effect taxonomy (root sources)

Matched by package path plus function name in `rootEffects`. This is the only model of the standard library, apart from stubs. No stdlib body is analyzed. A stdlib function outside the table contributes nothing.

- `net`: `net` `Dial*`, `Listen*`; `net/http` `Get`, `Post`, `Head`, `PostForm`, `ListenAndServe*`; methods `(*http.Client).{Do,Get,Post,Head,PostForm}`, `(*http.Server).ListenAndServe*`.
- `fs`: listed `os` functions (`Open`, `OpenFile`, `Create`, `ReadFile`, `WriteFile`, `Remove`, `RemoveAll`, `Mkdir`, `MkdirAll`, `Rename`, `Stat`, `Lstat`, `ReadDir`, `Chdir`, `Chmod`), all methods of `*os.File`, all of `io/ioutil`.
- `exec`: all of `os/exec`.
- `env`: `os` `Getenv`, `LookupEnv`, `Setenv`, `Unsetenv`, `Environ`, `Hostname`, `Getwd`; any reference to `os.Args`.
- `time`: `Now`, `Since`, `Until`, `Sleep`, `After`, `AfterFunc`, `Tick`, `NewTimer`, `NewTicker`.
- `rand`: all top-level `math/rand` and `math/rand/v2`; `crypto/rand` `Read`, `Int`, `Prime`, `Text`.
- `stdio`: `fmt` `Print*`, `Println`, `Scan*`; all of `log`; any reference to `os.Stdin`, `os.Stdout`, `os.Stderr`.
- `db`: methods of `*database/sql.DB`, `Tx`, `Stmt`, `Conn`.
- `exit`: `os.Exit`, `log.Fatal*`. `panic`: builtin `panic`, `log.Panic*`.
- `spawn`: a `go` statement. `global`: an assignment, `++`, `--`, op-assign or range-assign whose target is rooted at a package-level variable. `:=` never writes a global; `*p = x` is not followed.
- `unknown`: an interface method call (including type-parameter constraint methods), a call through a func-typed variable, field or result. A func literal's effects are charged to the enclosing declared function only when that function runs the literal (see "Driver stubs & closure attribution").

### Inference

Per declared function (`collect`), walk the body. A func literal the function runs is walked as part of it; any other literal gets a state of its own, which takes part in the fixed point. Direct effects are recorded with a witness `[self, root]`. Static callees in the same package become edges; callees in other packages are resolved through `EffectFact`s. A fixed point over the edges offers each callee's effects to the caller as `[self] + witness`, and an update happens only for a strictly shorter witness, so the loop terminates and ties keep the first witness found. Callee resolution uses `typeutil.Callee` and `Origin()`, so instantiated generics resolve to their declaration.

Facts: `EffectFact{Effects map[string]Witness}` on every declared function, `ContractFact{Allowed []string}` on contracted interface methods. Gob-encodable, registered in `FactTypes`. Stdlib packages export no facts.

Witness rendering is relative to the analyzed package: steps from that package are unqualified, others are qualified (`infra.Now`, `time.Now`, `(*Box).Set`). Interface steps render as `io.Writer.Write`. A contract step renders as `repo.(Repo).Get [contract]`, and a stub step as `io.(Writer).Write [stub]`.

### Contracts

- `//effectgo:pure`: the effect set must be empty.
- `//effectgo:allow a,b`: the effects must be a subset. `unknown` is allowed only if listed.
- `//effectgo:package pure|allow ...`: package doc comment only. Applies to every function without a directive of its own. The first valid one wins and conflicts are reported.
- Several directives on one function are all enforced (intersection). Invalid directives are reported and dropped; the package default still applies.
- Violation: `<Func> is declared <contract> but has effect <e> via <witness>`, one per (function, effect), at the function's name.

### Interface contracts and conformance

A directive on an interface method, or on the interface type (all explicitly declared methods; a method-level directive overrides it), gives a `ContractFact` on the method object. Importers read it with `ImportObjectFact`, so contracts work across packages. A call to a contracted method contributes the allowed effects with witness `Caller → (Repo).Get [contract]`. An uncontracted interface method contributes `unknown`.

Conformance (`checkConformance`): for each named non-generic type T in the package and each contracted interface I (in this package or any transitively imported one) with `types.Implements(T, I)` or `(*T, I)`, each implementing method's effects must be allowed by the contract. The message is `(*HTTPRepo).Get violates contract of (Repo).Get [allow db]: has effect net via (*HTTPRepo).Get → http.Get`, reported at the method's name when it is declared here, or at the type's name when it is promoted. **[test]** `ifaceuse` (violations for `HTTPRepo.Get`, `fakeClock.Now`, `badLocal.Ping`; OK for `SQLRepo`, `fakeClock.Sleep`, `okLocal.Ping`) and the method-level override on `fakeClock`.

### Effect polymorphism

A call through one of the declared function's own parameters `p` records the placeholder `param:i` (index among the parameters, receiver excluded) with witness `[F, dynamic call p]`. At a call site, the callee's placeholder is replaced by the effects of the argument (`argHits`):

| argument | contributes |
|---|---|
| func literal (directly in the call) | the literal's own effects, when the callee calls that parameter (the callee's effects contain `param:i`); nothing when the callee only stores it |
| identifier naming a parameter `j` of the caller | `param:j` (composition) |
| named function, method value, method expression, qualified identifier, explicit instantiation | that function's effects; its own placeholders become `unknown` |
| anything else | `unknown` |

The witness composes: `ViaOuter → polylib.Outer → polylib.Modify → clockFn → time.Now` **[test]** `poly.ViaOuter`. A method expression call has offset 1 **[test]** `poly.CallsMethodExpr`. Placeholders satisfy every contract, because the function only calls its own parameter: a function that only calls its parameter is pure **[test]** `poly.CallsNamedPure`, `polylib.Modify` (`{param:1}`, pure).

### Stubs (effects)

A stub on a target overrides the taxonomy (`effects time.Now pure` **[test]** `stubbed.Now`), and an `effects` rule on an interface method gives it an effect (`effects (io.Writer).Write net` **[test]** `stubbed.Send`, `stubbed.SendPure`). Precedence in `direct`: stub, then interface contract (or `unknown`), then taxonomy. A stub applies at every call site, including a call to a function of the analyzed package; that function's own fact still comes from its body **[reading]**.

Two target forms beyond methods and functions (see "Driver stubs & closure attribution"): a named func type `(pkg.T)` applies to every call through a value of that type, and a package-level func variable `pkg.Var` applies to calls through that variable. Both are looked up before the placeholder and before `unknown`, in `dynamicStub`.

Shipped effect rules (in `stubs/stdlib.stubs`): `(error).Error pure` removes the `unknown via error.Error` false positive **[test]** `shipped.ErrStr`; `(fmt.Stringer).String pure` **[test]** `shipped.Show`; `io.Copy`, `io.CopyN`, `io.ReadAll`, `io.ReadFull`, `io.WriteString` are `unknown` **[test]** `shipped.Copy`; io constructors are pure **[test]** `shipped.Wrap`. `io.Writer.Write`, `io.Reader.Read` and `io.Closer.Close` are deliberately not listed: their effects depend on the implementation.

### Reporting

`-effects.report` (`-report` in the standalone prototype) emits `effects of <Func>: {a, b}` for every declared function, `{}` when empty **[test]** `report`.

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

## Error sets (E)

### Model

Two engines, selected by `-errset.engine` (default `ssa` since P3, see "Flow-sensitive error sets (P3)"). The rules below describe the AST engine (`ast`, the P2 model), which is flow-insensitive, per package. The SSA engine uses the same variants, directives, stubs, contracts and fixed point; only the set of a value (rules 2 to 4) and the narrowing differ.

1. **Units.** Every `FuncDecl` with a body.
2. **Expression sets** for an error-typed expression: `nil` is empty; a package-level error variable is a sentinel; `T{}`, `&T{}`, `new(T)` with T implementing error is a type variant; a local variable is the union of everything assigned to it in the unit (fixed point, including `:=`, `var`, `if err := …`, type-switch implicit vars); a parameter, field or anything else is `?`.
3. **Calls.** `fmt.Errorf` with a constant format and `%w` arguments is the union of those arguments; with a constant format and no `%w` it is a fresh `new@pkg.F` (see "Report tooling & error variants"); other forms are `?`. `errors.New` is fresh. `errors.Join` is the union of its arguments. Stdlib callees are `?` unless a stub names them. Same-package callees use the fixed point; cross-package callees use the imported `ErrFact`; interface calls use the contract, the stub, or `?`; func-value calls are `?` unless they are a parameter (then `param:i`).
4. **Function set**: the union of error results over `return` statements, not descending into literals. Naked returns use the named result.
5. **Handler set**: for a function returning func literals, the union of those literals' own error sets, kept separately with `HasHandler`.
6. **Fixed point** over the package. Sets only grow, and a witness is replaced only by a strictly shorter one.
7. **Witness**: the callee names from the reporting function to the origin, e.g. `Service.Update → Store.checkTitle`.

Fact format (gob-encodable): `Variant{Key, Name, Kind, Methods, Chain, Status, DynamicStatus, Pkg, Ident, Orig}` (`Pkg`/`Ident` only for fresh variants, `Orig` only in collapsed ones), `ErrFact{Errors, HasHandler, Handler}` (an object fact on every function with a body), `ContractFact{Allowed}`, `ErrmapFact{Mapped, Ifaces}` on `//effectgo:errmap` functions. Interface matching compares method-signature strings qualified by package path, an approximation of `types.Implements` **[test]** `StatusError` (`*ValidationError` matches) and **[reading]** in general.

### Directives and checks

- `//effectgo:errors A, pkg.B, ?, iface:pkg.I` on a function: every inferred variant must be allowed. Violation: `<F> may return <variant> (via …), not in its //effectgo:errors contract`. Names resolve in the package scope or through the file's imports; unresolvable names are reported.
- `//effectgo:errmap` on `func(error) error`: exports which variants it maps (`errors.Is`, `==`, `switch` case, `errors.As` on a concrete type, type switch) and which interfaces it targets (`errors.As`, type switch). Signature is checked.
- `//effectgo:handler M` on a handler or handler constructor: the handler set (or the error set when there is none) must be covered by errmap M. A variant is covered if its key is mapped or its method set includes every method of an interface target. Uncovered: `handler <F> may return <variant> (via …), which errmap <M> does not map → HTTP 500`. Opaque gives `an unclassified error`; an unresolved `param:i` gives `an unresolved param:i error`. `-errset.ignoreopaque` skips both opaque kinds.
- `-errset.report`: `errors of <F>: A | B | ?` for each function with an error result (`(none)` if empty, `?` last), and `handler errors of <F>: …` where applicable **[test]** `report`, `polymorphism`.

### Stubs (errset)

An `errors` rule gives a function's error set. Stub > contract > inference > `?`. Variants: `pkg.Name`, `type:pkg.T`, `iface:pkg.I`, `?`, or `none`. A target type that is an alias (`fuego.ContextWithBody`) is followed to its origin when the package is in the import graph. A variant whose package is in the import graph is resolved against the types (a missing name or wrong kind is a hard error in the analyzed package). A package outside the graph is kept as its qualified string and compared by identity; its method set is unknown, so `iface:` coverage can only match by key **[test]** (partial export data, found while testing `net/url.Error`).

Shipped rules: `errors strconv.Atoi type:strconv.NumError` (and `ParseInt`, `ParseUint`, `ParseBool`) with the reasoning from `internal/strconv`; `errors net/url.Parse type:net/url.Error`; and `errors (github.com/go-fuego/fuego.ContextWithBody).Body type:…BadRequestError,type:…HTTPError,?` in `stubs/fuego.stubs`. The Fuego rule is honest: the body path can return unclassified errors (a user SerDes error, a form wrapping, a raw read error, a validator fault), so the rule keeps `?`. Source references are in the stub file (Fuego v0.20.0 `ctx.go`, `deserialization.go`, `validation.go`) **[reading]**.

### Report tooling & error variants

Decisions behind the report and variant changes, with the evaluation that motivated them (`/root/pp/go-rust/realworld-eval`).

- **Witness lines.** `-effects.witness` and `-errset.witness` print `witness of <F>: <variant> via <chain>` (handler sets: `witness of <F> [handler]: …`), one per variant. They are additive, so the `effects of` and `errors of` lines and `metrics.py` are unchanged. The chain excludes the function itself, every element is one line (whitespace runs collapse, an element longer than 80 characters is cut; a multi-line SQL string in a dynamic call otherwise broke the line), and `(direct)` marks a variant created in place (effect chains are cut the same way: the stored witness starts with the function). **[test]** `errset/testdata/src/witness`, `effects/testdata/src/witness`.
- **Test files.** The multichecker registers `-test` (default true) inside `Main`, so the default cannot be set before it runs. `cmd/effectgo` puts `-test=false` in front of the arguments of a standalone run unless the user gave `-test`/`--test`. A go vet invocation (arguments `-flags`, `-V=…`, or a `.cfg` file) is left alone. `effectgo openapi` already loads without tests (`packages.Config` has `Tests` false). **[test]** `cmd/effectgo` `TestWithoutTests`. `_testmain.go` packages in GOCACHE disappear with the default. Under `go vet` the test variants are passed by `go vet` itself; see the README, "Test files".
- **Fresh errors (`new@pkg.F`).** `errors.New(...)`, and `fmt.Errorf` with a constant format and no `%w`, are fresh when they reach a returned expression or a returned local through the flow sets. Such an error is created in place: no `errors.Is`/`errors.As` can match it. The variant is keyed by the *creating declared function* (closures count for their enclosing function), not by call site: a function with ten `errors.New` calls would otherwise add ten variants that differ only in position, and sets are what the analysis is meant to keep small. The witness still shows the creator (`new@domain.checkTitle via checkTitle` from a caller). A package-level `var ErrX = errors.New` stays a sentinel because the sentinel check runs first. A `fmt.Errorf` with a non-constant format stays `?`. **[test]** `errset/testdata/src/fresh`.
- **Fresh and the HTTP boundary.** A fresh variant is never covered by an errmap (`covered`), and `ownStatus` gives it 500, the same as `?`. The handler check says `may return fresh error new@…`; OpenAPI puts it under 500 with the `new@…` name and the reason. `-errset.ignoreopaque` silences fresh findings too, since they are the same class for the handler check. **[test]** `errset/testdata/src/fresh`, `openapi` `TestFreshIs500`.
- **Contracts.** `//effectgo:errors new` allows fresh variants; `?` allows opaque variants (including collapsed foreign ones). The two do not allow each other, because a fresh error is a known creation point and an opaque one is not: a contract that accepts "anything unknown" should not silently accept a fresh error and vice versa. `iface:` contracts never match fresh or opaque variants (they have no method set). **[test]** `errset` `TestFreshErrors` (contracts in `fresh.go`), `TestContractNewAndOpaqueDoNotAllowEachOther`.
- **Foreign variants.** `collapseForeign` maps a sentinel, type or interface whose identifier is unexported to `?` when its package is not the analyzed module or below it; it also maps every fresh variant whose creating function is in a foreign package, whatever the creator's name. The first run of the evaluation kept exported-creator fresh variants (`new@pgconn.Pipeline.Flush`, `new@pgx.Conn.Query`) and they filled the sets with dependency internals, while a fresh error is never matchable by name anyway; so the rule is broader than the unexported-identifier rule for fresh variants. The analyzed module is `pass.Module.Path`; `NeedModule` is requested by the standalone checker and by `effectgo openapi` (its own loader). Without module information nothing is collapsed, which is the case for GOPATH test data. The collapse is applied at the report boundary (`visible`): report lines, contract checks, the handler check, and the `errset.Result` sets that openapi and signature read. Facts keep the full identity, so a consumer in the declaring module still sees it. The status of a collapsed type is kept (`ownStatus` returns it) because the runtime status of an error does not depend on whether user code can name it. **[test]** `errset/collapse_test.go`.
- **Stub-dependent tests.** `EFFECTGO_STUBS` set to the shipped `stubs/stdlib.stubs` changes `(error).Error` (pure) and `strconv.Atoi` (`*strconv.NumError`). Under that variable, the errset and effects tests that expect unstubbed results failed (`effects` `TestKnownLimits`, `errset` `TestOpaqueHandlerReported`, `TestIgnoreOpaque`). The `effects` and `errset` test packages now clear the variable in `TestMain` (each test that wants stubs sets `stubs.Flag` or calls `t.Setenv`), so `go test ./...` passes whether or not it is set.

### Huma adapter

`effectgo http-huma` (`cmd/effectgo/http_huma.go`, `adapters/huma/`) checks the handlers of Huma v2 routes against Huma's own error handling. It reads the errset analyzer's results and facts. errset's model (flow-insensitive sets) is unchanged; it gained carried variants (`errset/carried.go`), the default errmap, and the branches of every `func(error) error` (`Result.Mappers`). The checks were run on kuzma-wms (`github.com/danielgtaylor/huma/v2` v2.37.2) with static analysis only.

**Huma's behaviour, verified in the module cache** (`/root/.local/share/go/pkg/mod/github.com/danielgtaylor/huma/v2@v2.37.2`):

- `StatusError` is the interface `GetStatus() int; Error() string` (`error.go:145-150`).
- `NewError` is a package variable of func type, returning `*ErrorModel` with the status (`error.go:231-249`). `NewErrorWithContext` is a variable too (`error.go:251`).
- The 40 constructors `Error400BadRequest … Error511NetworkAuthenticationRequired` are `return NewError(http.Status<Name>, msg, errs...)` (`error.go:281-477`). `Status304NotModified` (`error.go:276-278`) is the one more status helper.
- The dispatcher (`huma.go:1094-1106`): `errors.As(err, &StatusError)` writes the status (so a `%w`-wrapped status error keeps it), any other error is written as 500 "unexpected error occurred".
- Routes: `Get`/`Post`/`Put`/`Patch`/`Delete` take the path at argument 1 and the handler at argument 2 (`huma.go:2281-2383`). `Register` takes `huma.Operation{Method, Path}` at argument 1 and the handler at argument 2 (`huma.go:708`).

**Design.**

1. Route discovery (stub verbs, `stubs/huma.stubs`). `route pkg.Func <METHOD|@N.Field> <pathArg|@N.Field> <handlerArg>`. `@N.Field` reads a field of the composite literal that is argument N, so `Register` is `route …Register @1.Method @1.Path 2`. A field that is not a constant string, or an argument that is not a literal, makes the route unresolved with the reason (`internal/stubs/forms.go`). `openapi` skips rules with field references, because it only knows constant indexes.
2. Status-carrying constructors. `status pkg.Func <code>` (for example `status github.com/danielgtaylor/huma/v2.Error404NotFound 404`, one line per constructor, generated from `error.go:281-477`, each code checked against the `http.Status<Name>` of its body) and `status pkg.NewError arg:0` (the status is the first argument; `NewErrorWithContext` is `arg:1`). A call to a rule returns a *carried* variant (`Variant.Carried`): its `Status` is the call's status, its `Methods` are the result interface's (`StatusError`), and its display is `huma.Error404NotFound`, or `huma.NewError(404)` for the argument form. A non-constant status gives a variant with an unknown status (`huma.NewError(?)`). `errset/carried.go`.
3. Default errmap. `errdefault <pkgpath> passthrough=iface:<StatusError>,fallback=500` is turned into `errset.DefaultErrmap`: a carried variant keeps its status; a value that implements `StatusError` without a carried status (a type with a `GetStatus` method) has an unknown status; everything else gets the fallback. The `StatusCode` method of a project type is not read, which is correct for Huma (huma ignores it). No `//effectgo:handler` directive is needed.
4. Mapping evidence (`adapters/huma/evidence.go`). A variant is "mapped elsewhere" when some condition in the project tests it and its body returns a status below 500: `errors.Is(err, S)`, `errors.As(err, &T)`, `err == S`, or a call to a predicate that tests one concrete type with `errors.As` (`PredicateFact`, exported on `service.IsValidationError`). The func(error) error mappers (`errset.Result.Mappers`, which now holds every such function, with or without a directive) are evidence too. Handler-local guards count, because the spec says "mapped somewhere else".
5. Variants that reach the 500 fallback are classified (`adapters/huma/report.go`): `high`/`medium`/`info` as in the README table. A carried 500 (`huma.Error500InternalServerError`) is an intended 500, counted, never a finding. A variant with an unknown status is counted as "default", never a finding.

**Output.** `-json` writes one object per finding with the shared schema (`severity, framework, route, handler, pos, variant, status, via, reason`); `pos` is the route call site and `via` the witness chain (`(direct)` when the variant is created in the handler). The coverage goes to stderr in JSON mode and is printed after the findings in text mode.

**Tests.** `internal/stubs/forms_test.go` (the new forms, and parse errors that name the line), `adapters/huma/huma_test.go` (an analysistest fixture with a fake Huma package at the real import path, a project `service` package, and 15 routes: the raw sentinel, the mapped route, the `%w`-wrapped carried 404, `NewError` with a constant and with a variable status, a `GetStatus` type, a `StatusCode`-only type, a `Register` literal and a `Register` variable, a dynamic path, a fresh error, a handler-local `errors.Is` guard in another route, and a predicate), `adapters/huma/report_test.go` (the severities, cross-package evidence, the JSON schema and key order), `cmd/effectgo/http_huma_test.go` (positions).

**kuzma-wms, measured** (`runs/p2a/kuzma.http-huma.jsonl`, `runs/p2a/kuzma.http-huma.txt`; both with `-all`):

| | |
|---|---|
| route calls found (stub rules) | 134 (`Get` 54, `Post` 49, `Put` 15, `Delete` 7, `Patch` 1, `Register` 8; the same counts as `grep`) |
| resolved (constant method and path, handler resolved, default errmap applied) | 134; unresolved 0 |
| variants of resolved routes | 543, all with a known status (0 "default") |
| intended 500 (carried 500 from a constructor) | 18 |
| reaching the fallback 500 | 355 |
| findings: high / medium / info | 65 / 1 / 289 |
| routes with at least one finding | 85 |

The 289 info findings: 96 `context.Canceled`/`DeadlineExceeded` (foreign), 92 opaque `?`, 49 `sql.ErrNoRows`/`ErrTxDone` (foreign), 23 fresh `new@`, 20 `excelize`, 7 `strconv.NumError`, 2 other foreign. The one medium is an unexported service sentinel (`service.errForecastFallbackUnavailable`), which no handler can name.

The first version of the evidence (only `func(error) error` mappers) gave 41 high and 25 medium. The 24 medium findings that moved to high are variants that a handler guard maps (`ErrDuplicate`, `ErrInvalidTimezone`, `ErrInvalidOutboundStatus`, `ErrInvalidOperatorName`, `ErrDeleteBlocked`, `ErrInitFailed`, `ErrForecastCustomerMismatch`), and `validationError` through the `IsValidationError` predicate. The verification below is on the final run.

**Manual verification.** The seeded sample is `random.Random(20260510)` over the 65 high findings (20 drawn) and the 1 medium finding (all of them). The sample is saved as `runs/p2a/kuzma.http-huma.verification-sample.jsonl` (the same seed draws the same rows). Every row was read in the snapshot (static evidence only; nothing was run). "Guard" means the same handler tests the variant with `errors.Is` or a predicate and returns a status before the raw `return nil, err`, which errset does not model.

| # | sev | route (handler) | variant | verdict | evidence |
|---|---|---|---|---|---|
| 1 | high | GET /api/v1/outbound/summary/{customer_id} (outbound.go:60) | service.ErrInvalidTimezone | intended | raw return at outbound.go:61-63. The error is `fmt.Errorf("%w: %s", ErrInvalidTimezone, …)` (business_time.go:33) when the stored or configured timezone is invalid. The client cannot send a timezone here. The 400 in the settings PUT (settings.go:93-94) is for user input on another path. A 500 is right for a server-state fault. |
| 2 | high | POST /api/v1/locations/initialize/{customer_id} (entities.go:415) | service.ErrNotFound | false-positive (guard) | entities.go:420-422 returns 404 before the raw return at 423-424 |
| 3 | high | GET /api/v1/shipments/{batch_id}/detail (shipment.go:98) | service.ErrNotFound | false-positive (guard) | shipment.go:101-103 returns 404 before the raw return at 104 |
| 4 | high | PUT /api/v1/operators/{operator_id} (entities.go:251) | service.ErrInvalidOperatorName | false-positive (guard) | entities.go:256-258 returns 400 before the raw return at 262-263 |
| 5 | high | POST /api/v1/operators (entities.go:237) | service.ErrNotFound | false-positive (infeasible) | the only source is `return scanOperator(QueryRowContext(SELECT … WHERE id = ?))` (entities.go:210) after the INSERT of the same request: `sql.ErrNoRows` needs the row just inserted to be missing. Not guarded. Practically infeasible. |
| 6 | high | GET /api/v1/inbound/check-tracking (inbound.go:362) | service.ErrNotFound | false-positive (guard) | inbound.go:373-375 returns 400 before the raw return at 379 |
| 7 | high | GET /api/v1/inventory/items (inventory.go:92) | service.ErrNotFound | false-positive (variable reuse) | `err` is assigned by parseInventoryFilters (93) and by inventory.List (97). The raw return at 95 can only carry the filter error; the List error is answered by the fixed 500 at 102 |
| 8 | high | DELETE /api/v1/operators/{operator_id} (entities.go:268) | service.ErrDeleteBlocked | false-positive (guard) | entities.go:273-275 returns 400. The report's "returns it without mapping" is wrong for this route |
| 9 | high | GET /api/v1/customers/{customer_id} (entities.go:166) | service.ErrNotFound | false-positive (guard) | entities.go:168-170 |
| 10 | high | PUT /api/v1/customers/{customer_id} (entities.go:194) | service.ErrNotFound | false-positive (guard) | entities.go:196-198 |
| 11 | high | DELETE /api/v1/backups/{backup_id} (backups.go:199) | service.ErrNotFound | false-positive (guard) | backups.go:201-203 |
| 12 | high | GET /api/v1/backups (backups.go:150) | service.ErrNotFound | false-positive (Rows.Scan) | backup.go:66 is inside BackupService.List (backup.go:47) in `for rows.Next()`. Go 1.26.9 `database/sql` `Rows.Scan` (sql.go:3365) never returns `ErrNoRows`; only `Row.Scan` (sql.go:3486) does |
| 13 | high | POST /api/v1/backups/{backup_id}/restore (backups.go:210) | service.ErrNotFound | false-positive (guard) | backups.go:212-214 |
| 14 | high | POST /api/v1/operators (entities.go:237) | service.ErrDuplicate | false-positive (guard) | entities.go:242-244 returns 400 |
| 15 | high | GET /api/v1/customers (entities.go:154) | service.ErrNotFound | false-positive (Rows.Scan) | entities.go:47 is inside CustomerService.List (entities.go:30) in `for rows.Next()` |
| 16 | high | PUT /api/v1/operators/{operator_id} (entities.go:251) | service.ErrNotFound | false-positive (guard) | entities.go:253-255 |
| 17 | high | DELETE /api/v1/operators/{operator_id} (entities.go:268) | service.ErrNotFound | false-positive (guard) | entities.go:270-272 |
| 18 | high | GET /api/v1/shipments (shipment.go:75) | service.ErrNotFound | false-positive (Rows.Scan) | shipment.go:169 is inside ShipmentService.List (shipment.go:110) in `for rows.Next()` |
| 19 | high | PUT /api/v1/operators/{operator_id} (entities.go:251) | service.ErrDuplicate | false-positive (guard) | entities.go:259-261 |
| 20 | high | GET /api/v1/inbound/session/{session_id} (inbound.go:398) | service.ErrNotFound | false-positive (guard) | inbound.go:403-405 |
| 21 | medium | GET /api/v1/inbound/check-tracking (inbound.go:362) | service.errForecastFallbackUnavailable | false-positive (callee guard) | ensureForecastCached (inbound_forecast_fallback.go:52-54) returns `nil, nil` when `errors.Is(err, errForecastFallbackUnavailable)`, so the sentinel never leaves that function. errset keeps it in the function's set. The sentinel is unexported, so no handler can name it |

Sample result: 1 intended, 19 false positives (14 guards, 3 `Rows.Scan`, 1 infeasible read-back, 1 variable reuse), and 1 false positive among the medium (callee guard). Zero likely bugs in the sample.

Out-of-sample checks (not part of the seeded sample; listed because they were read while checking the sample):

| route | variant | verdict | evidence |
|---|---|---|---|
| PATCH /api/v1/auth/users/{user_id} (auth_admin.go:217) | service.ErrNotFound | **likely-bug** | UpdateUser returns only the Exec error (auth.go:1098-1146), so a missing id gives nil for a `display_name`-only body. The next `GetUserByID` (auth_admin.go:243) returns `ErrNotFound` (auth.go:330-331), returned raw at 245: the admin gets 500 where 404 is due |
| GET /api/v1/inbound/check-tracking (inbound.go:362) | service.validationError | **likely-bug** | with no `customer_id` and an empty `tracking_no` (the input has no required tag, inbound.go:114-117), CheckTrackingAutoResolve returns `newValidationError("tracking number is required")` (inbound_forecast_flow.go:30-31, errors.go:24-26), which the handler returns raw (inbound.go:379). The project maps validation to 400 elsewhere (settings.go:96-97, palletError) |
| PUT /api/v1/auth/users/{user_id}/warehouse-roles (auth_admin.go:294) | service.ErrNotFound | false-positive (race only) | SetUserWarehouseRoles calls GetUserByID first (auth.go:465) and the handler answers that error with 400; the raw GetUserByID at auth_admin.go:305 needs a delete between the two calls |
| PUT /api/v1/settings/{key} (settings.go:82) | service.validationError | false-positive (predicate guard) | settings.go:96-97 returns 400 for `service.IsValidationError(err)` |
| POST /api/v1/auth/login (auth.go:292) | service.ErrNotFound | false-positive (variable reuse) | `ss, err := svc.GetSessionByToken` (auth.go:302) answers failure with 401 (303-305); the raw return after `me, err := svc.BuildAuthMe` (306) reuses `err` |
| dashboard overview/trends/customer-ranking, inventory, shipments, outbound summary (ErrInvalidTimezone) | service.ErrInvalidTimezone | intended (by row 1) | same origin as row 1; not checked one by one |

Not checked individually: the other high findings of the "read-back after write" shape (`POST /api/v1/customers`, `POST /api/v1/references`, `POST /api/v1/auth/signup`, `POST /api/v1/backups/run`), which have the same structure as row 5, and the 30 highs without a textual guard beyond the sample. A textual heuristic (`errors.Is/As` on the variant's name inside the handler body) finds a guard for 35 of the 65 high findings. It is a triage aid and was not used for any verdict above.

**False-positive causes.** (1) errset does not model a guard in the same handler or a callee: `if errors.Is(err, S) { return status }` followed by `return nil, err` (14 of the 21 sampled rows). (2) `*sql.Rows.Scan` is treated as able to return `sql.ErrNoRows`; it never does (3 rows). (3) A read-back of a row the same request just wrote (1 row). (4) One `err` variable is reused for two calls, so the union of both assignments reaches each `return err` (2 rows). (5) A callee swallows the sentinel with a guard (1 row). The likely bugs are both real domain paths reaching the fallback: a row that the handler does not guard (`PATCH` user, `check-tracking` validation).

**Limitations.** Errset is flow-insensitive, as above. The intended/likely-bug split needs a reader: the "mapped elsewhere" rule can call a variant that is a server fault in one path and a bad input in another (`ErrInvalidTimezone`, row 1). Only `StatusError` types with carried statuses are read; a type whose `GetStatus` is not constant is "unknown", not 500. `sql.ErrNoRows` reaching a handler raw is reported as info (foreign), although it is a plausible not-found bug; this follows the severity definition, not a judgement. Route registration through a wrapper or a variable operation is unresolved, not followed. Output positions are file:line of the route call site, not of the return statement that produces the error.

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

### mustuse & exhaustive

Two analyzers ported from the rustvet prototype (`/root/pp/go-rust/proto-a-analyzer`, read-only). They bring Rust's `#[must_use]` and exhaustive `match` to Go, under the same contract-mode bar as the rest: a diagnostic must be a real risk, so a rule that cannot be proved is silent.

**Where.** `mustuse/mustuse.go` and `exhaustive/exhaustive.go`. Both run under `effectgo ./...` and `go vet -vettool`. `effectgo sig` runs only `signature.Analyzer`, so it never prints their diagnostics (checked by running `sig` on a module with discarded errors and missing cases).

**mustuse.** A bare expression statement whose call has a result that is an error, or a named type marked `//effectgo:mustuse` (or a pointer to one), is reported: `result of <callee> is discarded; handle it or assign to _`. Not reported: `_ = f()`, `go`, `defer`, and the `fmt.Print*` and `fmt.Fprint*` functions (the one allowlist that stays, by design). The mark is an `isMustUse` object fact, so it crosses packages; a generic type is marked by its origin, so every instance is must-use.

*Callee.* The callee is the static callee (`typeutil.Callee`), or, for a call through a local func variable, the function assigned to it when that is resolvable (`singleDefs`, `calleeOf`): the variable must have exactly one assignment, by `:=`, `=` or a one-value `var`, its address must not be taken, and the assigned expression must name a function, a qualified function, a method value or a method expression. Then `-mustuse.allow` matches by the callee's full name (`(*os.File).Close`). An unresolvable variable is named by its expression, and it is not allowlistable.

*Errors and errset.* A discarded error is reported only when the callee's error set can occur. `errset.Result.NoErrors` is the errset evidence: every function of the package with an empty error set, and every function the package names (calls, or uses as a value) when a stub rule says `errors <target> none`. mustuse exports the empty ones of its own package as an `errorFree` fact, so importers see them. A discarded value of a `//effectgo:mustuse` type is always reported, whatever the error set (`Both` in `mustuse/testdata/src/a`).

*Stubs.* The former built-in allowlist for `bytes.Buffer` and `strings.Builder` writes is gone; `stubs/stdlib.stubs` states `errors ... none` for those writers, with citations to the Go 1.26.9 source, and for `hash.Hash.Write`. Without a stub file these calls are reported (`mustuse/testdata/src/c`, `wn`). The writer rules for an interface are found through the receiver's static type and its embedded interfaces (`stubSetVia`, `stubInInterfaces`): `hash.Hash32` embeds `hash.Hash`, and the method is `io.Writer.Write` in the type checker, so a lookup by the method's own receiver would miss it (this was a real false positive in `order-mgmt`, `inquiry.go`, found by the real-world run and fixed with `mustuse/testdata/src/w`).

*Standard library.* A standard-library callee is opaque in errset unless a stub names it (`errset.IsStd`). mustuse follows the same rule: a fact derived from the source of a standard-library package does not count.

**exhaustive.** Two kinds of sealed type.

- An interface is sealed when it has an unexported method declared by its own package (the Go marker method: only that package can implement it, which is the convention `domain/event.go` of the disciplined corpus documents), or when it carries `//effectgo:sealed`. An unexported method inherited from another package does not seal an interface; the interface that embeds it is checked through the sealed interfaces it embeds. A sealed interface with `//effectgo:sealed` must have an unexported method of its own package (diagnostic otherwise). Its variants are the named types of the package whose `T` or `*T` implements it, sorted by name. A generic type is one variant whatever its instances.
- A named integer or string type with `//effectgo:sealed` is an enum. Its variants are the distinct values of the package-level constants of that type. Two constants with one value are one variant; a constant that is not exported is a variant only in its own package.

A type switch on a sealed interface, or an expression switch on an enum, with no `default` clause, must have a case for every variant: `missing cases in type switch on <Iface>: <V1>, <V2>` and `missing cases in switch on <Enum>: <C1>, ...`. A case that is a non-constant (enum) makes the switch undecided, so it is not reported. An interface case covers every variant that implements it. The message names are sorted.

**Test-only variants.** Variants are computed from the non-test files of the declaring package only (`isTestFile`). A type declared in a `_test.go` file is neither required nor counted, so the set is identical for the package, its test build and its importers, and the sealed fact of a package is the same whichever build exports it. A case in a test file that names a test-only type is still a case, and a switch in a test file is checked against the same set (`exhaustive/testdata/src/shape/shape_test.go`). A sealed interface or enum declared in a test file is not exported at all.

**Defects of the prototype, and their fixes.** Each has a test; the paths are relative to `exhaustive/testdata/src` or `mustuse/testdata/src`.

| Defect (prototype NOTES) | Fix | Test |
|---|---|---|
| Variants declared only in `_test.go` gave the package and its importers different sets | variants and sealed facts from non-test files only | `shape/shape_test.go` (`Hexagon` is not required by `Shape`, `client` agrees) |
| A sub-interface embedding a sealed interface was not checked (no fact) | `sealedOf` searches the embedded sealed interfaces; the variants are filtered by whether they implement the sub-interface | `shape.Drawable` (only `Circle` and `Square` implement it), `shape.Solid`, `wrap.Wrapped` (cross-package) |
| Generic variants were skipped | a generic named type is one variant; `Box[int]` and `Box[string]` cover it | `shape.Box` |
| Aliases: a case on an alias of a variant was a false "missing" (`types.Alias` is not `*types.Named`); an alias of a must-use type was not marked | `types.Unalias` on case types, operands and marked types; a directive on an alias is a diagnostic (its target is the defined type) | `shape.Alias`, `shape.ShapeAlias`, `d.Receipt`, `d.Aliased` |
| Pointer-to-interface and parenthesized operands | `(s).(type)` and `(*p).(type)` are checked (Go has no `p.(type)` on a pointer, so that form does not compile) | `shape.Parenthesized`, `shape.Dereferenced` |
| Function-variable calls could not be allowlisted by name | `singleDefs` resolution (above) | `mustuse/testdata/src/d`, `e` (with `-mustuse.allow=(*os.File).Close`) |
| Generic instances: the name dropped the type arguments, and an instance's mark was looked up on the instance | the callee's origin name is used; a mark on the origin covers every instance | `d.MakeBox`, `d.Gen` |

Not a defect, or not applicable: a sealed `*T` case and a `T` case both cover `T` (kept); an unexported variant of another package cannot exist, since the marker method is unexported (kept by construction).

**Enum switches (the optional item).** Implemented, but only for the explicit directive: `//effectgo:sealed` on a named integer or string type. There is no implicit enum rule. A named integer type with iota constants is common in legacy code and is often switched on partially on purpose, so reporting it without the directive would not be clearly correct. The disciplined corpus expects it without the directive (see the corpus result below).

**errset integration (summary).** `errset.Result.NoErrors`; `errset.IsStd`; `stubSetVia` and `stubInInterfaces` (errset/stubs.go); the `errorFree` fact (mustuse); the stub rules at the end of `stubs/stdlib.stubs`, cited to `bytes/buffer.go` (Write 199, WriteString 211, WriteByte 310, WriteRune 321 and 329, with the doc lines 190-192, 202-204, 299-301, 313-315), `strings/builder.go` (Write 90, WriteByte 98, WriteRune 107, WriteString 115, with the doc lines 85-86, 93-94, 101-102, 110-111) and `hash/hash.go:27-28` (`Hash.Write`: "It never returns an error.").

**Corpus result** (`realworld-eval/corpus/disciplined`, `runs/p4a/corpus.out`, stdlib stub file only):

- without `--future`: TP 12, FN 0, FP 0.
- with `--future`: TP 16, FN 1, FP 0. The missing one is `service/audit.go:23` (`Badge`, the enum switch on `domain.Status`): `domain.Status` has no `//effectgo:sealed`, so it is not an enum for effectgo. The other four future markers are reported: the `Deleted` type switch (sealed by the marker method) and the three discards (`Sweep`'s `Delete`, and the two `json.Encoder.Encode` calls).
- A scratch copy of the corpus with `//effectgo:sealed` added on the line above `type Status int` (`domain/domain.go:28`) gives TP 17, FN 0, FP 0. The corpus itself was not edited: the directive is a one-line request to the corpus owner.

**Real-world noise** (`mustuse` and `exhaustive` only, `-mustuse -exhaustive`, four stubs: stdlib, gin, huma, pgx; outputs in `runs/p4a/<project>.mustuse-exhaustive.err`). Static evidence only, see the report for the samples.

- order-mgmt: 13 mustuse findings, 0 exhaustive. Of these, 8 are best-effort closes on error paths or after a read, and 5 are `oplog.Insert` audit writes whose error is ignored on a success path (real risk).
- kuzma-wms: 0 findings.
- arcteryx-console: 25 mustuse findings, 0 exhaustive. 16 are `csv.Writer.Write` calls whose error is sticky and checked by `flushCSVWriter` (`Flush`, then `Error`): intended, but the analyzer cannot see the deferred check. 9 are `Rows.Close` after an error path or after `rows.Err()`: intended.
- Callees of the 38 findings: `(*encoding/csv.Writer).Write` 16, `(*database/sql.Rows).Close` 9, `oplog.Insert` 5, `(io.Closer).Close` 4, `(net.Conn).Close` 2, `(pgx.Tx).Rollback` 1, `(*crypto/tls.Conn).Close` 1.
- Before the `hash.Hash32` embedding fix, order-mgmt had 15 findings: the 2 extra were `hash.Write` calls on an `fnv.New32a()` value, which the doc contract makes provably nil.

## Signature (`effectgo sig`)

`signature.Analyzer` has `Requires: effects.Analyzer, errset.Analyzer`. It prints one diagnostic per `FuncDecl` of the package (`Name: Effect<A, E, R>`):

- **A**: the non-error results of the declared function, `void`, a single type, or a tuple `(A, B)`. For a handler, the handler's results. Types outside the package are qualified by package name.
- **E**: the errset set, or `never` when empty. Variants of the analyzed package are unqualified; `?` is last; `param:i` is shown.
- **R**: the effects set, sorted, `{}` when empty; placeholders are included.

The analyzer runs only through `effectgo sig`, which builds a multichecker with the signature analyzer alone. The dependencies run, but only the analyzers named on the command line print diagnostics (the checker prints `IsRoot` diagnostics only), so the contract diagnostics of effects and errset are not mixed in. **[run]** (the todo output with `sig` contains only `Effect<` lines, 89 of them, and no `HTTP 500` or `is declared` lines.) The analysistest in `signature/` includes a contract violation (`Clock`), but analysistest does not check dependency diagnostics, so the test does not prove this; the run does.

Generated test drivers (`_testmain.go`, which `go test` compiles from the build cache) are skipped: their file name does not end in `.go`. Without this, `init` and `main` of the driver appear in the output.

`sig` exits with status 3 when it prints signatures, because each signature is a diagnostic.

Known gaps: a function that returns a func value that is not a literal (for example `NoBody` in the todo example, which returns `Wrap(h)`) gets its func type as A, not the handler's results **[run]**. Errset does not follow method values or struct fields **[reading]**.

## Stub flags and environment

- `-stubs` is registered in `cmd/effectgo` on the global flag set, so `go vet` advertises it (it appears in `-flags`) and forwards it. **[run]** `go vet -vettool=… -stubs=…` on both examples.
- `$EFFECTGO_STUBS` is used when `-stubs` is empty. Both forms are accepted for both analyzers.
- Analyzer flags stay prefixed by the multichecker: `-effects.report`, `-errset.report`, `-errset.ignoreopaque`. The standalone prototypes had unprefixed names (`-report`); the single multichecker binary uses the prefixed names everywhere.
- **No stub files warning.** A standalone, `sig` or `openapi` run with neither a non-empty `-stubs` (or `--stubs`) argument nor a non-empty `$EFFECTGO_STUBS` prints one line to stderr: `effectgo: warning: no stub files loaded (set EFFECTGO_STUBS or -stubs); results for stdlib/driver calls will be less precise`. The decision reads the raw arguments, since the flag is parsed inside `multichecker.Main`. The go vet probes (`-flags`, `-V=…`) and a `.cfg` invocation never print it, because go vet runs the tool once per package. **[test]** `cmd/effectgo` `TestNoStubsWarning`. It was added after an eval run silently used no stub files (see "Determinism").

### Determinism

**Reported symptom.** On order-mgmt, a cold-cache run and a warm run differed in 78 report entries (67 effects, 11 errset). **Root cause: a configuration difference, not nondeterminism in the analyzer.** The saved cold report equals a run with `EFFECTGO_STUBS=stubs/stdlib.stubs`, and the saved warm report equals a run without it (every non-cache-path line is identical). Stubs change results: `io.ReadFull` becomes `unknown`, `(error).Error` becomes pure, `strconv.Atoi` gets an error set. The 78 entries are exactly the stub-dependent ones (42 gained `unknown`, 25 lost it, 11 errset). Reports are comparable only when they were produced with the same stub files.

**Checked, no difference.** Same configuration with fresh and warm caches, with `GOMAXPROCS=1`, and with the sequential action graph (`-debug=p`) against the parallel one. `go test -race` is clean for `effects`, `errset` and `cmd/effectgo`.

**Guard.** `cmd/effectgo/determinism_test.go` (`TestOutputIndependentOfScheduling`) runs both analyzers over `examples/layers` in the sequential graph and in parallel at `GOMAXPROCS` 1, 4 and 8, and requires identical diagnostics, effect sets and error sets, witness chains included. It guards scheduling and fact passing. It does not cover stub configuration, and it has not been shown to fail on a seeded ordering bug.

**Known path dependence.** Test packages include generated test main files that `go/packages` writes into GOCACHE. Their report lines carry the cache path (`/root/.cache/go-build/…-d:81:6: effects of main: {exit}`), so compare runs with the same cache location or filter those lines.

## Vet cache identity

**The bug.** go vet caches results under the vet tool's identity and the vet flags, not under the contents of stub files. An edited stub gave the same output with the prototype. **[run]** (baseline below.)

**The fix.** `cmd/effectgo` answers `-V=full` before `multichecker.Main`. It prints `effectgo version devel buildID=<sha256>`, where the hash covers the executable's own bytes and the contents of every file named by `$EFFECTGO_STUBS`. The format is the one that `cmd/go` parses (`internal/work/buildid.go`, `toolID`): the `devel` form whose last field is `buildID=`, whose content ID is the part after the last slash. `go vet` computes the tool ID for each run, so an edited stub gives a different action ID and the analysis runs again.

**Limitation.** `-stubs` values reach the tool only after the tool ID is computed (`-V=full` has no flags), so a file named only by `-stubs` is not in the ID. **[run]** Either name the file in `$EFFECTGO_STUBS`, or run `go vet -a`, which re-runs the analysis. Both are shown in the Merge section.

## HTTP responses: Gin (adapter)

### Gin adapter

Code: `adapters/gin/` (the analyzer), `cmd/effectgo/http_gin.go` (the subcommand), `errset/flow.go` (the read-only views of errset that the adapter uses), and the `sink` verb in `internal/stubs`. Tests: `adapters/gin/gin_test.go` with the fixture in `adapters/gin/testdata` (a fake gin package, a store, `httpapi.Detail`, and the handler shapes), `cmd/effectgo/http_gin_test.go` (severities and the JSON keys), `internal/stubs/stubs_test.go` (the verb).

**Design.** A Gin handler returns nothing, so the boundary is a call. A *sink* is a call that writes a response, declared by a stub line `sink <target> status=<i> [body=<j>]`. `stubs/gin.stubs` declares the gin writers (`JSON`, `AbortWithStatusJSON`, `String`, `Data`, and the rest; `Status` is not a sink because it only sets the pending code). A project helper is declared in a project stub file outside the snapshot, for example `sink <module>/internal/httpapi.Detail status=1 body=2`. Sinks are read by the callee's key (the `stubs.Target` string), so methods and functions use one rule form.

Mappers are detected, not annotated, by a rule engine that reads one function body and collects two kinds of rule per error variable:

- *variant rules*: `errors.Is(e, X)`, `errors.As(e, &T)`, `e == X` (a shape of errset's `ErrorTest`, exported for this use) in an `if` or a `switch` case whose body answers with a sink of constant status;
- *default rules*: the status of the catch-all: `if e != nil { sink }`, `if e == nil {} else { sink }`, the `else` of a test, a `switch` default, and the sink that follows an `if`-return on the same block (the fallthrough). Pending fallthroughs are scoped to their statement list.

A **sink mapper** is a function with a `*gin.Context` and an error parameter whose body yields at least one variant rule on the error parameter; its branches and its default are exported as a fact. A **status mapper** is `func(error) (int, bool)` whose error branches return a constant `N, true`; it is exported as a fact too. Handlers in another package use the facts.

**Handler analysis.** Every declaration and every func literal with a `*gin.Context` parameter is a handler. The engine runs on its body. The error variables of the handler come from errset's flow of the enclosing declaration (`errset.Result.Flows`, a read-only view of the same sets errset uses, so a variable's set is the union of its assignments; see the limitations). For each subject variable, the variants of its set get the first matching variant rule, else the default; a status of 5xx is a finding at the sink. A call `m(c, err)` to a sink mapper finalises the variants of `err` directly: a branch or the default. The status-mapper shape `if status, ok := m(err); ok { sink(c, status) ; return }` followed by a sink adds m's branches as variant rules on `err`, and the fallthrough as its default. A variable that is a parameter of the handler is not reported (its variants are unknown).

**Leaks.** A sink whose body argument (or the variadic tail of `c.String`) contains `err.Error()` on an error-typed expression, a `fmt.Sprint*`/`Errorf` argument of an error type, or a `+` with one of those, is a leak. The walk goes through composite literals up to two levels (`maxLiteralDepth`). The brief asked for one level; the second level is needed for the envelope `gin.H{"detail": gin.H{"error": err.Error()}}` of kuzma-wms (without it, its leaks are missed). A leak with a non-constant status (a variable such as `status` from a status mapper) reports status 0.

**Routes.** `r.GET/POST/PUT/PATCH/DELETE` on a router (through `RouterGroup`) with a constant path. Group prefixes resolve when the group is a variable assigned from `x.Group("const")` in the package. The handler is the last argument: a literal, a declared function, or a factory call whose returned literal is exported as a fact. A route joins to a finding by the handler's position.

**Severity (the driver).** The aggregate over all packages of the run gives the set of named variants with a non-5xx explicit mapping (mapper branches, status-mapper branches, and inline variant branches). `high`: a named variant (sentinel or type) that is in that set; `medium`: a named variant not in it; `info`: `?`, `new@`, or any non-named variant at 5xx; `leak`: a leak. A foreign named variant (`pgx.ErrNoRows`) is named and is marked `(foreign variant)` in the reason.

**Output.** `effectgo http-gin [-json] [-all] [-mappers] [-stubs ...] ./...`: JSON lines in the shared schema; `info` only with `-all`; `-mappers` lists the detected mappers on stderr; a summary line on stderr gives the counts (`info` is counted even when hidden).

**Changes to shared files.** `internal/stubs`: the `sink` verb (a variable-arity verb read outside the arity table). `errset`: `Result.Flows` (the per-declaration flow of each unit), `Flow.Of`, `ErrorTest`, `Branch.Matches`, `IsError`, `SentinelKey` in the new `errset/flow.go`. `cmd/effectgo/main.go`: one dispatch line. `stubs/gin.stubs`: the sink rules. The existing analyzers are unchanged: the `sink` verb is ignored by the others, and the errset results are the same.

### Results per project (static, run on the snapshots)

The runs used `EFFECTGO_STUBS` = the worktree's `stdlib`, `pgx`, `gin`, `huma` stubs plus each project's `runs/p2b/<project>.project.stubs`. Outputs: `runs/p2b/<project>.http-gin.jsonl` (`-json`, info hidden) and `.txt` (text). The counts are from the summary line on stderr (which counts info even when hidden).

| project | handlers | mappers | route registrations | high | medium | info (hidden) | leak |
|---|---|---|---|---|---|---|---|
| order-mgmt | 396 | 9 sink | 159 | 83 | 136 | 300 | 67 |
| arcteryx-console | 59 | 3 (1 status, 2 sink) | 44 | 0 | 5 | 49 | 26 |
| kuzma-wms | 20 | 0 | 2 | 0 | 0 | 0 | 6 |

Detected mappers against the known list:

- order-mgmt (9): `handleStoreError` (addresses, sink, default 500), `mailboxRestoreError` (sink, no default), `mailboxViewerError` (sink, default 502), `systemMailboxPoolRevalidationError` and `systemMailboxPoolReplacementError` (sink, default 500), `respondUpstreamError` (mall, sink, default 502, branch ErrNotConfigured to 503), and the three `diagnostics*Error` (`diagnosticsMailClassifierAutomationError`, `diagnosticsGlobalMailRuleError`, `diagnosticsMailClassificationError`, sink, default 500). Every named mapper of the brief is found. The brief said "about 10"; 9 were found, and the run does not show a tenth.
- arcteryx-console (3): `mallErrorStatus` (status, no default), `mallSyncError` (sink, default 500), `mallTradeCouponError` (sink, default 500). All three are found.
- kuzma-wms: none expected (its Gin part is middleware and two routes).

Totals from the JSON: order-mgmt 286 records (83 high, 136 medium, 67 leak); 117 of them have a route. arcteryx 31 records (5 medium, 26 leak). kuzma 6 leaks (4 with status 500, one 502, one 409). The high records of order-mgmt are 63 `pgx.ErrNoRows`, 8 `mall.ErrInvalidMonth`, 5 `readonly.ErrGiftCardOwnerNotFound`, 4 `giftinquiries.ErrInference*`, 1 `orderimport.ErrMailboxNotFound`, 1 `wms.ErrShipmentDestinationUnsupported`, 1 `wms.ErrShipmentInventoryConflict`. The medium records of order-mgmt are led by `pgx.ErrTxClosed` (34) and `pgx.ErrTxCommitRollback` (34), which are the default 500 of a failed `Tx.Commit` (intended, not reviewed in the sample), then `mall.ErrNotConfigured` (11) and `readonly.ErrGiftCardOwnerForbidden` (10, not reviewed).

### Manual verification

Static evidence only; the services were not run. Samples: fixed seed 20260 (Python `random.Random(20260)` on the records sorted by position), 15 of the high records of order-mgmt and 5 leaks of each project. arcteryx and kuzma have no high record, so there is nothing to sample. Snippets are shortened and contain no secrets.

**order-mgmt, high (15 of 83).**

| # | site | variant, status | verdict | evidence |
|---|---|---|---|---|
| 1 | giftinquiries/handlers.go:66 | pgx.ErrNoRows, 500 | false-positive (flow reuse) | `owner, err := resolveOwner(...)` (l.53); `if err != nil { return }`; then `jobs, err := store.ListKindsForUser(...)` (l.64) reuses `err`; the ErrNoRows is resolveOwner's, which already returned. |
| 2 | giftinquiries/order_completion.go:122 | pgx.ErrNoRows, 500 | false-positive (stub) | `LoadOrderContactState` runs `SELECT count(*), ... FROM contacts`, an aggregate that always returns one row; the `(Row).Scan` stub still lists ErrNoRows. |
| 3 | mall/cart.go:177 | pgx.ErrNoRows, 502 | false-positive (flow reuse) | `row, err := cartRow(...)`; `if errors.Is(err, pgx.ErrNoRows)` returns 404 (l.144); later `out, err := revalidateCartRows(...)` (l.175) reuses `err`. |
| 4 | mall/cart.go:207 | pgx.ErrNoRows, 500 | false-positive (stub) | `HasConnectedPersonalMailbox`: `SELECT EXISTS (...)`, always one row. |
| 5 | mall/cart.go:89 | pgx.ErrNoRows, 502 | false-positive (flow reuse) | `listing, ok, err := service.findListing(...)` (l.87); `err = pool.QueryRow(...).Scan(&id)` (l.109) assigns the same `err` later in `addCartItem`. errset's witness for findListing has no ErrNoRows. |
| 6 | order_import/handlers.go:468 | pgx.ErrNoRows, 500 | false-positive (flow reuse) | `GetMailboxForUser` (pg_store.go:1509-1510) maps ErrNoRows to ErrMailboxNotFound; the raw `return item, err` (l.1514) cannot carry it. |
| 7 | order_import/handlers.go:468 | orderimport.ErrMailboxNotFound, 500 | **likely-bug** (narrow) | `account, err := store.GetMailboxForUser(...)` answers `httpapi.Detail(c, 500, "db error")` (l.467-468); the store returns ErrMailboxNotFound for a missing or disabled mailbox, and the same variant is 404 elsewhere. Reachable only if the mailbox is disabled or deleted between `MarkMailboxSyncing` and this read. |
| 8 | order_import/handlers.go:624 | pgx.ErrNoRows, 500 | false-positive (stub) | `GetMailboxQuota`: `FROM (VALUES ($1::BIGINT)) requested(user_id) LEFT JOIN user_memberships`, always one row. |
| 9 | orders/writes.go:1331 | pgx.ErrNoRows, 500 | false-positive (flow reuse) | `if !errors.Is(err, pgx.ErrNoRows) { return err }` (l.166), then `_, err = tx.Exec(...)` (l.169); the function's final `return err` is the Exec result. |
| 10 | orders/writes.go:1540 | pgx.ErrNoRows, 500 | false-positive (stub) | `ManagedOrderWarehouse`: `SELECT EXISTS (...)` (longinventory/guard.go:36-47). |
| 11 | orders/writes.go:285 | pgx.ErrNoRows, 500 | false-positive (stub) | `SELECT COUNT(*) FROM channels WHERE ...`, an aggregate. |
| 12 | readonly/handlers.go:101 | pgx.ErrNoRows, 500 | false-positive (stub) | `buildOrderCountQuery` returns `SELECT COUNT(*) FROM orders ...` (l.1212). |
| 13 | readonly/handlers.go:191 | pgx.ErrNoRows, 500 | false-positive (stub) | `LoadAICompletions`: `SELECT to_regclass(...) IS NOT NULL AND ...` has no FROM and always returns one row (ai_completion.go:47-55). |
| 14 | readonly/order_completion_store.go:347 | pgx.ErrNoRows, 500 | false-positive (flow reuse) | `ResolveGiftCardOwner` turns ErrNoRows into ErrGiftCardOwnerNotFound (gift_cards.go:207-208), which the caller answers 404 (order_completion_store.go:342-344); the raw `return 0, err` (gift_cards.go:211) cannot carry ErrNoRows. |
| 15 | wms/inventory_status.go:35 | pgx.ErrNoRows, 500 | false-positive (seeded row) | `SELECT ... FROM wms_inventory_sync_state WHERE instance_code='wms_long'`; the migration seeds the row (wms/schema.go:681), so ErrNoRows is not reachable after setup. A DB error answered as 500 is intended. |

Tally: 1 likely-bug, 14 false-positive: 6 flow reuse (1, 3, 5, 6, 9, 14), 7 aggregate/EXISTS/FROM-less stub (2, 4, 8, 10, 11, 12, 13), 1 seeded singleton (15).

**order-mgmt, leak (5 of 67).**

| # | site | status | verdict | evidence |
|---|---|---|---|---|
| 1 | order_import/handlers.go:952 | 400 | **likely-bug** | `httpapi.Detail(c, http.StatusBadRequest, err.Error())` on `store.Sync(...)`; `ReferenceSyncService.Sync` returns the raw DB error of `loadReferenceConfig` and the upstream error of `fetchSyncState` (reference_sync.go:204-217). Internal errors leave as a 400 body. |
| 2 | robot_orders/robot_orders.go:385 | 400 | intended | `c.JSON(400, Response{Message: err.Error()})` on `ShouldBindJSON`: a binding message for the client. |
| 3 | wms/handlers.go:309 | 500 | **likely-bug** | `c.JSON(500, gin.H{"message": err.Error()})` on `rows.Err()`: a raw database error. |
| 4 | wms/handlers.go:440 | 500 | **likely-bug** | `err` from `service.GetShipmentRequestDetail`, which returns the raw `QueryRow` scan error (service/shipment: `return nil, err`); its text goes to the client. |
| 5 | wms/handlers.go:549 | 500 | **likely-bug** | `c.SaveUploadedFile(fileHeader, target)` fails with an OS error that names `data/shipment-payment-proofs/...`: a server path. |

**arcteryx-console, leak (5 of 26).** No high record exists, so the sample is the leaks only.

| # | site | status | verdict | evidence |
|---|---|---|---|---|
| 1 | api/handlers/mall.go:214 | 4xx from mallErrorStatus (not constant) | intended | `if status, ok := mallErrorStatus(err); ok { c.JSON(status, gin.H{"error": err.Error()}) }`; the sentinels are returned bare (store/mall_listings.go:72), so the text is a fixed domain message. |
| 2 | api/handlers/mall.go:282 | 4xx from mallErrorStatus | intended | the same shape as 1. |
| 3 | api/handlers/mall_import.go:162 | 400 | intended | `c.JSON(400, gin.H{"error": err.Error()})` on `normalizeMallImportRows`: a validation message. |
| 4 | api/handlers/mall_import.go:191 | 4xx from mallErrorStatus | intended | the same shape as 1. |
| 5 | api/handlers/mall_trade_coupon.go:310 | 400 | intended | `mallTradeCouponError`: `errors.Is(err, store.ErrInvalidTradeType)` answered 400 with `err.Error()`; the sentinel is returned bare (store/mall_trade.go:68). |

**kuzma-wms, leak (5 of 6).**

| # | site | status | verdict | evidence |
|---|---|---|---|---|
| 1 | httpapi/backups.go:193 | 500 | **likely-bug** | `err` from `BackupService.FilePath`: `fmt.Errorf("read backup file path: %w", err)` (service/backup.go:507-516), the database error, sent in `gin.H{"error": err.Error()}`. |
| 2 | httpapi/shipment.go:159 | 500 | **likely-bug** | `err` from `ShipmentService.OMSFileProxyTarget`, which returns the raw `QueryRowContext(...).Scan` error (service/shipment.go:798-809). |
| 3 | httpapi/shipment.go:170 | 500 | **likely-bug** (low) | `http.NewRequestWithContext(..., endpoint, ...)`: the URL error repeats `endpoint`, built from the internal `baseURL` (shipment.go:162). |
| 4 | httpapi/shipment.go:178 | 502 | **likely-bug** | `omsFileProxyHTTPClient.Do(req)` returns a `*url.Error` with the internal OMS URL, sent to the client. |
| 5 | middleware/idempotency.go:80 | 409 | **likely-bug** | `registerPending` returns the raw `db.ExecContext` error (`return err`), sent as the body of a 409: a database error with the wrong status. |

Tally of the 15 leak samples: 9 likely-bug, 6 intended. By project: order-mgmt 4 likely-bug and 1 intended (the robot binding message); arcteryx 5 intended (fixed sentinel texts and validation messages); kuzma 5 likely-bug.

### False-positive causes (in the sample, and in the design)

1. *Flow-insensitive reuse* (6 of 15 high in the sample). errset's sets are the union over a declaration, so an `err` that is assigned again (`err =`, `:=` in the same scope) carries the variants of every assignment, including those that were handled by an earlier `if errors.Is(...)` return. Fix: an ordered (per-program-point) flow for the variable; not done.
2. *`(Row).Scan` stub is over-approximate* (7 of 15 high). The shipped `pgx.stubs` lists `pgx.ErrNoRows` for every `Row.Scan`, including aggregates (`count(*)`, `max`), `SELECT EXISTS`, `FROM`-less selects and single-row joins over `VALUES`, which always return a row. This accounts for 63 of the 83 order-mgmt high records. The fix belongs in the pgx stub, not here (shared; it changes the effects and errset results of other analyses); a per-query rule would need the SQL text.
3. *Seeded singletons* (1 of 15). A not-found from a row that the schema always seeds.

### Limitations of the Gin adapter

- [reading] Flow-insensitive per declaration under the AST engine (causes 1 above); under the SSA engine (default) each rule and default is read at its own use, so a guard above the sink removes what it tests and a reassigned variable carries only its current value. The precision problem of order-mgmt that this caused under the AST engine is measured in "Flow-sensitive error sets (P3)".
- [reading] Only sinks with constant status are checked; a status variable is unknown (status 0) and blocks the default for its variants, so no finding is made. The mapper status shape with a variable status is covered only through the status mapper.
- [reading] Project wrappers with a constant status inside (`httpapi.MissingField`, `ValueError`, ...) are not sinks: the sink verb takes an argument index. Only helpers that receive the status as an argument are stubbed. The 4xx validation helpers are therefore invisible, which is safe for the 5xx check.
- [reading] Handler literals at package level (a `var h = func(c *gin.Context)`) have no declaration flow, so their error variants are unknown and produce no reach findings; their leaks and obs are still computed.
- [reading] A mapper that calls another mapper, or a status returned by a helper, is not composed; the inner call is read as an opaque default.
- [reading] The leak check reads the body argument and the variadic tail of `c.String`; a body built by a helper function (`gin.H` returned by a function) is not seen. The variadic tail and the two composite levels are pinned by the fixture (`TestLeaks`).
- [reading] The `route` of a handler reached through a variable (`h := X.Handle; r.GET(p, h)`) is not resolved.
- [reading] The severity classification uses the explicit observations of the whole run: a variant mapped on one route and reaching 5xx on another is `high` even when the two cannot share the data path.
- Not reviewed in the sample: the 20 non-`pgx.ErrNoRows` high records of order-mgmt (in particular `readonly.ErrGiftCardOwnerNotFound` (5) and `mall.ErrInvalidMonth` (8), which could be real), and all medium records. These need the same review.

## Limitations

### Effects

- **[test]** Interface method calls are `unknown` unless a contract or stub covers them (`limits.ErrStr`, `iface.Write`). `io.Writer.Write`, `io.Reader.Read` and `io.Closer.Close` are deliberately not stubbed.
- **[test]** Calls through func-typed local variables are `unknown`, even for a literal assigned once. A parameter is a placeholder instead (`iface.CallFunc` is `param:0`).
- **[test]** Builtin `print` and `println` are pure in the table (`limits.Builtin`).
- **[test]** A method value or function value that is not called does not count (`limits.MethodValue`).
- **[reading]** Stdlib effects outside the table are invisible: `bufio`, `io.Copy` (unless stubbed), `context`, `sync`, `(*time.Timer).Stop`, `(*net.Dialer).DialContext`, `(*rand.Rand).Intn`, `crypto/rand.Reader`. `log.Logger` methods are counted as stdio because the whole package is; `*rand.Rand` methods are not counted.
- **[reading]** Reflection (`reflect.Value.Call`, `reflect.MakeFunc`), cgo (`C.*`), package-level variable initializers, writes through a pointer (`*p = x`), dynamic dispatch through embedded interface fields, `defer` as an ordinary call, and `go f()` recording the edge to `f` (not tested).
- **[reading]** Func-valued struct fields and parameters of a closure are `unknown`, not placeholders.
- **[test]** Generic functions are analyzed once from the generic body; type-parameter method calls are `unknown` **[reading]**. Instantiations resolve by `Origin()` (`limits.Gen`).
- **[test]** Recursion terminates and propagates around cycles (`rec`).
- **[reading]** `os.Args` writes count as both `env` and `global`. Stdlib detection depends on the GOROOT path. Multiple `//effectgo:package` directives in different files: the first wins (not tested). Multiple contracts on one function are intersected (not tested).
- **[reading]** `init` functions are analyzed, but they are never called by name, so their effects do not propagate.
- **[reading]** Local interfaces (declared inside a function body) are not read for contracts. Generic named types are skipped by conformance, although their methods are inferred.

### Errset

- **[test]** No narrowing after `errors.Is` in the AST engine: `limitsast.overwritten` still reports `ErrNotFound` after `err = nil`. Any assignment anywhere in the function counts (flow-insensitivity). The SSA engine (default) gives `(none)` for the same function (`ssaflow.guardReset`).
- **[test]** Interface method calls are `?` unless a contract or stub covers them (`limits.viaInterface`).
- **[test]** Func-value calls: a parameter gives `param:i` and is substituted at call sites; other func values give `?` (`limits.viaFuncValue`).
- **[test]** Method values give `?` (`limits.viaMethodValue`), even when the method is known. Struct fields give `?` (`limits.viaField`).
- **[test]** Generics: a concrete sentinel returned by a generic function is tracked (`limits.genErr`); a type-parameter error gives `?` (`limits.wrapT`).
- **[test]** Stdlib errors are opaque except `fmt.Errorf` with `%w`, `errors.Join`, and the fresh cases `errors.New` and constant-format `fmt.Errorf` without `%w` (`new@pkg.F`, see "Report tooling & error variants"); `%v` gives `?` (`basic`, `opaque`, `basic.wrapV`). Stubs can name stdlib types (`strconv.NumError`).
- **[reading]** A non-constant format string gives `?`. Sentinel aliases (`var ErrAlias = ErrNotFound`) are separate variants.
- **[test]** Interface matching by method-signature strings: `*ValidationError` matches the `StatusError` target. **[reading]** in general.
- **[reading]** One witness per variant (the shortest). Directives are read on `FuncDecl`s and interface declarations only, not on literals.
- **[test]** Qualified names in directives need the package to be imported by the file (`api.v3Missing` is reported).
- **[test]** errmap semantics are approximate: any `==` or `switch` case against a sentinel counts as mapped, whether or not its branch is sensible (`httpres`).
- **[reading]** `defer` closures that change named results are included as assignments, not as a dataflow. Type-switch `default` is `?`. Mutexes are not analyzed.
- **[reading]** `errors.As` into a local value (`var st T; errors.As(err, &st)`) gives `st` no assignment, so `return st` yields an empty set. That is unsound.
- **[test]** Imported package scopes are partial (export data holds only what is referenced). A stub name missing from an imported package falls back to identity (`identityVariant`). Missing names in the analyzed package are hard errors.
- **[reading]** Handler directives on a function whose handler set contains `param:i` report `an unresolved param:i error`; handler sets are not substituted.
- **[reading]** A named-function argument whose own set contains `param:*` becomes `?`. Sentinels have only the method set of `error`, so only an `iface:` target whose methods are a subset of `{Error()}` covers them.
- **[reading]** "Implements" is approximated by signatures. Conformance walks every transitively imported package's contracted interfaces, which costs time on large graphs.

### Real-world (examples/todo)

- **[run]** The ErrConflict regression is reported on `v3Update` (via `Service.Update → Store.checkTitle`), and the existing tests of the example do not catch it (`go test ./...` passes in `examples/todo`).
- **[run]** Two opaque findings remain, from `c.Body()` on `v3Create` and `v3Update`. They are honest: the Fuego body path can return unclassified errors. `-errset.ignoreopaque` removes them. **[test]** (in the realworld prototype, same code) `v3Update` shares `err` between `parseID` and `c.Body()`, so the `parseID` branch also sees the body's `?` (flow-insensitivity).
- **[reading]** `v1Update` and `v2Update` are not annotated, so they are not checked as handlers. `v1Update` has `domain.ErrConflict` in its set (signature output).
- **[run]** Callback flow: `Store.modify` calls its `fn` parameter, so its set is `ErrNotFound | param:1`. At the call site in `Service.Update`, `param:1` is bound to the literal's own set, which is empty, so `Service.Update` shows no `?`. The `?` that the realworld prototype reported for `Store.modify` is gone for this reason.

## Deviations from the prototypes

- **One binary.** The prototypes had `effectsvet` (singlechecker) and `effectsvet-vet` (multichecker) because the flag names differed. `effectgo` is a multichecker for both uses, so its flags are always prefixed. The standalone `-report` became `-effects.report`.
- **One stub flag.** `-effects.stubs` and `-errset.stubs` are gone; `-stubs` replaces them.
- **Directive namespace shared.** Each analyzer skips the other's keywords (see Shared pieces).
- **Module of examples/todo.** Renamed from `github.com/faintghost/protob` to `github.com/faintghost/effectgo/examples/todo`; `go mod tidy` keeps its `go 1.26.5` directive, which is also the directive of Fuego v0.20.0. The examples `go.mod` files are separate modules.
- **realworld-fixed not imported.** It is the scratch copy with the regression fixed and the handlers changed. It is not part of the examples, because the task keeps the regressions in `examples/todo`.
- **`go vet` runs.** The Round-1 notes listed `go vet -vettool` integration as untested. It is now run for both examples (see Merge). The malformed-stub test (`TestMalformedStubIsHardError`) still runs the standalone binary.
- **The `sig` exit status** is 3 when it prints signatures.

## Not done

- No CI, no fix suggestions, no `-fix`.
- No per-instantiation effect facts for generics.
- No v1 or v2 handler checks; only the annotated v3 handlers are checked, as the realworld prototype did.
- `go install` from the private repository was not run (no network access to it from here).

## Merge

Everything below was produced from the final tree by `examples/run.sh` and by the commands shown. The prototypes in `proto-c/` were only read, and their binaries and test caches were not modified.

The module is `github.com/faintghost/effectgo`. Formerly goeffect: the two prototypes' module path, now retired.

### Commands and results

Build, vet, tests, gofmt and the grep check (`go build ./... && go vet ./... && go test ./... -count=1` at the root, plus `examples/todo` and `examples/layers`):

```
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

```
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

```
/root/pp/go-rust/effectgo/examples/todo/domain/service.go:42:19: Service.Update: Effect<Todo, ErrConflict | ErrInvalid | ErrNotFound, {}>
/root/pp/go-rust/effectgo/examples/todo/domain/todo.go:62:17: Store.modify:   Effect<Todo, ErrNotFound | param:1, {param:1}>
/root/pp/go-rust/effectgo/examples/todo/api/v3.go:53:6: v3Update:       Effect<domain.Todo, domain.ErrConflict | domain.ErrInvalid | domain.ErrNotFound | fuego.BadRequestError | fuego.HTTPError | ?, {unknown}>
```

### Vet cache identity: the proof

Run order: the same `GOCACHE` is used throughout, and it is not cleared between the runs. The stub edit removes `,?` from the Fuego `Body` rule in a copy of `fuego.stubs` (the copy is in a scratch directory; the shipped file is not changed).

```
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

```
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

```
effectgo version devel buildID=c425bdfce6f5e74ddaec0c778478683210a37a6732a60d4f65758af9c5b3ab1a
```

The hash changes whenever the binary or a stub file named in `EFFECTGO_STUBS` changes.

### Git

Two commits: the analyzer import, and the signature analyzer with the cache fix and the docs. Each was made with the existing git user configuration. Nothing was pushed, and no remote was added.


## OpenAPI

Error responses of the routes, derived from the errset analysis, with a drift check against a document. Three milestones, one commit each on `feat/openapi`: status inference, route discovery, the `openapi` subcommand (and the examples acceptance in two commits).

### Status inference (`errset/status.go`)

Each errmap rule (an `if errors.Is/As/==` branch, a switch case, a type-switch case, or the default) gives a status:

- the constant of a returned composite literal (`T{...}`, `&T{...}`) when its type has a constant `StatusCode`;
- otherwise the constant `Status` field of a literal (`fuego.HTTPError{Status: 409}` gives 409);
- `return err` is a passthrough: the variant keeps its own status;
- a return the analyzer cannot read, or a mix of statuses in one rule, gives an unknown status.

The status of a variant's type (`Variant.Status`, `Variant.DynamicStatus`) comes from, in order:

1. a `StatusFact` (exported on each `StatusCode() int` method whose returns are all the same constant; facts from dependencies work, so fuego's methods count);
2. a `status (pkg.Type) <code>` stub line (fallback only);
3. otherwise the status is dynamic (the type has `StatusCode` but no constant), and it is unknown.

A variant with no `StatusCode` and no stub is a plain error: Fuego sends it as 500. An opaque variant (`?`) is 500 as well, see "Fix: opaque errors → 500". An unknown status (an `iface:` variant, an unresolved `param:i`, a dynamic status) is reported as `default`, never as a guessed code.

Fuego v0.20.0 evidence (module cache, `github.com/go-fuego/fuego@v0.20.0`):

- A controller error goes to `s.ErrorHandler` and then `ctx.SerializeError`: `serve.go:177-180`, `ctx.go:531-537`.
- `ErrorHandler` returns a plain error unchanged (`return err` at `errors.go:208`; only `HTTPError` and `ErrorWithStatus` are converted, `errors.go:198-205`).
- The JSON error sender starts at 500 and only an `ErrorWithStatus` overrides it: `serialization.go:210-216` (`status := http.StatusInternalServerError` at 211; the override at 212-215). So a plain error reaches the client as 500.
- Constant `StatusCode` bodies, with status stub lines in `stubs/fuego.stubs`: `errors.go:115` (400), `:129` (404), `:143` (401), `:157` (403), `:171` (409), `:188` (406); `ctx.go:268` and `:285` (422).
- `fuego.HTTPError.StatusCode` is dynamic (`errors.go:90-95`: 500 when `Status` is 0, else `Status`). It has no stub line, so its status is unknown.
- The facts work across packages: with the `status` lines removed from a copy of `fuego.stubs` (the `route` rules kept), `fuego.BadRequestError` is still 400 and the todo document is unchanged. That 400 comes from the `StatusFact` that the errset analyzer exported for fuego's method.

Deviation: the task says an uncovered error maps to 500. That holds for plain errors. An uncovered error that implements `StatusCode` keeps its own status, which is what `ErrorHandler` does (`errors.go:237-239`). A sentinel (`errors.New`) is assumed plain; its initializer is not read. Recorded as a limitation.

Tests: `errset/testdata/src/status` with `TestStatusInference`. It covers constant inference (`ConstErr` is 409 although the stub says 418), the stub fallback (`StubErr` is 422), `ErrNotFound` mapped to `HTTPErr{Status: 409}`, the literal without a status (`ErrDup`, default), an uncovered sentinel and type (500), a passthrough of an `iface:` variant (default), and `?` (500, see the fix below; it was default before). The `-errset.statuses` flag prints the statuses per handler. For the todo example:

```
api/v3.go:53:6: statuses of v3Update: 400 domain.ErrInvalid | 400 fuego.BadRequestError | 404 domain.ErrNotFound | 500 domain.ErrConflict | 500 ? | default fuego.HTTPError
```

### Route discovery (`openapi/`)

- `route` stub lines name the registration functions: `route github.com/go-fuego/fuego.Put PUT 1 2` (path is argument 1, handler is argument 2; `net_http_mux.go:50-66`). The functions are generic; the rule names the origin.
- The analyzer finds calls to these functions whose path is a constant string. The handler argument is resolved to: a call to a handler constructor (its handler set and its errmap); a function identifier (its set and errmap); or a func literal. A literal cannot carry `//effectgo:handler`, so it is undocumented ("no errmap"). A call whose callee does not return a func literal (a wrapper such as `httpres.NoBody`) is undocumented; the wrapped handler is not guessed.
- The errmap is the `//effectgo:handler M` directive. No directive, or an unresolved one, is undocumented.
- Facts: the analyzers do not share facts. `checker.inheritFacts` gives an analyzer only the facts of its own dependency actions (`x/tools@v0.51.0/go/analysis/checker/checker.go:385-415`). So openapi exports its own `HandlerFact` (on each function: its set, its errmap, whether it is a constructor) and `ErrmapInfo` (on each errmap), and reads them across packages.
- `errset.Result` is now a struct: `Funcs` (as before, plus the directive and the errmap object), `Literals` (the set of each func literal with an error result) and `Errmaps` (this package's errmap facts). The signature analyzer reads `Result.Funcs`.
- Tests: `openapi/testdata` with `TestRouteDiscovery`, `TestConstructorWithErrmap`, `TestUnmappedPlainErrorIs500`, `TestCrossPackageConstructor`, `TestCrossPackageFunction`, `TestUndocumentedReasons`.

### The `effectgo openapi` subcommand

`effectgo openapi [-stubs=…] [-merge spec.json | -check spec.json] [packages]`. It loads the packages with `golang.org/x/tools/go/packages` (`LoadAllSyntax`), runs the openapi analyzer through `checker.Analyze` (`go doc golang.org/x/tools/go/analysis/checker Analyze` in v0.51.0 shows it), and aggregates the `Graph.Roots` results across packages. The other modes stay on the multichecker, which exits the process.

- Default: an OpenAPI 3 fragment. `paths.<path>.<method>.responses.<code>` with a `description` and an `x-effectgo-errors` list of `{variant, via}`; `default` for unknown statuses; `x-effectgo-handler` on the operation. Undocumented routes go to stderr with their reasons.
- `-merge`: replaces the error responses (4xx, 5xx and `default`) of each matched operation, keeps everything else, and references `#/components/schemas/HTTPError` when the document has that schema. An operation the document does not have is reported on stderr.
- `-check`: per operation, `missing` codes (the code produces them, the document does not; these block and exit 1), `stale` codes (documented, not produced; only for complete operations, where every status is known), `not in document`, and `unchecked` routes with their reasons.

### Output on examples/todo

The commands are run from `examples/todo` with `E=/root/pp/go-rust/effectgo` and `S=$E/stubs/stdlib.stubs,$E/stubs/fuego.stubs`.

Routes of v3, from `effectgo openapi -stubs=$S ./...`. The `PUT /v3/todos/{id}` operation, checked against the code:

```
PUT /v3/todos/{id}  x-effectgo-handler: v3Update(...)
  400  Bad Request: domain.ErrInvalid (via Service.Update → checkTitle); fuego.BadRequestError (via parseID)
  404  Not Found: domain.ErrNotFound (via Service.Update → Store.modify)
  500  Internal Server Error: unclassified error (via (Context).Body [stub]); domain.ErrConflict (via Service.Update → Store.checkTitle); an unclassified error becomes 500 unless it carries its own status
  default  Unknown status: fuego.HTTPError (via (Context).Body [stub])
```

- 400: `checkTitle` returns `ErrInvalid` (400 by the errmap), and `parseID` returns `fuego.BadRequestError` (its constant 400).
- 404: `Store.modify` returns `ErrNotFound`.
- 500: `ErrConflict` is not mapped by `httpres.ToHTTPError`, so it is 500.
- 500: the body's `?` (an unclassified error, 500 by the fix below) and `ErrConflict`.
- `default`: `fuego.HTTPError` from the body's validation. Its status is dynamic.

The other v3 routes, from the same run: `POST /v3/todos` has 400 (`ErrInvalid`, `BadRequestError`), 500 (the body's `?`) and `default` (`HTTPError`). `GET /v3/todos/{id}` and `DELETE /v3/todos/{id}` have 400 (`BadRequestError` via `parseID`) and 404 (`ErrNotFound`). `GET /v3/todos` has no error response, and that is right: `v3List` returns `svc.List(), nil`.

v1 and v2 are undocumented, with the reason from the run (stderr):

```
undocumented: GET /v1/todos (…/api/v1.go:30:2): no errmap: the handler has no //effectgo:handler directive
undocumented: POST /v2/todos (…/api/v2.go:13:2): WithBody(...) does not return a func literal, so its handler is not visible (it may wrap another handler)
undocumented: GET /v2/todos (…/api/v2.go:14:2): NoBody(...) does not return a func literal, so its handler is not visible (it may wrap another handler)
```

v1 has no directive. The server-wide `errorHandler` (`api/common.go`) is the real mapper for v1, v2 and v3 alike, but directives would change the errset output that `examples/run.sh` pins (the examples keep the regressions on purpose). So v1 stays undocumented. v2 wraps its handlers in `httpres.NoBody` and `httpres.WithBody`, which the analyzer does not see through.

### Fuego's own document, drift and merge

`examples/todo/cmd/openapispec` writes the document that `api.NewServer(...).OutputOpenAPISpec()` generates. `regen-openapi.sh` regenerates `testdata/openapi/fuego.json` and `testdata/openapi/effectgo-merged.json` (the merge of it).

Fuego's document does not match the task's premise of "404 only". It documents 400, 404 and 500 on every operation: 404 is hand-written by `docOpts` (`api/common.go`), and 400 and 500 are Fuego's generic defaults. So the drift is different from the one the task described:

```
$ effectgo openapi -stubs=$S -check testdata/openapi/fuego.json ./...
GET /v3/todos:
  stale 400: documented, but no variant produces it
  stale 404: documented, but no variant produces it
  stale 500: documented, but no variant produces it
POST /v3/todos:
  missing default: fuego.HTTPError
DELETE /v3/todos/{id}:
  stale 500: documented, but no variant produces it
GET /v3/todos/{id}:
  stale 500: documented, but no variant produces it
PUT /v3/todos/{id}:
  missing default: fuego.HTTPError
(v1 and v2 lines: unchecked, with reasons, as above)
summary: 2 missing, 5 stale, 0 not in document, 10 unchecked
exit 1
```

The 2 missing codes are the `default` responses of `POST /v3/todos` and `PUT /v3/todos/{id}`: the code has responses of unknown status (`fuego.HTTPError`), and the document has no `default`. The `?` of those routes is 500 now, so the code's 500 is produced by the `?` as well as by `ErrConflict` (on PUT); the document has 500 (Fuego's generic default), so there is no missing 500. The stale codes are Fuego's generic 500 on routes that cannot return one, and the 400 and 404 of `GET /v3/todos`, which takes no input and cannot fail.

`-merge` of the same document: `effectgo openapi -stubs=$S -merge testdata/openapi/fuego.json ./...` prints the merged document. The v3 `PUT` then has `400`, `404`, `500` and `default`. Each inserted response references `HTTPError`. `GET /v3/todos` keeps only its `200`. The v1 and v2 operations are unchanged (they are undocumented). There are no merge notes.

Validation: `api/openapi_validate_test.go` loads the merged document with kin-openapi (`openapi3.NewLoader().LoadFromData`, `doc.Validate`), the version of `examples/todo/go.mod`. The first run failed on Fuego's own output, not on the merge: Fuego documents the `struct{}` response of the Delete routes as a schema with an empty name (`components.schemas[""]`), which the validator rejects. The fix is in the example: the Delete routes return a named empty type, `deleted`, whose JSON is still `{}`; v2 converts its result with `result.Map`. After that:

```
--- PASS: TestMergedDocumentValidates
```

### Runtime cross-check

`api/openapi_crosscheck_test.go` drives `PUT /v3/todos/{id}` over `httptest`, seeded with `buy milk` (id 1) and `bread` (id 2). Each observed status must be a response key of that operation in `testdata/openapi/effectgo-merged.json`:

```
--- PASS: TestPutV3ObservedStatusesAreDocumented
    --- PASS: .../invalid_title          (observed 400)
    --- PASS: .../missing_id            (observed 404)
    --- PASS: .../duplicate_title_(ErrConflict)  (observed 500)
    --- PASS: .../malformed_form_body   (observed 500)
```

Since the fix below, each case also asserts attribution: the documented response of the observed status must list the variant that produces it (`x-effectgo-errors`). The 400 lists `domain.ErrInvalid`, the 404 lists `domain.ErrNotFound`, the duplicate title's 500 lists `domain.ErrConflict`, and the malformed form body's 500 lists `?`. Before the fix, the malformed form body's 500 was matched only by the key, which `ErrConflict` provided; it is now attributed to `?` on its own.

### Verification

```
== root: go build ./... && go vet ./... && go test ./... -count=1 && gofmt -l .
effects, errset, openapi, signature: ok (go test ./... -count=1)
gofmt -l .: no output
== examples/todo: go build ./... && go vet ./... && go test ./... -count=1
Go test: 35 passed in 7 packages
== examples/run.sh (bash): exit 0; the layers and todo outputs are unchanged from the Merge section
```

### Fix: opaque errors → 500

**The bug.** The opaque variant `?` (an error whose origin cannot be classified, e.g. `(Context).Body [stub]`) was reported as `default`, because `ownStatus` returned an unknown status for it. Fuego sends any error that does not carry its own status as 500 (`serialization.go:210-216`, and `ErrorHandler` returns a plain error unchanged, `errors.go:208`). So `?` is 500, and `default` documented it wrongly.

The runtime cross-check did not catch this. Its `malformed_form_body` case (`title=%zz` returns 500) passed only because `ErrConflict` (500) gave the 500 key. Had `ErrConflict` been mapped to 409, that subtest would have failed, although the code still returns 500.

**The fix.**

- `errset/status.go`, `ownStatus`: an opaque variant is 500 (with the reason in the comment). Param and iface variants, and a dynamic status, stay unknown (`default`).
- `openapi/report.go`: a 500 response that holds an opaque variant says "unclassified error" for it and adds "an unclassified error becomes 500 unless it carries its own status". `RouteError.Opaque` carries the variant kind to the report.
- The `plain error → 500` fallback stays hardcoded, not a `fallback` stub line. Fuego's rule is the server's error handler (`serialization.go:210-216`), not a property of a type. A stub keyed by a package would have to say which server handles a route, and the errset facts do not carry that. A stub would also need the status to reach `ErrmapFact.StatusOf`, which runs without a pass. That is more machinery than one constant, so the constant stays and this section is the record of it.

**Before** (`effectgo openapi`, `PUT /v3/todos/{id}`):

```
PUT /v3/todos/{id}  x-effectgo-handler: v3Update(...)
  400  Bad Request: domain.ErrInvalid (via Service.Update → checkTitle); fuego.BadRequestError (via parseID)
  404  Not Found: domain.ErrNotFound (via Service.Update → Store.modify)
  500  Internal Server Error: domain.ErrConflict (via Service.Update → Store.checkTitle)
  default  Unknown status: ? (via (Context).Body [stub]); fuego.HTTPError (via (Context).Body [stub])
```

**After:**

```
PUT /v3/todos/{id}  x-effectgo-handler: v3Update(...)
  400  Bad Request: domain.ErrInvalid (via Service.Update → checkTitle); fuego.BadRequestError (via parseID)
  404  Not Found: domain.ErrNotFound (via Service.Update → Store.modify)
  500  Internal Server Error: unclassified error (via (Context).Body [stub]); domain.ErrConflict (via Service.Update → Store.checkTitle); an unclassified error becomes 500 unless it carries its own status
  default  Unknown status: fuego.HTTPError (via (Context).Body [stub])
```

`-errset.statuses`, before: `… | 500 domain.ErrConflict | default fuego.HTTPError | default ?`; after: `… | 500 domain.ErrConflict | 500 ? | default fuego.HTTPError`.

**Drift.** `-check` against Fuego's document: the `default` lines of `POST /v3/todos` and `PUT /v3/todos/{id}` now name only `fuego.HTTPError`. Before they named `?` too. The counts are unchanged (2 missing, 5 stale). The 500 of `?` is not reported as missing, because Fuego's document already has 500 on every operation. Against the regenerated `effectgo-merged.json`, `-check` reports `ok` on all five v3 operations, and the 500 response of PUT lists `?`.

**Tests.**

- `errset/testdata/src/status`: `HUnclassified` is now 500 (it was default).
- `openapi/testdata/src/opaque` with `openapi/opaque_test.go`: `/opaque` (only `?`, 500, complete), `/dynamic` (only a dynamic status, default), `/conflict-and-opaque` (the coincidence case below), `/mixed` (conflict 409, `?` 500 and a dynamic status as default).
- The coincidence case: the errmap maps `ErrConflict` to 409, and the handler can also return `?`. The docs must still have 500 attributed to `?` and 409 attributed to `ErrConflict`, with no default. `TestConflictAndOpaqueDocs` checks both, and the 500 description.
- `examples/todo/api/openapi_crosscheck_test.go`: each case asserts the variant that its status must list (see "Runtime cross-check").

### Deviations

1. Fuego documents 400, 404 and 500 on each operation, not 404 only. The check reports that as it is.
2. The `?` is 500, not `default`: an unclassified error is sent as 500 (see the fix below). The `default` responses are only the dynamic statuses.
3. Uncovered errors that implement `StatusCode` keep their own status (see "Status inference").
4. The Delete routes return the named type `deleted` (an example change, needed for validation; the JSON body is unchanged).
5. The v1 and v2 routes are undocumented, not documented: no directives were added (see above).
6. `errset.Result` changed from a map to a struct, and the signature analyzer follows.

### Limitations

- Sentinels are assumed plain (their values are not inspected); a sentinel of an error type with a status is reported as 500.
- A `fuego.HTTPError` literal without a `Status` field is `default` (its `StatusCode` would give 500, but the analysis does not infer it).
- An opaque variant (`?`) is 500 even if its dynamic type could carry a status: the type is not visible, so the analysis cannot show that it does. Fuego would send a status-carrying error with that status.
- Errmap rules are read from the top level of the errmap body, and assignments before them are not tracked (flow-insensitive, as elsewhere). A rule that returns in more than one way with different statuses is unknown.
- A func literal handler is never documented (it has no directive).
- Routes are found only at static calls to the route functions. Route registration through a table, a wrapper or a variable is not seen (no guess, and no report either).
- `-merge` replaces all 4xx, 5xx and `default` responses of each matched operation. Success responses, schemas and parameters are kept.
- `-check` compares codes only: not the response schemas, the descriptions, or the success codes.
- Not run: `go install` from the private repository, and CI.

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

### Integration (P2: Huma + Gin)

Branch `integrate/p2`, created from `integrate/p1` (PR #1). Merges, in order, with `--no-ff`: `feat/http-gin` (3cdf6b1) and `feat/http-huma` (04afcd9).

**Conflicts and resolutions** (in `04afcd9`; the gin merge was clean).

- `internal/stubs/stubs.go`: the header lists the verbs of both branches (`status` with code or `arg:N`, `route` with `@N.Field`, `errdefault`, `sink`). The const block has `VerbSink` and `VerbErrdefault`. `sink` is read before the arity table (as in the gin branch); the unknown-verb message names all verbs. Both parsers survive.
- `errset/errset.go`: `Result` has both `Flows` (gin, read-only views) and `Mappers` (huma, every `func(error) error`). `run` builds both.
- `cmd/effectgo/main.go`: usage lines and dispatch for `http-gin` and `http-huma`.
- `README.md`: both usage lines.
- Build fix: both cmd files defined `relativize` with different signatures; the huma one is renamed `relativizeHuma`.

**Interaction tests** (all pass; fixture module `cmd/effectgo/testdata/integrate`, whose `fakes/` are the adapters' fake Huma and gin packages, via `replace`):

- (a) `TestInteractionCarriedThroughLocals` (cmd/effectgo/integrate_test.go). `Find` returns `e2`, two assignments after `huma.Error404NotFound`. The errset flow view (`Result.Flows`) keeps `Carried=true, Status=404`. The gin adapter reports `ThroughLocal` (a local `err` through `e2` into `c.JSON(500)`) with variant `huma.Error404NotFound`, route `GET /through`, status 500. The huma adapter sees the carried 404 with a known status and reports no 5xx finding.
- (b) `TestOneFileAllVerbs` and `TestMalformedLinesNameFileAndLine` (internal/stubs/integrate_test.go). One file holds sink (3 and 4 fields), `status` with a code, `arg:N`, and a type target, route with literal and `@N.Field` method and path, `errdefault`, `effects` and `errors`. Eleven malformed lines are each rejected with `bad.stubs:2:` in the error.
- (c) `TestShippedStubsLoadTogether`. The five shipped `stubs/*.stubs` load with `stubs.Load` and through `EFFECTGO_STUBS`. `effects`, `errset`, gin and huma then analyze the fixture with no error.

**Stub fix: `Rows.Scan` (no rule change was needed).** The brief expected `(*database/sql.Rows).Scan` to be modeled as returning `sql.ErrNoRows`. The evidence does not support that:

- Go 1.26.9 `database/sql/sql.go` (`go list -f '{{.Dir}}' database/sql`): `Rows.Scan` is at line 3365. It returns `rs.lasterr`, the closed-rows error, or a conversion error (`scanLocked`); it never returns `ErrNoRows`. `Row.Scan` is at line 3486, and its `return ErrNoRows` is at line 3513.
- `stubs/stdlib.stubs` has no errors rule for `(*database/sql.Rows).Scan`, so its set is opaque (`?`). The `(*database/sql.Row).Scan` rule (`ErrNoRows,?`) is correct and is unchanged.
- pgx v5.9.2 (module cache): `connRow.Scan` (the `pgx.Row` implementation) returns `ErrNoRows` at rows.go:111. `baseRows.Scan` (`pgx.Rows.Scan`) at rows.go:235 never does. `stubs/pgx.stubs` has the errors rule on `pgx.Row` only, so it is also correct.
- The ErrNoRows that the adapter notes attributed to `Rows.Scan` is the `SELECT COUNT(*)` `QueryRowContext(...).Scan(&total)` at kuzma-wms `internal/service/backup.go:74-76` (witness `sql.ErrNoRows via (Row).Scan [stub]`). That is `Row.Scan` on an aggregate, the documented limitation, and it stays. The Huma verification row 12 (GET /api/v1/backups) cites `Rows.Scan`; its verdict (false positive) holds, but the reason is the aggregate.
- Changes: a citation comment in `stubs/stdlib.stubs` (comment only). Tests: `TestRowsScanStubFix` (real `database/sql`, module mode: `RowsScan` has no `sql.ErrNoRows`, `RowScan` has it) and `TestPgxRowsStubRules` (parses `pgx.stubs`: the `pgx.Row` rule names ErrNoRows, `pgx.Rows.Scan` has no errors rule).

**`effectgo http`** (cmd/effectgo/http.go). One `packages.Load` and one `checker.Analyze` with both analyzers. The findings use the shared schema, sorted by severity (high, medium, leak, info), then position (route, variant break ties); info is hidden unless `-all`. `http-gin` and `http-huma` keep their output; their load code moved into `analyzeHTTP`, `ginFromGraph` and `humaFromGraph`. Tests: `TestHTTPUnionOfAdapters`, `TestRankOf`.

**Regression proof on the three snapshots** (`runs/integrate2/`, all runs under the lock; stubs = each branch's own four stub files plus the project stub file; pre-merge binaries built from `git archive` of `feat/http-gin` and `feat/http-huma`).

- Gin, `-json -all`: `order-mgmt` 586 rows, `kuzma-wms` 6, `arcteryx-console` 80. Pre-merge (`effectgo-pre-gin http-gin -json -all`) and merged (`http-gin -json -all`) outputs are identical as sorted sets: 0 differing lines on all three. The pre-merge summary lines match too.
- Huma, `-json -all`: kuzma-wms 355 rows, identical to pre-merge and to `runs/p2a/kuzma.http-huma.jsonl` (0 differing lines). order-mgmt and arcteryx-console have no huma routes.
- **Removed rows: none.** No row was removed; the stub fix changes nothing because no rule changed. The list is therefore empty.
- Union: `effectgo http -json -all` equals the sorted union of `http-gin -json -all` and `http-huma -json -all` on all three (order-mgmt 586, kuzma-wms 361, arcteryx-console 80; 0 differing lines).
- A caveat on the pre-merge gin baseline: an earlier pre-gin run produced `{}`, because the runner invoked the binary without the `http-gin` subcommand. Those files were discarded and re-run correctly.

**Metrics** (`metrics.py` on `effectgo -effects.report -errset.report ./...` with the four shipped stubs, no project stubs; baseline `runs/fix-stdlib/<project>.integrated-B.report`):

- order-mgmt and arcteryx-console: identical on every metric.
- kuzma-wms: `pct_opaque` 87.5 to 86.3, `pct_only_opaque` 16.3 to 14.7, `avg_named_variants` 2.59 to 2.61; all other metrics identical. Ten functions in `httpapi/` and `modules/pallet/` change from `?` to named huma carried variants (`resolveAuthForMe`, `requireBoundInboundSession`, `requireBoundInboundCommitToken`, `checkBoundInboundOperator`, `parseInventoryFilters`, `parseInventoryExportFilters`, `parseInventorySkuSummaryFilters`, `inventoryFiltersFromFields`, `sourceChannelError`, `palletError`).
- Cause: the huma branch's `stubs/huma.stubs` adds 43 `status` rules for the Huma constructors (`huma.Error4xx/5xx…`), so these calls carry statuses. `integrated-B` predates them: `git show integrate/p1:stubs/huma.stubs` has 0 `status` rules. The merged `stubs/huma.stubs` is identical to `feat/http-huma` (0 diff lines). This is the intended carried-status behavior, not a Rows.Scan change and not a merge bug. The brief expected only Rows.Scan changes; this difference is reported in full here.

**Verification.** `go build ./...`, `go vet ./...`, `go test ./... -count=1` (9 packages ok, under the lock), `gofmt -l .` (empty), `bash examples/run.sh` (exit 0, under the lock).

**Limitations of this integration.** The regression compares each pre-merge branch with the merged tree on the same snapshots; the stub files differ between the two branches (the merged `huma.stubs` and `gin.stubs` carry the sink and route forms), so each branch runs with its own stubs. The Huma fixture and the gin fixture are the adapters' fakes; they do not replace the real module packages.
