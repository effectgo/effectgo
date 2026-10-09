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
| `errset/status.go` | HTTP status inference for errmap branches (see "OpenAPI") |
| `errset/carried.go` | status-carrying errors (status-function rules), the default errmap, and the mapper branches (see "Huma adapter") |
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

Analysis is AST-plus-types, flow-insensitive, per package.

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

- **[test]** No narrowing after `errors.Is`: `limits.overwritten` still reports `ErrNotFound` after `err = nil`. Any assignment anywhere in the function counts (flow-insensitivity).
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
