---
title: Directives
description: "Every //effectgo: directive, where it may appear, what it checks, and the diagnostics it produces."
---

A directive is a comment that starts with `//effectgo:` (no space after the slashes). Directives are read by the analyzers in the same binary; each analyzer reads its own keywords and skips the others'. An unknown keyword is reported by each analyzer that reads the prefix, so in the combined binary a typo can appear twice. A directive is read only where its keyword applies (see [Placement rules](#placement-rules-that-are-easy-to-miss)); elsewhere it checks nothing, and the directive analyzer reports it.

Every directive is a contract: the analyzer infers what the code can do, and a directive states what it must not exceed. A violation is a diagnostic at the declaration. For the workflow, see [Contract mode](../guide/contract-mode.md).

The keyword is the first word after `//effectgo:`, and the words after it are its arguments, up to the first word that starts with `//`. So a trailing comment is not an argument: `//effectgo:pure // checked by CI` is the `pure` directive, and every analyzer reads it so. A word after a keyword that takes none is an argument error (`//effectgo:pure foo` is reported as `//effectgo:pure takes no arguments`, and the directive still applies). A keyword glued to a comment, as in `//effectgo:mustuse//note`, has no space before the `//`, so the word is not a known keyword and is reported as unknown.

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

A stream held in a pure function, such as a `json.Decoder`, is `unknown` unless the function creates and reads it itself. See [Streams in a pure function](../guide/contract-mode.md#streams-in-a-pure-function).

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

### `unknown` and the contracts

`unknown` is an effect like any other. It comes from a call through an interface method that has no contract and no stub, and from a call through a func value that is not resolved. The contracts treat it as follows:

- `//effectgo:pure` requires an empty set, so `unknown` violates it.
- `//effectgo:allow <list>` and `//effectgo:package allow <list>` allow `unknown` only when `unknown` is in the list. A package that says `allow db` does not allow an uncontracted interface call.

Verified with a package that says `//effectgo:package allow db`:

```text
LocalIface is declared //effectgo:package allow db but has effect unknown via LocalIface → Reader.Get
PureLocal is declared //effectgo:pure but has effect unknown via PureLocal → Reader.Get
```

Two ways out, and the first is the usual one. Put the contract on the interface, where the port is declared (`//effectgo:allow db` above `type Reader interface`). Each call through that interface then contributes `db` with the witness `[contract]`, and `unknown` disappears. Or list `unknown` in the allowed effects, which states that the package accepts calls it cannot see.

Two limits on where a contract on an interface is read:

- An interface declared at package level has its contract read whether it is exported or not (verified with an unexported `repo` interface).
- An interface declared inside a function body does not have its contract read. A comment on it is ignored and its calls stay `unknown` (verified). Move the interface to package level.

For which stdlib interface methods are modelled, see [Stub coverage](stub-coverage.md). `io.Writer.Write` is not modelled, so `w.Write` in a pure function is `unknown`; `context.Context.Done` is modelled as pure.

### Interface contracts

A directive on an interface method (or on the interface type) is a contract for every call through that interface. Importers see it, so contracts work across packages. Conformance is checked: each named type that implements the interface must satisfy the contract of each method it implements. The message is:

```text
(*HTTPRepo).Get violates contract of (Repo).Get [allow db]: has effect net via (*HTTPRepo).Get → http.Get
```

Reported at the method's name when it is declared in the analyzed package, or at the type's name when the method is promoted.

A type-level contract (`//effectgo:pure`, `//effectgo:allow` or `//effectgo:errors` on the interface type) applies to the methods the interface declares. An interface that only embeds other interfaces declares none, so its type-level contract checks nothing. The analyzer that reads the keyword reports it at the type's name:

```text
effectgo:pure on Base checks nothing: an interface contract covers the methods it declares, and Base declares none
```

The embedded interfaces keep their own contracts, and the embedding interface does not inherit them. Declare a method on the interface, or put the contract on the embedded interface.

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

A function has no error set when it has no error result and returns no func literal that has one. An errors directive on such a function checks nothing, and it is reported:

```text
effectgo:errors on NoErrResult checks nothing: it has no error result
```

A handler constructor, which returns a func literal with an error result, has its error set through that literal, so its errors directive is checked and is not reported.

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

#### How an errmap gives a status

A returned value is read for its HTTP status in one of three ways. Each was checked with `-errset.statuses`:

```go
//effectgo:errmap
func ToHTTPError(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return httpError{Status: http.StatusNotFound, Err: err} // read as 404
	}
	if errors.Is(err, domain.ErrPending) {
		return pendingError{Err: err} // read as 418 from its StatusCode method
	}
	return err // a passthrough: keeps the status the variant already has
}
```

- A composite literal of a type with a `Status` field is read from that field, when the field holds a constant (`http.StatusNotFound`, or `404`). The type needs no `StatusCode` method. Verified: a plain struct `httpError{Status: http.StatusNotFound, Err: err}` gives `404` in `-errset.statuses`.
- A type with a `StatusCode() int` method that returns a constant is read through the method. Verified: `418`.
- A value that is not a constant is not read. A variable (`code := 422` followed by `httpError{Status: code, ...}`) gives `default` in `-errset.statuses`, and no code is guessed. Write the constant in the literal, or use a `StatusCode` method.

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

A `func(http.ResponseWriter, *http.Request)` handler has no error result, so there is no error set to check. The directive is reported, and it checks nothing:

```text
effectgo:handler on H checks nothing: it has no error result; put the directive on the inner function that returns error (see docs/guide/http-checks.md#nethttp-handlers)
```

Put the directive on the function that returns the error, and write the response in an adapter. The pattern is in [HTTP checks](../guide/http-checks.md#nethttp-handlers). A handler constructor is checked through the func literal it returns when that literal has an error result; when it does not, the same message is printed.

## Must-use and exhaustive directives

### `//effectgo:mustuse`

Placed on a named type declaration. A call statement that discards a value of this type (or of a pointer to it) is reported, whatever the callee's error set:

```go
//effectgo:mustuse
type Receipt struct{ ID string }
```

The directive may have a trailing comment. An argument after the keyword is reported as `//effectgo:mustuse takes no arguments`, and the type is still must-use.

On a function, the directive is not read, because it marks a named type. The mustuse analyzer reports it, with one of these messages:

```text
effectgo:mustuse on NoResults checks nothing: it has no results; //effectgo:mustuse marks a named type
effectgo:mustuse on F checks nothing: //effectgo:mustuse marks a named type, not a function; put it on the named type that F returns
```

An alias is not allowed here: the directive on an alias is a diagnostic (`… applies to a defined type, not an alias; put it on the type the alias names`). Mark the type the alias names.

### `//effectgo:sealed`

Two uses:

- on an interface, to make it sealed. The interface must declare an unexported method of its own package (the marker method that only that package can implement); without one the directive is a diagnostic. An interface with an unexported method is sealed even without the directive;
- on a named integer or string type, which makes it an enum: its variants are the distinct values of its package-level constants.

A trailing comment after the keyword is allowed. An argument after it is reported as `//effectgo:sealed takes no arguments`, and the interface or enum is still sealed.

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

### When an interface needs `//effectgo:sealed`

- **An interface with an unexported method of its own package is sealed without the directive.** This is the usual way to seal an interface, and the analyzer checks its type switches as they are. Writing the directive on it too only states intent. Verified: `type Shape interface{ isShape() }` with a type switch that misses `Circle` is reported as `missing cases in type switch on Shape: Square`, with no directive.
- **An interface with only exported methods cannot be sealed by the directive.** The directive on it is a diagnostic, `sealed interface <Name> must have an unexported method`, and the interface is not checked. Add a marker method, `isShape()`, and the directive is not needed.
- **The directive is needed for enums.** A named integer or string type has no marker method, so `//effectgo:sealed` is the only way to make a switch over it checked.

The interface rule is the one to remember: the directive does not make an interface sealed by itself.

## Unknown keywords

```text
unknown effectgo directive "purr"                       (effects)
unknown directive //effectgo:purr                       (errset; on an interface method or type: "unknown directive //effectgo:purr on an interface method or type")
```

Each analyzer reports the keyword once, so the combined binary can print two lines for one typo. Where no analyzer reads directives, the directive analyzer reports the keyword instead: `unknown effectgo directive "purr" is not read here` on a type, or `//effectgo:purr is not attached to a function, interface or type declaration and is not read` on a var, a const, a function body, a literal or a struct field.

## Placement rules that are easy to miss

A directive is read only where its keyword's rules place it. This table gives the placements that are read:

| Keyword | Read on |
|---|---|
| `pure`, `allow` | a function or method, with or without a body; an interface type, or an alias of an interface literal; a method of an interface type |
| `errors` | a function or method with a body; an interface type, or an alias of one; a method of an interface type |
| `errmap`, `handler` | a function or method with a body |
| `mustuse` | a named type declaration, interface types included; the doc comment of a group that declares one type counts |
| `sealed` | an interface type, or a named type (exhaustive reports a named type that is neither an integer nor a string type); not in a `_test.go` file |
| `package` | the package doc comment only |

The doc comment of a group that declares several types is read by none of these keywords. Nor is a comment on a `var` or `const`, a comment inside a function body, a comment on a function literal, a comment on a struct field or on an embedded interface, or a comment that a blank line separates from its declaration. Some misplaced directives are reported by the analyzer that reads the keyword there, with its own message: `pure` and `allow` in a package doc comment, `package` on a function or on an interface, and `errmap` or `handler` on an interface. The others are reported by the directive analyzer, with these messages:

```text
//effectgo:pure is not attached to a function, interface or type declaration and is not read
//effectgo:pure on type Plain is not read: pure applies to functions, interfaces and interface methods
//effectgo:sealed on function F is not read: sealed applies to interfaces and named integer or string types
//effectgo:mustuse is in the doc comment of a group of several types and is not read: put it on each type
//effectgo:errors on function F is not read: F has no body, and only functions with a body are checked
//effectgo:sealed is not read in a _test.go file: sealed types are checked only in non-test files
```

The full list of messages, with their conditions, is in [Analyzers](analyzers.md#directive).

A comment with a space after the slashes is not a directive, and effectgo does not read it. When the word after `effectgo:` is a directive keyword, the directive analyzer reports it:

```go
// effectgo:pure
func Normalize(s string) string { ... }
```

```text
// effectgo:pure is not a directive: remove the space after //
```

Prose that mentions `effectgo:` is not matched, because the word after it must be a directive keyword.

Two more rules:

- `//effectgo:package` only in a package doc comment; the other directives are not valid there.
- A directive that names a package (`httpres.ToHTTPError`, `iface:…`) needs that package to be imported by the file. Otherwise the name cannot be resolved (`effectgo:errors: cannot resolve …`).

### Name resolution is per file

A name in a directive is resolved through the imports of the file that holds the declaration, and only that file. The package does not need to be imported by any other file of the same package. Verified with two files of one package: a function in the file that imports `pa` resolves `pa.ErrNotFound`; a function in a file that does not import `pa`, in the same package, reports:

```text
effectgo:errors: cannot resolve "pa.ErrNotFound" in Other
```

Three things follow from this:

- **Import the package in the file that carries the directive.** It is not enough that another file of the package does.
- **A package used only by a directive is an unused import to the compiler.** A directive is not a use, so `go vet` and `go build` report `imported and not used`. The code must use the package somewhere, for example in a `var _ = httpres.ToHTTPError` line, or in real code.
- **When every name of an `errors` directive fails to resolve, the contract is not checked.** Only the `cannot resolve` lines are printed. Without this, every real variant of the function would also read as `not in its //effectgo:errors contract`. If some names resolve, the contract is still checked against those names.
