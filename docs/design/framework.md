---
title: Framework (historical proposal)
description: "The historical framework proposal and its implementation milestones: typed HTTP handlers, schemas, analyzer checks and generated Effect TS clients."
---

Historical proposal opened **2026-10-15**, with implementation and acceptance notes added below. The initial proposal described work that was not yet implemented; that opening premise does not describe the current checkout. The first user was the Go port of novastar-marketing (`/root/pp/novastar/novastar-marketing/go`). For the current shared report command and its limits, read [Contract checks](../guide/contract-checks.md); for candidate validation and consumer source/runtime upgrades, read [Releasing](../guide/releasing.md).

## Why

effectgo today is an analyzer. It is used next to plain Go code: the novastar port wrote its own handler plumbing, its own JSON error shapes and `//effectgo:handler` / `//effectgo:errmap` directives by hand, and the analyzer checked them afterwards. That proves the analyzer reads ordinary code; it does not give a developer the Rust-like experience the project started from (see `../../借鉴Rust思维的Go框架开发构想.md`): a handler that states what it takes and what it can fail with, and a compiler that refuses the program when a failure is not handled.

The missing pieces are a runtime that makes the boundary typed, and a bridge that carries the types and the error sets to the TypeScript side, where the frontend and `pi-agent.ts` use Effect TS v4.

## Decisions

### The analyzer stays the compiler; no `Result[T]`

[Inference, not a monad](decisions.md#inference-not-a-monad) stands. Application code keeps `(T, error)`. The guarantees a Rust developer expects come from the analyzer, not from wrapper types:

| Rust | effectgo |
|---|---|
| `Result<T, E>` with a known `E` | the inferred error set of the function (errset) |
| `#[must_use]`, unused `Result` warning | mustuse |
| exhaustive `match` on an enum | exhaustive on `//effectgo:sealed` |
| `IntoResponse` must be implemented for the error | every variant of a handler's set must carry a status (new check, below) |
| `serde` derive with a typo in an attribute | schema tags checked at vet time (new check, below) |

The `Result[T]` prototype on Fuego (`proto-b-fuego-result`) is the evidence that the wrapper adds glue without adding a guarantee the analyzer cannot give.

### Go is the single source of truth; TypeScript is generated

Request, response and error types are declared once, in Go. `effectgo ts` generates an Effect TS v4 `HttpApi` definition from them: `Schema` values for payloads and responses (with the validation rules), and one tagged error class per error variant that a handler's inferred set contains, annotated with its HTTP status. `HttpApiClient.make` then gives the TypeScript caller `Effect<Out, ErrA | ErrB | TransportError, …>`, where `TransportError` is `HttpClientError | Schema.SchemaError` (the client's own failures, always present). A new error variant on the Go side is a compile error on the TypeScript side until it is handled or explicitly passed on. M0 showed this with `tsc`: an unhandled `ContactLocked` makes `Effect<…, ContactLocked | HttpClientError | SchemaError>` unassignable to `Effect<…, TransportError>`.

This is what "shared schema" means here: one definition, two runtimes, the same validation. The alternative, a schema language that both sides import, would make Go code depend on a description that is not Go; it is rejected.

### Layers stay in Go, and come later

A shared dependency model across the two languages (Effect `Layer` and `Context.Service` on both sides, with cross-process requirements) is not part of this proposal. The Go runtime starts with explicit constructor wiring in `main` and a `Scope` for closing resources. A Go `Layer` that follows Effect's naming is a later step, decided after the novastar migration shows what wiring hurts.

### A separate module, standard library only

The runtime lives in `github.com/effectgo/effectgo/rt`, its own module in this repository, so that importing it does not pull `golang.org/x/tools` into an application's `go.sum`. It depends on the standard library only. Add it to an application with `go get github.com/effectgo/effectgo/rt@v0.4.0`.

## The runtime

### `rt/schema`: types with checked tags

A request or response type is a plain Go struct. Field names come from `json` tags; validation comes from a small, closed vocabulary in a `schema` tag:

```go
type UpdateContact struct {
	ID        int64   `json:"id"        schema:"min=1"`
	Name      string  `json:"name"      schema:"trim,nonempty,max=200,singleline"`
	Email     string  `json:"email"     schema:"email"`
	Segment   Segment `json:"segment"`          // //effectgo:sealed string enum → Schema.Literals
	Notes     *string `json:"notes"     schema:"max=2000"` // pointer = optional
	Suppressed bool   `json:"suppressed"`
}
```

- Decoding is strict: unknown fields, missing required fields and wrong types are rejected, every issue is collected (not only the first), and the result is a `*schema.DecodeError` with a path per issue.
- A pointer field is optional; every other field is required. There is no implicit zero value for a missing field.
- An enum is a `//effectgo:sealed` string or integer type with a `Valid() bool` method written as a `switch` over its constants with no `default`. The decoder calls `Valid`; the exhaustive analyzer already reports a `switch` that misses a constant, so the runtime check cannot fall behind the type. The generator reads the constants statically and emits `Schema.Literals`.
- The vocabulary is fixed (`min`, `max`, `nonempty`, `trim`, `singleline`, `email`, `pattern=<name>` with named patterns registered in Go). Each entry has one Go check and one Effect Schema check, so the two sides cannot drift.
- The decoder uses reflection, computed once per type when a route is registered. Compile speed is unaffected; a malformed tag is a vet error before it is a startup panic.

### `rt/web`: typed handlers

```go
type Contact struct{ /* … */ }

// UpdateContact applies the edit and returns the stored contact.
func (s *Service) UpdateContact(ctx context.Context, in UpdateContact) (Contact, error)

func Routes(r *web.Router, s *Service) {
	web.Post(r, "/api/contact", s.UpdateContact)
	web.Get(r, "/api/contacts/{id}", s.GetContact) // path and query fields: `path:"id"`, `query:"page"`
}
```

- A handler is any `func(context.Context, In) (Out, error)`. `web.Get`, `web.Post`, … are generic over `In` and `Out`; the router decodes `In` from path, query, headers (`header:"X-Name"`), cookies (`cookie:"name"`) and JSON body with `rt/schema`, calls the handler, and encodes `Out`.
- Header and cookie fields keep their Go `json` name for schema decoding. Header tags read one HTTP header; cookie tags read one named cookie from the `Cookie` header. Missing values remain absent so required fields produce `*schema.DecodeError`. Cookie fields are server inputs, not generated client headers: browsers supply matching cookies from the origin cookie jar.
- A handler is a named function or a method value (`s.UpdateContact`), not a closure. The endpoint's error set is that function's inferred set; M0 showed that a closure inside a constructor, with errors passed to a shared `writeError(err error)`, leaves no set for any route. The analyzer reports a closure registered through `rt/web` (M2).
- Decoding failures belong to the framework, not the handler: a malformed path, query, header, cookie or body is `*schema.DecodeError`, status 400, tag `DecodeError`. It is part of every endpoint that has an input, and the generator adds it to every such endpoint's error list.
- `schema.Issue.Source` and `Value` are non-wire metadata (`json:"-"`) for custom error encoders. HTTP decoding records the declared path/query/header/cookie/body source and rejected value during the original decode, so a same-named query cannot replace body evidence. Missing/syntax values remain absent; rejected JSON fragments retain ownership and numeric precision. Default `DecodeError` JSON is unchanged.
- Errors are encoded by their own type. An error type that implements `StatusCode() int` is sent with that status and its exported JSON fields, plus `"_tag"` set to the Go type name. The status must be a constant returned by the method, which is what errset's `StatusFact` records; a computed status is reported (M2).
- Tags are Go type names, so two packages that both declare `ValidationError` collide. A collision among the variants of one API is a generation error that names both types; the fix is to rename one. There is no tag directive until a real case needs it. Anything else is a 500 with a generic body and a log line carrying the request ID; error text never reaches the client by accident (the real-world evaluation found `err.Error()` sent to clients in 90 places across three services).
- `rt/web` provides the boundary concerns that the novastar port wrote by hand: request ID, structured access log, body size limit, Host check, token guard for unsafe methods, security headers, static files with an SPA fallback. Each is a middleware a service can leave out.
- `WithPanicHandler` (M3c) passes each recovered panic to a function before the router writes its 500, with the request context (the request ID is there). The response is unchanged: a panic after the response has started is still re-raised, and `http.ErrAbortHandler` is not passed. Without it, a service that must count panics wraps its logger and reads the "handler panic" record, as novastar did.
- An encoder option keeps an existing wire format during a migration. For novastar it adds `"detail"` next to `"_tag"`, so the current React frontend and the generated client both read the same response.
- Optional cookie and header inputs use pointer fields (`*string`), just like optional JSON fields. Non-pointer fields remain required; missing input produces `DecodeError` before the handler. A nil pointer is absence, not an authentication decision. An empty cookie is present; an empty header is currently treated as absent.
- When migrating from Huma, return the JSON envelope directly: `rt/web` encodes the entire output, including any field named `Body`. Output `header:"Name"` fields write response headers for `string`, `[]string`, `http.Cookie`, and pointers to these types; nil pointers are omitted. Use `json:"-"` to exclude header fields from the JSON body. A custom error encoder owns its Content-Type as well as its status/body; manual OpenAPI responses must match those wire values and retain the existing operation ID. `ExternalError` supplies error shape/status only, not media type or operation metadata. See the [runnable migration example](../../examples/web/README.md#migrating-a-huma-execution-handler).

### HTTP metadata and explicit request modes

`web.SuccessStatus(code)` declares a constant successful status such as 201 or 202. `web.Response[T]` carries a concrete JSON body plus status and headers; `SuccessStatuses(...)` or `SuccessStatusRange(first, last)` declares its successful alternatives for the runtime and generated client. Error responses retain their status and follow the endpoint's error contract. Bodyless 204/205 responses have native empty-body codecs.

`web.FileResponse` transfers ownership of an `io.ReadCloser` to the router, which closes it on completion or cancellation. Seekable 200 responses retain `net/http` range and conditional GET behavior, including 206/304. Explicit response headers preserve inline disposition and provider content types; `Filename` supplies an attachment disposition when no explicit one exists. `web.StreamResponse` copies a reader while its provider callback is active, leaving reader cleanup with that provider. Generated clients decode binary responses as `Uint8Array`; use native `responseMode: 'decoded-and-response'` when the caller needs content type or disposition headers. `web.RedirectResponse` uses the declared redirect status, defaulting to 302.

Default JSON input retains `rt/schema` validation. `web.JSONBody()` instead preserves `encoding/json` omitted fields, null, unknown keys and custom unmarshaling; URL-owned fields are decoded independently and take precedence. `JSONBodyExact()` requires exactly one JSON value, `JSONBodyEmpty()` also accepts an empty body, and `JSONBodyBestEffort()` preserves a handler that deliberately ignores body decode failures. Declare these modes only where the existing endpoint requires them. DELETE can carry a declared body; header/cookie-only inputs do not create a JSON body.


### Multipart file inputs

File uploads use `form` tags on `web.File` fields, excluded from JSON decoding:

```go
type ImportInput struct {
	File web.File `form:"file" json:"-"`
	Attachment *web.File `form:"attachment" json:"-"`
}
```

`web.File` contains `Filename`, `ContentType`, and `Data []byte`. A value field is required; a pointer is optional. Multiple named files and path/query/header/cookie fields may coexist. Scalar and slice form fields require a `form` tag and the same JSON name; file fields use `json:"-"`. Embedded file fields, schema rules on files, and mixing multipart fields with separate JSON body fields are rejected. The handler remains responsible for extension, media-type and document-content validation; uploaded metadata is untrusted.

The entire upload, including multipart overhead and trailing bytes, is bounded by `WithMaxBody` (default 1 MiB). Files are buffered in memory without temporary files. Oversized requests return `BodyTooLarge` (413); malformed bodies, missing required files, duplicate parts and unknown parts return `DecodeError` (400) before the handler runs. This is a bounded upload API; streaming large files is outside its scope.
`web.FormBody()` preserves legacy multipart extraction where unknown parts are discarded and the first declared scalar/file part wins; request-size limits still apply.

`effectgo ts` emits file schemas with `HttpApiSchema.asMultipart()` for Effect `4.0.4`. The generated client's `payload` is a `FormData`: append a `File` or `Blob` with its filename and leave `Content-Type` unset so fetch supplies the boundary. The runtime and generator regressions exercise binary bytes and filenames over HTTP.


## What the analyzer adds

1. **Route discovery for `rt/web`.** `route` stub rules for `web.Get`, `web.Post`, … (`stubs/rt.stubs`). A function registered through them is a handler without `//effectgo:handler`.
2. **Status coverage.** For each handler, every variant of its inferred error set must have a constant status (errset's `StatusFact`). A variant without one is reported with its witness chain, as an unmapped errmap variant is today: `handler UpdateContact may return store.ErrLocked (via …), which has no StatusCode → HTTP 500`. A `?` in the set is reported only in contract mode, keeping [silence when unproved](decisions.md#silence-when-unproved). Implemented (M2b-1) as: contract mode is a handler that carries `//effectgo:errors`; without it the check is silent for `?`, unresolved parameters and interface variants. A closure registered as a route is reported; closures in `_test.go` files are not. A dynamic `StatusCode` is reported for routes but not for a passthrough errmap. Known gap: a sentinel declared as `error` whose initializer is a status-carrying value (`var ErrX error = NotFound{}`) is reported, because errset does not evaluate initializers.
3. **Schema tags.** A new analyzer checks every `schema` tag against the vocabulary and the field type (`email` on a non-string, an optional rule on a non-pointer, an unknown named pattern). Implemented (M2b-2) as `schematag`: it checks only the types that flow into `rt/web` route functions and `schema.Decode`/`Compile`, so a gorilla-style `schema` tag is not a finding; the path and query tags of the input are checked against the constant pattern, as `rt/web` checks them at registration. Enum types must implement `schema.Enum`, whose `Valid` is `//effectgo:pure`. A pattern name is checked for its format, not against the registry: a missing `RegisterPattern` is a panic at startup, and a vet pass cannot prove that no package registers the name. The check that a cross-package `Valid` is pure is not made where the type is not in a package that imports `rt/schema` (see [analyzers](../reference/analyzers.md#schematag)).

## The generator: `effectgo ts`

`effectgo ts -o src/lib/api.gen.ts ./...` loads the packages like `openapi` does, and for each discovered route writes one `HttpApiEndpoint`:

```ts
// Code generated by effectgo ts. DO NOT EDIT.
import { HttpApi, HttpApiEndpoint, HttpApiGroup } from "effect/http-api"
import { Schema } from "effect"

export class ContactNotFound extends Schema.TaggedError<ContactNotFound>()(
  "ContactNotFound", { detail: Schema.String }, { httpApiStatus: 404 }
) {}

export const UpdateContact = Schema.Struct({
  id: Schema.Int.check(Schema.isGreaterThanOrEqualTo(1)),
  email: Schema.String.check(Schema.isPattern(EMAIL)),
  segment: Schema.Literals(["hot", "nurture", "support_only", "at_risk", "invalid"]),
  notes: Schema.optionalKey(Schema.String.check(Schema.isMaxLength(2000))),
  // …
})

export const Api = HttpApi.make("care").add(
  HttpApiGroup.make("contacts").add(
    HttpApiEndpoint.post("updateContact", "/api/contact", {
      payload: UpdateContact,
      success: Contact,
      error: [ContactNotFound, ValidationError],
    }),
  ),
)
```

- The error list combines inferred handler errors, runtime decode/body/internal errors, and explicit `web.ExternalError[E]()` route options for middleware or custom encoders. External error bodies retain their Go JSON fields without requiring `_tag`; generated `Schema.Error` instances add `_tag` after decoding. Declaring an inferred handler error as external explicitly selects its untagged wire format. Use distinct literal code enums to discriminate envelopes sharing one HTTP status. Middleware execution and custom encoders still require HTTP integration checks: declarations do not prove their behavior. Options must be direct generic calls on the route, not opaque helper values.
- Path parameters are translated from Go's `{id}` to Effect's `:id`.
- The file imports `effect/http-api` even where it uses only `Schema`: the `httpApiStatus` annotation type-checks through a module augmentation in `HttpApiSchema.d.ts`, which applies only when that module is in the compilation.
- It exports `type TransportError = HttpClientError | Schema.SchemaError`, so an exhaustive handler can state what it leaves unhandled.
- The client needs an `HttpClient` layer: `NodeHttpClient.layerUndici` from `@effect/platform-node` on Node, or the generated `BrowserFetchHttpClient` layer for APIs containing cookie inputs. That layer configures Effect TS `4.0.4` `FetchHttpClient.RequestInit` with `credentials: "include"`; JavaScript cannot set a `Cookie` request header. Other browser or Node transport choices remain caller-controlled.
- Output is deterministic (sorted, stable names), so it can be committed and diffed in review.
- API names were checked against `effect@4.0.4` as installed in novastar: `Schema.TaggedError` (not `TaggedErrorClass`) with the status in its annotations argument, as `HttpApiError.BadRequest` does; `Schema.optionalKey`, `Schema.Literals`, `Schema.isPattern`, `Schema.isMaxLength`, `Schema.isGreaterThanOrEqualTo`.
- The target is exactly `effect@4.0.4`. `effect/http-api` is marked `@stability unstable` in 4.x; compilation of freshly generated mapping fixtures and the external-error HTTP decoder regression use that installed version.
- `TestBodyModeSchemas` checks native Effect payload encoders with the dependencies installed in `examples/web/ts`. Multipart optional fields accept omission but reject null; arrays require arrays of the declared element type, and file fields require `File` values. JSONBody accepts null, omitted fields and unknown object keys; only its empty-body modes also accept an omitted body. Strict JSON and DELETE payloads keep required fields and validation while optional pointer scalars accept omission or null. `TestGeneratedJSONRequestBoundary` checks freshly generated schemas before JSON serialization and sends their client requests over HTTP. `TestGoldenCompiles` generates the mapping fixtures on each run and compiles that output; no checked-in output snapshot or update flag is used.

### Mapping rules (decided for M3)

The one rule behind the others: **the TypeScript schema is never stricter than the Go decoder.** A client that rejects what the server accepts is a bug; a client that lets through what the server will reject only costs a round trip that ends in a typed `DecodeError`. Where a rule cannot be mirrored exactly, the generator emits the looser check and a comment saying what the server checks.

| Go | Payload, query, path (TS encodes, Go decodes) | Success, error fields (Go encodes with `encoding/json`, TS decodes) |
|---|---|---|
| `string`, ints, floats, `bool` | `Schema.String`, `Schema.Int` / `Schema.Number`, `Schema.Boolean`; 64-bit integer kinds use `Schema.Number` checked with `Number.isInteger` | same: accept the existing JSON numeric wire beyond 2^53 rather than rejecting it with Effect's safe-integer `Schema.Int`; JavaScript precision loss remains explicitly documented, not repaired by a representation change |
| `*T` (optional) | `Schema.optionalKey(Schema.NullOr(T))`: Go accepts a missing key and `null` | `Schema.NullOr(T)`; with `omitempty`, `Schema.optionalKey(…)` |
| `[]T`, `map[string]T` | `Schema.Array(T)`, `Schema.Record(Schema.String, T)` | wrapped in `Schema.NullOr`: a nil slice or map encodes as `null` |
| `time.Time` | `Schema.String` (RFC 3339; not parsed client side) | `Schema.String` |
| `json.RawMessage`, `any` | `Schema.Unknown` | `Schema.Unknown` |
| enum (implements `schema.Enum`) | `Schema.Literals([...])`, the constants of the type in its package | same |
| nested struct | a named `Schema.Struct`, emitted once | named structs are emitted once; anonymous structs are inline `Schema.Struct` values, including slice/map elements and pointer fields |

JSON payload schemas apply `jsonRequest(...)` at the generated request boundary. Before the original schema encodes and validates the payload, it removes own enumerable keys whose value is `undefined` from ordinary objects (including null-prototype objects), recursively through nested objects, maps and array elements. It copies only changed containers, leaves the caller's input untouched, and preserves `null`, `0`, `false` and `""`. Arrays keep their length, holes and `undefined` elements; the original schema still rejects an invalid element before HTTP. Effect's HTTP endpoint converts even `Schema.Unknown` payload fields to its JSON codec: an array's `undefined` element is rejected there, rather than silently converted to `null` by `JSON.stringify`. Dates, files, blobs, typed arrays, `FormData` and class instances keep their identity during normalization and are not traversed; they remain subject to the original JSON codec's acceptance rules. This is a schema encoding transform, so the generated client keeps its inferred input types and rejects missing required fields or invalid values before constructing the HTTP body. The transform applies to strict JSON and JSONBody modes; multipart, path and query schemas retain their existing types and validation.

Rules in `schema` tags, payload direction only:

| Rule | TypeScript | Note |
|---|---|---|
| `min=N`, `max=N` on numbers | `isGreaterThanOrEqualTo`, `isLessThanOrEqualTo` | exact |
| `min=N`, `max=N` on slices | `isMinLength`, `isMaxLength` | exact |
| `min=N`, `max=N` on strings | a comment | Go counts runes, JavaScript counts UTF-16 units, so any length check could reject what Go accepts |
| `nonempty` | on slices `isMinLength(1)`; on strings a comment | a string's emptiness depends on `trim` and on rune counting |
| `trim` | nothing | the server trims; the client sends what it has |
| `singleline` | `isPattern(/^[^\r\n]*$/)` | exact |
| `email` | `isPattern` with the same expression only if rt's check is that regular expression; otherwise a comment | |
| `pattern=NAME` | a comment | the expression is registered at run time and is not known to the generator |

Names and structure:

- A struct, enum or error is named by its Go type name. Two types with one name in one API are a generation error that names both, as for tags.
- Output and error shapes follow `encoding/json` field selection: untagged embedded structs promote exported fields (including through unexported embedded struct types); an explicit valid JSON name keeps an embedded field nested. Ordinary unexported fields and `json:"-"` are ignored. Invalid JSON tag names fall back to the Go name or embedded promotion. Shallower fields dominate deeper fields; at the same depth a single explicitly named field dominates untagged fields. Equally dominant candidates, including repeated embedding paths, stop analysis and generation with an ambiguity diagnostic, rather than generating a schema for omitted data. Ignored and dominated fields do not contribute schemas or unsupported-type diagnostics.
- A nil embedded pointer can omit each promoted key. That optional key is independent of the promoted field's own pointer nullability and `omitempty` behavior. Output `omitzero` also makes a key optional; `omitempty` on a struct does not omit it. Scalar `json:",string"` output fields use the JSON string wire type. Output `schema` tags do not validate responses. Custom JSON/text encoders retain the existing opaque/string contract, including promoted encoding methods.
- These output rules do not relax schema-mode input: embedded fields remain unsupported, duplicate JSON names and invalid schema rules still fail, and anonymous structs still require names in strict generated input schemas. Recursive schema dependencies retain the existing generation error.
- Output field selection aggregates repeated embedded types at their nearest depth and retains multiplicity for ambiguity checks. Repeated diamond embeddings do not expand every path; declaration order and nil-ancestor omission remain part of the selected field contract.
- An endpoint is named by its handler in lower camel case (`updateContact`). Endpoints are grouped by the handler's receiver type, or by its package for a plain function, in lower camel case (`client.store.updateContact`). The API is named by `-name`, defaulting to the last element of the module path.
- Error list of an endpoint, in this order: the handler's variants that have a constant status (sorted by status, then name); `DecodeError` when the endpoint has an input; `BodyTooLarge` when it reads a body; `InternalError`, always, since any handler can answer 500. Variants without a status (`?`, sentinels) are not listed: they arrive as `InternalError`, which is what the route check in errset reports.
- Middleware errors (`HostNotAllowed`, `TokenMismatch`) are deployment choices, not part of an endpoint, and are not listed.

Decided after the M3a plan (2026-10-15):

- `TransportError` is written `HttpClientError.HttpClientError | Schema.SchemaError` with `import type { HttpClientError } from "effect/http"`: in 4.0.4 `effect/http` exports `HttpClientError` as a namespace. M0's `exhaustive.ts` compiles with this form.
- `BodyTooLarge` is listed by method, as `rt/web` reads the body: POST, PUT and PATCH, whether or not `In` has body fields.
- Integer mapping follows the wire type's width: `int`, `uint`, `int64` and `uint64` use `Schema.Number.check(Schema.makeFilter(globalThis.Number.isInteger, { expected: "an integer" }))`; smaller integer kinds use `Schema.Int`. Effect 4.0.4's `Schema.Int` requires a safe integer, so using it for a 64-bit wire value would reject numbers the Go decoder accepts. The generated comment is `// server: 64-bit integer; values beyond 2^53 lose precision in JavaScript`. Fractions remain invalid; this mapping preserves numeric JSON and does not recover precision already lost by JavaScript.
- Path and query fields use the type mapping only. Their `schema` rules become comments, never checks.
- `rt` errors without fields (`BodyTooLarge`, `InternalError`) are classes with `{}` as their fields.
- A plain function's group is its package name (`types.Package.Name()`), in lower camel case.
- The error order differs from the M0 sample on purpose (status, then name; then `DecodeError`, `BodyTooLarge`, `InternalError`).
- A recursive type (`Node{Next *Node}`) is a generation error for now (`recursive type X is not supported`); `Schema.suspend` can express it when a real API needs one.
- An input with body fields only is referenced by its Go type name as `payload`. An input that mixes path or query fields with body fields gets an inline `Schema.Struct` of its body fields as `payload`, with no named declaration.
- In the success direction, `omitempty` makes keys optional for kinds that `encoding/json` considers empty: a pointer is `optionalKey(T)`, a slice or map `optionalKey(NullOr(…))`, and a scalar `optionalKey(T)`. Struct fields remain required unless `omitzero` or a nil embedded ancestor can omit them.
- `effect@4.0.4` has no `HttpApiEndpoint.delete`; DELETE is `HttpApiEndpoint.make("DELETE")(…)`, checked by `tsc`.
- `errset.Result` gains the constant statuses of `StatusCode` methods, so `tsgen` can export its own status fact; facts do not cross analyzers.
- `int`, `uint`, `int64` and `uint64` all get the 64-bit comment.

Decided after the M3c fixes from novastar (2026-10-15):

- **Custom JSON is not read from the Go fields.** A named type with `MarshalJSON` (on its value or its pointer) is `Schema.Unknown` in the response, with the comment `// server: custom JSON encoding (MarshalJSON); shape not checked`; a named type with `UnmarshalJSON` is `Schema.Unknown` in the request, with `// server: custom JSON decoding (UnmarshalJSON); shape not checked`. Such a type is written inline, so there is no named constant for it. A type with `MarshalText` (and no `MarshalJSON`) is `Schema.String` in the response, with `// server: text encoding (MarshalText); the JSON value is a string`. `MarshalText` is not applied to a request: `rt/schema` decodes by the Go kind and does not call `UnmarshalText`. `time.Time` is handled before these rules. An error type with its own JSON form is a tagged error with `{}` fields and the same comment.
- **Names.** Every error class and schema constant that the output writes is a name, the framework's own errors included (`InternalError`, `BodyTooLarge` and `DecodeError` and the `Issue` they carry, when a route uses them). Two types with one name are a generation error that lists both by package path and position. An enum is written inline as a literal union and has no name, so it cannot collide. The same variant twice in one error list is a generation error; a handler that returns the framework's own error lists it once.
- **Group and endpoint names** lower the leading capitals as Go writes an acronym: `API` is `api`, `HTTPServer` is `httpServer`, `URLMap` is `urlMap`, `Store` is `store`, `ID` is `id`. A digit ends the leading run, so `V2API` is `v2API`. A plural acronym lowers with its `s` when the `s` ends the word or a capital or digit follows it: `IDs` is `ids`, `IDsByName` is `idsByName`, `URLsV2` is `urlsV2`.

### Typed transparent proxy replies

`web.ProxyResponse[Success, Failure]` keeps a provider's two JSON bodies distinct. With `Status == 0`, the router writes `Body` at the route's default success status. A nonzero status writes `Failure` at that exact status, including nondefault 2xx and nonstandard statuses accepted by `net/http`; it is not reclassified as a successful reply. Failure status equal to the route's default success status, invalid HTTP statuses, and alternative-success metadata are rejected before writing. HEAD, informational responses, 204 and 304 keep their normal bodyless HTTP behavior.

The generator uses `Success` as the strict success schema and emits the named `Failure` schema separately. It does not advertise provider failures as successful alternatives or pretend that an open status set is one constant error status. A client that preserves arbitrary provider replies uses Effect 4.0.4's `responseMode: "response-only"`, checks the actual status, and decodes the default-success body with the generated success schema. Ordinary business preconditions remain typed handler errors. Existing `Response[T]`, file, stream and redirect behavior is unchanged.


## Milestones

Each milestone ends with evidence a reviewer can rerun; none starts before the previous one is accepted.

| | Scope | Accepted when |
|---|---|---|
| M0 | Spike in `examples/web`: two endpoints written by hand in Go, the TypeScript the generator should produce written by hand, one TS program calling them | **Accepted 2026-10-15.** `tsc` clean; 404 and 400 arrive as typed `ContactNotFound` / `ValidationError` in `Effect.catchTag`; an unhandled variant fails `tsc`; `effectgo sig` infers `Store.UpdateContact: Effect<Contact, ContactNotFound \| ValidationError, {}>`, but no set for the closure handlers (hence the named-handler rule) |
| M1 | `rt/schema` and `rt/web` | **Accepted 2026-10-15.** Coverage schema 97%, web 94.2%, scope 100% (`-race`); `go list -m all` lists only `rt`; decode of an 8-field contact edit ≈12.8 µs, 54 allocs; 1 MiB body cap by default (`WithMaxBody`); `Decode` panics on a type that does not compile, so a route's set holds only `BodyTooLarge \| schema.DecodeError` |
| M2 | Analyzer, in two steps. M2a (**accepted 2026-10-15**): `reflect.Type` stubs (a reflect panic is noted in the rule's comment, not counted as an effect, as for the rest of `stdlib.stubs`), `rt.stubs` route rules (the existing `route` resolution finds the generic `web.Get` calls; no analyzer change), the passthrough fix (root cause also in `errmapBranches`, which dropped a trailing default), `rt` at 0 findings under its own contracts; `examples/run.sh` output identical to `main`. M2b-1 (**accepted 2026-10-15**): route status coverage in errset (`errset/routes.go`, default run and `go vet`, no directive needed), function-literal registration reported (`_test.go` exempt), `?` reported only when the handler carries `//effectgo:errors`; the `errors.Is` passthrough miss confirmed and fixed; full `go test ./...` exit 0, `examples/run.sh` identical to `main`, corpus 17 findings / FP 0 unchanged. M2b-2 (**accepted 2026-10-15**): `schematag` analyzer in the default run, checking only types that flow into `rt` (`web.Get…` type arguments, `schema.Decode/Compile[T]`), so `schema:"…"` tags of other libraries (gorilla/schema) are not read; rule vocabulary kept in step with `rt/schema` by a test that parses its source; route checks (path tag vs pattern wildcard, body on GET/DELETE) move `rt/web` registration panics to vet time; `schema.Enum` with a `pure` `Valid`. Full `go test ./...` exit 0, `examples/run.sh` identical, corpus FP 0. The "unregistered pattern" diagnostic is removed: a registration in an importing package is invisible, so it could not be proved | testdata cases for each finding and for its silence; `examples/run.sh` green |
| M3 | `effectgo ts`. M3a (**accepted 2026-10-15**): `internal/rtshape` shared with `schematag` (its tests unchanged and passing), `tsgen` analyzer and `effectgo ts`; output for `examples/web` compiles with `tsc`. A type used both as request and response is a generation error (the two directions need different schemas). M3b (**accepted 2026-10-15**): mapping fixtures for every row of the mapping tables, with their current generated output compiled by `tsc` in `TestGoldenCompiles`; `examples/web` runs on the generated file; `run.sh` checks that `api.gen.ts` matches the generator. Path and query keys are the URL names from the `path`/`query` tags, not the JSON names. | **Chain proof done:** a new `ContactLocked` (423) in `Store.UpdateContact`, regenerated, makes the unchanged `exhaustive.ts` fail `tsc` with `Type 'ContactLocked' is not assignable to type 'TransportError'`; the same change with `errors.New` is a route finding in `go vet` and stays out of the list | **M3c (2026-10-15, from novastar, awaiting acceptance):** custom JSON and text encodings are not read from fields (Unknown or String, with a comment); name conflicts among the written names, the framework's included, are generation errors that list both types; an error list lists a variant once; group names lower acronyms (`API` is `api`); `rt/web` options and registration have effect rules in `stubs/rt.stubs`, so novastar's `allow unknown` is not needed; `WithPanicHandler` replaces novastar's logger wrapper for panic events. |
| M4 | novastar on `rt/web` | **Accepted 2026-10-15.** M4a: 8 read routes; M4b: 8 write routes, the hand-written pydantic-style decoder (`payload.go`, 874 lines) replaced by `schema` tags; unexpected 500s logged as `internal_error` with the error type chain and ops, no text. M4c: `src/lib/api.gen.ts` generated, `npm run check:api` guards it, a contract test drives the generated client against a sandboxed Go server (21 checks: typed `ValidationError`, `ReplyNotFound`, `DecodeError`; `bootstrap.stats` kept verbatim). `go test` 147 passed, `go vet` with effectgo 0 findings, `difftest.py` 148 checks ALL MATCH. **No frontend call is switched:** production runs the Python service, whose errors carry no `_tag`; switching waits for the Go service to go live, which is the owner's decision. |
| M5 | novastar increment 2b on the framework | **Done 2026-10-15 in novastar's working tree (not yet committed).** SMTP send and test-email; a standard-library IMAP client (LOGIN, LIST, EXAMINE, UID SEARCH, UID FETCH); `/api/mail-folders`, `/api/mail-test`, `/api/sync` with bounce linking and auto-suppression. Mail parsing is a `pure` package checked against CPython on 82 samples. `go test` 296 passed, `go vet` with effectgo 0 findings, `difftest.py` 246 checks ALL MATCH with no mail endpoint skipped, contract test passes. Verified against local fake servers only. |

## M4: the novastar wire contract (decided 2026-10-15)

novastar's React client reads an error as: any non-2xx; `result.error`, else `result.detail` when it is a string, else "请求内容无效" (`src/lib/api.ts`). The contract the Go port keeps is therefore that one, not byte equality with FastAPI's 422 bodies:

- Success bodies and business errors (status and the Chinese `detail` message) stay identical to the Python service; `difftest.py` keeps checking them field by field.
- A request the decoder rejects answers 4xx with no string `detail`, so the client shows "请求内容无效" as it does today. `difftest.py` checks "both sides reject with 4xx and no string `detail`" for these cases, and lists every case that changed and why (pydantic's lax coercion, such as `"1"` for an int, is accepted by Python and rejected by `rt`).
- novastar's error encoder adds `detail` next to `_tag`, so the current client and a generated client read the same response.
- Migration runs route by route: an outer mux sends migrated routes to the `rt/web` router and the rest to the existing handlers, and `difftest.py` stays green after every step.

## Findings carried into M2

From M1 (2026-10-15):

- **`rt/schema` against its own `pure` contract: 12 findings.** Ten are `unknown` from `reflect.Type` interface methods (`Kind`, `Elem`, `Implements`, `NumField`), which have no stub; two are the deliberate `panic` and the `global` pattern registry of `RegisterPattern` and `mustPlan`. M2 adds `reflect.Type` method stubs and gives the registry an explicit, narrower contract; the `pure` declaration stays.
- **Enum `Valid` is a pure contract.** `rt/schema` exports `type Enum interface{ Valid() bool }` with `//effectgo:pure` on `Valid`, so a user's `Valid` is checked instead of making every decode function `unknown`.
- **`errors.Is` passthrough is a silent miss** (read from the code, M2b confirms): `covered` treats `if errors.Is(err, ErrX) { return err }` as handled although `ErrX` carries no status and is sent as 500.
- **Passthrough errmaps are reported as HTTP 500.** `errset/errset.go` `checkHandler` treats a variant as handled only when the errmap matches it (`covered`); a variant with a constant `StatusCode` that the errmap returns unchanged (`return err`) is still reported `… which errmap X does not map → HTTP 500`. [errors.md](../reference/errors.md) says a passthrough keeps the variant's status. One of the two is wrong; the probe in M1 (`statuses of Decode: 400 schema.DecodeError` next to the 500 diagnostic) says the check is. M2 adds a testdata case and fixes `checkHandler`.
- **`web.Get` and friends have no route set.** Route discovery needs `route` rules for `rt/web`; whether the existing `route` resolution handles these generic functions is the first thing M2 checks.

## Known gaps after M2

- **`Valid` purity across packages.** The effects conformance check visits the named types of a package and the contracted interfaces in its import closure, so a type whose package does not import `rt/schema` (it satisfies `schema.Enum` structurally) is not checked. Fix with M3: share the type walker of `schematag`, and let effects check the `Valid` of every enum reached from an `rt` entry point.
- **`var ErrX error = NotFound{}`** is a sentinel whose declared type has no `StatusCode`, so a route returning it is reported although its dynamic type has a status (the initializer is not evaluated, as in `ownStatus`).
- **Facts do not cross analyzers** (x/tools passes a dependency's facts only to the same analyzer), so the generator in M3 cannot read effects, errset or exhaustive facts from another analyzer; it runs its own pass or shares a computed result.
- `examples/web` is not yet in `examples/run.sh`; it joins with the generator's mapping fixtures in M3.

## Backlog from the novastar migration (M4a, M4b)

Found by moving 16 real routes onto `rt/web`; each item names its evidence in novastar's `go/DOGFOOD.jsonl`.

- **`rt/schema` cannot tell a missing key from `null`.** A pointer field accepts both, so pydantic's "optional, but not null" (`str = None` in a model with no `Optional`) has no tag. novastar now accepts `{"smtp_port": null}` that Python rejected; the client never sends it. Proposal: a `nonnull` rule for pointer fields.
- **Errors that leave through an errmap are over-approximated.** Every novastar handler returns `mapError(err)`, so each endpoint's generated list is the union of everything `mapError` can return (`ReplyNotFound` appears on `/api/config`). errset could map each input variant through the errmap's branches instead of taking the errmap's whole output.
- **`rt/web` logs without the service's request ID.** A service with its own request-ID middleware cannot pass the ID to the router's log lines; novastar discards the router's logger and logs panics and 500s itself. Proposal: `WithRequestIDFrom(func(context.Context) string)`.
- **`rt/web` does not check `Content-Type`,** and an empty body decodes as `{}` for an input whose fields are all optional. Both differ from FastAPI; neither is reached by novastar's client.
- **Unstubbed standard library is pure.** `log/slog` and `runtime/debug` calls in a `//effectgo:pure` function are not reported, while `os.Stderr.Write` is (`fs`, `stdio`). A silent false negative of the stub coverage, not of `rt`.

## Risks

- **Route discovery through generic functions.** `web.Post[In, Out]` is generic; the `route` stub resolution has only been used with Fuego's generic functions in `openapi`. M2 confirms it on `rt/web` first.
- **Go types without an obvious Schema.** `time.Time`, maps, `json.RawMessage`, `any`. M1 fixes a mapping for each (ISO string, `Schema.Record`, `Schema.Unknown`) and the generator reports any other type instead of guessing.
- **`effect/http-api` changes in a 4.x minor.** Pinned, and caught by the `tsc` check of freshly generated mapping fixtures.
- **Reflection in decoding.** Measured in M1 against the novastar payloads; a generated decoder is the fallback, not the plan.
