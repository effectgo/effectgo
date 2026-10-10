---
title: Backlog
description: Known gaps that were found in use and deliberately not fixed in the documentation round, each with the evidence, the proposed change and why it is not a small fix.
---

Each item came from a first-time user, a probe run, or the evaluation of the dogfood project. The evidence is what was observed; the proposal is a direction, not a decided design. An item is removed when it is fixed, and the fix is recorded in the [journal](../journal/index.md), not here.

The three output-contract items (root-only reporting, test files by default, the summary line) were fixed in the [P6 journal entry](../journal/15-p6-output-contract.md) and are no longer listed here.

## B4. A `//effectgo:handler` on a `func(w, r)` handler checks nothing, without a message

**Evidence.** A `net/http` handler with the directive has no error result, so its error set is empty and no diagnostic is printed (verified). A user who writes the directive on the handler may believe it is checked.

**Proposal.** Two options. (a) Print a warning when a `//effectgo:handler` function has no error in its handler set and no error result. This needs care: a handler constructor that returns a literal has the same shape and would also be reported. (b) Read the errors that an adapter writes through a sink (the Gin check does this with `sink` rules), so that a net/http adapter can be checked without the inner-function split.

**Why it is not small.** Option (a) changes the diagnostics for handler constructors, and option (b) is a new rule form in the stub format.

**Interim.** The inner-function pattern is documented in [HTTP checks](../guide/http-checks.md#nethttp-handlers).

## B5. Timing on a small module is several seconds, and the cache hides the cost of `go vet`

**Evidence.** On a six-package module (about 1.8k lines), `effectgo ./...` took 3.9 to 4.7 s of wall time, and `effectgo sig ./...` about 3.7 s. `go vet -vettool` took about 0.2 s once its result was cached, so the first run after an edit is the only honest timing.

**Proposal.** None yet. Measure on the disciplined corpus and on a large service before choosing between caching facts, sharing the SSA build between analyzers, or narrowing the packages loaded. This is an observation, not a defect.
