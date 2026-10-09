---
title: Contract mode
description: "How to write new Go code with effectgo contracts: the workflow, worked examples from the example modules, the zero-false-positives promise, and what discipline means."
---

Contract mode is writing new code with effectgo contracts in place from the start. A contract is a directive that states what a function may do, a package, an interface, or a handler may produce. effectgo infers what the code actually does and reports every place where the two differ.

Use contract mode for new code. The same analyzers also work on existing code, where they show what is there; see [Getting started](getting-started.md).

## The workflow

1. **Draw the boundaries first.** Decide which packages are pure (no effects), which interfaces are ports (the only place where effects are allowed, and which kind), and which functions are HTTP handlers.
2. **State the contracts where the boundary is.** A package default for the pure core, a contract on each port interface, an error contract on a service method, and an errmap with a handler directive on each HTTP handler. Contracts are one line each.
3. **Write the code.** Use sentinel errors or typed errors, wrap with `%w`, and match with `errors.Is` and `errors.As`.
4. **Run the analyzers** on the package: `effectgo ./...`, or `go vet -vettool=$(which effectgo) ./...` in CI. Read each diagnostic: either the contract is wrong, or the code is.
5. **Look at the signatures** when you review: `effectgo sig ./...` shows the `Effect<A, E, R>` of each function, so a reviewer sees what a function does without reading its body.
6. **Repeat** when you add a dependency. A new library call that reaches the network inside a pure function shows up as a diagnostic on the next run, with the witness chain that leads to it.

For code that effectgo cannot see into (a generated client, a vendored driver, an external package), add a stub line in the module's `effectgo.stubs`; see [Stub files](../reference/stubs.md#discovery).

## Example 1: a pure domain and a port

From `examples/layers`. The package doc says the package is pure, and a function in it says so again:

```go
// Package domain holds the pure business rules of the todo app.
//
//effectgo:package pure
package domain

// Normalize trims a title and appends a stamp. Declared pure, but the stamp
// reaches time.Now through infra.Stamp: a two-hop leak.
//
//effectgo:pure
func Normalize(s string) string {
	return strings.TrimSpace(s) + infra.Stamp()
}
```

The persistence port states what its implementations may do:

```go
// Repo is the persistence port. Implementations may touch the database only.
//
//effectgo:allow db
type Repo interface {
	Get(id int) (string, error)
}
```

A function in the pure package that calls `Repo.Get` is reported, because the port allows `db`, which the package default does not. A function that states its own `//effectgo:allow db` is not reported. A type that implements `Repo` with network calls is reported by the conformance check. The diagnostics of this module are on the [Getting started](getting-started.md#reading-the-output) page.

The design in this example is the point: the domain stays pure, the port declares the one effect it allows, and each adapter is checked against the port.

## Example 2: errors and the HTTP boundary

From `examples/todo`, shortened. The service returns sentinel errors, and the HTTP layer maps them with an errmap:

```go
// ToHTTPError maps domain errors to HTTP errors. Anything not listed is
// returned unchanged; Fuego reports it as 500.
//
//effectgo:errmap
func ToHTTPError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return fuego.NotFoundError{Err: err, Detail: err.Error()}
	case errors.Is(err, domain.ErrInvalid):
		return fuego.BadRequestError{Err: err, Detail: err.Error()}
	default:
		return err
	}
}

//effectgo:handler httpres.ToHTTPError
func v3Update(svc *domain.Service) func(fuego.ContextWithBody[updateReq]) (domain.Todo, error) {
	...
}
```

effectgo infers the handler's error set from the service call, and checks it against the errmap. In the example, `ErrConflict` is returned by the service but not mapped, so the handler reports it as an HTTP 500:

```text
api/v3.go:53:6: handler v3Update may return domain.ErrConflict (via Service.Update → Store.checkTitle), which errmap httpres.ToHTTPError does not map → HTTP 500
```

Two ways to resolve it: add a case to the errmap, or accept the 500 and say so in a comment. Either is a decision, which is the point of the check.

An error contract works the same way. An illustrative one on a service method (the names are not in the example module):

```go
//effectgo:errors domain.ErrNotFound, domain.ErrInvalid
func (s *Service) Get(id int) (Todo, error) { ... }
```

A new `return domain.ErrConflict` in that method is reported as not in its contract.

## Example 3: a project stub for code without a body

From `examples/editor`. The store package has no body that effectgo can see through, so its effect is declared in the project file:

```text
# effectgo.stubs at the module root
effects example.com/editor/store.Save db
```

With that line, a pure function that calls `store.Save` is reported, because the call is `db`. The file is read with no configuration; see [Stub files](../reference/stubs.md).

## Conventions that keep the output quiet

The analyzers report a risk only when they can prove it. Code gets clean output when it follows these conventions, which are the ones the analyzers are built around. They are not a formal specification.

- **Errors are values with identity.** Each error that a caller may need to test is a sentinel (`var ErrNotFound = errors.New(...)`) or a named type. Wrap with `%w` when you add context. Test with `errors.Is` or `errors.As`, never by comparing the text.
- **Every error that leaves a handler has a mapping, or is a deliberate 500.** An errmap lists the mapped variants; anything else is a 500 by design. A plain `fmt.Errorf` without `%w` is a fresh error that no caller can match.
- **Errors are not discarded silently.** A call statement that drops an error is reported unless its error set is provably empty. Write `_ = f()` when you mean to discard it.
- **The pure core stays pure.** Effects come in through ports, and a port states what it allows.
- **Interfaces that cross a layer have a contract.** An uncontracted interface call is `unknown`, and `unknown` in a pure package is a finding.
- **Closed sets are closed by the type.** An interface with an unexported marker method is sealed; an enum that matters for switches is marked `//effectgo:sealed`.
- **External code is stubbed or contracted.** Anything the analyzer cannot see into is either declared in `effectgo.stubs` or listed in a contract.

## Common questions

Short answers, each with the page that has the details and the evidence:

- **When does an interface need `//effectgo:sealed`?** Not when it has an unexported marker method: that seals it. The directive is for enums and for intent. See [Directives](../reference/directives.md#when-an-interface-needs-effectgosealed).
- **How do `unknown`, `//effectgo:pure` and `//effectgo:package allow` interact?** An uncontracted interface call is `unknown`. It violates `pure`, and it violates `allow` unless `unknown` is listed. Put the contract on the interface. See [Directives](../reference/directives.md#unknown-and-the-contracts).
- **Which file's imports resolve a directive name?** The file that holds the declaration. Another file of the package does not help. See [Directives](../reference/directives.md#name-resolution-is-per-file).
- **Which stdlib interface methods are modelled?** The generated list in [Stub coverage](../reference/stub-coverage.md). Anything else called through an interface is `unknown`.
- **How do I use the errmap and handler checks with `net/http`?** Put the directive on a method that returns an error, and write the response in an adapter. See [HTTP checks](http-checks.md#nethttp-handlers).
- **Why is a status `default` or `500`?** The errmap did not give a constant status for that variant. See [Directives](../reference/directives.md#how-an-errmap-gives-a-status) and [Error sets](../reference/errors.md#http-status).
- **How do I install from the private repository?** Build from a clone with `go install ./cmd/effectgo`. See [Getting started](getting-started.md#install).
- **Why does a clean standalone run miss a test finding?** Standalone runs skip `_test.go` files. Run `go vet` too. See [Getting started](getting-started.md#a-clean-run-is-not-a-full-run).

## The zero-false-positives promise

On the disciplined corpus, the analyzers report exactly the findings that the corpus marks as intended:

- with the default engine, 12 true positives, 0 false negatives and 0 false positives on the default expectations;
- with the zero-configuration run and the future-marked expectations, 17 true positives, 0 false negatives and 0 false positives.

The baseline AST error-set engine, which the default replaced, had 8 false positives on the same corpus. Most came from a variable whose earlier guard was ignored.

This is a measured result on one corpus. The corpus lives outside this repository, so the numbers are not something a reader can rerun from here, and the promise is not a general guarantee for any code. What the promise does mean, and what the analyzers are designed to keep, is this: a diagnostic is reported only when the code is provably at risk, and when the analysis cannot prove it, it stays silent. A finding on disciplined code is therefore a finding you should read, not noise. The design choices behind it are in [Decisions](../design/decisions.md#silence-when-unproved).

## What discipline means in practice

Disciplined code is code that follows the conventions above in every package the analyzers see. It is not a style guide and it is not a checklist you must pass: each convention is there because one of the analyzers depends on it to keep its diagnostics precise. When a diagnostic is not what you expect, check the convention it depends on first. The usual causes are an error that was wrapped with `%v` instead of `%w`, a call through an interface that has no contract, and a library call that has no stub.

## Related

- [Directives](../reference/directives.md): the syntax and the errors of each directive.
- [Analyzers](../reference/analyzers.md): the exact message of each diagnostic.
- [HTTP checks](http-checks.md): the same error-set inference applied to HTTP handlers, as a showcase.
