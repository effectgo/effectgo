---
title: Directives
description: "Every //effectgo: directive, where it may appear, what it checks, and the diagnostics it produces."
---

A directive is a comment that starts with `//effectgo:` (no space after the slashes). Directives are read by the analyzers in the same binary; each analyzer reads its own keywords and skips the others'. An unknown keyword is reported by each analyzer that reads the prefix, so in the combined binary a typo can appear twice.

Every directive is a contract: the analyzer infers what the code can do, and a directive states what it must not exceed. A violation is a diagnostic at the declaration. For the workflow, see [Contract mode](../guide/contract-mode.md).

## Summary

| Directive | Analyzer | Where | Checks |
|---|---|---|---|
| `//effectgo:pure` | effects | function, interface method, interface type | the effect set is empty |
| `//effectgo:allow <effects>` | effects | function, interface method, interface type | the effect set is a subset of the list |
| `//effectgo:package pure` / `//effectgo:package allow <effects>` | effects | package doc comment only | default contract for every function of the package |
| `//effectgo:errors <variants>` | errset | function, interface method, interface type | every error variant is in the list |
| `//effectgo:errmap` | errset | a `func(error) error` | (declares an errmap; checks its signature) |
| `//effectgo:handler <errmap>` | errset | an HTTP handler or handler constructor | every error variant is mapped by the errmap |
| `//effectgo:mustuse` | mustuse | a named type declaration | a discarded value of this type is reported |
| `//effectgo:sealed` | exhaustive | an interface with an unexported method, or a named integer or string type | switches cover every variant |

## Effects directives

### `//effectgo:pure`

Placed on a function, an interface method or an interface type.

```go
//effectgo:pure
func Normalize(s string) string {
	return strings.TrimSpace(s) + infra.Stamp()
}
```

On a function, the inferred effect set must be empty. On an interface method, every implementation must satisfy the contract (see [Effects](effects.md#interface-contracts)). On an interface type, the directive applies to every explicitly declared method, and a method-level directive overrides it.

Takes no arguments; `//effectgo:pure foo` is reported as `//effectgo:pure takes no arguments`.

Diagnostic (effects), one per effect, at the function name:

```text
Normalize is declared //effectgo:pure but has effect time via Normalize → infra.Stamp → time.Now
```

### `//effectgo:allow <effects>`

Takes a comma- or space-separated list of effect names (see [Effects](effects.md#effect-names)).

```go
//effectgo:allow db
func LoadAllowed(r repo.Repo, id int) (string, error) {
	return r.Get(id)
}
```

The effect set must be a subset of the list. `unknown` is allowed only when it is listed. Several directives on one function are all enforced (their intersection).

Errors reported by the directive parser:

- `//effectgo:allow needs at least one effect name`
- `unknown effect name "x" in //effectgo:allow`

Diagnostic for a function that exceeds its contract. The text after "declared" is the contract, here the package default of `examples/layers/domain`, which is `//effectgo:package pure`:

```text
Save is declared //effectgo:package pure but has effect net via Save → infra.Fetch → http.Head
```

For a function's own `//effectgo:allow` the form is the same: `<Func> is declared //effectgo:allow <effects> but has effect <e> via <witness>`.

### `//effectgo:package pure` and `//effectgo:package allow <effects>`

Valid only in a package doc comment (the comment before `package`):

```go
// Package domain holds the pure business rules of the todo app.
//
//effectgo:package pure
package domain
```

The package directive is the default for every function of the package that has no directive of its own. A function's own directives replace it. When two files of a package carry different package directives, the first valid one wins and the other is reported as `conflicting package directive ...; keeping ...`.

The default does not apply to a function declared in a `_test.go` file. A table of func values in a test is test data, not the code under contract, so a test function under `//effectgo:package pure` is not checked against it. A directive written on a test function itself is still enforced, so a test can state a contract on purpose:

```go
// TestPure is a test of the pure package: its own directive is still checked.
//
//effectgo:pure
func TestPure(t *testing.T) {
	// ...
}
```

Only the effects analyzer applies package defaults; the errset analyzer reads the directive but does not use it. The rule is the same under `effectgo ./...` and under `go vet`, which also analyzes test files.

Errors reported by the parser:

- `//effectgo:package is only valid in a package doc comment`
- `//effectgo:package needs "pure" or "allow <effects>"`
- `//effectgo:<kw> is not valid in a package doc comment; use //effectgo:package` (any other directive in a package doc comment)

### Interface contracts

A directive on an interface method (or on the interface type) is a contract for every call through that interface. Importers see it, so contracts work across packages. Conformance is checked: each named type that implements the interface must satisfy the contract of each method it implements. The message is:

```text
(*HTTPRepo).Get violates contract of (Repo).Get [allow db]: has effect net via (*HTTPRepo).Get → http.Get
```

Reported at the method's name when it is declared in the analyzed package, or at the type's name when the method is promoted.

## Errset directives

### `//effectgo:errors <variants>`

Placed on a function, an interface method or an interface type. The list names the error variants the function may return (an illustrative line; the names must exist):

```go
//effectgo:errors domain.ErrNotFound, domain.ErrInvalid, new, ?, iface:example.com/app.Retryable
func (s *Service) Update(...) error { ... }
```

Each item is one of:

| Item | Allows |
|---|---|
| `pkg.Name` (for example `domain.ErrNotFound`) | that sentinel or error type; the name is resolved in the package scope or through the file's imports |
| `iface:pkg.I` | every variant whose method set implements the interface (not fresh or opaque variants) |
| `new` | every fresh variant (`new@…`) |
| `?` | every opaque variant (`?`), including collapsed foreign variants |

`new` and `?` do not allow each other. A name that cannot be resolved is reported as `effectgo:errors: cannot resolve "<name>" in <owner>`.

Violation:

```text
Service.Update may return domain.ErrConflict (via Store.checkTitle), not in its //effectgo:errors contract
```

### `//effectgo:errmap`

Placed on a function of type `func(error) error`. It declares an errmap: a function that maps domain errors to HTTP errors. The analyzer reads which variants the function maps (from `errors.Is`, `==`, `errors.As` on a concrete type, a `switch` case, or a type switch) and which interfaces it targets, and exports that coverage.

```go
// ToHTTPError maps domain errors to HTTP errors.
//
//effectgo:errmap
func ToHTTPError(err error) error {
	var withStatus fuego.ErrorWithStatus
	if errors.As(err, &withStatus) {
		return err
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return fuego.NotFoundError{Err: err, Detail: err.Error()}
	case errors.Is(err, domain.ErrInvalid):
		return fuego.BadRequestError{Err: err, Detail: err.Error()}
	default:
		return err
	}
}
```

Diagnostic when the signature is wrong:

```text
effectgo:errmap: <Func> must have signature func(error) error
```

### `//effectgo:handler <errmap>`

Placed on an HTTP handler, or on a function that returns a handler literal (a handler constructor). The argument names an errmap function, which may be qualified (`httpres.ToHTTPError`):

```go
//effectgo:handler httpres.ToHTTPError
func v3Update(svc *domain.Service) func(fuego.ContextWithBody[updateReq]) (domain.Todo, error) {
	...
}
```

The handler's error set (for a constructor, the set of the returned literal) must be covered by the errmap. A variant is covered when its key is mapped, or when it is an interface target whose methods it implements. Every uncovered variant is reported as an HTTP 500 finding:

```text
handler v3Update may return domain.ErrConflict (via Service.Update → Store.checkTitle), which errmap httpres.ToHTTPError does not map → HTTP 500
handler v3Create may return an unclassified error (via (Context).Body [stub]), which errmap httpres.ToHTTPError does not map → HTTP 500
handler <F> may return fresh error new@pkg.F (…), which errmap <M> does not map (errors.Is and errors.As cannot match it) → HTTP 500
handler <F> may return an unresolved param:0 error (…), which errmap <M> does not map → HTTP 500
```

`-errset.ignoreopaque` silences the unclassified and fresh findings. Diagnostics about the directive itself:

- `effectgo:handler: cannot resolve "<name>"`
- `effectgo:handler: <name> is not a function`
- `effectgo:handler: <name> is not a //effectgo:errmap function`

Status of each variant under the errmap: `-errset.statuses` prints it (see [CLI](cli.md#analyzer-flags)); the HTTP status rules are in [Errors](errors.md#http-status).

## Must-use and exhaustive directives

### `//effectgo:mustuse`

Placed on a named type declaration. A call statement that discards a value of this type (or of a pointer to it) is reported, whatever the callee's error set:

```go
//effectgo:mustuse
type Receipt struct{ ID string }
```

An alias is not allowed here: the directive on an alias is a diagnostic (`… applies to a defined type, not an alias; put it on the type the alias names`). Mark the type the alias names.

### `//effectgo:sealed`

Two uses:

- on an interface, to make it sealed. The interface must declare an unexported method of its own package (the marker method that only that package can implement); without one the directive is a diagnostic. An interface with an unexported method is sealed even without the directive;
- on a named integer or string type, which makes it an enum: its variants are the distinct values of its package-level constants.

```go
//effectgo:sealed
type Status int

const (
	Active Status = iota
	Archived
)
```

Diagnostics:

- `sealed interface <I> must have an unexported method` (an interface marked sealed without one)
- `<Name>: //effectgo:sealed applies to interfaces and to integer or string types`

Without the directive, a named integer type is not an enum: the exhaustive analyzer does not infer enums (see [Analyzers](analyzers.md#exhaustive)).

## Unknown keywords

```text
unknown effectgo directive "purr"                       (effects)
unknown directive //effectgo:purr                       (errset; on an interface method or type: "unknown directive //effectgo:purr on an interface method or type")
```

Each analyzer reports the keyword once, so the combined binary can print two lines for one typo.

## Placement rules that are easy to miss

- `//effectgo:package` only in a package doc comment; the other directives are not valid there.
- Directives are read on function declarations and interface declarations. A directive on a function literal is not read.
- A directive that names a package (`httpres.ToHTTPError`, `iface:…`) needs that package to be imported by the file. Otherwise the name cannot be resolved (`effectgo:errors: cannot resolve …`).
