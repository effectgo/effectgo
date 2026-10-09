# effectgo

effectgo infers Effect-TS-style signatures — Effect<A, E, R> — for plain Go code. No rewrite into a monad: errors (E) and side effects (R) are inferred statically, cross-package, and enforced by contracts at `go vet` time.

Formerly goeffect: two prototypes (the effects and errset analyzers), now merged into one module and one binary.

## 30-second example

`examples/layers/domain/todo.go` declares a function pure, but one of its callees reaches the clock two hops down:

```go
// Normalize trims a title and appends a stamp. Declared pure, but the stamp
// reaches time.Now through infra.Stamp: a two-hop leak.
//
//effectgo:pure
func Normalize(s string) string {
	return strings.TrimSpace(s) + infra.Stamp()
}
```

`infra.Stamp` calls `time.Now().Format(...)`. Running the checker on `examples/layers` with the shipped stubs prints, among four findings, this line:

```
$ go vet -vettool=$(which effectgo) -stubs=/abs/path/to/effectgo/stubs/stdlib.stubs ./...
domain/todo.go:31:6: Normalize is declared //effectgo:pure but has effect time via Normalize → infra.Stamp → time.Now
```

go vet runs the tool from each package directory, so pass stub paths as absolute paths.

The message is the witness chain: each step is a call that the analyzer followed, and the last step is the taxonomy entry that introduced the effect.

Signatures are printed by the `sig` subcommand. For the todo example (`examples/todo`, with the shipped stubs):

```
$ effectgo sig ./...
domain/service.go:42:19: Service.Update: Effect<Todo, ErrConflict | ErrInvalid | ErrNotFound, {}>
domain/todo.go:62:17: Store.modify:   Effect<Todo, ErrNotFound | param:1, {param:1}>
api/v3.go:53:6: v3Update:       Effect<domain.Todo, domain.ErrConflict | domain.ErrInvalid | domain.ErrNotFound | fuego.BadRequestError | fuego.HTTPError | ?, {unknown}>
```

`param:1` is the effect of calling the function's own second parameter. `?` is an error whose origin is unknown. `unknown` is an effect that is not known (for example, an interface method call without a contract).

## Install

```
GOPRIVATE=github.com/faintghost/* go install github.com/faintghost/effectgo/cmd/effectgo@latest
```

The repository is private, so `GOPRIVATE` is needed for the module proxy to be bypassed. The `go install` line above is the documented route; it was not run from this environment, because the module is not published. From a clone, the verified route is:

```
go build -o bin/effectgo ./cmd/effectgo
```

Requires Go 1.26.

## Usage

One binary, two ways to run it:

```
effectgo ./...                                      # standalone: effects and errset diagnostics
go vet -vettool=$(which effectgo) ./...             # the same checks through go vet
effectgo sig ./...                                  # signatures only (see below)
effectgo openapi [-merge|-check spec.json] ./...    # OpenAPI error responses (see below)
```

Flags (the same spelling standalone and under `go vet`):

| Flag | Meaning |
|---|---|
| `-stubs=a.stubs,b.stubs` | stub files read by both analyzers. Falls back to `$EFFECTGO_STUBS` when empty. |
| `-effects.report` | also print `effects of <F>: {…}` for every declared function |
| `-errset.report` | print `errors of <F>: …` (and `handler errors of <F>: …`) for every function with an error result |
| `-errset.ignoreopaque` | do not report opaque (`?`) errors in handler checks |

`go vet` forwards only the flags that the tool advertises with `-flags`. `-stubs` is advertised, so it works through `go vet -vettool`. The prototypes had per-analyzer `-effects.stubs` and `-errset.stubs`; those are gone, in favour of the single shared flag.

Results are cached by `go vet` under the tool's identity. effectgo puts a hash of its own executable and of every stub file named in `$EFFECTGO_STUBS` into that identity, so editing a stub invalidates the cache. This covers the stub files named in `$EFFECTGO_STUBS` only. `go vet` asks the tool for its identity (`-V=full`) without any flags, so a stub file passed only through `-stubs` is not in the identity, and an edit to it can leave a stale cached vet result. For `go vet` use, put every stub file in `$EFFECTGO_STUBS` (as well as, or instead of, `-stubs`), or run `go vet -a` after editing a stub. The standalone run has no cache and is not affected. See `NOTES.md`, "Vet cache identity".

Recommended for `go vet`:

```
export EFFECTGO_STUBS=/abs/path/stubs/stdlib.stubs,/abs/path/stubs/fuego.stubs
go vet -vettool=$(which effectgo) ./...
```

## Directive reference

Directives are comments that start with `//effectgo:`. Each analyzer reads its own keywords and ignores the other's.

Effects (`effects` analyzer):

| Directive | Where | Meaning |
|---|---|---|
| `//effectgo:pure` | function, or interface method or type | the effect set must be empty |
| `//effectgo:allow net,time` | function, or interface method or type | the effects must be a subset of the listed names |
| `//effectgo:package pure` / `//effectgo:package allow a,b` | package doc comment only | default for every function in the package without its own directive |

Effect names: `net fs exec env time rand stdio db spawn panic exit global unknown`. A function with several directives must satisfy all of them.

Errset (`errset` analyzer):

| Directive | Where | Meaning |
|---|---|---|
| `//effectgo:errors A, pkg.B, ?, iface:pkg.I` | function, or interface method or type | every inferred error variant must be allowed |
| `//effectgo:errmap` | `func(error) error` | the function maps domain errors to HTTP errors; its coverage is exported |
| `//effectgo:handler M` | an HTTP handler or handler constructor | every error the handler can return must be covered by errmap `M`; uncovered ones are reported as HTTP 500 |

Unknown keywords are reported. In the combined binary, an unknown keyword is reported once by each analyzer, so a typo can appear twice.

## Stub file format

A stub file describes code that effectgo does not analyze, as a `.d.ts` file does in TypeScript. One rule per line, `#` starts a comment, fields are separated by spaces. Each verb has a fixed number of fields:

```
effects <target> <effect,effect,...>     # or: pure        (read by effects)
errors  <target> <variant,variant,...>   # or: none        (read by errset)
status  (pkgpath.Type) <code>            #                 (read by errset)
route   pkgpath.Func <METHOD> <pathArgIndex> <handlerArgIndex>  (read by openapi)
```

Targets: `pkgpath.Func`, `(pkgpath.Type).Method`, `(*pkgpath.Type).Method`, `(pkgpath.Iface).Method`, `(error).Error`. A `status` target is a type. Variants: `pkgpath.Name` (sentinel), `type:pkgpath.Type`, `iface:pkgpath.Iface`, or `?`. Indexes are 0-based.

Shipped files: `stubs/stdlib.stubs` (both verbs, with evidence comments) and `stubs/fuego.stubs` (the Fuego `Body()` rule, with source references). A malformed line is a hard error that names the file and line.

## OpenAPI error docs

`effectgo openapi` derives the error responses of each route from its handler's error set and its `//effectgo:handler` errmap. Fuego's documentation of errors is written by hand (`OptionAddError`); this command states what the code can return, with a witness for each response, and it can check or merge a document.

```
$ cd examples/todo && effectgo openapi -stubs=$STUBS ./...    # a fragment; undocumented routes go to stderr
PUT /v3/todos/{id}  (x-effectgo-handler: v3Update(...))
  400  Bad Request: domain.ErrInvalid (via Service.Update → checkTitle); fuego.BadRequestError (via parseID)
  404  Not Found: domain.ErrNotFound (via Service.Update → Store.modify)
  500  Internal Server Error: unclassified error (via (Context).Body [stub]); domain.ErrConflict (via Service.Update → Store.checkTitle); an unclassified error becomes 500 unless it carries its own status
  default  Unknown status: fuego.HTTPError (via (Context).Body [stub])
```

In the examples below, `STUBS` is the comma-separated absolute paths of `stubs/stdlib.stubs` and `stubs/fuego.stubs`.

The fragment has `paths.<path>.<method>.responses.<code>`. Each response has a `description` and an `x-effectgo-errors` list. A variant whose status is unknown goes under `default`, and it is never given a guessed code. An unclassified error (`?`) is 500: Fuego sends any error that does not carry its own status as 500, so it goes under `500` with that explanation, not under `default`. v1 and v2 of the example are undocumented: v1 has no errmap directive, and v2 wraps its handlers in `httpres.NoBody` and `WithBody`. The reasons are printed.

```
effectgo openapi -stubs=$STUBS -check spec.json ./...   # drift; exit 1 when a code is missing from spec.json
effectgo openapi -stubs=$STUBS -merge spec.json ./...   # the document with the error responses replaced
```

`-check` reports `missing` codes (the code can produce them, the document does not; these exit 1), `stale` codes (documented, but no variant produces them; only for complete routes), and `unchecked` routes. Against Fuego's own document for the todo server, it reports:

```
POST /v3/todos:
  missing default: fuego.HTTPError
PUT /v3/todos/{id}:
  missing default: fuego.HTTPError
summary: 2 missing, 5 stale, 0 not in document, 10 unchecked
```

Fuego's document has 400, 404 and 500 on every operation, not 404 only, so the stale lines are its generic 500s. `-merge` keeps everything except the 4xx, 5xx and `default` responses of the routes it knows, and references the `HTTPError` schema. The merged document of the example validates with kin-openapi, and a runtime test checks that the statuses observed for `PUT /v3/todos/{id}` are documented. NOTES.md, "OpenAPI", has the full outputs and the evidence.

## `effectgo sig`

`effectgo sig ./...` runs only the signature analyzer. It prints one line per declared function, `Effect<A, E, R>`:

- **A** is the non-error result type; `void` if none; a tuple if several. For a function that returns a handler literal, A is the handler's result.
- **E** is the inferred error set from errset, or `never` if the set is empty. For a handler, E is the handler set.
- **R** is the inferred effect set from effects, with `param:i` placeholders.

The subcommand exits with status 3 when it printed something, because each signature is a diagnostic. The normal run (`effectgo ./...`) prints no signatures; `sig` mode runs the two analyzers as dependencies and does not print their diagnostics.

## Comparison with related tools

This section describes the other tools at a high level, as their own documentation summarises them. It was not re-checked against their current releases.

- **goexhauerrors** checks call sites for exhaustive handling of errors. effectgo instead infers, for each function, the set of errors it can return and the effects it can perform, so a caller sees the signature without reading the call site.
- **Capslock** reports the capabilities that a whole program can reach. effectgo reports per-function effect sets and checks them against contracts written in the code, so a violation is attributed to the function that declared the contract, with a witness chain.
- **effectgo** adds the HTTP boundary check: an error that a handler can return must be mapped by the project's error-to-HTTP function, or it becomes an HTTP 500.

## Limitations

The full list, with `[test]` and `[reading]` tags, is in `NOTES.md`. The main ones:

- Interface method calls and calls through func values are `unknown` or `param:i` unless a contract or stub covers them.
- Stdlib effects and errors are modelled by a table and by stubs. Anything outside them is invisible (effects) or opaque (`?`, errset).
- Errset is flow-insensitive: an error variable that is assigned anywhere in a function contributes to its set everywhere.
- `go vet` cache: see "Usage" above for files named only by `-stubs`.

## Layout

| Path | Purpose |
|---|---|
| `effects/` | the effects analyzer |
| `errset/` | the errset analyzer |
| `signature/` | the `Effect<A, E, R>` analyzer, reached through `effectgo sig` |
| `openapi/` | the route analyzer and the OpenAPI fragment, merge and check output, reached through `effectgo openapi` |
| `internal/stubs/` | the shared stub-file parser |
| `cmd/effectgo/` | the binary |
| `stubs/` | shipped stub files |
| `examples/layers/` | effects demo (its own module) |
| `examples/todo/` | errset realworld (its own module) |
| `examples/run.sh` | builds the binary and runs it on both examples |

Run `go test ./...` at the root. `NOTES.md` has the design notes and the outputs of the merge.
