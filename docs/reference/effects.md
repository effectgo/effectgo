---
title: Effects
description: The effect taxonomy, how effect sets are inferred across packages, contracts, interface contracts, parameter polymorphism and closure attribution.
---

The effects analyzer computes, for every declared function, the set of side effects it can perform. The set is written `R` in `Effect<A, E, R>`. Effects are inferred from source; the standard library is modelled by a table and by stub files, and nothing in it is analyzed.

Contracts (`//effectgo:pure`, `//effectgo:allow`, `//effectgo:package`) are checked against the inferred set. See [Directives](directives.md).

## Effect names

`net fs exec env time rand stdio db spawn panic exit global unknown`

A set may also contain `param:i`, the effect of calling the function's own parameter `i` (index among the parameters, receiver excluded). Witness chains name the functions that lead to an effect.

## Root sources (taxonomy)

Root effects are matched by package path and function name. This table is the whole model of the standard library, apart from stubs. A standard-library function outside the table contributes nothing.

| Effect | Roots |
|---|---|
| `net` | `net`: `Dial*`, `Listen*`. `net/http`: `Get`, `Post`, `Head`, `PostForm`, `ListenAndServe*`; methods `(*http.Client).{Do,Get,Post,Head,PostForm}`, `(*http.Server).ListenAndServe*` |
| `fs` | listed `os` functions (`Open`, `OpenFile`, `Create`, `ReadFile`, `WriteFile`, `Remove`, `RemoveAll`, `Mkdir`, `MkdirAll`, `Rename`, `Stat`, `Lstat`, `ReadDir`, `Chdir`, `Chmod`); every method of `*os.File`; all of `io/ioutil` |
| `exec` | all of `os/exec` |
| `env` | `os`: `Getenv`, `LookupEnv`, `Setenv`, `Unsetenv`, `Environ`, `Hostname`, `Getwd`; any reference to `os.Args` |
| `time` | `Now`, `Since`, `Until`, `Sleep`, `After`, `AfterFunc`, `Tick`, `NewTimer`, `NewTicker` |
| `rand` | all top-level `math/rand` and `math/rand/v2`; `crypto/rand`: `Read`, `Int`, `Prime`, `Text` |
| `stdio` | `fmt`: `Print*`, `Println`, `Scan*`; all of `log`; any reference to `os.Stdin`, `os.Stdout`, `os.Stderr` |
| `db` | methods of `*database/sql.DB`, `Tx`, `Stmt`, `Conn` |
| `exit` | `os.Exit`, `log.Fatal*` |
| `panic` | the builtin `panic`, `log.Panic*` |
| `spawn` | a `go` statement |
| `global` | an assignment, `++`, `--`, an op-assign or a range-assign whose target is rooted at a package-level variable. `:=` never writes a global; `*p = x` is not followed |
| `unknown` | an interface method call (including type-parameter constraint methods) and a call through a func-typed variable, field or result that is not otherwise resolved |

Builtin `print` and `println` are pure in the table.

Stdlib functions that write to or read from an argument (`fmt.Fprintf`, `io.Copy`, `bufio.Writer.Flush`) are not in the table. Their stub rules name the methods they call, with via items (see [Stub files](stubs.md#effects)), and the effects of those methods are evaluated at each call site from the argument's static type. An interface argument such as `io.Writer` is charged exactly as a direct call of its method would be: its contract, or `unknown`. A stored writer or reader, such as the one of `json.NewEncoder(w)`, is evaluated where its method is called on the constructor's result or on a trusted local variable that holds it (see [Stub files](stubs.md#effects)); any other stored value makes the method `unknown`.

## Inference

For each declared function, the body is walked. A func literal that the function runs is walked as part of it (see [closure attribution](#closure-attribution)); any other literal is analyzed on its own. Direct effects carry a witness, the chain from the function to the root.

Static callees in the same package are edges. Callees in other packages contribute through their exported effect facts. A fixed point over the edges offers each callee's effects to the caller with the caller prepended to the witness. A witness is replaced only by a strictly shorter one, so the loop terminates and ties keep the first witness found. Recursion terminates and propagates around cycles. Callees are resolved to their generic origin, so an instantiated generic uses its declaration.

Witnesses are rendered relative to the analyzed package. Steps in that package are unqualified; other steps are qualified (`infra.Now`, `time.Now`, `(*Box).Set`). Interface steps render as `io.Writer.Write`, a contract step as `repo.(Repo).Get [contract]`, and a stub step as `io.(Writer).Write [stub]`.

Stdlib packages export no facts. Every other package exports one `EffectFact` per declared function.

## Precedence of a call

For a call, the first match wins:

1. a stub `effects` rule for the callee (a named func type or a func variable is looked up before the parameter placeholder); a via item of the rule is evaluated at the call site, after the rule's fixed effects;
2. for an interface method, the interface contract if there is one, else `unknown`;
3. the root taxonomy;
4. otherwise, the callee's effect set from its body or its imported fact. A call through a parameter of the function gives `param:i`; a call through any other func value gives `unknown`.

Stubs are described in [Stub files](stubs.md#effects).

## Contracts

- `//effectgo:pure`: the set must be empty.
- `//effectgo:allow a,b`: the set must be a subset. `unknown` is allowed only if it is listed.
- `//effectgo:package pure` or `allow …` in a package doc comment: the default for every function of the package without a directive of its own.
- Several directives on one function are all enforced (their intersection). An invalid directive is reported and dropped; the package default still applies.

Violation, one per function and effect, at the function's name:

```text
Normalize is declared //effectgo:pure but has effect time via Normalize → infra.Stamp → time.Now
```

## Interface contracts

A directive on an interface method, or on the interface type (for every explicitly declared method; a method-level directive overrides it), gives the method a contract that importers see. A call to a contracted method contributes the allowed effects, with the witness `Caller → (Repo).Get [contract]`. A call to an uncontracted interface method contributes `unknown`.

Conformance: for each named, non-generic type `T` in the package and each contracted interface `I` (in this package or any imported one) that `T` or `*T` implements, each implementing method's effects must be allowed by the contract. Violation:

```text
(*HTTPRepo).Get violates contract of (Repo).Get [allow db]: has effect net via (*HTTPRepo).Get → http.Get
```

Reported at the method's name when the method is declared in the analyzed package, or at the type's name when it is promoted.

## Parameter polymorphism

A call through one of the function's own parameters `p` records the placeholder `param:i` with witness `[F, dynamic call p]`. Placeholders satisfy every contract, because the function only calls its own parameter: a function that only calls its parameter is pure. At a call site the placeholder is replaced by what the argument contributes:

| Argument | Contributes |
|---|---|
| func literal written in the call | the literal's own effects, when the callee calls that parameter; nothing when the callee only stores it |
| an identifier naming parameter `j` of the caller | `param:j` (composition) |
| a named function, method value, method expression, qualified identifier or explicit instantiation | that function's effects; its own placeholders become `unknown` |
| anything else | `unknown` |

The witness composes through the chain: `ViaOuter → polylib.Outer → polylib.Modify → clockFn → time.Now`. A method expression call is offset by one.

## Closure attribution

A func literal is charged to the function that contains it only when that function runs it:

- called in place, `func(){ … }()`, including under `defer` and `go` (a `go` also adds `spawn`);
- assigned once to a local variable, and that variable is called by code that runs (the function itself, or a literal the function runs). Any other assignment to the variable, or none, disqualifies it. A call made inside a literal that is only passed or stored does not make the local literal run.

Any other literal (passed, stored in a field, returned, sent) gets a state of its own. A literal argument contributes its effects through parameter polymorphism when the callee calls that parameter; a callee that only stores it contributes nothing. A callee with no body information (stdlib without a stub, a dependency without facts, a call through a func value) may run the literal, so the literal is charged there. Interface methods count as known: a literal passed to one is not charged separately.

The witness of a passed literal names it: `CallsLiteralClock → polylib.Modify → func literal (poly.go:39) → time.Now`.

A function that returns or stores a literal that is not run is pure with respect to that literal.

## Report output

- `-effects.report` prints `effects of <F>: {a, b}` for every declared function (`{}` when empty).
- `-effects.witness` prints one `witness of <F>: <effect> via <chain>` per effect.

Both are informational and do not change the contract diagnostics. See [CLI](cli.md#analyzer-flags).

## Limitations

The main limits are in [Limitations](../design/limitations.md#effects). The short version: interface calls and calls through func values are `unknown` or `param:i` unless a contract or stub covers them; stdlib effects outside the table and the stub files are invisible; a writer that a value stores is not tracked (its methods are `unknown`); reflection, cgo and writes through pointers are not followed.
