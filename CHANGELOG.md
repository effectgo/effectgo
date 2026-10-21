# Changelog

## 0.2.0-rc.1 - 2026-10-21
- Add the candidate `effectgo contract` command with shared text/JSON diagnostics for the six static analyzers and Gin/Huma checks, including coarse rule IDs, native error severity, exit gates, deterministic ordering, and explicit coverage limits.
- Exercise one shared JSON corpus through raw `rt/web` HTTP decoding and a freshly generated Effect 4.0.4 client, covering omission/null, nested collections, unknown-key normalization, missing required fields, and scalar type errors.
- Document the real `examples/web` generation, TypeScript typecheck, Huma migration commands, and the release-candidate validation boundary.

## Unreleased

## 0.1.0 - 2026-10-20

- Reject ExternalError contracts during generation when runtime schema registration cannot accept them; retain strict input decoding and JSON output semantics.
- Omit explicit undefined object keys before generated JSON payload encoding, including nested DTOs, while preserving null, scalar values, array positions and special objects.
- Add opt-in HTTP finding and unresolved-route failure policies with machine-readable coverage, preserving default CLI exit behavior.
- Require real TypeScript compilation and HTTP decoding in the release gate; missing cross-language dependencies fail the release check. Run each discovered CLI/generator test in a fresh race-enabled process to bound accumulated analysis metadata under the shared memory cap.
- Share source verification and pin updates across applications, retaining exact build-input inventories, file hashes, platform binary hashes and runtime provenance checks.
- Clarify current JSON mappings, custom error contracts and route-option diagnostics.

- Add bounded multipart uploads to `rt/web` using required `web.File` and optional `*web.File` fields with `form` tags.
- Generate Effect 4.0.4 multipart `FormData` clients and reject unsupported mixed upload contracts during analysis and generation.
- Exercise upload metadata, binary contents, malformed multipart boundaries, duplicate/missing parts and request-size limits over HTTP.
- Preserve output header/cookie fields already used by dogfood applications, including cookie security attributes and repeated response headers.
- Analyze production web example sources separately from deliberate fault-injection test handlers in the example acceptance script.
- Add explicit success statuses, typed JSON response metadata, owned file streams, callback-scoped streams and redirects; preserve range/conditional responses and provider headers.
- Preserve declared `encoding/json` request semantics, exact single-value framing, DELETE bodies and scalar multipart fields without changing URL-field ownership or request-size limits.
- Generate native Effect success/status and binary codecs, distinct instantiated generic schemas, and explicit middleware error envelopes; verify current generated clients through TypeScript compilation and real HTTP decoding.
- Honor encoding/json byte slices in schema analysis and validate multipart null/omission boundaries with executable Effect schemas.
- Describe embedded and anonymous JSON response structs with encoding/json field names, dominance, nil embedded-pointer omission and inline Effect schemas; reject ambiguous output fields while retaining strict schema input restrictions.
- Extend the generated-client TypeScript/HTTP decode regression to cover inherited fields and anonymous nested rows encoded by Go's JSON encoder.
- Retain declared HTTP validation sources and owned rejected-value evidence without adding them to default error JSON; bound repeated embedded-type traversal by distinct types and fields rather than embedding paths.

## 0.0.2 - 2026-10-17

- Document optional header/cookie pointer semantics and Huma-to-rt/web envelope migration.
- Add real HTTP migration regressions for authentication boundaries, top-level responses, custom error media types, and preserved manual OpenAPI operation IDs.

## 0.0.1 - 2026-10-17

### Features

- Add Rust-inspired effect, error-set, must-use, exhaustive, schema, HTTP, and signature analysis.
- Add `rt/web` typed handlers and `rt/schema` runtime decoding.
- Add Effect TypeScript `4.0.4` `HttpApi` generation.
- Add header and multiple-cookie inputs with browser-safe `credentials: "include"` transport generation.

### Fixes

- Reject invalid route and schema contracts during generation and analysis.
- Keep browser clients from generating forbidden `Cookie` request headers.

### Documentation

- Document the runtime, analyzer architecture, Effect TypeScript contract, and release version `0.0.1`.
