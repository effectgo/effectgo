---
title: Error sets
description: Error variants, how each function's error set is inferred and narrowed by guards, fresh and opaque errors, foreign collapse, error contracts, and how variants map to HTTP statuses.
---

The errset analyzer computes, for every function with an error result, the set of error variants it can return. The set is written `E` in `Effect<A, E, R>`. It is the basis of the error contracts, the handler check, OpenAPI, and the HTTP adapters.

## Variants

| Form | Name in output | What it is | Matchable with `errors.Is`/`As` |
|---|---|---|---|
| sentinel | `pkg.ErrX` | a package-level error variable (`var ErrX = errors.New(…)` is a sentinel, not fresh) | yes |
| type | `pkg.T` | a value or pointer of a type that implements `error` (`&T{}`, `new(T)`, `T{}`, a conversion) | yes, when the type is named |
| interface | `iface:pkg.I` | an interface target, from a stub or a contract | matched by method set |
| carried | `huma.Error404NotFound`, `huma.NewError(404)` | a call to a status constructor (see [HTTP status](#http-status)) | yes |
| fresh | `new@pkg.F` | an error created in `F` without wrapping (`errors.New`, or `fmt.Errorf` with a constant format and no `%w`) | no |
| opaque | `?` | an error whose origin is unknown | no |
| param | `param:i` | the error of the function's own func-typed parameter `i` (in a set of a function that calls it) | substituted at call sites |

`?` also covers a foreign variant that has been collapsed (see [Foreign variants](#foreign-variants)).

Sets are printed with `|` between variants, `?` last, and `(none)` when empty:

```text
errors of Service.Update: domain.ErrConflict | domain.ErrInvalid | domain.ErrNotFound
errors of ToHTTPError: fuego.BadRequestError | fuego.NotFoundError | ?
```

## How a set is computed

The default engine is **SSA**, flow-sensitive over the program's SSA values (`-errset.engine=ssa`). The older **AST** engine (`-errset.engine=ast`) is flow-insensitive per declaration and is kept for comparison. Both share the variants, directives, stubs, contracts and the fixed point; they differ in the set of a value and in narrowing.

Under SSA:

1. **Values.** A call takes the callee's set (with `param:i` bound to the arguments). A load of a sentinel global is that sentinel. `nil` is empty. A conversion to an interface of a concrete error type T is T. A type assertion is the asserted type. A `Phi` (a merge point) is the union of its incoming values, each taken on its own edge. `fmt.Errorf` with a constant format takes its `%w` arguments, and without `%w` it is fresh. `errors.Join` is the union of its arguments. Anything unresolved is `?`.
2. **Captured or address-taken variables** have the union of every value stored into them, except a named result, which is read at each return (see [Named results and defers](#named-results-and-defers)).
3. **Narrowing.** Each branch edge narrows the value it tests (rules below).
4. **Returns.** A function's set is the union of the effective sets of its error result at every `return`. A literal's set is its own returns. A returned literal gives the function a *handler set*.
5. **Fixed point.** Sets only grow, and a witness is replaced only by a strictly shorter one. A unit that does not converge in 200 rounds falls back to the AST engine. Generic function bodies are not lowered by SSA and use the AST result.
6. **Per-use answers.** Each use of an error expression sees the variants that reach it. An adapter reads a variable at its own use, so a variable reused for two calls carries only the variants of the call that reaches the sink.

Under AST, a local variable is the union of every assignment to it in the function, and a call through a local variable is opaque.

## Narrowing

On the true edge of a branch whose condition has one of these forms, the subject value `v` is narrowed. On the false edge, the opposite is applied (`!` and `switch` follow the same rules; an `if`-chain is a `switch`). A *wrapper* is a concrete type with an `Unwrap`, `Is` or `As` method, or a sentinel of such a type. An *unknown origin* is an opaque, param or interface variant.

| Condition | True edge | False edge |
|---|---|---|
| `errors.Is(v, S)` | keep `S`, its wrappers, unknown origins | remove `S` |
| `v == S` | keep `S` and unknown origins (no wrappers: `==` does not unwrap) | remove `S` |
| `v == nil` | empty (the nil edge carries nothing) | unchanged |
| `errors.As(v, &x)`, `x` of concrete type `T` | keep `T`, wrappers, unknown origins | remove `T` |
| `errors.As(v, &x)`, `x` of interface `I` | keep the variants that implement `I` (a fresh error only if `I` is `error`), wrappers, unknown origins | remove the known variants that implement `I` |
| `_, ok := errors.AsType[T](v); ok` | as `errors.As` for target `T` | as `errors.As` for target `T` |
| `w, ok := v.(T)` | as `errors.As` for `T`, without wrappers | as `errors.As` for `T` |
| `P(v)`, where `P` is a predicate | the test that `P` names, on `v` | the same, negated |

A **predicate** is a `func(error) bool` whose every return is a constant false, a call of `errors.Is` or `errors.As` on its parameter (or of another predicate), or the constant true reached only through the true edge of such a test. It exports its test to importing packages, so `IsValidation(err)` narrows at its call sites in other packages.

For Go 1.26's `errors.AsType[T]`, SSA follows the boolean result through `ok`, `!ok`, and an assignment followed by a later test. Recognition uses the standard-library package path and the generic function's origin, so import aliases work and unrelated functions named `AsType` do not narrow. Concrete pointer/value error types and interface targets use the same matching rules as `errors.As`. The matched value has the concrete target's error variant; an interface result remains opaque. Type arguments must satisfy the standard library's `error` constraint.

An AsType predicate exports a test only for a straight-line `return ok`, or an entry `if ok` whose two branches return `true` and `false`. Additional control flow is not treated as an equivalent test; in particular, an earlier `return false` must not remove the target from the caller's false branch.

The AST engine does not narrow after `errors.AsType`, just as it does not narrow after `errors.As`; use the default SSA engine for these guards. The AST readers used by HTTP status maps and the Gin/Huma adapters recognize `if _, ok := errors.AsType[T](err); ok`. They do not recognize a negated condition, an assignment in an earlier statement, or a compound condition as an AsType mapping rule. Huma does not export an AsType wrapper as a mapping predicate. This HTTP mapping restriction also applies with the SSA error-set engine.

A reassigned variable, and a value reset by a guard (`err = nil`), no longer carry the old variants.

Only `errors.Is`, `errors.As`, `errors.AsType`, `==` and type assertions narrow. `errors.Unwrap` chains, `reflect`, and user `Is`/`As` methods are not evaluated.

Consequences to expect: findings can disappear (the guarded path no longer reaches a sink), and a few can appear (a call through a local closure is resolved to the literal, and a per-use read sees the variable at its own use).

## Named results and defers

A function with a named error result (`func f() (err error)`) keeps it in a variable, and so does any result that a closure captures. A `return` stores its value into that variable, runs the deferred calls, and returns what the variable holds. The set of such a function is computed from that sequence, not from every value ever stored into the variable:

- each `return` contributes the value it stores, narrowed as usual. A `return err` that reads the result contributes the value the result holds at that point: the store that reaches the read along a straight line of blocks (for example the assignment in `if err = f(); err != nil { return err }`);
- a closure that writes the result contributes what it writes. A deferred closure runs at the return, so its write is part of the result;
- a deferred closure that only reads the result reads the value at the return. It does not add the values of earlier assignments.

```go
func Hold(r Reservation) (err error) {
	tx, err := db.Begin() // a raw driver error: stored, but not returned
	if err != nil {
		return storageErr("begin", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	return storageErr("insert", insert(tx, r))
}
```

The set of `Hold` is the set of `storageErr`, not the raw error of `db.Begin`. A closure that wraps the result keeps the variants it wraps:

```go
defer func() {
	if err != nil {
		err = fmt.Errorf("commit: %w", err) // the wrapped value's variants
	}
}()
```

A `%v` in the wrap makes the write opaque (`?`), as it does everywhere else. Four cases keep the older, wider set:

- **the address of the result escapes**: `defer fill(&err)` passes `&err` to a call that may write it, so the set adds `?`;
- **the function calls `recover`**: a recovered panic returns the result as it was at the panic, which can be any stored value, so the set is the union of every store;
- **a naked return** (`return` with no values) returns the variable as it is, so its set is the union of the stores;
- **a read across a join** (two paths that assign the result meet before the read) has no single reaching store, so it reads the union of the stores;
- **a closure started with `go`** or called in the body is not a deferred call, so it reads the union of the stores.

The AST engine (`-errset.engine=ast`) keeps the union for every variable. The rules are in the errset package (`ssadefer.go`). The effects analyzer does not have this problem: its effects come from calls, not from values.

## Fresh errors (`new@`)

`errors.New(...)`, and `fmt.Errorf` with a constant format and no `%w`, produce a fresh variant when they reach a returned expression or a returned local. The variant is keyed by the **declared function that creates it** (a literal counts for its enclosing function), not by call site, so that sets stay small. The witness still shows the creator. `new@` variants cannot be matched by `errors.Is` or `errors.As`.

- A non-constant format in `fmt.Errorf` gives `?`.
- A package-level `var ErrX = errors.New(...)` is a sentinel, because the sentinel check runs first.
- In the handler check and in OpenAPI, a fresh error is not covered by any errmap and is an HTTP 500.

## Opaque errors (`?`)

`?` is an error whose origin is not known: a parameter, a field, a map value, a channel receive, an interface call without a contract or stub, a stdlib error that no stub names (except the fresh and `%w` cases above). `%v` in `fmt.Errorf` gives `?`.

## Foreign variants

Two kinds of variant are shown as `?` when they come from outside the analyzed module:

- a sentinel, type or interface whose identifier is unexported (for example `pgconn.connLockError`): code outside its package cannot name it, so it cannot be matched;
- a fresh `new@` variant created by a function of a foreign package, whatever the creator's name (for example `new@pgconn.Pipeline.Flush`). A fresh error is never matchable, so the name only exposes a dependency's internals.

The original name is kept in witness lines: `witness of F: ? (pgconn.connLockError) via …`.

Exported foreign sentinels and types are kept (`pgx.ErrNoRows`, `sql.ErrNoRows`, `context.Canceled`, `*url.Error`). The analyzed module is the module of the package whose report or check runs; a package is inside it when its path is the module path or below it. Without module information nothing is collapsed. A collapsed type keeps its HTTP status.

## Witnesses

`-errset.witness` prints one `witness of <F>: <variant> via <chain>` line per variant (`witness of <F> [handler]: …` for handler sets). The chain is the callee names from the function down to the origin, the function itself excluded. `(direct)` means the variant is created in the function. The chain is the shortest one found.

## Error contracts

`//effectgo:errors <items>` on a function, interface method or interface type: every variant of the function's set must be allowed by the list (see [Directives](directives.md)). `new` allows fresh variants and `?` allows opaque variants. The two do not allow each other, because a fresh error is a known creation point and an opaque one is not. `iface:` items never match fresh or opaque variants, which have no method set.

Violation:

```text
Service.Update may return domain.ErrConflict (via Store.checkTitle), not in its //effectgo:errors contract
```

## Error maps and the handler check

`//effectgo:errmap` on a `func(error) error` exports the variants it maps and the interfaces it targets. Its mapping is approximate: any `==` or `switch` case against a sentinel counts as mapped, whatever the branch returns.

`//effectgo:handler <errmap>` checks that every variant of the handler's set is covered. A variant is covered when its key is mapped, or when it is an interface target whose methods it implements. Uncovered variants are HTTP 500 findings (see [Directives](directives.md)). `-errset.ignoreopaque` silences the unclassified and fresh ones.

## HTTP status

`-errset.statuses` prints, for each handler, the status of each variant:

```text
statuses of v3Update: 400 domain.ErrInvalid | 400 fuego.BadRequestError | 404 domain.ErrNotFound | 500 domain.ErrConflict | 500 ? | default fuego.HTTPError
```

The status of a variant comes from, in order:

1. a `StatusFact`: a `StatusCode() int` method whose returns are all one constant (facts from dependencies work);
2. a `status (pkg.Type) <code>` stub line, as a fallback;
3. otherwise the status is dynamic, and it is unknown.

Rules of an errmap give statuses: the constant of a returned composite literal (`T{…}` or `&T{…}`) when its type has a constant `StatusCode`; otherwise the constant `Status` field of a literal; `return err` is a passthrough that keeps the variant's own status; a return that cannot be read, or a mix of statuses in one rule, is unknown.

How to audit an errmap: run `effectgo -errset.statuses ./...`. Each handler prints the status of every variant, so a `default` or a `500` that you did not intend is visible at once. The rules that produce a status, including the constant `Status` field and the `StatusCode` method, are in [Directives](directives.md#how-an-errmap-gives-a-status).

Then the sending rule:

- a plain error (no status) and an opaque `?` are **500**: Fuego's error sender starts at 500 and only an error that carries a status overrides it;
- an interface variant, an unresolved `param:i`, or a dynamic status is **default** (unknown). It is never given a guessed code.

Fuego evidence is in the `fuego.stubs` file and in the [OpenAPI journal](../journal/05-p1-openapi.md). Huma's rules, which differ (a status-carrying error keeps its status; any other error is 500), are in [HTTP checks](../guide/http-checks.md#huma).

## Interface matching

An interface target is matched by comparing method-signature strings qualified by package path. This approximates `types.Implements`. A sentinel has only the method set of `error`, so only an `iface:` target whose methods are a subset of `{Error()}` can cover it.

## Limitations

See [Limitations](../design/limitations.md#error-sets).
