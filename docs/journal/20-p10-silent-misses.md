---
title: "P10: two silent misses (errors.As predicates with an early return, methods promoted from an embedded interface)"
description: "An errors.As predicate with a constant false return before its test exports no test, as errors.AsType already did. A method that a type promotes from an embedded interface is checked against the contract of the interface the type satisfies. Both changes are recorded with their before and after, and the measurements are against the untouched main."
---

### Two silent misses (P10)

Branch `fix/p10-silent-misses`, from `main` at `3df9d52`. Both were false negatives, and both broke the rule that a contract is either checked or reported, never silently passed.

**errors.As predicates: a semantic change.** A predicate whose constant `return false` comes before its `errors.As` test exported the test. The false edge of a call then removed the target, although the predicate can return false for a target. The callers of such a predicate were silent. The same predicate written with `errors.AsType` exported nothing, so it was already reported. Before and after, with the same caller:

```go
func IsBusyAs(err error) bool {
	if disabled {
		return false
	}
	var b *BusyError
	return errors.As(err, &b)
}

//effectgo:errors ErrA
func Caller() error {
	err := source() // a *BusyError
	if IsBusyAs(err) {
		return ErrA
	}
	return err
}
```

Before, the SSA engine printed nothing for `Caller`, and the AST engine reported it, because it does not narrow on predicates at all. After, both engines report it:

```text
Caller may return pred.BusyError (via source), not in its //effectgo:errors contract
```

The rule that is now applied is the second of the two options the design allowed, and it is the stricter one. The fact format (`TestFact`) has no polarity: a fact is applied to both edges of a call, so a test that holds on the true edge only cannot be expressed without changing the format. The predicate therefore exports nothing. The rule is the one `errors.AsType` already had: every return of the predicate is the test itself, or a constant on an edge of the same test. A constant `return false` that is not on the negative edge of the test is refused, and so is a constant true that is not on its positive edge. The straight-line forms still export: `return errors.As(p, &t)`, `if errors.As(p, &t) { return true }; return false` (with or without an else), and `if !errors.As(p, &t) { return false }; return true`. A declaration may come before them.

The rule is applied in three places, which must agree:

- the SSA engine, `errset/guards.go` (`predicateFact`, `falseEdgeOf`), which narrows the callers;
- the Huma adapter, `adapters/huma/evidence.go` (`exportPredicates`, `errorsAsShape`), whose predicates feed the mapping evidence. Before, it exported any predicate with one `errors.As` call in its body, including `return errors.As(err, &t) && cond`, which the SSA engine never exported. Now it reads the same straight-line forms. The consequence is that `if P(err) { return 400 }` gives no mapping evidence for such a predicate. No example, corpus or dogfood project has one;
- the AST engine of errset, which has no predicate reader of its own: it does not narrow on predicates, so it reports the same callers (`errset/testdata/src/predcontract`, run under both engines).

**Methods promoted from an embedded interface.** `checkConformance` skipped a method that a type gets from an embedded interface field (`isIfaceMethod`). A type that embeds `http.ResponseWriter` and defines no `Write` satisfied a contracted `Writer` (`allow stdio`) unchecked, although its `Write` writes to the client. The method is now evaluated as a call through the embedded interface, as a call site would evaluate it: its stub, its contract, or `unknown` for an uncontracted interface. The witness starts with the promoted method, and the report is at the embedded field:

```text
web/web.go:11:7: (*statusWriter).Write violates contract of (Writer).Write [allow stdio]: has effect net via (*statusWriter).Write [promoted from embedded http.ResponseWriter] → http.(ResponseWriter).Write [stub]
```

Its `io.Writer` variant, an uncontracted embedded interface, reports `unknown` in the same form. The other cases of the check are unchanged, and each is pinned by `effects/testdata/src/promoted` (`TestPromotedInterfaceMethods`):

- a type that declares its own `Write` is checked on that method only, so there is no second report;
- a type that embeds a contracted interface whose contract allows the effects is silent, and one whose contract does not is reported with the contract's witness (`promotedtimed`);
- a method promoted from a struct of this package was already checked at the struct's method. The check of the embedding type reaches the same diagnostic at the same position. The command-line output already showed it once, but `analysistest` counted it twice, so the check now reports each position and message once;
- a method promoted from a struct of another package is reported at the type's name, as before.

The same gap exists in the error-set conformance check: it infers no error set for an interface method, so a method promoted from an embedded interface is skipped (`TestPromotedErrorSetLimit`). It is pinned as a limit and not changed in this phase, because the change would add findings of a new kind for every embedded uncontracted interface, and that is a decision for the error-set side.

**Measurements.** The baseline is the binary built from `3df9d52` (`feedback/tmp/p10/base/effectgo`). The final binary is built from the final commit of the branch.

- `examples/run.sh` exits 0, and its 61 lines are identical to the baseline.
- The disciplined corpus gives the same output as the baseline, with 17 findings. `check.py --future` reports TP 17, FN 0, FP 0.
- `dogfood/reservations` gives 0 findings, and `dogfood2/importer` gives 2 findings, both identical to the baseline.
- The probe module (one predicate of each shape, one caller, the promoted types) gives 3 findings with the SSA engine at the baseline and 6 now, the three new ones being `CallerAs`, the `statusWriter` violation and the `io.Writer` violation. With `-errset.engine=ast` it gives 4 at the baseline and 6 now.
- `go test ./...` passes in all 17 packages: 15 have tests and pass, and 2 have no test files. errset also passes with `EFFECTGO_TEST_ENGINE=ast`. `go vet ./...` and `gofmt -l .` are clean.

**Tests.** Each fix has analysistest cases, and an end-to-end case through the standalone run.

- `errset/testdata/src/predearly`: the early-return predicates are not exported and their callers are reported (`TestPredicateEarlyFalse`). This package was `predlimit`, and its expectations are flipped. The straight-line `IsBusyTail` still exports, and `CallerTail` is narrowed by it.
- `errset/testdata/src/predcontract`: the callers under both engines (`TestPredicateEarlyFalseBothEngines`).
- `adapters/huma` fixture `asEarlyError`: no mapping evidence for the early-return predicate (`TestAsTypeMappingEvidence`). `service.IsBad`, the straight-line form with a declaration first, still gives its evidence.
- `effects/testdata/src/promoted` and `promotedtimed`, `promotedlib` for the foreign struct (`TestPromotedInterfaceMethods`).
- `errset/testdata/src/embedlimit` (`TestPromotedErrorSetLimit`), the limit above.
- `cmd/effectgo/silent_test.go` with the scratch modules under `testdata/predicate` and `testdata/promoted`: `TestStandalonePredicateEarlyFalse` and `TestStandalonePromotedInterface`.

**Limits left.** A predicate with an early constant return now narrows its callers on neither edge. The true edge could still be narrowed, since `true` implies that the target is in the chain, but the format cannot express a one-sided fact, so the precision on that edge is lost, which is safe. The error-set analogue of the promoted-method gap is pinned above. The `GAP` paragraph of the contract-mode guide, which P9 added, described the effects gap; it is replaced by the behaviour.
