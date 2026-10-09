---
title: Backlog
description: Known gaps that were found in use and deliberately not fixed in the documentation round, each with the evidence, the proposed change and why it is not a small fix.
---

Each item came from a first-time user, a probe run, or the evaluation of the dogfood project. The evidence is what was observed; the proposal is a direction, not a decided design. An item is removed when it is fixed, and the fix is recorded in the [journal](../journal/index.md), not here.

## B1. One uncontracted interface call prints one diagnostic per function on the path

**Evidence.** One call through an uncontracted interface produced five diagnostics, one for each function that reaches the call. The first line named the calling function, which reads as a problem with that function. The root, `scanner.Scan`, is only the last step of the witness.

**Proposal.** Group the diagnostics by the origin of the effect (`unknown` on the interface method), and print the origin first. Alternatively, print one line per origin and list the callers on it.

**Why it is not small.** It changes the output format of the effects analyzer. Every testdata expectation that counts diagnostics would change, and the zero-false-positive corpus would need a re-run to show that no finding was added or lost.

## B2. A standalone run skips test files, so it can look clean when `go vet` is not

**Evidence.** On one repository, `effectgo ./...` was clean, and `go vet -vettool=effectgo ./...` reported three findings, all in `_test.go` files. A test function with its own `//effectgo:pure` directive is checked by `go vet` and not by the standalone run (verified).

**Proposal.** Either analyze test files in the standalone run by default (`-test` on), or make the standalone run say which test variants it skipped.

**Why it is not small.** It changes the default of a command that users run in scripts. It needs a decision on the default, and a check that the disciplined corpus gives the same result with test files included.

**Interim.** [Getting started](../guide/getting-started.md#a-clean-run-is-not-a-full-run) tells the user to run both commands.

## B3. A clean run gives no sign that the packages were analysed

**Evidence.** A first run on a six-file package exited 0 with no output. The user could not tell from the output that the package was analysed, and used `effectgo sig` to confirm.

**Proposal.** A summary on stderr, for example `effectgo: analysed 6 packages, 2 test variants skipped`, printed only when there is no diagnostic, or always with a flag to turn it off.

**Why it is not small.** The driver's output is compared in the examples script (`examples/run.sh`) and by scripts of other users; a new line changes both. It is a decision about the output contract.

**Interim.** `effectgo sig` confirms that a package was analysed ([Getting started](../guide/getting-started.md#a-clean-run-is-not-a-full-run)).

## B4. A `//effectgo:handler` on a `func(w, r)` handler checks nothing, without a message

**Evidence.** A `net/http` handler with the directive has no error result, so its error set is empty and no diagnostic is printed (verified). A user who writes the directive on the handler may believe it is checked.

**Proposal.** Two options. (a) Print a warning when a `//effectgo:handler` function has no error in its handler set and no error result. This needs care: a handler constructor that returns a literal has the same shape and would also be reported. (b) Read the errors that an adapter writes through a sink (the Gin check does this with `sink` rules), so that a net/http adapter can be checked without the inner-function split.

**Why it is not small.** Option (a) changes the diagnostics for handler constructors, and option (b) is a new rule form in the stub format.

**Interim.** The inner-function pattern is documented in [HTTP checks](../guide/http-checks.md#nethttp-handlers).

## B5. Timing on a small module is several seconds, and the cache hides the cost of `go vet`

**Evidence.** On a six-package module (about 1.8k lines), `effectgo ./...` took 3.9 to 4.7 s of wall time, and `effectgo sig ./...` about 3.7 s. `go vet -vettool` took about 0.2 s once its result was cached, so the first run after an edit is the only honest timing.

**Proposal.** None yet. Measure on the disciplined corpus and on a large service before choosing between caching facts, sharing the SSA build between analyzers, or narrowing the packages loaded. This is an observation, not a defect.
