---
title: P2: Huma adapter
description: The Huma v2 route check: carried statuses, the default errmap, mapping evidence, the kuzma-wms measurements and the manual verification.
---

### Huma adapter

`effectgo http-huma` (`cmd/effectgo/http_huma.go`, `adapters/huma/`) checks the handlers of Huma v2 routes against Huma's own error handling. It reads the errset analyzer's results and facts. errset's model (flow-insensitive sets) is unchanged; it gained carried variants (`errset/carried.go`), the default errmap, and the branches of every `func(error) error` (`Result.Mappers`). The checks were run on kuzma-wms (`github.com/danielgtaylor/huma/v2` v2.37.2) with static analysis only.

**Huma's behaviour, verified in the module cache** (`/root/.local/share/go/pkg/mod/github.com/danielgtaylor/huma/v2@v2.37.2`):

- `StatusError` is the interface `GetStatus() int; Error() string` (`error.go:145-150`).
- `NewError` is a package variable of func type, returning `*ErrorModel` with the status (`error.go:231-249`). `NewErrorWithContext` is a variable too (`error.go:251`).
- The 40 constructors `Error400BadRequest … Error511NetworkAuthenticationRequired` are `return NewError(http.Status<Name>, msg, errs...)` (`error.go:281-477`). `Status304NotModified` (`error.go:276-278`) is the one more status helper.
- The dispatcher (`huma.go:1094-1106`): `errors.As(err, &StatusError)` writes the status (so a `%w`-wrapped status error keeps it), any other error is written as 500 "unexpected error occurred".
- Routes: `Get`/`Post`/`Put`/`Patch`/`Delete` take the path at argument 1 and the handler at argument 2 (`huma.go:2281-2383`). `Register` takes `huma.Operation{Method, Path}` at argument 1 and the handler at argument 2 (`huma.go:708`).

**Design.**

1. Route discovery (stub verbs, `stubs/huma.stubs`). `route pkg.Func <METHOD|@N.Field> <pathArg|@N.Field> <handlerArg>`. `@N.Field` reads a field of the composite literal that is argument N, so `Register` is `route …Register @1.Method @1.Path 2`. A field that is not a constant string, or an argument that is not a literal, makes the route unresolved with the reason (`internal/stubs/forms.go`). `openapi` skips rules with field references, because it only knows constant indexes.
2. Status-carrying constructors. `status pkg.Func <code>` (for example `status github.com/danielgtaylor/huma/v2.Error404NotFound 404`, one line per constructor, generated from `error.go:281-477`, each code checked against the `http.Status<Name>` of its body) and `status pkg.NewError arg:0` (the status is the first argument; `NewErrorWithContext` is `arg:1`). A call to a rule returns a *carried* variant (`Variant.Carried`): its `Status` is the call's status, its `Methods` are the result interface's (`StatusError`), and its display is `huma.Error404NotFound`, or `huma.NewError(404)` for the argument form. A non-constant status gives a variant with an unknown status (`huma.NewError(?)`). `errset/carried.go`.
3. Default errmap. `errdefault <pkgpath> passthrough=iface:<StatusError>,fallback=500` is turned into `errset.DefaultErrmap`: a carried variant keeps its status; a value that implements `StatusError` without a carried status (a type with a `GetStatus` method) has an unknown status; everything else gets the fallback. The `StatusCode` method of a project type is not read, which is correct for Huma (huma ignores it). No `//effectgo:handler` directive is needed.
4. Mapping evidence (`adapters/huma/evidence.go`). A variant is "mapped elsewhere" when some condition in the project tests it and its body returns a status below 500: `errors.Is(err, S)`, `errors.As(err, &T)`, `err == S`, or a call to a predicate that tests one concrete type with `errors.As` (`PredicateFact`, exported on `service.IsValidationError`). The func(error) error mappers (`errset.Result.Mappers`, which now holds every such function, with or without a directive) are evidence too. Handler-local guards count, because the spec says "mapped somewhere else".
5. Variants that reach the 500 fallback are classified (`adapters/huma/report.go`): `high`/`medium`/`info` as in the README table. A carried 500 (`huma.Error500InternalServerError`) is an intended 500, counted, never a finding. A variant with an unknown status is counted as "default", never a finding.

**Output.** `-json` writes one object per finding with the shared schema (`severity, framework, route, handler, pos, variant, status, via, reason`); `pos` is the route call site and `via` the witness chain (`(direct)` when the variant is created in the handler). The coverage goes to stderr in JSON mode and is printed after the findings in text mode.

**Tests.** `internal/stubs/forms_test.go` (the new forms, and parse errors that name the line), `adapters/huma/huma_test.go` (an analysistest fixture with a fake Huma package at the real import path, a project `service` package, and 15 routes: the raw sentinel, the mapped route, the `%w`-wrapped carried 404, `NewError` with a constant and with a variable status, a `GetStatus` type, a `StatusCode`-only type, a `Register` literal and a `Register` variable, a dynamic path, a fresh error, a handler-local `errors.Is` guard in another route, and a predicate), `adapters/huma/report_test.go` (the severities, cross-package evidence, the JSON schema and key order), `cmd/effectgo/http_huma_test.go` (positions).

**kuzma-wms, measured** (`runs/p2a/kuzma.http-huma.jsonl`, `runs/p2a/kuzma.http-huma.txt`; both with `-all`):

| | |
|---|---|
| route calls found (stub rules) | 134 (`Get` 54, `Post` 49, `Put` 15, `Delete` 7, `Patch` 1, `Register` 8; the same counts as `grep`) |
| resolved (constant method and path, handler resolved, default errmap applied) | 134; unresolved 0 |
| variants of resolved routes | 543, all with a known status (0 "default") |
| intended 500 (carried 500 from a constructor) | 18 |
| reaching the fallback 500 | 355 |
| findings: high / medium / info | 65 / 1 / 289 |
| routes with at least one finding | 85 |

The 289 info findings: 96 `context.Canceled`/`DeadlineExceeded` (foreign), 92 opaque `?`, 49 `sql.ErrNoRows`/`ErrTxDone` (foreign), 23 fresh `new@`, 20 `excelize`, 7 `strconv.NumError`, 2 other foreign. The one medium is an unexported service sentinel (`service.errForecastFallbackUnavailable`), which no handler can name.

The first version of the evidence (only `func(error) error` mappers) gave 41 high and 25 medium. The 24 medium findings that moved to high are variants that a handler guard maps (`ErrDuplicate`, `ErrInvalidTimezone`, `ErrInvalidOutboundStatus`, `ErrInvalidOperatorName`, `ErrDeleteBlocked`, `ErrInitFailed`, `ErrForecastCustomerMismatch`), and `validationError` through the `IsValidationError` predicate. The verification below is on the final run.

**Manual verification.** The seeded sample is `random.Random(20260510)` over the 65 high findings (20 drawn) and the 1 medium finding (all of them). The sample is saved as `runs/p2a/kuzma.http-huma.verification-sample.jsonl` (the same seed draws the same rows). Every row was read in the snapshot (static evidence only; nothing was run). "Guard" means the same handler tests the variant with `errors.Is` or a predicate and returns a status before the raw `return nil, err`, which errset does not model.

| # | sev | route (handler) | variant | verdict | evidence |
|---|---|---|---|---|---|
| 1 | high | GET /api/v1/outbound/summary/`{customer_id}` (outbound.go:60) | service.ErrInvalidTimezone | intended | raw return at outbound.go:61-63. The error is `fmt.Errorf("%w: %s", ErrInvalidTimezone, …)` (business_time.go:33) when the stored or configured timezone is invalid. The client cannot send a timezone here. The 400 in the settings PUT (settings.go:93-94) is for user input on another path. A 500 is right for a server-state fault. |
| 2 | high | POST /api/v1/locations/initialize/`{customer_id}` (entities.go:415) | service.ErrNotFound | false-positive (guard) | entities.go:420-422 returns 404 before the raw return at 423-424 |
| 3 | high | GET /api/v1/shipments/`{batch_id}`/detail (shipment.go:98) | service.ErrNotFound | false-positive (guard) | shipment.go:101-103 returns 404 before the raw return at 104 |
| 4 | high | PUT /api/v1/operators/`{operator_id}` (entities.go:251) | service.ErrInvalidOperatorName | false-positive (guard) | entities.go:256-258 returns 400 before the raw return at 262-263 |
| 5 | high | POST /api/v1/operators (entities.go:237) | service.ErrNotFound | false-positive (infeasible) | the only source is `return scanOperator(QueryRowContext(SELECT … WHERE id = ?))` (entities.go:210) after the INSERT of the same request: `sql.ErrNoRows` needs the row just inserted to be missing. Not guarded. Practically infeasible. |
| 6 | high | GET /api/v1/inbound/check-tracking (inbound.go:362) | service.ErrNotFound | false-positive (guard) | inbound.go:373-375 returns 400 before the raw return at 379 |
| 7 | high | GET /api/v1/inventory/items (inventory.go:92) | service.ErrNotFound | false-positive (variable reuse) | `err` is assigned by parseInventoryFilters (93) and by inventory.List (97). The raw return at 95 can only carry the filter error; the List error is answered by the fixed 500 at 102 |
| 8 | high | DELETE /api/v1/operators/`{operator_id}` (entities.go:268) | service.ErrDeleteBlocked | false-positive (guard) | entities.go:273-275 returns 400. The report's "returns it without mapping" is wrong for this route |
| 9 | high | GET /api/v1/customers/`{customer_id}` (entities.go:166) | service.ErrNotFound | false-positive (guard) | entities.go:168-170 |
| 10 | high | PUT /api/v1/customers/`{customer_id}` (entities.go:194) | service.ErrNotFound | false-positive (guard) | entities.go:196-198 |
| 11 | high | DELETE /api/v1/backups/`{backup_id}` (backups.go:199) | service.ErrNotFound | false-positive (guard) | backups.go:201-203 |
| 12 | high | GET /api/v1/backups (backups.go:150) | service.ErrNotFound | false-positive (Rows.Scan) | backup.go:66 is inside BackupService.List (backup.go:47) in `for rows.Next()`. Go 1.26.9 `database/sql` `Rows.Scan` (sql.go:3365) never returns `ErrNoRows`; only `Row.Scan` (sql.go:3486) does |
| 13 | high | POST /api/v1/backups/`{backup_id}`/restore (backups.go:210) | service.ErrNotFound | false-positive (guard) | backups.go:212-214 |
| 14 | high | POST /api/v1/operators (entities.go:237) | service.ErrDuplicate | false-positive (guard) | entities.go:242-244 returns 400 |
| 15 | high | GET /api/v1/customers (entities.go:154) | service.ErrNotFound | false-positive (Rows.Scan) | entities.go:47 is inside CustomerService.List (entities.go:30) in `for rows.Next()` |
| 16 | high | PUT /api/v1/operators/`{operator_id}` (entities.go:251) | service.ErrNotFound | false-positive (guard) | entities.go:253-255 |
| 17 | high | DELETE /api/v1/operators/`{operator_id}` (entities.go:268) | service.ErrNotFound | false-positive (guard) | entities.go:270-272 |
| 18 | high | GET /api/v1/shipments (shipment.go:75) | service.ErrNotFound | false-positive (Rows.Scan) | shipment.go:169 is inside ShipmentService.List (shipment.go:110) in `for rows.Next()` |
| 19 | high | PUT /api/v1/operators/`{operator_id}` (entities.go:251) | service.ErrDuplicate | false-positive (guard) | entities.go:259-261 |
| 20 | high | GET /api/v1/inbound/session/`{session_id}` (inbound.go:398) | service.ErrNotFound | false-positive (guard) | inbound.go:403-405 |
| 21 | medium | GET /api/v1/inbound/check-tracking (inbound.go:362) | service.errForecastFallbackUnavailable | false-positive (callee guard) | ensureForecastCached (inbound_forecast_fallback.go:52-54) returns `nil, nil` when `errors.Is(err, errForecastFallbackUnavailable)`, so the sentinel never leaves that function. errset keeps it in the function's set. The sentinel is unexported, so no handler can name it |

Sample result: 1 intended, 19 false positives (14 guards, 3 `Rows.Scan`, 1 infeasible read-back, 1 variable reuse), and 1 false positive among the medium (callee guard). Zero likely bugs in the sample.

Out-of-sample checks (not part of the seeded sample; listed because they were read while checking the sample):

| route | variant | verdict | evidence |
|---|---|---|---|
| PATCH /api/v1/auth/users/`{user_id}` (auth_admin.go:217) | service.ErrNotFound | **likely-bug** | UpdateUser returns only the Exec error (auth.go:1098-1146), so a missing id gives nil for a `display_name`-only body. The next `GetUserByID` (auth_admin.go:243) returns `ErrNotFound` (auth.go:330-331), returned raw at 245: the admin gets 500 where 404 is due |
| GET /api/v1/inbound/check-tracking (inbound.go:362) | service.validationError | **likely-bug** | with no `customer_id` and an empty `tracking_no` (the input has no required tag, inbound.go:114-117), CheckTrackingAutoResolve returns `newValidationError("tracking number is required")` (inbound_forecast_flow.go:30-31, errors.go:24-26), which the handler returns raw (inbound.go:379). The project maps validation to 400 elsewhere (settings.go:96-97, palletError) |
| PUT /api/v1/auth/users/`{user_id}`/warehouse-roles (auth_admin.go:294) | service.ErrNotFound | false-positive (race only) | SetUserWarehouseRoles calls GetUserByID first (auth.go:465) and the handler answers that error with 400; the raw GetUserByID at auth_admin.go:305 needs a delete between the two calls |
| PUT /api/v1/settings/`{key}` (settings.go:82) | service.validationError | false-positive (predicate guard) | settings.go:96-97 returns 400 for `service.IsValidationError(err)` |
| POST /api/v1/auth/login (auth.go:292) | service.ErrNotFound | false-positive (variable reuse) | `ss, err := svc.GetSessionByToken` (auth.go:302) answers failure with 401 (303-305); the raw return after `me, err := svc.BuildAuthMe` (306) reuses `err` |
| dashboard overview/trends/customer-ranking, inventory, shipments, outbound summary (ErrInvalidTimezone) | service.ErrInvalidTimezone | intended (by row 1) | same origin as row 1; not checked one by one |

Not checked individually: the other high findings of the "read-back after write" shape (`POST /api/v1/customers`, `POST /api/v1/references`, `POST /api/v1/auth/signup`, `POST /api/v1/backups/run`), which have the same structure as row 5, and the 30 highs without a textual guard beyond the sample. A textual heuristic (`errors.Is/As` on the variant's name inside the handler body) finds a guard for 35 of the 65 high findings. It is a triage aid and was not used for any verdict above.

**False-positive causes.** (1) errset does not model a guard in the same handler or a callee: `if errors.Is(err, S) { return status }` followed by `return nil, err` (14 of the 21 sampled rows). (2) `*sql.Rows.Scan` is treated as able to return `sql.ErrNoRows`; it never does (3 rows). (3) A read-back of a row the same request just wrote (1 row). (4) One `err` variable is reused for two calls, so the union of both assignments reaches each `return err` (2 rows). (5) A callee swallows the sentinel with a guard (1 row). The likely bugs are both real domain paths reaching the fallback: a row that the handler does not guard (`PATCH` user, `check-tracking` validation).

**Limitations.** Errset is flow-insensitive, as above. The intended/likely-bug split needs a reader: the "mapped elsewhere" rule can call a variant that is a server fault in one path and a bad input in another (`ErrInvalidTimezone`, row 1). Only `StatusError` types with carried statuses are read; a type whose `GetStatus` is not constant is "unknown", not 500. `sql.ErrNoRows` reaching a handler raw is reported as info (foreign), although it is a plausible not-found bug; this follows the severity definition, not a judgement. Route registration through a wrapper or a variable operation is unresolved, not followed. Output positions are file:line of the route call site, not of the return statement that produces the error.

