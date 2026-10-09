---
title: Analyzers and diagnostics
description: What each analyzer (effects, errset, mustuse, exhaustive, signature) reports, with the exact message formats, and when each one is silent.
---

The binary contains five analyzers. Four report diagnostics in the normal run; `signature` prints signatures and runs only under `effectgo sig`.

| Analyzer | Enabled by | Reports | Reads |
|---|---|---|---|
| `effects` | default | contract violations; with flags, effect sets and witnesses | `//effectgo:pure`, `allow`, `package`; `effects` stub rules |
| `errset` | default | error contract violations, handler coverage, directive errors; with flags, error sets and witnesses | `//effectgo:errors`, `errmap`, `handler`; `errors`, `status`, `sink` (Gin), `route` stub rules |
| `mustuse` | default | discarded errors and discarded `//effectgo:mustuse` values | `//effectgo:mustuse`; errset's "never fails" evidence |
| `exhaustive` | default | switches on sealed types that miss a variant | `//effectgo:sealed` |
| `signature` | `effectgo sig` only | one `Effect<A, E, R>` line per function | the other three, as dependencies |

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
| `effectgo:errors: cannot resolve "<name>" in <owner>` | an item of an errors directive does not resolve |
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

## mustuse

| Message | When |
|---|---|
| `result of <callee> is discarded; handle it or assign to _` | a call statement discards an error result |
| `result of <callee> is discarded: <T> is //effectgo:mustuse; handle it or assign to _` | a call statement discards a value of a `//effectgo:mustuse` type; `<T>` is the type, for example `*a.Charge` |
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
| `sealed interface <I> must have an unexported method` | `//effectgo:sealed` on an interface without one |
| `<Name>: //effectgo:sealed applies to interfaces and to integer or string types` | the directive is on another kind of declaration |

Rules:

- An interface is **sealed** when it declares an unexported method in its own package, or when it carries `//effectgo:sealed`. Its variants are the named types of its package that implement it (`T` or `*T`), sorted by name. Generic types count once.
- A named integer or string type with `//effectgo:sealed` is an **enum**. Its variants are the distinct values of its package-level constants; only exported constants are required outside the package.
- A case that is a non-constant leaves an enum switch undecided, so it is not reported. An interface case covers every variant that implements it.
- Variants come from non-test files only: a type declared in a `_test.go` file is neither required nor counted.
- There is no implicit enum rule. A named integer type without the directive is not checked.

Message examples: `missing cases in type switch on Shape: Square, Triangle` and `missing cases in switch on Status: Archived`.

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
