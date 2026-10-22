---
title: Known limitations
description: What effectgo does not see or does not decide, grouped by analyzer, each tagged as verified by a test or run, or as established by reading the code.
---

Every item carries one tag:

- **[verified]**: pinned by a test in `testdata/` or a `*_test.go` file, or checked by running a command.
- **[by reading]**: follows from the code, or from the source of a stub, and was not run. Treat it as a claim to check before relying on it.

Items that were true in an earlier round and have since been fixed are not listed. Where a limitation applies only to the older AST engine, the item says so; the default engine is `ssa`.

## Effects

- **[verified]** Interface method calls are `unknown` unless a contract or a stub covers them (`limits.ErrStr`, `iface.Write`). `io.Writer.Write`, `io.Reader.Read` and `io.Closer.Close` are deliberately not stubbed, because their effects depend on the implementation.
- **[verified]** A call through a func-typed local variable is charged when the variable holds a literal assigned once that the function runs (closure attribution). Any other call through a func-typed variable is `unknown`, and a parameter is a placeholder (`iface.CallFunc` is `param:0`).
- **[verified]** Builtin `print` and `println` are pure in the table (`limits.Builtin`).
- **[verified]** A method value or function value that is never called does not count (`limits.MethodValue`).
- **[verified]** Generic functions are analyzed once from the generic body; type-parameter method calls are `unknown` (`limits.Gen` for instantiations).
- **[verified]** Recursion terminates and propagates around cycles (`rec`).
- **[by reading]** Stdlib effects outside the taxonomy and the stub files are invisible: `context`, `sync`, `(*time.Timer).Stop`, `(*net.Dialer).DialContext`, `(*rand.Rand).Intn`, `crypto/rand.Reader`, and the stdlib functions and methods that write through a writer or read through a reader that no stub names (for example `(*text/template.Template).Execute` is covered, but `gzip` and `zlib` writers are not). `log.Logger` methods count as `stdio` because the whole package does; `*rand.Rand` methods are not counted.
- **[by reading]** A writer or reader that a value stores is tracked only in the constrained form of the [stub files](../reference/stubs.md#effects): the receiver is the constructor call itself, or a local variable with one assignment that is used only through selectors (other than `Reset` and `Init`). A stored value in a field, a parameter, an escaped or reassigned variable is `unknown`. `text/tabwriter` is not tracked (`Init` re-points it), so its `Write` and `Flush` are `unknown`. `log.Logger` methods stay `stdio` whatever writer they hold; the writer's own effects are charged where `log.New` or `SetOutput` binds it.
- **[by reading]** A via item ([stub files](../reference/stubs.md#effects)) charges the effects of the argument's method at the call site. Standard-library functions that call a function value they are given (`sort.Slice`, `strings.Map`) are not covered by via items; a callback that they run is charged only as a literal argument (see [closure attribution](../reference/effects.md#closure-attribution)).
- **[by reading]** Functions that a template calls, and that `text/template` reaches through reflection, are not followed.
- **[by reading]** The standard `log` package writes to the writer installed by `log.SetOutput`. A function that only calls `log.Print*` is charged `stdio`, not the writer's effects; the writer is charged to the function that calls `SetOutput`.
- **[by reading]** Not followed: reflection (`reflect.Value.Call`, `reflect.MakeFunc`), cgo (`C.*`), package-level variable initializers, writes through a pointer (`*p = x`), dynamic dispatch through embedded interface fields, `go f()` as an edge to `f` (not tested).
- **[by reading]** Func-valued struct fields, and func-valued parameters of a closure, are `unknown`, not placeholders.
- **[by reading]** A field source rule ([stub files](../reference/stubs.md#field-sources)) states what the field holds; the analysis does not check it. `(*net/http.Request).Body` and `(*net/http.Response).Body` are `net` by the net/http contract (`net/http/request.go:174-188`, `net/http/response.go:55-72`). A test or client that substitutes an in-memory reader is over-approximated to `net`, which is sound for the contract, but the same rule charges a body that is a file or a pipe with `net`.
- **[by reading]** Writes to a field are not tracked: a body that the same function assigns to `r.Body` before it is read is still charged with the field's rule. A field source is recognized only where the field is loaded and used directly (a method call on it, a via or stored argument, or a single-assignment local). A reader that reaches a call another way, such as one returned by a function from the field, is `unknown`.
- **[verified]** A field of another struct type with the same name is not a field source: `Other.Body` in `fieldsrc.DecodeOther` stays `unknown`, and a local that is reassigned or whose address is taken (`fieldsrc.DecodeReassigned`, `fieldsrc.DecodeAddr`) is not trusted.
- **[by reading]** `os.Args` writes count as both `env` and `global`. Stdlib detection depends on the source layout of the installation (see [architecture](architecture.md#standard-library-detection)).
- **[by reading]** Two package directives in different files: the first valid one wins and the other is reported, but this is not pinned by a test. Several directives on one function are intersected, also untested.
- **[by reading]** `init` functions are analyzed but never called by name, so their effects do not propagate to other functions.
- **[verified]** Local interfaces (declared inside a function body) are not read for contracts: a comment on one is ignored and its calls are `unknown`. Declare the interface at package level ([Directives](../reference/directives.md#unknown-and-the-contracts)). Generic named types are skipped by conformance, although their methods are inferred.
- **[by reading]** Conformance walks the contracted interfaces of every transitively imported package, which costs time on large import graphs.

## Error sets

- **[verified]** Interface method calls are `?` unless a contract or stub covers them (`limits.viaInterface`).
- **[verified]** Func-value calls: a parameter gives `param:i`, substituted at call sites; other func values give `?` (`limits.viaFuncValue`).
- **[verified]** Method values give `?` even when the method is known (`limits.viaMethodValue`). Struct fields give `?` (`limits.viaField`).
- **[verified]** Generics: a concrete sentinel returned by a generic function is tracked (`limits.genErr`); a type-parameter error gives `?` (`limits.wrapT`). Generic function bodies are not lowered by SSA, so they use the AST result.
- **[verified]** Stdlib errors are opaque except `fmt.Errorf` with `%w`, `errors.Join`, and the fresh cases `errors.New` and constant-format `fmt.Errorf` without `%w`. `%v` gives `?` (`basic`, `opaque`, `basic.wrapV`). Stubs can name stdlib types (`strconv.NumError`).
- **[verified]** Interface matching compares method-signature strings: `*ValidationError` matches the `StatusError` target. In general the matching is an approximation of `types.Implements`, **[by reading]**.
- **[verified]** Qualified names in directives need the package to be imported by the file (`api.v3Missing` is reported). The import of another file of the same package does not count ([Directives](../reference/directives.md#name-resolution-is-per-file)).
- **[verified]** A `//effectgo:handler` on a `func(http.ResponseWriter, *http.Request)` handler has no error set and is not checked, with no diagnostic. The pattern that is checked is in [HTTP checks](../guide/http-checks.md#nethttp-handlers). Recorded in the [backlog](backlog.md).
- **[verified]** errmap semantics are approximate: any `==` or `switch` case against a sentinel counts as mapped, whether or not its branch makes sense (`httpres`).
- **[verified]** Imported package scopes are partial (export data holds only what is referenced). A stub name missing from an imported package falls back to identity; a missing name in the analyzed package is a hard error.
- **[by reading]** Sentinel aliases (`var ErrAlias = ErrNotFound`) are separate variants.
- **[by reading]** A non-constant format string in `fmt.Errorf` gives `?`.
- **[by reading]** Only `errors.Is`, `errors.As`, `errors.AsType`, `==` and type assertions narrow. `errors.Unwrap` chains, `reflect`, and `Is`/`As` methods on user types are not evaluated (their types are treated as wrappers when they declare the method).
- **[by reading]** Narrowing is per value and per edge. It does not relate two values (`if n > 0` and a later `if n <= 0` are not correlated), and it does not reason about `len`, `nil` slices or other non-error tests.
- **[by reading]** A predicate's test is read from a direct return, or from an edge of an `if` whose condition is the test, only. A predicate that calls a function outside the package, or tests through a field, is not a predicate.
- **[verified]** A predicate exports its test only when every return is the test itself or a constant on an edge of that test, for `errors.As` and `errors.AsType` alike. A constant `return false` before the test exports nothing, so its callers keep the target on the false edge and are reported (`TestPredicateEarlyFalse`, `predearly.CallerAs`, `predearly.CallerType`). The straight-line form still exports and narrows (`predearly.CallerTail`). The AST engine does not narrow on predicates, and reports the same callers (`TestPredicateEarlyFalseBothEngines`).
- **[verified]** An `errors.AsType` predicate exports its test only in the straight-line forms that the SSA engine reads: `return ok`, or the test followed by `return true` and `return false`, with an if or an if with an else (`astype.isBusyElse`). Any other shape exports nothing, so the caller keeps the target on its false edge (`astype.filteredBusy`, `astype.partialPredicate`).
- **[verified]** The error-set conformance check does not compare a method that a type promotes from an embedded interface with the `//effectgo:errors` contract of an interface that the type satisfies. The interface method has no inferred error set, so the check skips it, while the same method declared on the type is checked (`TestPromotedErrorSetLimit`, `embedlimit`). The effects analyzer does check such a method since P10 (see [contract mode](../guide/contract-mode.md)).
- **[by reading]** The value that `v, ok := errors.AsType[T](err)` yields is the variant of the concrete type T, as for a comma-ok type assertion, and `?` for an interface T.
- **[by reading]** A captured or address-taken error variable has the union of its stores, not its value at each point. A field, a map or slice element, a channel receive and a range value are opaque. A named result is the exception: each return reads its own value, and a deferred closure reads the value at the return (see [Named results and defers](../reference/errors.md#named-results-and-defers)).
- **[verified]** A deferred closure that writes a named result contributes what it writes, computed from the value it reads (`deferflow.Wrapped`, `deferflow.WrappedV`, `deferflow.Overwrite`). A deferred closure that only reads the result does not widen its set (`deferflow.Named`, `deferflow.Hold`).
- **[by reading]** A read of a named result takes its reaching store only along a straight chain of single-predecessor blocks; across a join it takes the union of the stores. A guard such as `if errors.Is(err, S)` narrows the load it tests, not the store that reaches a later `return err`, so that return keeps the store's full set.
- **[by reading]** The writes of a deferred closure are not checked against the return's own guards: a store under a guard that the return would not take is still included.
- **[by reading]** A named result whose address reaches a call adds `?`, because the call may write it through the pointer. A function that calls `recover` returns the union of the stores of its named results.
- **[by reading]** `errors.As` into a local value (`var st T; errors.As(err, &st)`) may leave `st` without an assignment, so `return st` can yield an empty set. This is unsound in that case.
- **[by reading]** Handler directives on a function whose handler set contains `param:i` report `an unresolved param:i error`; handler sets are not substituted.
- **[by reading]** A named-function argument whose own set contains `param:*` becomes `?`.
- **[by reading]** The per-use answers exist for error-typed expressions that SSA evaluates through a debug reference. An expression without one falls back to its variable's union, then to `?`.
- **AST engine only.** **[by reading]** A named result that a deferred closure reads or writes has the union of every store; the SSA engine reads the value at each return. **[verified]** No narrowing after `errors.Is` in the AST engine: `limitsast.overwritten` reports `ErrNotFound` after `err = nil`, and any assignment anywhere in the function counts. The SSA engine gives `(none)` for the same function (`ssaflow.guardReset`). **[verified]** Calls through a local variable holding a literal are resolved by SSA and opaque in AST.
- **AST engine only.** **[verified]** No narrowing after `errors.AsType`, including `!ok` (`TestASTAsTypeLimit`, `limitsasttype.negated`). Use the default SSA engine for `ok`, `!ok`, and a later test of an assigned result.
- **[by reading]** HTTP mapping readers use AST even with the SSA engine. They recognize AsType in the if-init form `if _, ok := errors.AsType[T](err); ok`, and Huma exports a predicate in the straight-line forms of `errors.As` and `errors.AsType` (the same forms as the SSA engine's predicate rule, so the two agree on which predicates test a type). A negation, an assignment in an earlier statement, and a compound condition are not AsType mapping rules. The errmap's coverage set remains approximate, like `errors.As`: merely calling AsType counts its target as covered; status inference separately reads the branch.

## Real-world examples

These are findings of the example modules, kept on purpose so that the examples show them.

- **[verified]** In `examples/todo`, the `ErrConflict` path is reported on `v3Update` (via `Service.Update → Store.checkTitle`), and the example's tests do not catch it: `go test ./...` passes there.
- **[verified]** `v3Create` and `v3Update` have an opaque finding from `c.Body()`. It is honest: the Fuego body path can return unclassified errors. `-errset.ignoreopaque` removes it.
- **[by reading]** `v1` has no errmap directive and `v2` wraps its handlers in `httpres.NoBody` and `WithBody`, so neither is checked as a handler. `v1Update` has `domain.ErrConflict` in its set (see the signature output).
- **[verified]** The callback case of `Store.modify` is `ErrNotFound | param:1`; at the call site in `Service.Update`, `param:1` is bound to the literal's own set, which is empty, so no `?` is shown there.

## Gin adapter

- **[by reading]** Each rule and default is read at its own use under SSA. Under the AST engine, reads are per declaration, and a reassigned `err` carries the variants of every assignment.
- **[by reading]** Only sinks with a constant status are checked. A status variable is unknown (status 0), which blocks the default for its variants, so no finding is made. A mapper with a variable status is covered only through the status mapper.
- **[by reading]** Project wrappers with a constant status inside (`httpapi.MissingField`, `ValueError`, …) are not sinks: the `sink` verb takes an argument index. The 4xx validation helpers are therefore invisible, which is safe for the 5xx check.
- **[by reading]** Handler literals at package level (`var h = func(c *gin.Context)`) have no declaration flow, so their error variants are unknown and produce no reach findings. Their leaks are still computed.
- **[by reading]** A mapper that calls another mapper, or a status returned by a helper, is not composed; the inner call is read as an opaque default.
- **[by reading]** The leak check reads the body argument and the variadic tail of `c.String`, and up to two composite-literal levels (`TestLeaks`). A body built by a helper function is not seen.
- **[by reading]** A route whose handler is reached through a variable (`h := X.Handle; r.GET(p, h)`) is not resolved.
- **[by reading]** Severity uses the observations of the whole run: a variant mapped on one route and reaching 5xx on another is `high`, even when the two cannot share the data path.
- **[by reading]** The `(Row).Scan` rule of `pgx.stubs` lists `pgx.ErrNoRows` for every row scan, which causes most of the false positives in the Gin evaluation (aggregates, `EXISTS`, `FROM`-less selects). The fix belongs in the stub and needs the SQL text; see [stub files](../reference/stubs.md#shipped-stub-files).

## Huma adapter

- **[by reading]** Only `StatusError` types with carried statuses are read. A type whose `GetStatus` is not constant is reported as unknown, not as 500. A type that implements `GetStatus` (Huma's other way to carry a status) is reported as unknown, not read.
- **[by reading]** The analysis does not know that `Rows.Scan` never returns `sql.ErrNoRows`.
- **[by reading]** The "mapped elsewhere" evidence can call a variant a server fault on one path and a bad input on another (`ErrInvalidTimezone`). The intended/likely-bug split needs a reader.
- **[by reading]** `sql.ErrNoRows` reaching a handler raw is `info` (foreign), although it may be a plausible not-found bug. This follows the severity definition, not a judgement.
- **[by reading]** Route registration through a wrapper, or with a non-constant method or path, is unresolved (with the reason), not followed.
- **[by reading]** Positions are the route call site, not the return statement that produces the error.

## OpenAPI

- **[by reading]** Sentinels are assumed plain: their values are not inspected. A sentinel of an error type with a status is reported as 500.
- **[by reading]** A `fuego.HTTPError` literal without a `Status` field is `default`; the analysis does not infer that its `StatusCode` gives 500.
- **[by reading]** An opaque `?` is 500 even when its dynamic type could carry a status, because the type is not visible.
- **[by reading]** Errmap rules are read from the top level of the errmap body, and assignments before them are not tracked. A rule that returns in more than one way with different statuses is unknown.
- **[by reading]** A func-literal handler is never documented, because it has no directive.
- **[by reading]** Routes are found only at static calls to the route functions. Registration through a table, a wrapper or a variable is not seen: no guess, and no report.
- **[by reading]** `-merge` replaces all 4xx, 5xx and `default` responses of each matched operation; success responses, schemas and parameters are kept.
- **[by reading]** `-check` compares status codes only, not response schemas, descriptions or success codes.

## Signature

- **[verified]** A function that returns a func value that is not a literal (for example `NoBody` in `examples/todo`, which returns `Wrap(h)`) gets its func type as A, not the handler's results.
- **[by reading]** Errset does not follow method values or struct fields.

## Must-use and exhaustive

- **[verified]** `exhaustive` cannot see variants declared in another package's test files.
- **[verified]** An interface with only exported methods cannot be sealed: `//effectgo:sealed` on it is reported and the interface is not checked. A marker method is the way to seal it ([Directives](../reference/directives.md#when-an-interface-needs-effectgosealed)).
- **[by reading]** A deferred error check, a sticky error that is read later (as in `csv.Writer`, checked by `Flush` and `Error`), is not seen: such a discard is reported.
- **[verified]** `mustuse` needs the writer rules of the stub configuration to be silent on the always-nil writers. The zero configuration and the shipped `stdlib.stubs` have them; an explicit file without them reports `buf.WriteString`.

## Editor plugin

- **[verified]** golangci-lint's result cache does not see stub file contents. After a project `effectgo.stubs` edit, run `golangci-lint cache clean`. `go vet` is not affected.
- **[by reading]** The vet identity covers the project file of the module containing the working directory, the `$EFFECTGO_STUBS` files and the shipped files. A `-stubs` file and a project file of another module below the working directory are not covered; use `go vet -a`.
- **[verified]** File scope (`lintOnSave: file`) is broken for multi-file packages: a single file does not type-check alone (`undefined: WithTx`). Use package scope.
- **[by reading]** Relative `stubs` paths in the plugin settings resolve against the module root of the working directory, not against the config file.
- **[verified]** The zero configuration loads all five shipped stub files. Results differ from a `stdlib.stubs`-only run.
- **[by reading]** Historical plugin checks used a local checkout while the repository was private. The current public route uses `version: v0.4.0` in `.custom-gcl.yml`; publication and a successful CLI download do not prove a Go-proxy custom plugin build or an editor session.
- **[verified]** The plugin reports only the five analyzers' diagnostics. The report flags, witnesses and the HTTP subcommands are not available in the editor.
- **[by reading]** Editors were not run. The VS Code settings, the problem matcher, Neovim's working directory and Windows paths were not observed.

## Not implemented

The items that are deliberately left for later, with the reason and the evidence, are in the [backlog](backlog.md).

- Routine CI runs build/vet, tooling regressions and documentation checks; the full release gate remains separate. No fix suggestions or `-fix` implementation.
- No per-instantiation effect facts for generics.
- No handler checks for `v1` and `v2` of the todo example; only the annotated `v3` handlers are checked.
- Historical `go install` from the private repository was not run because the build environment lacked access. The current public install command is documented in [Getting started](../guide/getting-started.md#install); historical results are not evidence for that route.

## Tags used in the journal

The journal keeps the original tags `[test]`, `[run]` and `[reading]` where the phase used them. `[test]` and `[run]` correspond to **[verified]** here, and `[reading]` to **[by reading]**.
