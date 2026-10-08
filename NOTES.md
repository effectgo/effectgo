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
| `signature/` | the `Effect<A, E, R>` analyzer, reached through `effectgo sig` |
| `internal/stubs/` | the stub-file parser shared by both analyzers |
| `cmd/effectgo/` | the binary: a multichecker, plus the `-V=full` identity and the `sig` subcommand |
| `stubs/stdlib.stubs` | stdlib rules: `effects` lines (effects) and `errors` lines (errset) in one file |
| `stubs/fuego.stubs` | Fuego `Body()` rule (errset), with source references |
| `examples/layers/` | the effects demo (own module) |
| `examples/todo/` | the errset realworld (own module): ErrConflict unmapped, and the form-body `?` path |
| `examples/run.sh` | builds the binary and runs it on both examples, standalone and under go vet |

Module: `github.com/faintghost/effectgo`, `go 1.26`, `golang.org/x/tools v0.51.0`.

## Shared pieces

### One directive prefix, two readers

Both analyzers read the `//effectgo:` prefix. Effects owns `pure`, `allow` and `package`. Errset owns `errors`, `errmap` and `handler`. Each analyzer skips the other's keywords silently. An unknown keyword is reported by both analyzers, so in the combined binary a typo can appear twice. **[run]** (`unknown effectgo directive "purr"` from effects and `unknown directive //effectgo:purr` from errset, one each.) Each analyzer's own unknown-keyword tests are **[test]** (`effects/testdata/src/allow`, `errset/testdata/src/contracts`).

### One stub format, one parser

`internal/stubs` parses the file format: the verb, the field count, and the target. It does not interpret the value. Effects interprets `effects` rules (effect names, `pure`). Errset interprets `errors` rules (variants, `none`). A rule of the other verb is validated for shape and then ignored. Loading is in `stubs.Load`, from the `-stubs` flag or from `$EFFECTGO_STUBS`.

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
- `unknown`: an interface method call (including type-parameter constraint methods), a call through a func-typed variable, field or result. Calls inside a func literal are attributed to the enclosing declared function.

### Inference

Per declared function (`collect`), walk the body including closures. Direct effects are recorded with a witness `[self, root]`. Static callees in the same package become edges; callees in other packages are resolved through `EffectFact`s. A fixed point over the edges offers each callee's effects to the caller as `[self] + witness`, and an update happens only for a strictly shorter witness, so the loop terminates and ties keep the first witness found. Callee resolution uses `typeutil.Callee` and `Origin()`, so instantiated generics resolve to their declaration.

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
| func literal (directly in the call) | nothing; its body is already part of the caller |
| identifier naming a parameter `j` of the caller | `param:j` (composition) |
| named function, method value, method expression, qualified identifier, explicit instantiation | that function's effects; its own placeholders become `unknown` |
| anything else | `unknown` |

The witness composes: `ViaOuter → polylib.Outer → polylib.Modify → clockFn → time.Now` **[test]** `poly.ViaOuter`. A method expression call has offset 1 **[test]** `poly.CallsMethodExpr`. Placeholders satisfy every contract, because the function only calls its own parameter: a function that only calls its parameter is pure **[test]** `poly.CallsNamedPure`, `polylib.Modify` (`{param:1}`, pure).

### Stubs (effects)

A stub on a target overrides the taxonomy (`effects time.Now pure` **[test]** `stubbed.Now`), and an `effects` rule on an interface method gives it an effect (`effects (io.Writer).Write net` **[test]** `stubbed.Send`, `stubbed.SendPure`). Precedence in `direct`: stub, then interface contract (or `unknown`), then taxonomy. A stub applies at every call site, including a call to a function of the analyzed package; that function's own fact still comes from its body **[reading]**.

Shipped effect rules (in `stubs/stdlib.stubs`): `(error).Error pure` removes the `unknown via error.Error` false positive **[test]** `shipped.ErrStr`; `(fmt.Stringer).String pure` **[test]** `shipped.Show`; `io.Copy`, `io.CopyN`, `io.ReadAll`, `io.ReadFull`, `io.WriteString` are `unknown` **[test]** `shipped.Copy`; io constructors are pure **[test]** `shipped.Wrap`. `io.Writer.Write`, `io.Reader.Read` and `io.Closer.Close` are deliberately not listed: their effects depend on the implementation.

### Reporting

`-effects.report` (`-report` in the standalone prototype) emits `effects of <Func>: {a, b}` for every declared function, `{}` when empty **[test]** `report`.

## Error sets (E)

### Model

Analysis is AST-plus-types, flow-insensitive, per package.

1. **Units.** Every `FuncDecl` with a body.
2. **Expression sets** for an error-typed expression: `nil` is empty; a package-level error variable is a sentinel; `T{}`, `&T{}`, `new(T)` with T implementing error is a type variant; a local variable is the union of everything assigned to it in the unit (fixed point, including `:=`, `var`, `if err := …`, type-switch implicit vars); a parameter, field or anything else is `?`.
3. **Calls.** `fmt.Errorf` with a constant format and `%w` arguments is the union of those arguments (other forms are `?`). `errors.Join` is the union of its arguments. Stdlib callees are `?` unless a stub names them. Same-package callees use the fixed point; cross-package callees use the imported `ErrFact`; interface calls use the contract, the stub, or `?`; func-value calls are `?` unless they are a parameter (then `param:i`).
4. **Function set**: the union of error results over `return` statements, not descending into literals. Naked returns use the named result.
5. **Handler set**: for a function returning func literals, the union of those literals' own error sets, kept separately with `HasHandler`.
6. **Fixed point** over the package. Sets only grow, and a witness is replaced only by a strictly shorter one.
7. **Witness**: the callee names from the reporting function to the origin, e.g. `Service.Update → Store.checkTitle`.

Fact format (gob-encodable): `Variant{Key, Name, Kind, Methods, Chain}`, `ErrFact{Errors, HasHandler, Handler}` (an object fact on every function with a body), `ContractFact{Allowed}`, `ErrmapFact{Mapped, Ifaces}` on `//effectgo:errmap` functions. Interface matching compares method-signature strings qualified by package path, an approximation of `types.Implements` **[test]** `StatusError` (`*ValidationError` matches) and **[reading]** in general.

### Directives and checks

- `//effectgo:errors A, pkg.B, ?, iface:pkg.I` on a function: every inferred variant must be allowed. Violation: `<F> may return <variant> (via …), not in its //effectgo:errors contract`. Names resolve in the package scope or through the file's imports; unresolvable names are reported.
- `//effectgo:errmap` on `func(error) error`: exports which variants it maps (`errors.Is`, `==`, `switch` case, `errors.As` on a concrete type, type switch) and which interfaces it targets (`errors.As`, type switch). Signature is checked.
- `//effectgo:handler M` on a handler or handler constructor: the handler set (or the error set when there is none) must be covered by errmap M. A variant is covered if its key is mapped or its method set includes every method of an interface target. Uncovered: `handler <F> may return <variant> (via …), which errmap <M> does not map → HTTP 500`. Opaque gives `an unclassified error`; an unresolved `param:i` gives `an unresolved param:i error`. `-errset.ignoreopaque` skips both opaque kinds.
- `-errset.report`: `errors of <F>: A | B | ?` for each function with an error result (`(none)` if empty, `?` last), and `handler errors of <F>: …` where applicable **[test]** `report`, `polymorphism`.

### Stubs (errset)

An `errors` rule gives a function's error set. Stub > contract > inference > `?`. Variants: `pkg.Name`, `type:pkg.T`, `iface:pkg.I`, `?`, or `none`. A target type that is an alias (`fuego.ContextWithBody`) is followed to its origin when the package is in the import graph. A variant whose package is in the import graph is resolved against the types (a missing name or wrong kind is a hard error in the analyzed package). A package outside the graph is kept as its qualified string and compared by identity; its method set is unknown, so `iface:` coverage can only match by key **[test]** (partial export data, found while testing `net/url.Error`).

Shipped rules: `errors strconv.Atoi type:strconv.NumError` (and `ParseInt`, `ParseUint`, `ParseBool`) with the reasoning from `internal/strconv`; `errors net/url.Parse type:net/url.Error`; and `errors (github.com/go-fuego/fuego.ContextWithBody).Body type:…BadRequestError,type:…HTTPError,?` in `stubs/fuego.stubs`. The Fuego rule is honest: the body path can return unclassified errors (a user SerDes error, a form wrapping, a raw read error, a validator fault), so the rule keeps `?`. Source references are in the stub file (Fuego v0.20.0 `ctx.go`, `deserialization.go`, `validation.go`) **[reading]**.

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
- **[test]** Stdlib errors are opaque except `fmt.Errorf` with `%w` and `errors.Join`; `%v` gives `?` (`basic`, `opaque`, `basic.wrapV`). Stubs can name stdlib types (`strconv.NumError`).
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

