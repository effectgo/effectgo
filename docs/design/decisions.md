---
title: Design decisions
description: The key decisions behind effectgo, each with the reason it was made and the evidence that the journal records.
---

Each entry states the decision, the reason, and where the evidence is. The reasons come from the measurements and integrations in the [journal](../journal/index.md); where a reason is a judgment rather than a measurement, the entry says so.

## Inference, not a monad

**Decision.** effectgo infers `Effect<A, E, R>` for plain Go functions. Code is not rewritten into a monad, and no wrapper type is required.

**Reason.** The point is to see what a function can do without changing how it is written, so the analysis can be applied to existing code. Contracts (`//effectgo:pure`, `//effectgo:errors`, …) are the only thing written, and they are optional.

## One binary, one driver

**Decision.** One `effectgo` binary is a multichecker over all analyzers, with prefixed flags (`-effects.report`, `-errset.witness`, `-mustuse.allow`). The prototypes had separate binaries with unprefixed flags.

**Reason.** One binary with one flag namespace works under `go vet -vettool`, standalone, and in the golangci-lint plugin without a second configuration. See [Prototypes and deviations](../journal/01-prototypes.md).

## One directive prefix, each analyzer skips the others'

**Decision.** All directives use `//effectgo:`. Each analyzer reads its own keywords and ignores the rest.

**Reason.** A single prefix keeps the vocabulary in one place for the reader. The cost is that a typo is reported by each analyzer that reads the prefix (sometimes twice).

## Stubs instead of body analysis for the standard library and drivers

**Decision.** The standard library is not analyzed from source. It is modelled by a taxonomy for effects and by stub files for both effects and errors. Driver and framework code (pgx, gin, Huma, Fuego) is covered by stub files too.

**Reason.** Before the driver stubs, the real services were dominated by `unknown` and `?`. The three causes measured in the real-world evaluation were driver interfaces (pgx, `database/sql` rows), framework methods that reach gin's guarded panic and an interface render call, and closures whose effects were charged to the function that registers them. Stub rules are one line each, carry their source reference, and are reviewable without reading an analyzer. The measurements are in [the driver stubs journal](../journal/02-p1-driver-stubs.md).

## Stub format modelled on `.d.ts`

**Decision.** A stub file is one rule per line, `verb target value`, with `#` comments. Rules describe code that is not analyzed, the way declaration files describe JavaScript.

**Reason.** A line-oriented format is easy to diff and to cite (each rule carries `file:line` of its source), and one parser serves every analyzer. A malformed line is a hard error with the file and line.

## Zero configuration, and explicit replaces it

**Decision.** With no `-stubs` and no `EFFECTGO_STUBS`, the shipped stub files are read, and then the module's `effectgo.stubs`. When either variable is set, exactly those files are read: the zero configuration is not merged in.

**Reason.** A silent run without stub files reported less than it should, and an eval run used no stubs without saying so. Zero configuration makes the default correct; the explicit form stays predictable, because a set of files is never a mix of two sources. Measured results differ between the two configurations, so a comparison needs the same set (see [determinism](../journal/03-p1-determinism-vet-identity.md)).

## The stub set is decided per package, by every analyzer the same way

**Decision.** `internal/stubs` decides the stub set from the analyzed package's path. effects, errset, mustuse (through errset), the HTTP adapters and openapi use that one resolver.

**Reason.** Two analyzers that read different stub sets would disagree about the same code. The shared resolver is what makes `hash.Hash32` find the writer rule of `hash.Hash` in zero-configuration mode too.

## Opt-in enums

**Decision.** An integer or string type is an enum only with `//effectgo:sealed`. There is no implicit enum rule.

**Reason.** Named integer types with `iota` constants are common in legacy code and are often switched on partially on purpose. Reporting them without a directive would not be clearly correct, so the directive is the request. Sealed interfaces need no directive: the unexported marker method already states that only the package can implement them.

## Silence when unproved

**Decision.** A diagnostic is reported only when the code is provably at risk. Where the analysis cannot prove the risk, it does not report.

**Reason.** The analyzers are meant to run on every save and in CI, where a false positive teaches people to ignore the output. Concretely: a discarded error is reported only when errset cannot prove the callee's error set empty; a `//effectgo:mustuse` value is reported whatever the error set; `_ = f()` is the explicit discard. The corpus result is the test: zero false positives on the disciplined corpus under the default engine (see [contract mode](../guide/contract-mode.md#the-zero-false-positives-promise)).

## Honest unknowns

**Decision.** An error whose origin is unknown is `?`. An effect that is not known is `unknown`. A status that cannot be read is `default`, never a guessed code. A call that cannot be resolved is not guessed.

**Reason.** A guessed answer looks like a fact in a report. Each unknown is visible in the output, and the reader can add a stub or a contract to resolve it. The one place where a default is applied is documented: plain errors are HTTP 500, because the server's error handler does so (see below).

## Flow-sensitive error sets by default

**Decision.** The default error-set engine is SSA-based and flow-sensitive. The earlier flow-insensitive engine remains behind `-errset.engine=ast`.

**Reason.** On the disciplined corpus the AST engine had 12 true positives and 8 false positives; the SSA engine had 12 true positives and none. Most false positives were a variable whose earlier guard was ignored, which the SSA engine follows. The cost is time: the median wall time was 1.55× on order-mgmt and 1.37× on kuzma-wms, within the 2× target on the medians, with the worst paired run at 1.94×. See [the P3 journal](../journal/10-p3-flow-sensitive.md).

## Fresh errors keyed by the creating function

**Decision.** `errors.New` and constant `fmt.Errorf` without `%w` produce a variant named after the declared function that creates them, not after the call site.

**Reason.** A function with ten `errors.New` calls would otherwise add ten variants that differ only in position, and error sets are meant to stay small. The creator is kept in the name and in the witness, so a reader can still find the source.

## Fresh and opaque variants are separate in contracts

**Decision.** `//effectgo:errors new` allows fresh variants and `?` allows opaque ones; the two do not allow each other.

**Reason.** A fresh error is a known creation point and an opaque one is not. A contract that accepts "anything unknown" should not silently accept a fresh error, and the reverse is equally surprising.

## Foreign unexported names collapse to `?`

**Decision.** A sentinel, type or interface from outside the analyzed module whose identifier is unexported is shown as `?`, and so is a fresh variant created by a function of a foreign package. The original name stays in witness lines only.

**Reason.** Code outside a package cannot name an unexported identifier, so it cannot match one with `errors.Is` or `errors.As`. The first run of the evaluation showed exported-creator fresh variants (`new@pgconn.Pipeline.Flush`) filling sets with dependency internals. The collapse applies at the report boundary; facts keep the full identity, and the HTTP status of a collapsed type is kept, because the runtime status does not depend on whether user code can name the type.

## Plain errors are HTTP 500 by the server, not by a stub

**Decision.** An error that carries no status is 500 in the OpenAPI and HTTP checks, as is an opaque `?`. The rule is a constant in the analysis, not a stub line.

**Reason.** The rule is the server's error handler (Fuego's sender starts at 500), not a property of a type. A stub keyed by package would have to say which server handles a route, and the facts do not carry that. The fix that made `?` a 500 instead of `default` is recorded in [the OpenAPI journal](../journal/05-p1-openapi.md).

## Closure attribution: only what a function runs

**Decision.** A func literal is charged to the function that contains it only when that function runs it: in place, under `defer` or `go`, or through a local variable that is assigned once. A literal that is passed, stored, returned or sent is charged through the parameter it is passed to, or not at all.

**Reason.** Route registration functions and handler constructors had inherited the effects of every literal they registered, which made their sets wrong (on kuzma-wms the share of functions with `db` went from 41.0% to 35.2% with the two features alone; the measurements are in [the driver stubs journal](../journal/02-p1-driver-stubs.md)). The rule matches what the code does.

## Witnesses are additive

**Decision.** Witness lines (`-effects.witness`, `-errset.witness`) are printed next to the report lines, and never change them.

**Reason.** The reports are compared between runs by scripts; changing their lines would break every comparison. Witnesses are shortest chains, one per variant, and the width of each step is capped at 80 characters so a long string in a dynamic call cannot break the line.

## The vet cache must see stub edits

**Decision.** effectgo's `-V=full` identity hashes its own executable and the stub files the analysis reads from the working directory.

**Reason.** `go vet` caches results under the tool identity. An edited stub gave the same cached output, which is a silent error. The identity now changes when a stub changes. The remaining gap (a file named only by `-stubs`) is documented in [CLI](../reference/cli.md#go-vet-and-the-cache).

## Standard-library detection from the file layout

**Decision.** A package is standard library when its source is under a Go installation root, judged from the file path. The environment does not decide.

**Reason.** Standard-library detection that compared against `$GOROOT` gave wrong answers when the shell's `GOROOT` pointed at another toolchain: the integrated `pct_panic` went from 2–10% to 59–70%. The layout-based rule does not depend on the environment (see [architecture](architecture.md#standard-library-detection)).

## Writer rules are stubs, not a built-in allowlist

**Decision.** `bytes.Buffer`, `strings.Builder` and `hash.Hash` writes are stated as `errors … none` rules, with the source lines cited in the file. The earlier allowlist in mustuse is gone. The only built-in allowlist that remains is `fmt.Print*` and `fmt.Fprint*`.

**Reason.** The knowledge is in a cited file that a reviewer can check. A built-in list in an analyzer is invisible from the stub configuration. The stub form also needed the lookup through embedded interfaces (`hash.Hash32` embeds `hash.Hash`), recorded in [the P4a journal](../journal/11-p4a-mustuse-exhaustive.md).

## Must-use and exhaustive are ports, under the same bar

**Decision.** `mustuse` and `exhaustive` are ported from the rustvet prototype and held to the same rule: report only when provably at risk.

**Reason.** The Rust `#[must_use]` and exhaustive `match` are the models, and the prototype's defects (test-only variants, aliases, generic instances, pointer operands) were fixed each with a test. See [the P4a journal](../journal/11-p4a-mustuse-exhaustive.md).

## Editors through golangci-lint

**Decision.** The editor integration is a golangci-lint v2 module plugin, not a language server.

**Reason.** golangci-lint v2 is the lint runner that editors already call, and it has a plugin system. golangci-lint v2 has no language server, so the plugin runs in the lint pass; the verification shows that the golangci-lint result cache does not see stub edits (documented as a limitation). See [Editor integration](../guide/editor.md).

## HTTP adapters only as a showcase

**Decision.** The HTTP checks cover Gin, Huma and Fuego (through the OpenAPI command), and they are presented as a showcase of what error-set inference enables. The core does not depend on them, and no other framework adapter is built.

**Reason.** The adapters were built against real services to test whether inferred error sets are precise enough to check the HTTP boundary. Each further framework would need its own stub rules and mapping evidence; that work is not planned here. The showcase is described in [HTTP checks](../guide/http-checks.md).

## Positioning against related tools

These are described from their own documentation and were not re-checked against their current releases.

- **goexhauerrors** checks call sites for exhaustive handling of errors. effectgo infers, for each function, the errors it can return and the effects it can perform, so a caller sees the signature without reading the call site.
- **Capslock** reports the capabilities a whole program can reach. effectgo reports per-function effect sets and checks them against contracts written in the code, so a violation is attributed to the function that declared the contract, with a witness chain.
- effectgo adds the HTTP boundary check: an error a handler can return must be mapped by the project's error-to-HTTP function, or it becomes an HTTP 500.
