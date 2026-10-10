---
title: HTTP checks
description: "A showcase of what error-set inference enables at the HTTP boundary: OpenAPI error documentation for Fuego, and checks of Huma and Gin handlers against the errors they can return."
---

This page is a showcase, not the main line of effectgo. The main line is the error set of each function, which the [contract mode](contract-mode.md) uses. An HTTP handler is one place where the error set has a visible consequence: an error that the handler returns is turned into a response, and the response status depends on the error's type. If the set is known, the mapping can be checked, and the response documentation can be derived.

Three commands show this, for three frameworks:

| Command | Framework | Question it answers |
|---|---|---|
| `effectgo openapi` | Fuego | which error responses does each route have, and does the OpenAPI document say so? |
| `effectgo http-huma` | Huma v2 | which errors of each route reach Huma's fallback HTTP 500? |
| `effectgo http-gin` | Gin | which error variants reach a 5xx response, and does a response body leak error text? |

`effectgo http` runs the Huma and Gin checks over one load, and prints one list. The flags are in [Command line](../reference/cli.md).

What each check does not do is as important as what it does: no runtime behaviour is observed, no request is sent, and no service is started. The findings are static. The evaluation numbers below come from static runs on real services and from manual reading of samples; they are not a claim about production behaviour.

## Fuego and OpenAPI

Fuego documents an operation's errors only when the code says so. `effectgo openapi` derives them from the handler's error set and its errmap, with a witness for each response, so the document can be checked against the code.

The handler and the errmap are declared as in [Contract mode](contract-mode.md#example-2-errors-and-the-http-boundary). The command prints the response fragment for the routes it can resolve; here is the `PUT /v3/todos/{id}` operation of `examples/todo`, shortened from the output:

```json
{
  "responses": {
    "400": {
      "description": "Bad Request: domain.ErrInvalid (via Service.Update → checkTitle); fuego.BadRequestError (via (Context).Body [stub])",
      "x-effectgo-errors": [
        { "variant": "domain.ErrInvalid", "via": "Service.Update → checkTitle" },
        { "variant": "fuego.BadRequestError", "via": "(Context).Body [stub]" }
      ]
    },
    "404": {
      "description": "Not Found: domain.ErrNotFound (via Service.Update → Store.modify)",
      "x-effectgo-errors": [
        { "variant": "domain.ErrNotFound", "via": "Service.Update → Store.modify" }
      ]
    },
    "500": {
      "description": "Internal Server Error: unclassified error (via (Context).Body [stub]); domain.ErrConflict (via Service.Update → Store.checkTitle); an unclassified error becomes 500 unless it carries its own status",
      "x-effectgo-errors": [
        { "variant": "?", "via": "(Context).Body [stub]" },
        { "variant": "domain.ErrConflict", "via": "Service.Update → Store.checkTitle" }
      ]
    },
    "default": {
      "description": "Unknown status: fuego.HTTPError (via (Context).Body [stub])",
      "x-effectgo-errors": [
        { "variant": "fuego.HTTPError", "via": "(Context).Body [stub]" }
      ]
    }
  },
  "x-effectgo-handler": "v3Update(...)"
}
```

Reading it:

- `400`: `ErrInvalid` is mapped to 400 by the errmap, and `BadRequestError` carries 400 itself.
- `404`: `Store.modify` returns `ErrNotFound`, which the errmap maps to 404.
- `500`: `ErrConflict` is not mapped, so it is 500; so is the unclassified error from the body decoder. A plain error reaches the client as 500 because the server's error handler does that (see [Error sets](../reference/errors.md#http-status)).
- `default`: `fuego.HTTPError` carries a status that is only known at run time, so it is listed under `default` and no code is guessed.
- Undocumented routes (v1 and v2 of the example) are printed on stderr with the reason. v1 has no errmap directive; v2 wraps its handlers in helpers that the analysis does not see through.

The check compares a document with the code:

```sh
$ effectgo openapi -stubs=stubs/stdlib.stubs,stubs/fuego.stubs -check testdata/openapi/fuego.json ./...
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
POST /v1/todos: unchecked: no errmap: the handler has no //effectgo:handler directive (…/api/v1.go:29:2)
…
summary: 2 missing, 5 stale, 0 not in document, 10 unchecked
```

`missing` codes (the code can produce them, the document does not) make the command exit 1. `stale` codes are documented but no variant produces them; they are reported only for operations whose statuses are all known. Fuego's generated document documents 400, 404 and 500 on every operation, so its generic 500s appear as stale.

`-merge` writes the document with the error responses of the known routes replaced, and keeps everything else:

```text
effectgo openapi -stubs=… -merge testdata/openapi/fuego.json ./... > merged.json
```

Scope of the check: it compares status codes, not schemas, descriptions or success responses. It does not find routes that are registered through a table, a wrapper or a variable. Those are reported as unchecked, not guessed.

## Huma

`effectgo http-huma` checks the routes of a Huma v2 service. It finds each route call (`huma.Get`, `Post`, `Put`, `Patch`, `Delete`, `Register`), resolves the handler's error set, maps it through Huma's default error handling, and reports each variant that reaches the fallback 500. No directive is needed: the handlers are literals and Huma's rules are stubs.

Huma's rule, from its source: an error that implements `huma.StatusError`, or wraps one with `%w`, keeps its status; any other error becomes 500. The stub file states it as:

```text
errdefault github.com/danielgtaylor/huma/v2 passthrough=iface:github.com/danielgtaylor/huma/v2.StatusError,fallback=500
```

Findings are graded by severity. `info` is hidden unless `-all`:

| Severity | A variant that reaches 500 |
|---|---|
| `high` | a named variant of the project (a sentinel or a typed error) that another place in the project maps to a non-5xx status: a mapper, or a guard in a handler such as `if errors.Is(err, ErrNotFound) { return huma.Error404NotFound(…) }` |
| `medium` | a named project variant that nothing maps |
| `info` | `?` (opaque), `new@` (fresh), and foreign named errors (`sql.ErrNoRows`, `context.Canceled`) |

A route whose method or path is not a constant, or whose handler is not a literal or a resolvable function, is reported as unresolved with the reason. Nothing is guessed.

Output: text by default, grouped by severity; `-json` prints one object per finding, and the coverage summary goes to stderr. The integration fixture used by the tests shows a route that carries a Huma 404 through two local variables and writes a Gin response with a constant 500:

```sh
$ effectgo http-gin -all ./app          # the Gin check on the same fixture
app/app.go:25: medium: huma.Error404NotFound -> 500 (GET /through) in ThroughLocal (app/app.go:21)
    via carry; inline `err != nil` at app/app.go:24
    huma.Error404NotFound reaches a 500 response and no other mapping in the project covers it (foreign variant)
```

The Huma check on the same fixture reports nothing, because the Huma route answers the carried 404 with its own status.

Measured on a real Huma service of 134 routes (static evidence only; the figures are from the journal, not rerun here): all 134 routes resolve and 543 variants all have a known status. With the first (flow-insensitive) engine, 355 reached the fallback 500, with 65 high, 1 medium and 289 info findings. With the default flow-sensitive engine the same snapshot gives 23 high, 0 medium and 251 info. Most of the removed high findings were guards that the earlier engine did not model; see [Known false positives](#known-false-positives) and the [Huma journal](../journal/07-p2-huma-adapter.md).

## Gin

A Gin handler returns nothing: it writes the response with `c.JSON`, `c.AbortWithStatusJSON`, or a project helper. The boundary is therefore a call, not a return value. `effectgo http-gin` finds the response calls (sinks), reads their constant status, and reports each error variant that reaches a 5xx response. It also reports error text written into a response body.

A sink is declared by a stub line. The Gin writers are in `stubs/gin.stubs`; a project helper goes into the project's `effectgo.stubs`:

```text
sink github.com/example/app/internal/httpapi.Detail status=1 body=2
```

Mappers are found without annotations: a function that takes a `*gin.Context` and an error, and answers each error variant with a constant status, is a sink mapper. A function `func(error) (int, bool)` whose branches return a constant status is a status mapper. `-mappers` lists the ones it found on stderr.

Severities:

| Severity | Meaning |
|---|---|
| `high` | a named variant reaches a 5xx response, and the same variant is mapped to a non-5xx status elsewhere in the project: an inconsistent mapping |
| `medium` | a named variant reaches a 5xx response, and no mapping covers it |
| `info` | `?` or `new@` reaches a 5xx response; hidden unless `-all` |
| `leak` | a response body carries `err.Error()` or an error formatted into a string |

The leak check is the one most likely to matter in review: `c.JSON(500, gin.H{"message": err.Error()})` sends a database or file-system message to the client. The check reads the body argument and up to two levels of composite literal.

Measured on three real Gin services (static evidence; from the journal). order-mgmt had 396 handlers and 9 detected sink mappers. With the first engine it reported 83 high, 136 medium, 300 info and 67 leak findings; with the default flow-sensitive engine, 48 high, 78 medium and 274 info. In the manual sample of 15 leak findings across the three services, 9 were likely bugs and 6 were intended; the reading is in the [Gin journal](../journal/08-p2-gin-adapter.md).

## Both adapters

`effectgo http` runs the Huma and Gin checks over one package load and prints one list, sorted by severity (`high`, `medium`, `leak`, `info`), then by position. Its JSON is the shared schema:

```text
{"severity","framework","route","handler","pos","variant","status","via","reason"}
```

`framework` is `gin` or `huma`. The output is the union of `http-gin -json -all` and `http-huma -json -all`, and each subcommand still works on its own.

## net/http handlers

The checks above read a handler's error set. A `net/http` handler, `func(http.ResponseWriter, *http.Request)`, has no error result, so it has no error set: a `//effectgo:handler` directive on it is reported, because it checks nothing. The check needs an inner function that returns an error, and the handler is a thin adapter around it. The dogfood reservation service uses this shape, and it is the recommended pattern:

```go
// ToHTTPError maps the domain errors that the handlers can return.
//
//effectgo:errmap
func ToHTTPError(err error) error { ... }

// doGet returns the reservation, or the domain error. The directive checks
// that every error it can return is mapped by ToHTTPError.
//
//effectgo:handler ToHTTPError
func (a API) doGet(r *http.Request) (reservationJSON, error) { ... }

// get is the net/http handler. It has no error result, so it carries no
// directive; the adapter writes the value, or the mapped error.
func (a API) get(w http.ResponseWriter, r *http.Request) {
	out, err := a.doGet(r)
	writeResult(w, http.StatusOK, out, err)
}
```

Checked on a small module with this shape: `doGet` with a sentinel the errmap maps reports nothing; a method whose sentinel the errmap does not map is reported as `handler ... may return ht.ErrOther ..., which errmap httpx.ToHTTPError does not map → HTTP 500`. The method form works with a result of `error` alone as well as with `(T, error)`. `effectgo -errset.statuses` prints the status of each variant of `doGet`, so the mapping can be read without running the service.

Three things to remember:

- Write the adapter with the same errmap as the directive. The directive checks one errmap; an adapter that maps with another one is not checked.
- A directive on a `func(w, r)` handler is reported as checking nothing. Move it to the inner function that returns the error.
- The status of a variant in `-errset.statuses` is what the errmap gives; the status the adapter writes is the adapter's own. Keep them the same.

## Known false positives

The HTTP checks are static, and their known sources of false positives are these. The numbers are from the evaluation recorded in the journals.

- **Guards the analysis did not model.** A handler that tests a variant with `errors.Is` and returns a status before a raw `return err` reaches the same return. The flow-sensitive engine (the default) follows such guards; the earlier engine did not. Most of the high findings of the Huma run were of this kind before the change.
- **Row scans that cannot return `sql.ErrNoRows`.** `pgx.stubs` lists `pgx.ErrNoRows` for every row scan. Aggregates (`count(*)`), `SELECT EXISTS`, `FROM`-less selects and single-row `VALUES` joins always return a row. On order-mgmt, 63 of the 83 first-engine high findings were `pgx.ErrNoRows`, and the manual sample attributed seven of its fifteen high rows to this shape. The fix belongs in the stub, and it needs the query text: a rule per query, which a stub cannot express.
- **A read-back of a row the same request just wrote.** The variant is possible in the code and infeasible in practice.
- **A variable reused for two calls.** `err` is assigned by two calls, and the set of the first reaches a return that is for the second. The flow engine reads the variable at each use.

Some findings are real. On the Huma service, the sample found an unguarded `ErrNotFound` read after a mutation whose outcome the handler reported as 500 where a 404 was due, and a validation error that the handler returned raw while the project maps validation to 400 elsewhere. The reading that found them is in the [Huma journal](../journal/07-p2-huma-adapter.md).

## What the checks are for

A check like this reports where the code's answer and the project's own mapping disagree. Whether the disagreement is a bug, an intended 500, or a false positive is a judgement, and the severities are a triage aid: `high` means "the project maps this variant elsewhere", not "this is a bug".
