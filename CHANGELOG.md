# Changelog

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
