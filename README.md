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
effectgo ./...                                      # standalone: effects, errset, mustuse and exhaustive diagnostics (no test files)
go vet -vettool=$(which effectgo) ./...             # the same checks through go vet
effectgo sig ./...                                  # signatures only (see below)
effectgo openapi [-merge|-check spec.json] ./...    # OpenAPI error responses (see below)
effectgo http-gin [-json] [-all] [-mappers] ./...   # Gin responses against the error sets (see below)
effectgo http-huma [-json] [-all] ./...             # Huma routes: variants that reach HTTP 500 (see below)
effectgo http [-json] [-all] ./...                  # both HTTP checks in one load, one list (see below)
```

Standalone runs skip `_test.go` files by default (`-test=false`). Pass `-test=true` to analyze them too. `go vet` is different: it decides by itself which packages and test variants it passes to the tool, so `-test` has no effect there (see "Test files" below).

Flags (the same spelling standalone and under `go vet`):

| Flag | Meaning |
|---|---|
| `-stubs=a.stubs,b.stubs` | stub files read by both analyzers. When empty, `$EFFECTGO_STUBS` is used; when that is empty too, the zero configuration applies (see "Stub files" below). |
| `-test` | analyze `_test.go` files too (standalone only; default `false`) |
| `-effects.report` | also print `effects of <F>: {…}` for every declared function |
| `-effects.witness` | also print one `witness of <F>: <effect> via <chain>` line per effect |
| `-errset.report` | print `errors of <F>: …` (and `handler errors of <F>: …`) for every function with an error result |
| `-errset.witness` | also print one `witness of <F>: <variant> via <chain>` line per variant (`witness of <F> [handler]: …` for handler sets) |
| `-errset.ignoreopaque` | do not report opaque (`?`) and fresh (`new@`) errors in handler checks |

### Stub files (zero configuration)

With neither `-stubs` nor `$EFFECTGO_STUBS` set, every run reads the stub files of the analyzed code's module, and nothing has to be configured:

- the stub files shipped inside the binary (`stubs/*.stubs`, embedded with `go:embed`, so a binary or the golangci-lint plugin works without a checkout);
- the project file `effectgo.stubs` at the module root (the nearest `go.mod` above the analyzed file), when the module has one. Its format is the stub format of `NOTES.md`; a malformed line is an error that names the file and line.

Setting `-stubs` or `$EFFECTGO_STUBS` replaces the whole zero configuration: then exactly those files are read, and neither the shipped stubs nor the project file are. The former "no stub files" warning is gone, because the zero configuration always loads the shipped stubs.

### Witness lines

`-effects.witness` and `-errset.witness` add lines next to the report lines; the `effects of` and `errors of` lines are unchanged:

```
$ effectgo -errset.report -errset.witness ./...
domain/service.go:42:19: errors of Service.Update: domain.ErrConflict | domain.ErrInvalid | domain.ErrNotFound
domain/service.go:42:19: witness of Service.Update: domain.ErrConflict via Store.checkTitle
domain/service.go:42:19: witness of Service.Update: domain.ErrInvalid via checkTitle
domain/service.go:42:19: witness of Service.Update: domain.ErrNotFound via Store.modify
```

The chain is the callee names from the function down to the origin, the function itself excluded. `(direct)` means the variant is created in the function. Effect chains use the same convention (`witness of F: time via Now → time.Now`). Witness lines print without the report flags too.

### Flow-sensitive error sets (the default engine)

The errset analyzer computes each function's error set over SSA values, so an error variable has the set of the value it holds at each point, not the union of every assignment to it. A guard narrows the value on each branch edge:

- `errors.Is(err, S)`: true keeps `S`, its wrappers and unknown origins; false removes `S`;
- `err == S`: as `errors.Is`, without the wrappers (`==` does not unwrap);
- `err == nil` / `err != nil`: the nil edge carries nothing;
- `errors.As(err, &x)` with a concrete type or an interface, and `v, ok := err.(T)`: true keeps the matching variants, false removes them;
- a predicate `func(error) bool` that tests its parameter with `errors.Is` or `errors.As` (for example `IsValidation`);
- `!` and `switch` (an if-chain) follow the same rules.

A reassigned variable and a value reset in a guard (`err = nil`) therefore no longer carry the old errors. Findings can disappear (the guarded path no longer reaches the sink) and a few can appear (a call through a local closure is resolved to the literal, and a per-use default reads the variable at its own use). `-errset.engine=ast` selects the previous flow-insensitive engine; the flag is accepted by `effectgo`, `sig`, `openapi`, `http`, `http-gin` and `http-huma`. See NOTES.md, "Flow-sensitive error sets (P3)", for the rules, the evaluation and the limitations.

### Error variants: `new@` and `?`

- `new@<pkg>.<Func>`: a fresh error created in `<Func>` without wrapping: `errors.New(...)`, or `fmt.Errorf` with a constant format and no `%w`, when the call is in a returned expression or flows into a returned local. There is one variant per creating function, not per call site, so that sets stay small. No `errors.Is` or `errors.As` can match it. A package-level `var ErrX = errors.New(...)` is still a sentinel.
- `?`: an error of unknown origin (opaque). It also covers a collapsed foreign variant, see below.
- `//effectgo:errors` accepts `new` (every fresh variant) and `?` (every opaque variant). They do not allow each other.
- In handler checks and in OpenAPI, a fresh error is an unclassified error: it is not covered by any errmap and becomes HTTP 500.

### Foreign variants: collapsed to `?`

Two kinds of variant are shown as `?` when they come from outside the analyzed module:

- a sentinel, type or interface whose identifier is unexported (for example `pgconn.connLockError`). Code outside its package cannot name it, so it cannot be matched with `errors.Is` or `errors.As`;
- a fresh `new@` variant created by a function of a foreign package, whatever the creator's name (for example `new@pgconn.Pipeline.Flush`). A fresh error is never matchable, so the name only exposes a dependency's internals.

The original name is kept in witness lines only: `witness of F: ? (pgconn.connLockError) via …`. The analyzed module is `pass.Module` of the package whose report or check runs. A package is inside it when its path is the module path or below it. Exported foreign sentinels and types (`pgx.ErrNoRows`, `sql.ErrNoRows`, `context.Canceled`, `*url.Error`) are kept. Without module information (`pass.Module` is nil, as in GOPATH test data) nothing is collapsed. The status of a collapsed type is kept.

### Test files

`effectgo ./...` and `effectgo sig ./...` skip test files by default. This removes the `_testmain.go` packages of the build cache as well. `-test=true` restores them. `go vet -vettool` is not changed by effectgo: `go vet` includes test variants itself, and effectgo cannot exclude them there. Run standalone for report output without test code.

`go vet` forwards only the flags that the tool advertises with `-flags`. `-stubs` is advertised, so it works through `go vet -vettool`. The prototypes had per-analyzer `-effects.stubs` and `-errset.stubs`; those are gone, in favour of the single shared flag.

Results are cached by `go vet` under the tool's identity. effectgo puts a hash of its own executable and of every stub file that the analysis reads from the working directory into that identity: the zero configuration (the shipped files and the module's `effectgo.stubs`), or the files named in `$EFFECTGO_STUBS`. An edit to any of them invalidates the cache. `go vet` asks the tool for its identity (`-V=full`) without any flags, so a file passed only through `-stubs` is not in the identity, and an edit to it can leave a stale cached vet result. For that case, name the file in `$EFFECTGO_STUBS` as well, or run `go vet -a` after editing it. Project files of other modules below the working directory are not hashed either. The standalone run has no cache and is not affected. See `NOTES.md`, "Vet cache identity".

`go vet` needs no stub configuration any more (the zero configuration applies). To pin the stub files explicitly, for example to use a different set:

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
| `//effectgo:errors A, pkg.B, new, ?, iface:pkg.I` | function, or interface method or type | every inferred error variant must be allowed; `new` allows fresh (`new@`) variants, `?` allows opaque ones |
| `//effectgo:errmap` | `func(error) error` | the function maps domain errors to HTTP errors; its coverage is exported |
| `//effectgo:handler M` | an HTTP handler or handler constructor | every error the handler can return must be covered by errmap `M`; uncovered ones are reported as HTTP 500 |

Must-use and exhaustive matching (`mustuse` and `exhaustive` analyzers, see "Must-use results and exhaustive matching" below):

| Directive | Where | Meaning |
|---|---|---|
| `//effectgo:mustuse` | a named type declaration | a call that discards a value of this type (or of a pointer to it) is reported; an alias is not allowed here, mark the type it names |
| `//effectgo:sealed` | an interface with an unexported method, or a named integer or string type | a type switch on the interface, or an expression switch on the type, with no `default`, must cover every variant (see below) |

Unknown keywords are reported. In the combined binary, an unknown keyword is reported once by each analyzer, so a typo can appear twice.

## Must-use results and exhaustive matching

Two Rust-style guarantees, in the same `effectgo` binary and under the same rule: a diagnostic is reported only when the code is provably at risk, and everything else is silent.

**Must-use (`mustuse`).** A bare call statement is reported when one of its results is an error, or a `//effectgo:mustuse` type:

```
result of os.Remove is discarded; handle it or assign to _
result of domain.Charge is discarded; handle it or assign to _
```

`_ = f()` is the explicit discard. `go` and `defer` statements are not reported. The only built-in allowlist is `fmt.Print*` and `fmt.Fprint*`; `-mustuse.allow=os.Remove,(*os.File).Close` adds fully-qualified names (for a call through a local func variable, the function assigned to it is matched).

*Integration with error sets.* An error result is reported only when errset cannot prove the callee's error set empty. A function that always returns nil is silent, and so is a stub rule `errors <target> none`. The shipped `stubs/stdlib.stubs` states this for the writers that never fail (`bytes.Buffer`, `strings.Builder`, `hash.Hash.Write`, with the source lines in the file). These rules need the stub file: without it, `buf.WriteString("x")` is reported, since the former built-in allowlist for those writers is gone. A `//effectgo:mustuse` value is reported whatever the error set.

**Exhaustive (`exhaustive`).** A type switch on a sealed interface without `default` must have a case for every variant:

```
missing cases in type switch on Shape: Square, Triangle
missing cases in switch on Status: Archived
```

An interface is sealed when it has an unexported method declared in its own package (the marker-method pattern: only that package can implement it), or when it carries `//effectgo:sealed`. Its variants are the named types of the package that implement it, so a switch in another package sees the same set. Types declared in `_test.go` files are not variants. An interface that embeds a sealed interface is checked against the variants that implement it. A named integer or string type with `//effectgo:sealed` is an enum: its variants are the distinct values of its constants, and only exported constants are required outside the package. A case that is not a constant leaves the switch undecided. See NOTES.md, "mustuse & exhaustive", for the rules and the defects of the prototype that were fixed.

## Stub file format

A stub file describes code that effectgo does not analyze, as a `.d.ts` file does in TypeScript. One rule per line, `#` starts a comment, fields are separated by spaces. Each verb has a fixed number of fields:

```
effects <target> <effect,effect,...>     # or: pure        (read by effects)
errors  <target> <variant,variant,...>   # or: none        (read by errset)
status  (pkgpath.Type) <code>            #                 (read by errset)
route   pkgpath.Func <METHOD> <pathArgIndex> <handlerArgIndex>  (read by openapi)
sink    <target> status=<argIndex> [body=<argIndex>]   (read by http-gin)
status  pkgpath.Func <code | arg:N>       # a constructor that carries its status (read by errset)
route   pkgpath.Func <METHOD|@N.Field> <pathArgIndex|@N.Field> <handlerArgIndex>  (read by http-huma, openapi)
errdefault <pkgpath> passthrough=iface:pkgpath.Iface,fallback=<code>   (read by http-huma)
```

`@N.Field` reads a field of the composite literal that is argument N (Huma's `Register(api, huma.Operation{Method, Path}, h)` is `route … Register @1.Method @1.Path 2`). A status target can be a function or method (`status pkgpath.Func 404`) or a type (`status (pkgpath.Type) 400`). A file may hold every form; a malformed line is an error that names its file and line. The shipped `stubs/*.stubs` load together (`EFFECTGO_STUBS=stubs/fuego.stubs,stubs/gin.stubs,stubs/huma.stubs,stubs/pgx.stubs,stubs/stdlib.stubs`).

Targets: `pkgpath.Func`, `(pkgpath.Type).Method`, `(*pkgpath.Type).Method`, `(pkgpath.Iface).Method`, `(error).Error`. Two more forms name a function value rather than a declared function: `(pkgpath.Type)`, a named func type (`effects (context.CancelFunc) pure` covers every call through a value of that type), and `pkgpath.Var`, a package-level variable of func type (`effects github.com/danielgtaylor/huma/v2.NewError pure`). A `status` target is a type. Variants: `pkgpath.Name` (sentinel), `type:pkgpath.Type`, `iface:pkgpath.Iface`, or `?`. Indexes are 0-based.

Shipped files, and when to load them:

| file | load it when |
|---|---|
| `stubs/stdlib.stubs` | always. Standard-library rules: `error.Error`, `fmt.Stringer`, io, `context` (`Err` errors, `CancelFunc`), database/sql `Row`/`Rows`/`Result` and their errors, `io/fs` getters, `http.ResponseWriter.Header`. |
| `stubs/pgx.stubs` | the service uses pgx v5. Gives the pool, connection, `Tx`, `Rows` and `Row` methods the `db` effect, and their sentinel errors. |
| `stubs/gin.stubs` | the service uses gin. Response writers and body readers are `net`; query, header and context-state methods are pure. Removes the false `panic` and `unknown` on every handler that calls `c.JSON`. |
| `stubs/huma.stubs` | the service uses Huma v2. `huma.NewError` is pure; the 40 status constructors, `NewError` with a constant status, the route functions and the default errmap (see "HTTP check: Huma"). |
| `stubs/fuego.stubs` | the service uses Fuego. The `Body()` rule, with source references. |

`stubs/gin.stubs` and `stubs/huma.stubs` describe the default implementations. Each file's header gives the module version it was checked against and the file:line of every rule. Judgment calls are explained in the comments: for example, `sql.Result` is pure and `io.Closer.Close` stays `unknown`.

Load several at once, comma-separated and absolute: `EFFECTGO_STUBS=/abs/stubs/stdlib.stubs,/abs/stubs/pgx.stubs,/abs/stubs/gin.stubs`. A malformed line is a hard error that names the file and line.

Closure attribution is not a stub: a func literal is charged to the function that contains it only when that function runs it (called in place, deferred, started with `go`, or called through a local variable). A literal that is passed, stored or returned is charged only through the parameter it is passed to. See `NOTES.md`, "Driver stubs & closure attribution".

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

## HTTP check: Huma

`effectgo http-huma` checks the routes of a Huma v2 service. It finds each route call (`huma.Get`, `Post`, `Put`, `Patch`, `Delete`, `Register`), resolves its handler's error set, maps that set through Huma's default error handling, and reports every variant that reaches the HTTP 500 fallback. No directive is needed in the code: the handlers are literals, and the default errmap is a stub rule.

```
$ cd service && effectgo http-huma ./...          # text, grouped by severity (info hidden)
$ cd service && effectgo http-huma -json -all ./... # one JSON object per finding, all severities
```

Huma answers an error with the status it carries. An error that implements `huma.StatusError` (or wraps one with `%w`) keeps its status; any other error becomes 500 (`huma.go:1094-1106`). The stub file says so as the `errdefault` rule:

```
errdefault github.com/danielgtaylor/huma/v2 passthrough=iface:github.com/danielgtaylor/huma/v2.StatusError,fallback=500
```

The stub verbs that the Huma adapter reads (see `stubs/huma.stubs`, and NOTES.md "Huma adapter"):

| verb | meaning |
|---|---|
| `route pkg.Func <METHOD\|@N.Field> <pathArg\|@N.Field> <handlerArg>` | a route call; `@N.Field` reads a field of the composite literal argument N, which is how `huma.Register(api, huma.Operation{Method, Path}, h)` is read |
| `status pkg.Func <code\|arg:N>` | a call returns an error that carries that status (`huma.Error404NotFound`), or the status is argument N (`huma.NewError(409, …)`) |
| `status (pkg.Type) <code>` | the existing type rule (Fuego's `StatusCode`); unchanged |
| `errdefault <pkgpath> passthrough=iface:…,fallback=<code>` | the default errmap of the routes of a package |

A route whose method or path is not a constant, or whose handler is not a literal or a resolvable function, is reported as unresolved with the reason. Nothing is guessed.

Severity (`info` is hidden unless `-all`):

| severity | a variant that reaches 500 |
|---|---|
| `high` | a named variant of the project's module (sentinel or typed error) that some other place in the project maps to a non-5xx status (a mapper, or a handler guard such as `if errors.Is(err, ErrNotFound) { return huma.Error404NotFound(…) }`) |
| `medium` | a named project variant that nothing maps |
| `info` | `?` (opaque), `new@` (fresh), and foreign named errors (`sql.ErrNoRows`, `context.Canceled`) |

The JSON schema is shared with the other HTTP adapters: `{"severity","framework","route","handler","pos","variant","status","via","reason"}`. `pos` is the route call site and `handler` names the literal (`func literal (file:line)`).

The result on the kuzma-wms backend (134 routes, static evidence only, see NOTES.md "Huma adapter"): 134 of 134 routes resolve; 543 variants, all with a known status; 355 reach the fallback 500 (65 high, 1 medium, 289 info). Most high findings are guard false positives: the same handler already maps the variant before the raw return. The manual check of a seeded sample of 21 rows found 19 false positives, 1 intended 500, and 1 medium false positive; the out-of-sample checks found 2 likely bugs. The table and the causes are in NOTES.md.

Limitations: the errset sets are flow-sensitive (see "Flow-sensitive error sets"), so a guard that precedes a raw return, a variable that is reused for another call, and a guard inside a callee are modelled; the analysis does not know that `Rows.Scan` never returns `sql.ErrNoRows`; a type that implements `GetStatus` (huma's other way to carry a status) is reported as unknown, not read.

## `effectgo sig`

`effectgo sig ./...` runs only the signature analyzer. It prints one line per declared function, `Effect<A, E, R>`:

- **A** is the non-error result type; `void` if none; a tuple if several. For a function that returns a handler literal, A is the handler's result.
- **E** is the inferred error set from errset, or `never` if the set is empty. For a handler, E is the handler set.
- **R** is the inferred effect set from effects, with `param:i` placeholders.

The subcommand exits with status 3 when it printed something, because each signature is a diagnostic. The normal run (`effectgo ./...`) prints no signatures; `sig` mode runs the two analyzers as dependencies and does not print their diagnostics.

## HTTP check: Gin

`effectgo http-gin` is the Gin counterpart of the errmap check. A Gin handler returns nothing: it writes the response with `c.JSON`, `c.AbortWithStatusJSON` or a project helper such as `httpapi.Detail(c, status, msg)`. So the boundary is a call, not a return value. The subcommand finds the response calls (sinks), reads their constant status, and reports each error variant that reaches a 5xx response. It also reports error text written into a response body.

```
$ EFFECTGO_STUBS=$STUBS,/abs/project.stubs effectgo http-gin -json ./...   # records of the shared HTTP schema
$ effectgo http-gin -mappers ./...                                          # text, and the mappers on stderr
```

A sink is declared by a stub line, `sink <target> status=<argIndex> [body=<argIndex>]`, with 0-based argument indexes. The Gin writers are in `stubs/gin.stubs`. A project helper is declared in the project's own stub file, outside the code:

```
sink github.com/example/app/internal/httpapi.Detail status=1 body=2
```

Mappers are found without annotations. A sink mapper is a function with a `*gin.Context` and an error parameter whose errors.Is, errors.As, `==` or switch branches answer with constant statuses; its default is the catch-all status. A status mapper is `func(error) (int, bool)` whose branches return a constant status. At a call site, `m(c, err)` gives the mapped variants their status and the rest the default; `if status, ok := m(err); ok { sink(c, status) }` followed by a sink gives the rest the fallthrough status.

Each record is one JSON object per line, with `severity`, `framework` (`gin`), `route`, `handler`, `pos`, `variant`, `status`, `via`, and `reason`. Severities:

| severity | meaning |
|---|---|
| `high` | a named variant (sentinel or type) reaches a 5xx response, and the same variant is mapped to a non-5xx status elsewhere in the project: an inconsistent mapping |
| `medium` | a named variant reaches a 5xx response; no mapping covers it |
| `info` | `?` or `new@` reaches a 5xx response; hidden unless `-all` |
| `leak` | a sink body carries `err.Error()` or an error formatted into a string (up to two composite literals deep); the status is the response status |

The analysis reads each sink and each variant rule at its own use (errset's flow, see "Flow-sensitive error sets"): a variable reused for two calls carries only the variants of the call that reaches the sink, and a guard above a sink removes what it tests. See NOTES.md, "Gin adapter", for the limitations and the evaluation on three services.

## HTTP check: both adapters (`effectgo http`)

`effectgo http [-json] [-all] [-stubs …] ./...` runs the Gin and the Huma adapters over one load of the packages and prints their findings as one list in the shared schema (`severity, framework, route, handler, pos, variant, status, via, reason`). The list is sorted by severity (`high`, `medium`, `leak`, `info`), then by position. `info` is hidden unless `-all`. The output is the union of `http-gin -json -all` and `http-huma -json -all`; the two subcommands are unchanged and still work on their own.

```
$ cd service && effectgo http ./...             # text: both checks, one list
$ cd service && effectgo http -json -all ./...  # one JSON object per finding
```

The summary line on stderr counts both adapters (the `info` counts include the hidden findings).

## Editor integration

The contract analyzers (`effects`, `errset`) also run in the editor, through a golangci-lint v2 module plugin (`github.com/faintghost/effectgo/plugin`). The plugin exposes only the contract analyzers, not the report flags or the HTTP subcommands.

```
cd examples/editor && golangci-lint custom     # builds ./bin/custom-gcl with the plugin
```

Then enable `effectgo` in the module's `.golangci.yml` (example: `examples/editor/.golangci.yml`) and point the editor at the binary:

- VS Code: `go.lintTool: "golangci-lint-v2"` with `go.alternateTools`, or `go.vetFlags: ["-vettool=…/effectgo"]`.
- Neovim: nvim-lint's `golangcilint` linter with `cmd` set to the binary.

The recipes, the exact invocations and what was and was not verified are in [docs/editor/](docs/editor/README.md). Stub files need no configuration (see "Stub files"). A known limitation: golangci-lint's result cache does not see stub file edits, so after editing a project `effectgo.stubs` run `golangci-lint cache clean` once; `go vet` does see them. Latency and the verification matrix are in `NOTES.md`, "Editor integration".

## Comparison with related tools

This section describes the other tools at a high level, as their own documentation summarises them. It was not re-checked against their current releases.

- **goexhauerrors** checks call sites for exhaustive handling of errors. effectgo instead infers, for each function, the set of errors it can return and the effects it can perform, so a caller sees the signature without reading the call site.
- **Capslock** reports the capabilities that a whole program can reach. effectgo reports per-function effect sets and checks them against contracts written in the code, so a violation is attributed to the function that declared the contract, with a witness chain.
- **effectgo** adds the HTTP boundary check: an error that a handler can return must be mapped by the project's error-to-HTTP function, or it becomes an HTTP 500.

## Limitations

The full list, with `[test]` and `[reading]` tags, is in `NOTES.md`. The main ones:

- Interface method calls and calls through func values are `unknown` or `param:i` unless a contract or stub covers them.
- Stdlib effects and errors are modelled by a table and by stubs. Anything outside them is invisible (effects) or opaque (`?`, errset), except `errors.New` and constant-format `fmt.Errorf` without `%w`, which are fresh (`new@pkg.F`).
- Errset is flow-sensitive over SSA values, with the guard forms listed above; a captured or address-taken error variable is the union of its stores; a field, map value or channel receive is opaque. Generic function bodies, which SSA does not lower, use the AST engine (flow-insensitive). `-errset.engine=ast` selects the AST engine for everything.
- `go vet` cache: see "Usage" above for files named only by `-stubs`.
- `mustuse` needs the stub file for the always-nil writers (see "Must-use results"). `exhaustive` cannot see variants declared in another package's test files, and it does not know a sealed interface that has no unexported method. A deferred error check (a sticky error that is read later, as in `csv.Writer`) is not seen: such a discard is reported.

## Layout

| Path | Purpose |
|---|---|
| `effects/` | the effects analyzer |
| `errset/` | the errset analyzer |
| `signature/` | the `Effect<A, E, R>` analyzer, reached through `effectgo sig` |
| `openapi/` | the route analyzer and the OpenAPI fragment, merge and check output, reached through `effectgo openapi` |
| `mustuse/` | the must-use analyzer: discarded errors and `//effectgo:mustuse` results |
| `exhaustive/` | the exhaustive analyzer: type switches on sealed interfaces, switches on sealed enums |
| `internal/stubs/` | the shared stub-file parser |
| `adapters/gin/` | the Gin response check (`effectgo http-gin`): sinks, mappers, handler findings, leaks, routes |
| `adapters/huma/` | the Huma route check (`effectgo http-huma`): routes, carried statuses, the default errmap, findings |
| `cmd/effectgo/http.go` | the umbrella `effectgo http`: both adapters over one package load |
| `cmd/effectgo/` | the binary |
| `stubs/` | shipped stub files |
| `examples/layers/` | effects demo (its own module) |
| `examples/todo/` | errset realworld (its own module) |
| `examples/run.sh` | builds the binary and runs it on both examples |

Run `go test ./...` at the root. `NOTES.md` has the design notes and the outputs of the merge.
