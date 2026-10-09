---
title: "P1: OpenAPI error documentation"
description: "The OpenAPI error responses milestone: status inference, route discovery, the openapi subcommand, the Fuego drift check, and the 500 fix."
---

## OpenAPI

Error responses of the routes, derived from the errset analysis, with a drift check against a document. Three milestones, one commit each on `feat/openapi`: status inference, route discovery, the `openapi` subcommand (and the examples acceptance in two commits).

### Status inference (`errset/status.go`)

Each errmap rule (an `if errors.Is/As/==` branch, a switch case, a type-switch case, or the default) gives a status:

- the constant of a returned composite literal (`T{...}`, `&T{...}`) when its type has a constant `StatusCode`;
- otherwise the constant `Status` field of a literal (`fuego.HTTPError{Status: 409}` gives 409);
- `return err` is a passthrough: the variant keeps its own status;
- a return the analyzer cannot read, or a mix of statuses in one rule, gives an unknown status.

The status of a variant's type (`Variant.Status`, `Variant.DynamicStatus`) comes from, in order:

1. a `StatusFact` (exported on each `StatusCode() int` method whose returns are all the same constant; facts from dependencies work, so fuego's methods count);
2. a `status (pkg.Type) <code>` stub line (fallback only);
3. otherwise the status is dynamic (the type has `StatusCode` but no constant), and it is unknown.

A variant with no `StatusCode` and no stub is a plain error: Fuego sends it as 500. An opaque variant (`?`) is 500 as well, see "Fix: opaque errors → 500". An unknown status (an `iface:` variant, an unresolved `param:i`, a dynamic status) is reported as `default`, never as a guessed code.

Fuego v0.20.0 evidence (module cache, `github.com/go-fuego/fuego@v0.20.0`):

- A controller error goes to `s.ErrorHandler` and then `ctx.SerializeError`: `serve.go:177-180`, `ctx.go:531-537`.
- `ErrorHandler` returns a plain error unchanged (`return err` at `errors.go:208`; only `HTTPError` and `ErrorWithStatus` are converted, `errors.go:198-205`).
- The JSON error sender starts at 500 and only an `ErrorWithStatus` overrides it: `serialization.go:210-216` (`status := http.StatusInternalServerError` at 211; the override at 212-215). So a plain error reaches the client as 500.
- Constant `StatusCode` bodies, with status stub lines in `stubs/fuego.stubs`: `errors.go:115` (400), `:129` (404), `:143` (401), `:157` (403), `:171` (409), `:188` (406); `ctx.go:268` and `:285` (422).
- `fuego.HTTPError.StatusCode` is dynamic (`errors.go:90-95`: 500 when `Status` is 0, else `Status`). It has no stub line, so its status is unknown.
- The facts work across packages: with the `status` lines removed from a copy of `fuego.stubs` (the `route` rules kept), `fuego.BadRequestError` is still 400 and the todo document is unchanged. That 400 comes from the `StatusFact` that the errset analyzer exported for fuego's method.

Deviation: the task says an uncovered error maps to 500. That holds for plain errors. An uncovered error that implements `StatusCode` keeps its own status, which is what `ErrorHandler` does (`errors.go:237-239`). A sentinel (`errors.New`) is assumed plain; its initializer is not read. Recorded as a limitation.

Tests: `errset/testdata/src/status` with `TestStatusInference`. It covers constant inference (`ConstErr` is 409 although the stub says 418), the stub fallback (`StubErr` is 422), `ErrNotFound` mapped to `HTTPErr{Status: 409}`, the literal without a status (`ErrDup`, default), an uncovered sentinel and type (500), a passthrough of an `iface:` variant (default), and `?` (500, see the fix below; it was default before). The `-errset.statuses` flag prints the statuses per handler. For the todo example:

```text
api/v3.go:53:6: statuses of v3Update: 400 domain.ErrInvalid | 400 fuego.BadRequestError | 404 domain.ErrNotFound | 500 domain.ErrConflict | 500 ? | default fuego.HTTPError
```

### Route discovery (`openapi/`)

- `route` stub lines name the registration functions: `route github.com/go-fuego/fuego.Put PUT 1 2` (path is argument 1, handler is argument 2; `net_http_mux.go:50-66`). The functions are generic; the rule names the origin.
- The analyzer finds calls to these functions whose path is a constant string. The handler argument is resolved to: a call to a handler constructor (its handler set and its errmap); a function identifier (its set and errmap); or a func literal. A literal cannot carry `//effectgo:handler`, so it is undocumented ("no errmap"). A call whose callee does not return a func literal (a wrapper such as `httpres.NoBody`) is undocumented; the wrapped handler is not guessed.
- The errmap is the `//effectgo:handler M` directive. No directive, or an unresolved one, is undocumented.
- Facts: the analyzers do not share facts. `checker.inheritFacts` gives an analyzer only the facts of its own dependency actions (`x/tools@v0.51.0/go/analysis/checker/checker.go:385-415`). So openapi exports its own `HandlerFact` (on each function: its set, its errmap, whether it is a constructor) and `ErrmapInfo` (on each errmap), and reads them across packages.
- `errset.Result` is now a struct: `Funcs` (as before, plus the directive and the errmap object), `Literals` (the set of each func literal with an error result) and `Errmaps` (this package's errmap facts). The signature analyzer reads `Result.Funcs`.
- Tests: `openapi/testdata` with `TestRouteDiscovery`, `TestConstructorWithErrmap`, `TestUnmappedPlainErrorIs500`, `TestCrossPackageConstructor`, `TestCrossPackageFunction`, `TestUndocumentedReasons`.

### The `effectgo openapi` subcommand

`effectgo openapi [-stubs=…] [-merge spec.json | -check spec.json] [packages]`. It loads the packages with `golang.org/x/tools/go/packages` (`LoadAllSyntax`), runs the openapi analyzer through `checker.Analyze` (`go doc golang.org/x/tools/go/analysis/checker Analyze` in v0.51.0 shows it), and aggregates the `Graph.Roots` results across packages. The other modes stay on the multichecker, which exits the process.

- Default: an OpenAPI 3 fragment. `paths.<path>.<method>.responses.<code>` with a `description` and an `x-effectgo-errors` list of `{variant, via}`; `default` for unknown statuses; `x-effectgo-handler` on the operation. Undocumented routes go to stderr with their reasons.
- `-merge`: replaces the error responses (4xx, 5xx and `default`) of each matched operation, keeps everything else, and references `#/components/schemas/HTTPError` when the document has that schema. An operation the document does not have is reported on stderr.
- `-check`: per operation, `missing` codes (the code produces them, the document does not; these block and exit 1), `stale` codes (documented, not produced; only for complete operations, where every status is known), `not in document`, and `unchecked` routes with their reasons.

### Output on examples/todo

The commands are run from `examples/todo` with `E=/root/pp/go-rust/effectgo` and `S=$E/stubs/stdlib.stubs,$E/stubs/fuego.stubs`.

Routes of v3, from `effectgo openapi -stubs=$S ./...`. The `PUT /v3/todos/{id}` operation, checked against the code:

```text
PUT /v3/todos/{id}  x-effectgo-handler: v3Update(...)
  400  Bad Request: domain.ErrInvalid (via Service.Update → checkTitle); fuego.BadRequestError (via parseID)
  404  Not Found: domain.ErrNotFound (via Service.Update → Store.modify)
  500  Internal Server Error: unclassified error (via (Context).Body [stub]); domain.ErrConflict (via Service.Update → Store.checkTitle); an unclassified error becomes 500 unless it carries its own status
  default  Unknown status: fuego.HTTPError (via (Context).Body [stub])
```

- 400: `checkTitle` returns `ErrInvalid` (400 by the errmap), and `parseID` returns `fuego.BadRequestError` (its constant 400).
- 404: `Store.modify` returns `ErrNotFound`.
- 500: `ErrConflict` is not mapped by `httpres.ToHTTPError`, so it is 500.
- 500: the body's `?` (an unclassified error, 500 by the fix below) and `ErrConflict`.
- `default`: `fuego.HTTPError` from the body's validation. Its status is dynamic.

The other v3 routes, from the same run: `POST /v3/todos` has 400 (`ErrInvalid`, `BadRequestError`), 500 (the body's `?`) and `default` (`HTTPError`). `GET /v3/todos/{id}` and `DELETE /v3/todos/{id}` have 400 (`BadRequestError` via `parseID`) and 404 (`ErrNotFound`). `GET /v3/todos` has no error response, and that is right: `v3List` returns `svc.List(), nil`.

v1 and v2 are undocumented, with the reason from the run (stderr):

```text
undocumented: GET /v1/todos (…/api/v1.go:30:2): no errmap: the handler has no //effectgo:handler directive
undocumented: POST /v2/todos (…/api/v2.go:13:2): WithBody(...) does not return a func literal, so its handler is not visible (it may wrap another handler)
undocumented: GET /v2/todos (…/api/v2.go:14:2): NoBody(...) does not return a func literal, so its handler is not visible (it may wrap another handler)
```

v1 has no directive. The server-wide `errorHandler` (`api/common.go`) is the real mapper for v1, v2 and v3 alike, but directives would change the errset output that `examples/run.sh` pins (the examples keep the regressions on purpose). So v1 stays undocumented. v2 wraps its handlers in `httpres.NoBody` and `httpres.WithBody`, which the analyzer does not see through.

### Fuego's own document, drift and merge

`examples/todo/cmd/openapispec` writes the document that `api.NewServer(...).OutputOpenAPISpec()` generates. `regen-openapi.sh` regenerates `testdata/openapi/fuego.json` and `testdata/openapi/effectgo-merged.json` (the merge of it).

Fuego's document does not match the task's premise of "404 only". It documents 400, 404 and 500 on every operation: 404 is hand-written by `docOpts` (`api/common.go`), and 400 and 500 are Fuego's generic defaults. So the drift is different from the one the task described:

```sh
$ effectgo openapi -stubs=$S -check testdata/openapi/fuego.json ./...
GET /v3/todos:
  stale 400: documented, but no variant produces it
  stale 404: documented, but no variant produces it
  stale 500: documented, but no variant produces it
POST /v3/todos:
  missing default: fuego.HTTPError
DELETE /v3/todos/{id}:
  stale 500: documented, but no variant produces it
GET /v3/todos/{id}:
  stale 500: documented, but no variant produces it
PUT /v3/todos/{id}:
  missing default: fuego.HTTPError
(v1 and v2 lines: unchecked, with reasons, as above)
summary: 2 missing, 5 stale, 0 not in document, 10 unchecked
exit 1
```

The 2 missing codes are the `default` responses of `POST /v3/todos` and `PUT /v3/todos/{id}`: the code has responses of unknown status (`fuego.HTTPError`), and the document has no `default`. The `?` of those routes is 500 now, so the code's 500 is produced by the `?` as well as by `ErrConflict` (on PUT); the document has 500 (Fuego's generic default), so there is no missing 500. The stale codes are Fuego's generic 500 on routes that cannot return one, and the 400 and 404 of `GET /v3/todos`, which takes no input and cannot fail.

`-merge` of the same document: `effectgo openapi -stubs=$S -merge testdata/openapi/fuego.json ./...` prints the merged document. The v3 `PUT` then has `400`, `404`, `500` and `default`. Each inserted response references `HTTPError`. `GET /v3/todos` keeps only its `200`. The v1 and v2 operations are unchanged (they are undocumented). There are no merge notes.

Validation: `api/openapi_validate_test.go` loads the merged document with kin-openapi (`openapi3.NewLoader().LoadFromData`, `doc.Validate`), the version of `examples/todo/go.mod`. The first run failed on Fuego's own output, not on the merge: Fuego documents the `struct{}` response of the Delete routes as a schema with an empty name (`components.schemas[""]`), which the validator rejects. The fix is in the example: the Delete routes return a named empty type, `deleted`, whose JSON is still `{}`; v2 converts its result with `result.Map`. After that:

```text
--- PASS: TestMergedDocumentValidates
```

### Runtime cross-check

`api/openapi_crosscheck_test.go` drives `PUT /v3/todos/{id}` over `httptest`, seeded with `buy milk` (id 1) and `bread` (id 2). Each observed status must be a response key of that operation in `testdata/openapi/effectgo-merged.json`:

```text
--- PASS: TestPutV3ObservedStatusesAreDocumented
    --- PASS: .../invalid_title          (observed 400)
    --- PASS: .../missing_id            (observed 404)
    --- PASS: .../duplicate_title_(ErrConflict)  (observed 500)
    --- PASS: .../malformed_form_body   (observed 500)
```

Since the fix below, each case also asserts attribution: the documented response of the observed status must list the variant that produces it (`x-effectgo-errors`). The 400 lists `domain.ErrInvalid`, the 404 lists `domain.ErrNotFound`, the duplicate title's 500 lists `domain.ErrConflict`, and the malformed form body's 500 lists `?`. Before the fix, the malformed form body's 500 was matched only by the key, which `ErrConflict` provided; it is now attributed to `?` on its own.

### Verification

```text
== root: go build ./... && go vet ./... && go test ./... -count=1 && gofmt -l .
effects, errset, openapi, signature: ok (go test ./... -count=1)
gofmt -l .: no output
== examples/todo: go build ./... && go vet ./... && go test ./... -count=1
Go test: 35 passed in 7 packages
== examples/run.sh (bash): exit 0; the layers and todo outputs are unchanged from the Merge section
```

### Fix: opaque errors → 500

**The bug.** The opaque variant `?` (an error whose origin cannot be classified, e.g. `(Context).Body [stub]`) was reported as `default`, because `ownStatus` returned an unknown status for it. Fuego sends any error that does not carry its own status as 500 (`serialization.go:210-216`, and `ErrorHandler` returns a plain error unchanged, `errors.go:208`). So `?` is 500, and `default` documented it wrongly.

The runtime cross-check did not catch this. Its `malformed_form_body` case (`title=%zz` returns 500) passed only because `ErrConflict` (500) gave the 500 key. Had `ErrConflict` been mapped to 409, that subtest would have failed, although the code still returns 500.

**The fix.**

- `errset/status.go`, `ownStatus`: an opaque variant is 500 (with the reason in the comment). Param and iface variants, and a dynamic status, stay unknown (`default`).
- `openapi/report.go`: a 500 response that holds an opaque variant says "unclassified error" for it and adds "an unclassified error becomes 500 unless it carries its own status". `RouteError.Opaque` carries the variant kind to the report.
- The `plain error → 500` fallback stays hardcoded, not a `fallback` stub line. Fuego's rule is the server's error handler (`serialization.go:210-216`), not a property of a type. A stub keyed by a package would have to say which server handles a route, and the errset facts do not carry that. A stub would also need the status to reach `ErrmapFact.StatusOf`, which runs without a pass. That is more machinery than one constant, so the constant stays and this section is the record of it.

**Before** (`effectgo openapi`, `PUT /v3/todos/{id}`):

```text
PUT /v3/todos/{id}  x-effectgo-handler: v3Update(...)
  400  Bad Request: domain.ErrInvalid (via Service.Update → checkTitle); fuego.BadRequestError (via parseID)
  404  Not Found: domain.ErrNotFound (via Service.Update → Store.modify)
  500  Internal Server Error: domain.ErrConflict (via Service.Update → Store.checkTitle)
  default  Unknown status: ? (via (Context).Body [stub]); fuego.HTTPError (via (Context).Body [stub])
```

**After:**

```text
PUT /v3/todos/{id}  x-effectgo-handler: v3Update(...)
  400  Bad Request: domain.ErrInvalid (via Service.Update → checkTitle); fuego.BadRequestError (via parseID)
  404  Not Found: domain.ErrNotFound (via Service.Update → Store.modify)
  500  Internal Server Error: unclassified error (via (Context).Body [stub]); domain.ErrConflict (via Service.Update → Store.checkTitle); an unclassified error becomes 500 unless it carries its own status
  default  Unknown status: fuego.HTTPError (via (Context).Body [stub])
```

`-errset.statuses`, before: `… | 500 domain.ErrConflict | default fuego.HTTPError | default ?`; after: `… | 500 domain.ErrConflict | 500 ? | default fuego.HTTPError`.

**Drift.** `-check` against Fuego's document: the `default` lines of `POST /v3/todos` and `PUT /v3/todos/{id}` now name only `fuego.HTTPError`. Before they named `?` too. The counts are unchanged (2 missing, 5 stale). The 500 of `?` is not reported as missing, because Fuego's document already has 500 on every operation. Against the regenerated `effectgo-merged.json`, `-check` reports `ok` on all five v3 operations, and the 500 response of PUT lists `?`.

**Tests.**

- `errset/testdata/src/status`: `HUnclassified` is now 500 (it was default).
- `openapi/testdata/src/opaque` with `openapi/opaque_test.go`: `/opaque` (only `?`, 500, complete), `/dynamic` (only a dynamic status, default), `/conflict-and-opaque` (the coincidence case below), `/mixed` (conflict 409, `?` 500 and a dynamic status as default).
- The coincidence case: the errmap maps `ErrConflict` to 409, and the handler can also return `?`. The docs must still have 500 attributed to `?` and 409 attributed to `ErrConflict`, with no default. `TestConflictAndOpaqueDocs` checks both, and the 500 description.
- `examples/todo/api/openapi_crosscheck_test.go`: each case asserts the variant that its status must list (see "Runtime cross-check").

### Deviations

1. Fuego documents 400, 404 and 500 on each operation, not 404 only. The check reports that as it is.
2. The `?` is 500, not `default`: an unclassified error is sent as 500 (see the fix below). The `default` responses are only the dynamic statuses.
3. Uncovered errors that implement `StatusCode` keep their own status (see "Status inference").
4. The Delete routes return the named type `deleted` (an example change, needed for validation; the JSON body is unchanged).
5. The v1 and v2 routes are undocumented, not documented: no directives were added (see above).
6. `errset.Result` changed from a map to a struct, and the signature analyzer follows.

### Limitations

- Sentinels are assumed plain (their values are not inspected); a sentinel of an error type with a status is reported as 500.
- A `fuego.HTTPError` literal without a `Status` field is `default` (its `StatusCode` would give 500, but the analysis does not infer it).
- An opaque variant (`?`) is 500 even if its dynamic type could carry a status: the type is not visible, so the analysis cannot show that it does. Fuego would send a status-carrying error with that status.
- Errmap rules are read from the top level of the errmap body, and assignments before them are not tracked (flow-insensitive, as elsewhere). A rule that returns in more than one way with different statuses is unknown.
- A func literal handler is never documented (it has no directive).
- Routes are found only at static calls to the route functions. Route registration through a table, a wrapper or a variable is not seen (no guess, and no report either).
- `-merge` replaces all 4xx, 5xx and `default` responses of each matched operation. Success responses, schemas and parameters are kept.
- `-check` compares codes only: not the response schemas, the descriptions, or the success codes.
- Not run: `go install` from the private repository, and CI.

