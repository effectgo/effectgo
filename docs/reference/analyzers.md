---
title: Analyzers and diagnostics
description: What each analyzer (effects, errset, mustuse, exhaustive, schematag, directive, signature) reports, with the exact message formats, and when each one is silent.
---

The binary contains seven analyzers. Six report diagnostics in the normal run; `signature` prints signatures and runs only under `effectgo sig`.

| Analyzer | Enabled by | Reports | Reads |
|---|---|---|---|
| `effects` | default | contract violations; with flags, effect sets and witnesses | `//effectgo:pure`, `allow`, `package`; `effects` stub rules |
| `errset` | default | error contract violations, handler coverage, directive errors; with flags, error sets and witnesses | `//effectgo:errors`, `errmap`, `handler`; `errors`, `status`, `sink` (Gin), `route` stub rules |
| `mustuse` | default | discarded errors and discarded `//effectgo:mustuse` values | `//effectgo:mustuse`; errset's "never fails" evidence |
| `exhaustive` | default | switches on sealed types that miss a variant | `//effectgo:sealed` |
| `schematag` | default | schema tags, field types and enum `Valid` methods of the types that rt reads; route inputs against their pattern and method | `schema` tags of types that flow into `rt/web` and `rt/schema`; `rt/web` route functions |
| `directive` | default | a directive where no analyzer reads it; a comment with a space after `//` that names a directive keyword | every `//effectgo:` comment, by its placement |
| `signature` | `effectgo sig` only | one `Effect<A, E, R>` line per function | the other analyzers, as dependencies |

A diagnostic is reported only when the code is provably at risk. A rule that cannot be proved is silent. Each analyzer is deterministic: the same input gives the same output (see [Determinism](../journal/03-p1-determinism-vet-identity.md)).

## Root rule

A contract violation often shows up in several functions at once. When `Outer` calls `Inner` and `Inner` breaks its own contract, `Outer` breaks its contract through `Inner`. Fixing `Inner` fixes `Outer`, so reporting both repeats one fact, and the first line names the caller as if the caller were at fault. The analyzers report the root only.

- **effects.** A violation of `F` for effect `e` is not reported when every source of `e` in `F` is a call to a declared function that itself violates its own contract for `e`. The callee's violation is reported, and fixing it fixes `F`.
- **errset.** A violation of `F` for variant `X` is not reported when `X` reaches `F` through a declared callee that itself violates its contract for `X`, and no other call of `F` may return `X` unless that call is such a callee.

The rule is conservative: `F` is reported when any of these holds.

- `F` has the effect or the variant from its own body, or from a stub, an interface method, a call through a func value, or a func-typed argument.
- A callee that is a source does not violate: it has no contract, its contract allows the effect or variant, or it is a stub or an interface method.
- A callee is not analyzed: the standard library, or a dependency without facts.
- For errset, `F` is a handler (its effective set is the handler set).

Example, from a scratch module with `Stamp`, `Post` and `Close` under `//effectgo:pure`, and `Find` and `Get` under `//effectgo:errors ?`. `Stamp` and `Find` are the roots:

```text
ROOT/ledger/ledger.go:12:6: Stamp is declared //effectgo:pure but has effect time via Stamp → time.Now
ROOT/ledger/ledger.go:24:6: Find may return ledger.ErrMissing, not in its //effectgo:errors contract
effectgo: 1 package analysed (0 test variants), 2 findings, 3 duplicates hidden (-effects.all or -errset.all to show them)
```

`Post`, `Close` and `Get` are hidden. `-effects.all` prints the effects ones, and `-errset.all` the errset one:

```text
ROOT/ledger/ledger.go:16:6: Post is declared //effectgo:pure but has effect time via Post → Stamp → time.Now
ROOT/ledger/ledger.go:20:6: Close is declared //effectgo:pure but has effect time via Close → Post → Stamp → time.Now
ROOT/ledger/ledger.go:12:6: Stamp is declared //effectgo:pure but has effect time via Stamp → time.Now
ROOT/ledger/ledger.go:24:6: Find may return ledger.ErrMissing, not in its //effectgo:errors contract
ROOT/ledger/ledger.go:28:6: Get may return ledger.ErrMissing (via Find), not in its //effectgo:errors contract
effectgo: 1 package analysed (0 test variants), 5 findings
```

The flags are `-effects.all` and `-errset.all`; each one turns the rule off for its analyzer, standalone and under `go vet` alike (`go vet -vettool=effectgo -effects.all ./...`). The hidden count in the summary line counts the violations that the rule suppressed in the root packages (see [CLI](cli.md#summary-line)). The rule is the same under `go vet`, where the analyzers run through the tool interface; the summary line is not printed there. Facts carry the information across packages: a callee in a dependency that violates its contract hides its callers in the importing package.

## effects

| Message | When |
|---|---|
| `<F> is declared <contract> but has effect <e> via <witness>` | a function exceeds its contract; one per function and effect, at the name, unless the [root rule](#root-rule) hides it. `<contract>` is the directive text, for example `//effectgo:pure`, `//effectgo:allow db`, `//effectgo:package pure` |
| `(<Recv>).<M> violates contract of (<I>).<M> [<contract>]: has effect <e> via <witness>` | a method implementation exceeds an interface contract |
| `effects of <F>: {<e>, …}` | `-effects.report`; `{}` when empty |
| `witness of <F>: <e> via <chain>` | `-effects.witness` |
| `unknown effectgo directive "<kw>"` | a keyword the effects analyzer does not know |
| `//effectgo:package is only valid in a package doc comment` | a package directive elsewhere |
| `conflicting package directive "<d>"; keeping "<first>"` | two package directives in one package |
| `effectgo:<kw> on <Name> checks nothing: an interface contract covers the methods it declares, and <Name> declares none` | a `pure` or `allow` on an interface type that declares no method of its own (it only embeds); reported at the type name |

Example, from `examples/layers`:

```text
domain/todo.go:31:6: Normalize is declared //effectgo:pure but has effect time via Normalize → infra.Stamp → time.Now
```

Effect names and their roots are in [Effects](effects.md).

## errset

| Message | When |
|---|---|
| `errors of <F>: <variants>` | `-errset.report`, for each function with an error result; `(none)` when empty |
| `handler errors of <F>: <variants>` | `-errset.report`, for a handler (a function returning handler literals) |
| `witness of <F>: <variant> via <chain>` | `-errset.witness`; `(direct)` when created in `<F>` |
| `witness of <F> [handler]: <variant> via <chain>` | `-errset.witness`, for a handler set |
| `statuses of <F>: <code> <variant> \| …` | `-errset.statuses`, for each handler; `default <variant>` for unknown statuses |
| `<F> may return <variant><via>, not in its //effectgo:errors contract` | a function returns a variant its contract does not allow |
| `(<Recv>).<M> violates contract of (<I>).<M>: may return <variant><via>` | an implementation returns a variant its interface contract does not allow |
| `handler <F> may return <variant><via>, which errmap <M> does not map → HTTP 500` | a handler returns a variant the errmap does not cover |
| `handler <F> may return an unclassified error<via>, which errmap <M> does not map → HTTP 500` | the same, for an opaque `?` (silenced by `-errset.ignoreopaque`) |
| `handler <F> may return fresh error <new@…><via>, which errmap <M> does not map (errors.Is and errors.As cannot match it) → HTTP 500` | the same, for a fresh `new@` variant (silenced by `-errset.ignoreopaque`) |
| `handler <F> may return an unresolved <param:i> error<via>, which errmap <M> does not map → HTTP 500` | a handler returns the error of an unresolved parameter |
| `route <METHOD> <path>: handler is a function literal; register a named function or method value so its error set can be checked` | a route registers a closure (see [Route handlers](#route-handlers)); not reported in `_test.go` files |
| `route <METHOD> <path>: handler <F> may return <variant><via>, which has no constant StatusCode → HTTP 500` | a variant of a route handler's error set has no constant error status (see [Route handlers](#route-handlers)) |
| `route <METHOD> <path>: handler <F> may return an unclassified error<via>, which has no constant StatusCode → HTTP 500` | the same, for an unknown origin (`?`, an unresolved parameter, an interface): only when the handler has an `//effectgo:errors` directive (contract mode); silenced by `-errset.ignoreopaque` |
| `effectgo:errors: cannot resolve "<name>" in <owner>` | an item of an errors directive does not resolve |
| `effectgo:errors on <Name> checks nothing: an interface contract covers the methods it declares, and <Name> declares none` | an errors directive on an interface type that declares no method of its own (it only embeds); reported at the type name |
| `effectgo:errors on <F> checks nothing: it has no error result` | an errors directive on a function with no error set: no error result, and no returned func literal with one |
| `effectgo:handler on <F> checks nothing: it has no error result; put the directive on the inner function that returns error (see docs/guide/http-checks.md#nethttp-handlers)` | a handler directive on a function with no error set, such as a `func(w, r)` handler |
| `effectgo:errmap: <F> must have signature func(error) error` | an errmap has the wrong signature |
| `effectgo:handler: cannot resolve "<name>"` / `… is not a function` / `… is not a //effectgo:errmap function` | a handler directive names something else |
| `unknown directive //effectgo:<kw>` | a keyword the errset analyzer does not know (also `… on an interface method or type`) |

`<via>` is ` (via <chain>)` when the variant comes through callees, and empty for a direct variant. Example, from `examples/todo`:

```text
api/v3.go:53:6: handler v3Update may return domain.ErrConflict (via Service.Update → Store.checkTitle), which errmap httpres.ToHTTPError does not map → HTTP 500
```

`-errset.report` and `-errset.witness` for the same package:

```text
domain/service.go:42:19: errors of Service.Update: domain.ErrConflict | domain.ErrInvalid | domain.ErrNotFound
domain/service.go:42:19: witness of Service.Update: domain.ErrConflict via Store.checkTitle
domain/service.go:42:19: witness of Service.Update: domain.ErrInvalid via checkTitle
domain/service.go:42:19: witness of Service.Update: domain.ErrNotFound via Store.modify
```

### Route handlers

A route rule in the stub files (`route <target> <METHOD> <path> <handler>`) makes the function that the call registers a route. `stubs/rt.stubs` names the `rt/web` functions `Get`, `Post`, `Put`, `Patch` and `Delete`. The route check needs no directive and runs in every run, default, `go vet` and the editor plugin alike. Every variant of the handler's error set must have a constant error status of its own, because `rt/web` encodes an error by its `StatusCode` and sends any other error as 500 (`ErrorBody` in `rt/web/web.go`):

```text
route POST /api/contact: handler Store.UpdateContact may return main.ErrLocked, which has no constant StatusCode → HTTP 500
```

- A variant is reported when it provably has no constant status: a fresh error (`errors.New`), a sentinel, a type whose method set has no `StatusCode`, or a type whose `StatusCode` is not a constant (a dynamic status). A constant outside 400 to 599 is not an error status and is reported too.
- A variant of unknown origin (`?`, an unresolved parameter, an interface) is reported only in contract mode, that is when the handler has an `//effectgo:errors` directive. Without the directive the check is silent. `-errset.ignoreopaque` silences the unknown-origin and the fresh findings of routes, as it does for handlers.
- A sentinel is taken to have no status of its own, as the errmap checks take it (errset does not evaluate initializers). A sentinel whose declared type has a `StatusCode` method is an unknown origin instead. Known gap: a sentinel declared as `error` whose initializer is a status-carrying value (`var ErrX error = NotFound{}`) is reported.
- A closure registered as a handler is reported, because its error set is not inferred. A closure in a `_test.go` file is not reported: a test registers closures to test the framework.
- A handler in another package is checked through its error-set fact. An interface method, a variable and a call are not resolved, so they are silent.

The route check does not use an errmap: the `rt/web` status rule is the variant's own `StatusCode`, not a mapping. The `DecodeError` (400) and `BodyTooLarge` (413) that `rt/web` adds to a route are not in the handler's set, so they are not checked.

## mustuse

| Message | When |
|---|---|
| `result of <callee> is discarded; handle it or assign to _` | a call statement discards an error result |
| `result of <callee> is discarded: <T> is //effectgo:mustuse; handle it or assign to _` | a call statement discards a value of a `//effectgo:mustuse` type; `<T>` is the type, for example `*a.Charge` |
| `//effectgo:mustuse takes no arguments` | the directive has a word after the keyword; a trailing `// comment` is not an argument. The type is still must-use |
| `effectgo:mustuse on <F> checks nothing: it has no results; //effectgo:mustuse marks a named type` | the directive is on a function with no results |
| `effectgo:mustuse on <F> checks nothing: //effectgo:mustuse marks a named type, not a function; put it on the named type that <F> returns` | the directive is on a function with results |
| `<Name>: //effectgo:mustuse applies to a defined type, not an alias; put it on the type the alias names` | the directive is on an alias |

Rules:

- A **bare call statement** is reported. `_ = f()` is the explicit discard and is silent. `go` and `defer` statements are not reported.
- An **error** result is reported only when errset cannot prove the callee's error set empty. A function that always returns nil is silent, and so is a callee with a stub rule `errors <target> none`. The shipped `stdlib.stubs` states this for `bytes.Buffer`, `strings.Builder` and `hash.Hash.Write`, with source lines in the file.
- A **`//effectgo:mustuse` value** is reported whatever the error set.
- The built-in allowlist is `fmt.Print*` and `fmt.Fprint*`. `-mustuse.allow=os.Remove,(*os.File).Close` adds fully qualified names. A call through a local func variable is matched by the function assigned to it, when that is unique.
- The writer rules (`none`) come from the stub configuration. Under an explicit `-stubs` file that omits them, `buf.WriteString("x")` is reported.

Message examples:

```text
result of os.Remove is discarded; handle it or assign to _
result of a.Mark is discarded: *a.Charge is //effectgo:mustuse; handle it or assign to _
```

The two forms tell apart the error case (the callee returns an error that is not checked) and the marker case (the value's type is marked). A call that returns both a marked value and an error reports the marker form.

## exhaustive

| Message | When |
|---|---|
| `missing cases in type switch on <I>: <V1>, <V2>` | a type switch on a sealed interface has no `default` and misses a variant |
| `missing cases in switch on <E>: <C1>, …` | an expression switch on a sealed enum has no `default` and misses a constant |
| `//effectgo:sealed takes no arguments` | the directive has a word after the keyword; a trailing `// comment` is not an argument. The interface or enum is still sealed |
| `sealed interface <I> must have an unexported method` | `//effectgo:sealed` on an interface without one |
| `<Name>: //effectgo:sealed applies to interfaces and to integer or string types` | the directive is on another kind of declaration |

Rules:

- An interface is **sealed** when it declares an unexported method in its own package, or when it carries `//effectgo:sealed`. Its variants are the named types of its package that implement it (`T` or `*T`), sorted by name. Generic types count once.
- A named integer or string type with `//effectgo:sealed` is an **enum**. Its variants are the distinct values of its package-level constants; only exported constants are required outside the package.
- A case that is a non-constant leaves an enum switch undecided, so it is not reported. An interface case covers every variant that implements it.
- Variants come from non-test files only: a type declared in a `_test.go` file is neither required nor counted.
- There is no implicit enum rule. A named integer type without the directive is not checked.

Message examples: `missing cases in type switch on Shape: Square, Triangle` and `missing cases in switch on Status: Archived`.

## schematag

Checks the types that flow into rt: the input and output type arguments of `web.Get`, `Post`, `Put`, `Patch` and `Delete`, and the type argument of `schema.Decode` and `schema.Compile`. It follows every struct, slice element, map value and pointer reachable from them. A type that never reaches rt is not checked, so a `schema:"..."` tag of another library (gorilla/schema) is not a finding. A call in a `_test.go` file is not checked: a test registers bad routes on purpose, as errset exempts a closure in a test.

| Message | When |
|---|---|
| `field S.F: schema tag "T": unknown rule "R"; the rules are email, max, min, nonempty, pattern, singleline, trim` | a rule that `rt/schema` does not have |
| `field S.F: schema tag "T": "A" is not an integer` (also `is not an unsigned integer`, `is not a number`, `is not a length`) | a `min=`/`max=` argument that does not parse for the field's kind |
| `field S.F: schema tag "T": rule "R" does not apply to K` | a rule on a field of the wrong kind, e.g. `email` on an `int`, `min` on a `bool`, `nonempty` on an `int` |
| `field S.F: schema tag "T": rule "R" takes no argument; write R` | `trim=1`, `email=x` |
| `field S.F: schema tag "T": rule "R" needs a value; write R=N` | `min` or `max` with no value |
| `field S.F: schema tag "T": rule "pattern" needs a pattern name; write pattern=NAME` | `pattern` with no name |
| `field S.F: schema tag "T": rule "R" is repeated; keep one` | the same rule twice |
| `field S.F: schema tag "T": empty rule; remove the extra comma` | `trim,,max=3` or a trailing comma |
| `field S.F: type T is not supported by schema; use a string, …` | a `chan`, `func`, array, `complex`, `uintptr` or unsafe pointer field |
| `field S.F: byte slices are not supported; use a string or json.RawMessage` | `[]byte` |
| `field S.F: map key must be a string kind, got K; use map[string]T` | a map with a non-string key |
| `field S.F: non-empty interface I is not supported; use a concrete type or any` | an interface with methods |
| `field S.F: embedded fields are not supported; declare the field by name` | an embedded field |
| `field S.F: json name "N" is used by two fields of S; rename one` | two fields with the same JSON name |
| `field S.F: unexported field has a schema tag; schema reads only exported fields, so export it or remove the tag` | a schema tag on an unexported field |
| `field S.F: type E is a sealed enum with no Valid() bool method, so Decode cannot limit its values; add func (E) Valid() bool …` | a `//effectgo:sealed` enum that does not implement `schema.Enum` |
| `route M P: field F has path:"x", but the pattern has no {x}; add {x} to the pattern or change the tag` | a path tag whose name is not a wildcard of the constant pattern |
| `route M P: pattern {x} has no path field; add a field with path:"x"` | a wildcard with no path field |
| `route M P: path field F must be a scalar (string, number, bool or time.Time), got T` | a path field of another kind |
| `route M P: query field F must be a scalar or a slice of scalars, got T` | a query field of another kind |
| `route M P: field F has an empty query name` | `query:""` |
| `route M P: field F has both path and query tags; keep one` | both tags on one field |
| `route M P: field F has a path or query tag but json:"-"; remove the json "-"` | a path or query field that the JSON decode skips |
| `route M P: M takes no body, but field F has no path or query tag; …` | a body field on GET or DELETE |
| `route M P: pattern has {x}, but the input is web.NoInput; …` | a wildcard with a `NoInput` input |
| `route M P: input T must be a struct or web.NoInput` | a non-struct input |

`M P` is the method and the constant pattern of the route; a pattern that is not a constant is written as its source text, and the checks that need the pattern are not made. A problem of a field declared in the package is reported at the field, once. A problem of a field declared in another package is reported at the call that brings the type into rt, with the field's position appended as `(field declared at …)`. Output types get the schema checks only.

Rules:

- The vocabulary and the supported kinds are those of `rt/schema`. `schematag_test.go` reads `rt/schema/schema.go` and fails when a rule, a kind's category or a supported kind differs from the analyzer's copy, because this module cannot import rt.
- A pattern name (`pattern=NAME`) is checked for its format only: a name is required, and the rule must be on a string field. Whether the name is registered is not checked. Proving that no package in the program registers it would need the whole program, which a single vet pass does not have; a missing registration is caught at startup instead, because `rt/web` panics when it registers a route whose pattern name is unknown.
- A sealed enum is one that `exhaustive` treats as sealed (`//effectgo:sealed` on a named integer or string type, in a non-test file). The enum fact is exported by this analyzer, so it is seen across packages.
- The `Valid` check requires `Valid() bool` on `T` or `*T`, as `schema.Enum` does. It does not check that `Valid` is pure. The `effects` analyzer checks `Valid` against the `//effectgo:pure` contract of `schema.Enum` only in a package that imports `rt/schema` (directly or through another package), because its conformance check looks at the contracted interfaces of the package's imports. A `Valid` in a package that does not import `rt/schema` is not checked.

## directive

The directive analyzer reports a `//effectgo:` directive that no analyzer reads where it is written, and a comment that has the form of a directive but a space after the slashes. Each analyzer reads its keywords only at the placements listed in [Directives](directives.md#placement-rules-that-are-easy-to-miss). The analyzer keeps a table of those placements, and it reports a directive only where no analyzer reads or reports it, so each misplaced directive is printed once.

| Message | When |
|---|---|
| `//effectgo:<kw> is not attached to a function, interface or type declaration and is not read` | a directive on a `var` or `const`, in a function body, on a function literal, on a struct field, on an embedded interface, or in a comment that a blank line separates from its declaration |
| `//effectgo:<kw> on function <F> is not read: <kw> applies to <what>` | `sealed` on a function; `errors`, `errmap` or `handler` on a function without a body, where the message reads `<F> has no body, and only functions with a body are checked` |
| `//effectgo:<kw> on method <T>.<M> is not read: <kw> applies to <what>` | `sealed` or `mustuse` on a named method of an interface |
| `//effectgo:<kw> on type <T> is not read: <kw> applies to <what>` | `pure`, `allow`, `errors`, `errmap` or `handler` on a type that is not an interface |
| `//effectgo:<kw> is in the doc comment of a group of several types and is not read: put it on each type` | a keyword on the doc comment of a `type` group that declares several types, where no analyzer reads it |
| `//effectgo:<kw> is in the doc comment of a type group, and the type has a doc comment of its own: put it on the type's doc comment` | a keyword on the doc comment of a one-type group whose type has a doc comment of its own, for a keyword that reads only the type's own doc comment |
| `//effectgo:sealed is not read in a _test.go file: sealed types are checked only in non-test files` | `sealed` on a type or interface declared in a `_test.go` file |
| `//effectgo:<kw> is not read in a package doc comment: only //effectgo:package is valid there` | `errors`, `errmap`, `handler`, `mustuse` or `sealed` in a package doc comment; effects reports `pure`, `allow` and unknown keywords there itself |
| `//effectgo:package is only valid in a package doc comment` | `package` wherever no analyzer reports it: a var, a const, a body, a struct field, a type that is not an interface, a group |
| `unknown effectgo directive "<kw>" is not read here` | an unknown keyword on a type declaration or a type group |
| `empty //effectgo: directive is not read here` | `//effectgo:` with no keyword on a type declaration or a type group |
| `// effectgo:<kw> is not a directive: remove the space after //` | a comment with a space after the slashes, when the word after `effectgo:` is a directive keyword; any position |

`<what>` is `functions, interfaces and interface methods` for `pure`, `allow` and `errors`; `functions` for `errmap` and `handler`; `named type declarations` for `mustuse`; and `interfaces and named integer or string types` for `sealed`.

Rules:

- A directive that an analyzer reads at its place is silent. So is a directive that another analyzer reports itself (`package` on a function, `pure` in a package doc comment, `errmap` on an interface): that analyzer's message is the only one.
- A directive is placed by its keyword, read as the words after the prefix up to the first word that starts with `//`. A keyword with an argument it does not take is placed like the bare keyword; the analyzer that reads it reports the argument (see [Directives](directives.md#placement-rules-that-are-easy-to-miss)).
- The spacing check reads every comment in the file, whatever it sits on. It needs a directive keyword right after `effectgo:`; prose that mentions `effectgo:` is not reported.
- The analyzer has no flags. It runs in the standalone run, under `go vet`, and in the golangci-lint plugin.

Message examples, from the probe of the inert placements (its file is `p.go`):

```text
p.go:41:1: //effectgo:pure is not attached to a function, interface or type declaration and is not read
p.go:57:1: // effectgo:pure is not a directive: remove the space after //
p.go:78:7: //effectgo:pure is not attached to a function, interface or type declaration and is not read
```

## signature

`effectgo sig` prints one line per declared function:

```text
domain/service.go:42:19: Service.Update: Effect<Todo, ErrConflict | ErrInvalid | ErrNotFound, {}>
```

- **A**: the non-error results; `void` if none; a tuple if several. For a function returning a handler literal, A is the handler's results.
- **E**: the error set (see [Error sets](errors.md)), or `never` when empty. Variants of the analyzed package are unqualified.
- **R**: the effect set, sorted, `{}` when empty. Placeholders such as `param:1` are shown.

The subcommand exits with status 3 when it printed a signature, because each signature is a diagnostic. The normal run prints no signatures. Known gap: a function that returns a func value that is not a literal (for example a wrapper such as `NoBody(h)`) has its func type as A, not the handler's results.

## Diagnostic formats in the other modes

The HTTP checks (`http-gin`, `http-huma`, `http`) and `openapi` print their own formats: see [CLI](cli.md) and [HTTP checks](../guide/http-checks.md).
