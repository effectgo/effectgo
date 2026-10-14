---
title: "P9: errors.AsType narrows like errors.As"
description: "errors.AsType[T](err) narrows the error set on both edges of its ok result, with the matching rules of errors.As. The predicate facts and the HTTP readers recognize the same forms. errors.As is unchanged, and a limit of its predicate rule is pinned."
---

### errors.AsType narrows like errors.As (P9)

Branch `fix/errors-astype`, from `main` at `3e5543d`. A user's probe showed a false positive: after `if _, ok := errors.AsType[*BusyError](err); ok { return ErrBusy }`, the false branch still held `*BusyError`, while the same guard written with `errors.As` was narrowed. The fix reads the ok result of the generic call as the `errors.As` test of its type argument.

**Detection.** The call is recognized by the standard library's generic function: the origin of the SSA instance, or the callee of the AST call, whose package is `errors` and whose name is `AsType`. A function of another package with that name does not narrow.

**Narrowing.** Both edges of the ok result narrow the error argument, as `errors.As` does, with the same target matching (concrete types, pointers, values and interfaces) and the same wrapper handling. A negated test and a later test of an assigned result narrow too.

**Predicates and readers.** A predicate exports its test only in three straight-line forms. An early constant `return false` before the test would make the false edge drop a target that the predicate does not exclude, so that shape exports nothing. The errmap readers, the status branches, and the Gin and Huma adapters read the if-init form `if _, ok := errors.AsType[T](err); ok`. Huma also exports the straight-line predicate forms.

**Left as it was.** `errors.As` is unchanged. Its predicate rule accepts an early constant false return, and `predlimit.CallerAs` is a false negative that this causes. It is pinned as a limit. The AST engine does not narrow on either form.

**Measurements.** The numbers are from the final tree of the branch, and the references are the untouched `3e5543d`.

- The probe module prints `0 findings`, exit 0. At `3e5543d` it printed one finding, exit 3.
- `examples/run.sh` exits 0, and its 61 lines are identical to `3e5543d`.
- The disciplined corpus gives the same output as `3e5543d`: 17 findings. `check.py --future` reports TP 17, FN 0, FP 0.
- `dogfood/reservations` gives 0 findings, and `dogfood2/importer` gives 2 findings, both identical to `3e5543d`.
- `go test -count=1` passes in all 17 packages: 15 have tests and pass, and 2 have no test files. errset also passes with `EFFECTGO_TEST_ENGINE=ast`. `go vet` and `gofmt -l` are clean.
