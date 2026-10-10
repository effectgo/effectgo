---
title: Prototypes, deviations and open items
description: The two prototypes that preceded effectgo, the deviations from them, and what was left undone.
---

## Deviations from the prototypes

- **One binary.** The prototypes had `effectsvet` (singlechecker) and `effectsvet-vet` (multichecker) because the flag names differed. `effectgo` is a multichecker for both uses, so its flags are always prefixed. The standalone `-report` became `-effects.report`.
- **One stub flag.** `-effects.stubs` and `-errset.stubs` are gone; `-stubs` replaces them.
- **Directive namespace shared.** Each analyzer skips the other's keywords (see Shared pieces).
- **Module of examples/todo.** Renamed from `github.com/faintghost/protob` to `github.com/faintghost/effectgo/examples/todo`; `go mod tidy` keeps its `go 1.26.5` directive, which is also the directive of Fuego v0.20.0. The examples `go.mod` files are separate modules.
- **realworld-fixed not imported.** It is the scratch copy with the regression fixed and the handlers changed. It is not part of the examples, because the task keeps the regressions in `examples/todo`.
- **`go vet` runs.** The Round-1 notes listed `go vet -vettool` integration as untested. It is now run for both examples (see Merge). The malformed-stub test (`TestMalformedStubIsHardError`) still runs the standalone binary.
- **The `sig` exit status** is 3 when it prints signatures.

## Not done

- No CI, no fix suggestions, no `-fix`.
- No per-instantiation effect facts for generics.
- No v1 or v2 handler checks; only the annotated v3 handlers are checked, as the realworld prototype did.
- `go install` from the private repository was not run (no network access to it from here).

