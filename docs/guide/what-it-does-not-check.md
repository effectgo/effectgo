---
title: What effectgo does not check
description: The properties that effectgo leaves to other tools (data races, goroutine leaks, deadlocks, channel semantics, ignored cancellation, timing), the tool to use for each, how the effects of goroutines are charged, and how error contracts meet callbacks.
---

effectgo proves facts about what a function does, from its code: which side effects it can perform, and which errors it can return. A clean run says nothing about the properties below, because the analysis does not model them. Each row names the tool that does check it.

## Not checked, and what to use instead

| Property | Why effectgo does not check it | Use instead |
|---|---|---|
| Data races | A `global` effect records a write to a package-level variable. It does not record which goroutine reads or writes which memory, and a write through a pointer or a field of a shared value is not followed. | `go test -race ./...`. The race detector finds a race only on the paths that the tests run. |
| Goroutine leaks | A `spawn` effect records that a function starts a goroutine. It does not record whether the goroutine ever ends. | A leak check in the test: compare `runtime.NumGoroutine()` before and after the code, with a short wait; or `go.uber.org/goleak`, whose `VerifyNone` fails the test when a goroutine is left running. |
| Deadlocks and blocking | Channel operations, `select`, `sync.Mutex` and `sync.WaitGroup` have no effect in the taxonomy. A function can block forever and still be pure. | Tests with a deadline: `go test -timeout`, or a `select` on a timer in the test. The runtime reports a deadlock only when every goroutine is asleep. |
| Channel and select semantics | The order of sends and receives, a send on a closed channel, a nil channel and the choice of a `select` case are not modelled. | Unit tests that drive the goroutine through its channels; `go test -race`. |
| Ignoring cancellation | effectgo does not check that a function that takes a `context.Context` passes it on, or that it checks `ctx.Done()`. | A test that cancels the context before, or during, the call and asserts the result. The error set of the function still checks which errors it returns, including `context.Canceled`. |
| Timing bugs | A `time` effect says that a function reads the clock or sleeps. It does not say that the function is correct under a delay, a timeout, or a clock that jumps. | Inject the clock, and test with a fake one. For code that starts timers, `testing/synctest` runs the code in a bubble with a fake clock (Go 1.25 and later). |

A rule that the analysis cannot prove stays silent, so a function that is clean here may still contain one of these bugs.

## Goroutines: where their effects are charged

A `go` statement adds `spawn` to the function that contains it, with the witness `F → go statement`. A goroutine literal, `go func() { … }()`, is part of the function that starts it: the literal's effects are charged to that function as well, so the function is checked against the whole of what the goroutine does. A literal that is passed to a function, or stored, is not charged to its creator; it is charged to the callee if that callee is known to run it (see [closure attribution](../reference/effects.md#closure-attribution)).

For this file:

```go
// Flush starts a goroutine. Its effects are charged to Flush.
//
//effectgo:allow db
func Flush(s string) {
	go func() {
		log.Println("flushed", s)
	}()
}
```

effectgo reports the two effects of the goroutine, charged to `Flush`, the function that starts it:

```text
gr/gr.go:10:6: Flush is declared //effectgo:allow db but has effect spawn via Flush → go statement
gr/gr.go:10:6: Flush is declared //effectgo:allow db but has effect stdio via Flush → log.Println
```

## Errors through a callback

A function that calls a func-typed parameter has the placeholder `param:i` in its error set: the errors of the callback, not known yet. A callee's own contract always allows its placeholder, so `Each` below is clean. The concrete errors are substituted at each call site, and the caller's contract is checked against them.

```go
package cb

// Each calls op and returns its error.
func Each(op func() error) error {
	return op()
}

// Load promises only ErrNotFound. The callback returns ErrBusy.
//
//effectgo:errors ErrNotFound
func Load() error {
	return Each(func() error { return ErrBusy })
}
```

The report is about `Load`. The callback's error is substituted at the call of `Each`, and the contract does not allow it:

```text
cb/cb.go:21:6: Load may return cb.ErrBusy (via Each), not in its //effectgo:errors contract
```

The same holds for a callback that is a named function, and for one that the caller passes through its own parameter: the placeholder then composes with the caller's. A callback that is the result of another call is `?`, which the contract allows only if it lists `?`.

## Related

- [Error sets](../reference/errors.md): the variants, the placeholders and the contracts.
- [Effects](../reference/effects.md): the taxonomy, closure attribution and the precedence of a call.
- [Limitations](../design/limitations.md): the cases that the analysis does not follow.
