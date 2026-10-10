---
title: "P4a: must-use and exhaustive matching"
description: The mustuse and exhaustive analyzers, ported from the rustvet prototype, with their defects and their corpus results.
---

### mustuse & exhaustive

Two analyzers ported from the rustvet prototype (`/root/pp/go-rust/proto-a-analyzer`, read-only). They bring Rust's `#[must_use]` and exhaustive `match` to Go, under the same contract-mode bar as the rest: a diagnostic must be a real risk, so a rule that cannot be proved is silent.

**Where.** `mustuse/mustuse.go` and `exhaustive/exhaustive.go`. Both run under `effectgo ./...` and `go vet -vettool`. `effectgo sig` runs only `signature.Analyzer`, so it never prints their diagnostics (checked by running `sig` on a module with discarded errors and missing cases).

**mustuse.** A bare expression statement whose call has a result that is an error, or a named type marked `//effectgo:mustuse` (or a pointer to one), is reported: `result of <callee> is discarded; handle it or assign to _`. Not reported: `_ = f()`, `go`, `defer`, and the `fmt.Print*` and `fmt.Fprint*` functions (the one allowlist that stays, by design). The mark is an `isMustUse` object fact, so it crosses packages; a generic type is marked by its origin, so every instance is must-use.

*Callee.* The callee is the static callee (`typeutil.Callee`), or, for a call through a local func variable, the function assigned to it when that is resolvable (`singleDefs`, `calleeOf`): the variable must have exactly one assignment, by `:=`, `=` or a one-value `var`, its address must not be taken, and the assigned expression must name a function, a qualified function, a method value or a method expression. Then `-mustuse.allow` matches by the callee's full name (`(*os.File).Close`). An unresolvable variable is named by its expression, and it is not allowlistable.

*Errors and errset.* A discarded error is reported only when the callee's error set can occur. `errset.Result.NoErrors` is the errset evidence: every function of the package with an empty error set, and every function the package names (calls, or uses as a value) when a stub rule says `errors <target> none`. mustuse exports the empty ones of its own package as an `errorFree` fact, so importers see them. A discarded value of a `//effectgo:mustuse` type is always reported, whatever the error set (`Both` in `mustuse/testdata/src/a`).

*Stubs.* The former built-in allowlist for `bytes.Buffer` and `strings.Builder` writes is gone; `stubs/stdlib.stubs` states `errors ... none` for those writers, with citations to the Go 1.26.9 source, and for `hash.Hash.Write`. Without a stub file these calls are reported (`mustuse/testdata/src/c`, `wn`). The writer rules for an interface are found through the receiver's static type and its embedded interfaces (`stubSetVia`, `stubInInterfaces`): `hash.Hash32` embeds `hash.Hash`, and the method is `io.Writer.Write` in the type checker, so a lookup by the method's own receiver would miss it (this was a real false positive in `order-mgmt`, `inquiry.go`, found by the real-world run and fixed with `mustuse/testdata/src/w`).

*Standard library.* A standard-library callee is opaque in errset unless a stub names it (`errset.IsStd`). mustuse follows the same rule: a fact derived from the source of a standard-library package does not count.

**exhaustive.** Two kinds of sealed type.

- An interface is sealed when it has an unexported method declared by its own package (the Go marker method: only that package can implement it, which is the convention `domain/event.go` of the disciplined corpus documents), or when it carries `//effectgo:sealed`. An unexported method inherited from another package does not seal an interface; the interface that embeds it is checked through the sealed interfaces it embeds. A sealed interface with `//effectgo:sealed` must have an unexported method of its own package (diagnostic otherwise). Its variants are the named types of the package whose `T` or `*T` implements it, sorted by name. A generic type is one variant whatever its instances.
- A named integer or string type with `//effectgo:sealed` is an enum. Its variants are the distinct values of the package-level constants of that type. Two constants with one value are one variant; a constant that is not exported is a variant only in its own package.

A type switch on a sealed interface, or an expression switch on an enum, with no `default` clause, must have a case for every variant: `missing cases in type switch on <Iface>: <V1>, <V2>` and `missing cases in switch on <Enum>: <C1>, ...`. A case that is a non-constant (enum) makes the switch undecided, so it is not reported. An interface case covers every variant that implements it. The message names are sorted.

**Test-only variants.** Variants are computed from the non-test files of the declaring package only (`isTestFile`). A type declared in a `_test.go` file is neither required nor counted, so the set is identical for the package, its test build and its importers, and the sealed fact of a package is the same whichever build exports it. A case in a test file that names a test-only type is still a case, and a switch in a test file is checked against the same set (`exhaustive/testdata/src/shape/shape_test.go`). A sealed interface or enum declared in a test file is not exported at all.

**Defects of the prototype, and their fixes.** Each has a test; the paths are relative to `exhaustive/testdata/src` or `mustuse/testdata/src`.

| Defect (prototype NOTES) | Fix | Test |
|---|---|---|
| Variants declared only in `_test.go` gave the package and its importers different sets | variants and sealed facts from non-test files only | `shape/shape_test.go` (`Hexagon` is not required by `Shape`, `client` agrees) |
| A sub-interface embedding a sealed interface was not checked (no fact) | `sealedOf` searches the embedded sealed interfaces; the variants are filtered by whether they implement the sub-interface | `shape.Drawable` (only `Circle` and `Square` implement it), `shape.Solid`, `wrap.Wrapped` (cross-package) |
| Generic variants were skipped | a generic named type is one variant; `Box[int]` and `Box[string]` cover it | `shape.Box` |
| Aliases: a case on an alias of a variant was a false "missing" (`types.Alias` is not `*types.Named`); an alias of a must-use type was not marked | `types.Unalias` on case types, operands and marked types; a directive on an alias is a diagnostic (its target is the defined type) | `shape.Alias`, `shape.ShapeAlias`, `d.Receipt`, `d.Aliased` |
| Pointer-to-interface and parenthesized operands | `(s).(type)` and `(*p).(type)` are checked (Go has no `p.(type)` on a pointer, so that form does not compile) | `shape.Parenthesized`, `shape.Dereferenced` |
| Function-variable calls could not be allowlisted by name | `singleDefs` resolution (above) | `mustuse/testdata/src/d`, `e` (with `-mustuse.allow=(*os.File).Close`) |
| Generic instances: the name dropped the type arguments, and an instance's mark was looked up on the instance | the callee's origin name is used; a mark on the origin covers every instance | `d.MakeBox`, `d.Gen` |

Not a defect, or not applicable: a sealed `*T` case and a `T` case both cover `T` (kept); an unexported variant of another package cannot exist, since the marker method is unexported (kept by construction).

**Enum switches (the optional item).** Implemented, but only for the explicit directive: `//effectgo:sealed` on a named integer or string type. There is no implicit enum rule. A named integer type with iota constants is common in legacy code and is often switched on partially on purpose, so reporting it without the directive would not be clearly correct. The disciplined corpus expects it without the directive (see the corpus result below).

**errset integration (summary).** `errset.Result.NoErrors`; `errset.IsStd`; `stubSetVia` and `stubInInterfaces` (errset/stubs.go); the `errorFree` fact (mustuse); the stub rules at the end of `stubs/stdlib.stubs`, cited to `bytes/buffer.go` (Write 199, WriteString 211, WriteByte 310, WriteRune 321 and 329, with the doc lines 190-192, 202-204, 299-301, 313-315), `strings/builder.go` (Write 90, WriteByte 98, WriteRune 107, WriteString 115, with the doc lines 85-86, 93-94, 101-102, 110-111) and `hash/hash.go:27-28` (`Hash.Write`: "It never returns an error.").

**Corpus result** (`realworld-eval/corpus/disciplined`, `runs/p4a/corpus.out`, stdlib stub file only):

- without `--future`: TP 12, FN 0, FP 0.
- with `--future`: TP 16, FN 1, FP 0. The missing one is `service/audit.go:23` (`Badge`, the enum switch on `domain.Status`): `domain.Status` has no `//effectgo:sealed`, so it is not an enum for effectgo. The other four future markers are reported: the `Deleted` type switch (sealed by the marker method) and the three discards (`Sweep`'s `Delete`, and the two `json.Encoder.Encode` calls).
- A scratch copy of the corpus with `//effectgo:sealed` added on the line above `type Status int` (`domain/domain.go:28`) gives TP 17, FN 0, FP 0. The corpus itself was not edited: the directive is a one-line request to the corpus owner.

**Real-world noise** (`mustuse` and `exhaustive` only, `-mustuse -exhaustive`, four stubs: stdlib, gin, huma, pgx; outputs in `runs/p4a/<project>.mustuse-exhaustive.err`). Static evidence only, see the report for the samples.

- order-mgmt: 13 mustuse findings, 0 exhaustive. Of these, 8 are best-effort closes on error paths or after a read, and 5 are `oplog.Insert` audit writes whose error is ignored on a success path (real risk).
- kuzma-wms: 0 findings.
- arcteryx-console: 25 mustuse findings, 0 exhaustive. 16 are `csv.Writer.Write` calls whose error is sticky and checked by `flushCSVWriter` (`Flush`, then `Error`): intended, but the analyzer cannot see the deferred check. 9 are `Rows.Close` after an error path or after `rows.Err()`: intended.
- Callees of the 38 findings: `(*encoding/csv.Writer).Write` 16, `(*database/sql.Rows).Close` 9, `oplog.Insert` 5, `(io.Closer).Close` 4, `(net.Conn).Close` 2, `(pgx.Tx).Rollback` 1, `(*crypto/tls.Conn).Close` 1.
- Before the `hash.Hash32` embedding fix, order-mgmt had 15 findings: the 2 extra were `hash.Write` calls on an `fnv.New32a()` value, which the doc contract makes provably nil.

