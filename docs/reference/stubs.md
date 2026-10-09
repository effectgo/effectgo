---
title: Stub files
description: The stub file format, every verb and value form, target syntax, precedence, zero-configuration discovery, and the shipped stub files.
---

A stub file describes code that effectgo does not analyze, the way a `.d.ts` file describes JavaScript. effectgo infers effects and errors from source bodies it can see; for the standard library, for driver packages, and for framework code it cannot see into, a stub states the answer. Each rule cites its source where it matters, and the shipped files are the worked examples.

## Format

- One rule per line. Fields are separated by spaces. `#` starts a comment that runs to the end of the line.
- The first field is the verb. Each verb has a fixed number of fields (the arity below), except `sink`.
- A malformed line is a hard error that names the file and line, for example `stubs/x.stubs:12: effects wants <target> <effects> with no spaces, got 4 fields`.

## Verbs

| Verb | Fields | Read by | Meaning |
|---|---|---|---|
| `effects <target> <effects>` | 3 | effects | the effects of a call; `pure` means none. Effect names: see [Effects](effects.md#effect-names) |
| `errors <target> <variants>` | 3 | errset | the error set of a call; `none` means no error can occur |
| `status <target> <code \| arg:N>` | 3 | errset, huma adapter | the HTTP status that an error carries (see below) |
| `route <pkg.Func> <METHOD \| @N.Field> <pathArg \| @N.Field> <handlerArg>` | 5 | openapi, huma adapter | a route registration call |
| `errdefault <pkgpath> passthrough=iface:<pkg.Iface>,fallback=<code>` | 3 | huma adapter | the default errmap of the routes of a package |
| `sink <target> status=<argIndex> [body=<argIndex>]` | 3 or 4 | gin adapter | a call that writes an HTTP response |

Other verbs are an error (`unknown verb "x" (want effects, errors, status, route, errdefault or sink)`). Analyzers ignore the verbs they do not read after validating their shape.

### `effects`

```text
effects time.Now pure                       # the call has no effect
effects (io.Writer).Write net               # every call through io.Writer is net
effects (context.CancelFunc) pure           # every call through a value of this func type
effects github.com/danielgtaylor/huma/v2.NewError pure   # calls through this package-level func variable
```

Effect names are checked: `unknown effect name "x" for <target>`.

Rules are in layers: the shipped files, then the project file or the `-stubs` files. Within a layer, a second `effects` rule for the same target is an error, and it names the first one:

```text
effectgo.stubs:3: duplicate effects rule for time.Now, first defined at /repo/effectgo.stubs:1
```

A rule from a project file (or an explicit `-stubs` file) for a target that a shipped rule also has **replaces** the shipped rule. The project file wins, so a project can state a different effect for a function that the shipped set covers. The replacement is the same as the `errors` verb, where a later rule already replaces an earlier one. Two shipped rules for one target are an error as well.

### `errors`

```text
errors strconv.Atoi type:strconv.NumError                     # a type variant
errors net/url.Parse type:net/url.Error
errors (github.com/go-fuego/fuego.ContextWithBody).Body type:github.com/go-fuego/fuego.BadRequestError,type:github.com/go-fuego/fuego.HTTPError,?
errors (*bytes.Buffer).Write none                              # never returns a non-nil error
```

Variants:

| Form | Meaning |
|---|---|
| `pkgpath.Name` | a sentinel variable |
| `type:pkgpath.Type` | an error type |
| `iface:pkgpath.Iface` | an interface (matched by method set) |
| `?` | an opaque error |
| `none` | the whole list: no error |

`param:` is not allowed in stub files. A name in a package that is part of the analyzed import graph must exist there (a missing name is a hard error); a name in a package outside the graph is kept as written and matched by identity.

### `status`

Two uses. On a type, it gives the status of every value of that type:

```text
status (github.com/go-fuego/fuego.BadRequestError) 400
```

On a function, it gives the status that the function's error carries (the constructor form). `arg:N` reads the status from argument N:

```text
status github.com/danielgtaylor/huma/v2.Error404NotFound 404
status github.com/danielgtaylor/huma/v2.NewError arg:0
```

A code must be in 100–599. A type target must not be a pointer.

### `route`

Names a registration function, the index of its path argument, and of its handler argument. The method is a constant (`GET`, `POST`, `PUT`, `PATCH`, `DELETE`) or the field of a composite literal argument (`@N.Field`). The target must be a function, not a method or a type.

```text
route github.com/go-fuego/fuego.Put PUT 1 2
route github.com/danielgtaylor/huma/v2.Register @1.Method @1.Path 2
```

`@N.Field` reads field `Field` of the composite literal that is argument N. A path given as `@N.Field` is read the same way. The OpenAPI analyzer uses only constant indexes and skips rules with field references.

### `errdefault`

The policy for errors that carry no status of their own, in a package's routes:

```text
errdefault github.com/danielgtaylor/huma/v2 passthrough=iface:github.com/danielgtaylor/huma/v2.StatusError,fallback=500
```

A value that implements the `passthrough` interface keeps the status it carries; every other error gets `fallback`. Both keys are required.

### `sink`

Declares a response writer for the Gin check. `status` and `body` are 0-based argument indexes; `body` is optional. The target must be a function or a method.

```text
sink github.com/example/app/internal/httpapi.Detail status=1 body=2
```

## Targets

| Form | Example | Notes |
|---|---|---|
| `pkgpath.Func` | `time.Now` | a package-level function |
| `(pkgpath.Type).Method` | `(io.Writer).Write`, `(pkg.T).M` | a method on a value receiver, or an interface method |
| `(*pkgpath.Type).Method` | `(*os.File).Close` | a pointer receiver; keyed separately from the value form in effects |
| `(error).Error` | `(error).Error` | the predeclared `error` interface |
| `(pkgpath.Type)` | `(context.CancelFunc)` | a named func type: applies to every call through a value of that type |
| `pkgpath.Var` | `github.com/danielgtaylor/huma/v2.NewError` | a package-level variable of func type: applies to calls through it |

A bare `(pkgpath.Type)` target is only valid for `effects`, `errors` and `status`. Generic functions are keyed by their origin.

## Precedence

- **Effects.** For a call, a stub rule wins over the interface contract, which wins over the root taxonomy. Named func type and func variable stubs are looked up before the placeholder for a parameter and before `unknown`. A stub applies at every call site, including a call to a function of the analyzed package; that function's own fact still comes from its body.
- **Error sets.** A stub `errors` rule wins over a contract, which wins over inference; a call the analysis cannot see into gives `?`. A stub on a function gives its set at each call site.
- **Status.** `status` on a type is used only when the type has no constant `StatusCode` method (the method's `StatusFact` is read from the source first).

## Discovery

The stub set of an analysis is decided per analyzed package, from its path, and every analyzer uses the same rule:

1. If `-stubs` is set, or `$EFFECTGO_STUBS` when `-stubs` is empty, exactly those files are read. Nothing else is read: the shipped files and the project file are not.
2. Otherwise (the **zero configuration**), the shipped stub files (embedded in the binary with `go:embed`) are read, and then `effectgo.stubs` at the root of the analyzed module, when the module has one. The module root is the nearest directory at or above the analyzed file that holds a `go.mod`. A project rule replaces a shipped rule for the same target (see [effects](#effects)).

A standalone run (`effectgo ./...`) checks the stub files it will read before the analyzers start: the explicit files, or in zero configuration the shipped files and the `effectgo.stubs` of the working directory's module and of each pattern's module. A bad rule is then printed once, as `effectgo: <file>:<line>: <message>`, and the run exits 1. Without this check, the same error would be printed once per package, followed by a `failed prerequisites` line for every dependent package. The check does not cover a module below the working directory that is not named by a pattern; there, an error is still reported per package. Under `go vet`, each package is a separate process, so each reports its own stub error.

`-stubs` takes comma-separated paths, absolute paths recommended. The explicit set replaces the zero configuration; an empty file given this way is still explicit, so it reads no shipped rules (the analyzers' own tests use one for this).

The project file takes the same format. It is the place for facts that the code cannot state, for example in `examples/editor/effectgo.stubs`:

```text
# Project stub file of the editor example. The store package has no body that effectgo can see through,
# so its effect is declared here.
effects example.com/editor/store.Save db
```

A later `-stubs` file does not merge with the zero configuration; to add a project rule to the shipped set, use `effectgo.stubs` at the module root.

## Shipped stub files

| File | Load it when | Covers |
|---|---|---|
| `stubs/stdlib.stubs` | always (it is part of the zero configuration) | `error.Error` pure; `fmt.Stringer`; `io` copy and read functions; `io` constructors; `context` (`Err` errors, `CancelFunc`); database/sql `Row`, `Rows`, `Result` and their errors; `io/fs` getters; `net/http` `ResponseWriter` (`Header` pure, `Write` and `WriteHeader` net), `Flusher.Flush`, `Hijacker.Hijack`, and the helpers `Error`, `NotFound`, `Redirect` (net), `ServeFile` (fs and net), `ServeContent` and `ServeFileFS` (net and unknown); `strconv` parse errors; `net/url.Parse`; the always-nil writers `bytes.Buffer`, `strings.Builder` and `hash.Hash.Write` (`none`, with source lines in the file) |
| `stubs/pgx.stubs` | the service uses pgx v5 | the pool, connection, `Tx`, `Rows`, `Row`, `BatchResults` methods as `db`, and the sentinels `pgx.ErrNoRows`, `pgx.ErrTxClosed`, `pgx.ErrTxCommitRollback` |
| `stubs/gin.stubs` | the service uses gin | response writers and body readers as `net`; query, header and context-state readers as `pure`; the `sink` rules of the Gin check |
| `stubs/huma.stubs` | the service uses Huma v2 | `huma.NewError` (pure, a package-level variable); the 40 status constructors (`status`); `NewError` (`arg:0`); the route functions; the default errmap (`errdefault`) |
| `stubs/fuego.stubs` | the service uses Fuego | the `Body()` rule (`errors`, with the unclassified `?` kept), status rules, the route functions, and the `StatusCode` rules |

Each shipped file's header gives the module version it was checked against, and each rule cites `file:line` in its comment. Judgment calls are explained in the comments: for example, `sql.Result` methods are pure, `sql.Rows.Scan` is `db`, `io.Closer.Close` stays `unknown`, and `io.Writer.Write`, `io.Reader.Read` are deliberately not listed. The shipped files are also embedded in the binary, so a plain install needs no files on disk.

A caution from the real-world evaluation: the `(Row).Scan` rule in `pgx.stubs` lists `pgx.ErrNoRows` for every row scan, including aggregate queries that always return a row. The Gin check reports those as false positives (see [HTTP checks](../guide/http-checks.md#known-false-positives)). The fix belongs in the stub and is a per-query rule, which a stub cannot express.

## Checking a stub file

- `effectgo -stubs=<file> ./...` fails with the file and line for a syntax error.
- `effectgo -effects.report -errset.report -stubs=<file> ./...` shows the effects and errors that the rules produce.
- `go vet` caches results under the tool identity, which includes the shipped files and the project file, but not a file named only by `-stubs`. See [CLI](cli.md#go-vet-and-the-cache).
