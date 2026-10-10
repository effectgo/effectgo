---
title: "P8: no silent directives (placement, arguments, trailing comments, interfaces without methods)"
description: "A new directive analyzer reports each directive that no analyzer reads where it is written. Trailing comments and arguments are read by one shared keyword rule, and a type-level contract on an interface that declares no methods is reported. The B4 handler item is closed."
---

### No silent directives (P8)

Branch `feat/inert-directives`, from `main` at the P7b merge (`7d37ea1`). The contract mode of effectgo is only as good as the directives it reads, so a directive that is written down but checks nothing must not be silent. A probe of inert placements and spellings found eight that printed nothing: a directive on a `var`, in a function body, on a function literal, in a comment that a blank line detaches from its declaration, with a space after the slashes, on a function with no error result, on a net/http handler, and on a function with no results. The first round of this phase reported them. The second round covered the forms that the first round had left open.

**Placement.** The new analyzer `directive` reports a directive that no analyzer reads at its site, and a comment with a space after the slashes that names a directive keyword. Its site table mirrors the readers in effects, errset, mustuse and exhaustive. Where another analyzer already reports the misplacement (`pure` in a package doc comment, `package` on a function, `errmap` on an interface), that analyzer's message is the only one. The shape checks stay with their owners: errset reports `errors` and `handler` on a function with no error set, and mustuse reports the directive on a function, which mustuse never reads as a contract.

**B4.** The backlog item for a `//effectgo:handler` on a net/http handler is closed by the first check. The check reads the handler set, not the outer signature. A handler constructor that returns a func literal with an error result is checked through that literal and is not reported; the errset tests and the todo example rely on that shape. The entry is removed from the backlog.

**Keywords, trailing comments and arguments.** mustuse and exhaustive matched their directive text exactly, so `//effectgo:mustuse // the caller handles it` silently did not make the type must-use. Effects has always read a keyword as the words after the prefix, up to the first word that starts with `//`. That rule moved to `internal/dirparse`, and every analyzer that reads a keyword now uses it. errset split the comment at the first space: on a function, `//effectgo:mustuse` followed by a tab and a comment printed `unknown directive //effectgo:mustuse<TAB>//` (checked on the binary before this phase). With the shared rule it is the mustuse keyword, and mustuse reports the function. An argument that a keyword does not take is reported (`//effectgo:mustuse takes no arguments`, `//effectgo:sealed takes no arguments`), and the directive still applies, as an argument to `pure` does in effects. A keyword glued to a comment (`//effectgo:mustuse//note`) is an unknown keyword, and the directive analyzer reports it where no analyzer reads it.

**Interfaces that only embed.** A type-level contract applies to the methods the interface declares. An interface that declares none checked nothing, and it was silent. effects now reports `pure` and `allow` there, and errset reports `errors`, with the message `effectgo:pure on Base checks nothing: an interface contract covers the methods it declares, and Base declares none`. The contract semantics are unchanged: the embedded interfaces keep their own contracts, and the embedding interface does not inherit them.

**Left silent on purpose.** `//effectgo:sealed` on a named integer type with no constants is silent. It is not a placement, no analyzer treats an empty enum as inert, and the probe is held at its twelve lines. Reporting it would be a separate decision.

**Index.** Entry 17 was missing from the index table. It is listed now, with this entry.

**Measurements.** The numbers are from the final tree of the branch. Where a comparison applies, the reference is the binary from before this phase.

- The probe module prints 12 diagnostics, exit 3: the four it printed before and eight new ones, one per line.
- The disciplined corpus gives the same output as before the phase: 17 findings. `check.py --future` reports TP 17, FN 0, FP 0.
- `dogfood/reservations` gives 0 findings, and `dogfood2/importer` gives 2 findings, both with output identical to before.
- `examples/run.sh` exits 0, and its 61 lines are identical to before.
- `go test -count=1` passes in all 17 packages: 15 have tests and pass, and 2 have no test files. `go vet` and `go build` are clean, and gofmt lists no file.

**Verification.** Each new form has a `// want` case in the testdata of its analyzer. The combined binary is checked on two modules: the probe (`cmd/effectgo/testdata/inert`) and a forms module (`cmd/effectgo/testdata/forms`), where each problem line has exactly one diagnostic and no other line has one. The errset tests run under both error-set engines.
