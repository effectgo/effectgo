---
title: "P2: Gin adapter"
description: "The Gin response check: sinks, mappers, leaks, the results per project and the manual verification of its findings."
---

## HTTP responses: Gin (adapter)

### Gin adapter

Code: `adapters/gin/` (the analyzer), `cmd/effectgo/http_gin.go` (the subcommand), `errset/flow.go` (the read-only views of errset that the adapter uses), and the `sink` verb in `internal/stubs`. Tests: `adapters/gin/gin_test.go` with the fixture in `adapters/gin/testdata` (a fake gin package, a store, `httpapi.Detail`, and the handler shapes), `cmd/effectgo/http_gin_test.go` (severities and the JSON keys), `internal/stubs/stubs_test.go` (the verb).

**Design.** A Gin handler returns nothing, so the boundary is a call. A *sink* is a call that writes a response, declared by a stub line `sink <target> status=<i> [body=<j>]`. `stubs/gin.stubs` declares the gin writers (`JSON`, `AbortWithStatusJSON`, `String`, `Data`, and the rest; `Status` is not a sink because it only sets the pending code). A project helper is declared in a project stub file outside the snapshot, for example `sink <module>/internal/httpapi.Detail status=1 body=2`. Sinks are read by the callee's key (the `stubs.Target` string), so methods and functions use one rule form.

Mappers are detected, not annotated, by a rule engine that reads one function body and collects two kinds of rule per error variable:

- *variant rules*: `errors.Is(e, X)`, `errors.As(e, &T)`, `e == X` (a shape of errset's `ErrorTest`, exported for this use) in an `if` or a `switch` case whose body answers with a sink of constant status;
- *default rules*: the status of the catch-all: `if e != nil { sink }`, `if e == nil {} else { sink }`, the `else` of a test, a `switch` default, and the sink that follows an `if`-return on the same block (the fallthrough). Pending fallthroughs are scoped to their statement list.

A **sink mapper** is a function with a `*gin.Context` and an error parameter whose body yields at least one variant rule on the error parameter; its branches and its default are exported as a fact. A **status mapper** is `func(error) (int, bool)` whose error branches return a constant `N, true`; it is exported as a fact too. Handlers in another package use the facts.

**Handler analysis.** Every declaration and every func literal with a `*gin.Context` parameter is a handler. The engine runs on its body. The error variables of the handler come from errset's flow of the enclosing declaration (`errset.Result.Flows`, a read-only view of the same sets errset uses, so a variable's set is the union of its assignments; see the limitations). For each subject variable, the variants of its set get the first matching variant rule, else the default; a status of 5xx is a finding at the sink. A call `m(c, err)` to a sink mapper finalises the variants of `err` directly: a branch or the default. The status-mapper shape `if status, ok := m(err); ok { sink(c, status) ; return }` followed by a sink adds m's branches as variant rules on `err`, and the fallthrough as its default. A variable that is a parameter of the handler is not reported (its variants are unknown).

**Leaks.** A sink whose body argument (or the variadic tail of `c.String`) contains `err.Error()` on an error-typed expression, a `fmt.Sprint*`/`Errorf` argument of an error type, or a `+` with one of those, is a leak. The walk goes through composite literals up to two levels (`maxLiteralDepth`). The brief asked for one level; the second level is needed for the envelope `gin.H{"detail": gin.H{"error": err.Error()}}` of kuzma-wms (without it, its leaks are missed). A leak with a non-constant status (a variable such as `status` from a status mapper) reports status 0.

**Routes.** `r.GET/POST/PUT/PATCH/DELETE` on a router (through `RouterGroup`) with a constant path. Group prefixes resolve when the group is a variable assigned from `x.Group("const")` in the package. The handler is the last argument: a literal, a declared function, or a factory call whose returned literal is exported as a fact. A route joins to a finding by the handler's position.

**Severity (the driver).** The aggregate over all packages of the run gives the set of named variants with a non-5xx explicit mapping (mapper branches, status-mapper branches, and inline variant branches). `high`: a named variant (sentinel or type) that is in that set; `medium`: a named variant not in it; `info`: `?`, `new@`, or any non-named variant at 5xx; `leak`: a leak. A foreign named variant (`pgx.ErrNoRows`) is named and is marked `(foreign variant)` in the reason.

**Output.** `effectgo http-gin [-json] [-all] [-mappers] [-stubs ...] ./...`: JSON lines in the shared schema; `info` only with `-all`; `-mappers` lists the detected mappers on stderr; a summary line on stderr gives the counts (`info` is counted even when hidden).

**Changes to shared files.** `internal/stubs`: the `sink` verb (a variable-arity verb read outside the arity table). `errset`: `Result.Flows` (the per-declaration flow of each unit), `Flow.Of`, `ErrorTest`, `Branch.Matches`, `IsError`, `SentinelKey` in the new `errset/flow.go`. `cmd/effectgo/main.go`: one dispatch line. `stubs/gin.stubs`: the sink rules. The existing analyzers are unchanged: the `sink` verb is ignored by the others, and the errset results are the same.

### Results per project (static, run on the snapshots)

The runs used `EFFECTGO_STUBS` = the worktree's `stdlib`, `pgx`, `gin`, `huma` stubs plus each project's `runs/p2b/<project>.project.stubs`. Outputs: `runs/p2b/<project>.http-gin.jsonl` (`-json`, info hidden) and `.txt` (text). The counts are from the summary line on stderr (which counts info even when hidden).

| project | handlers | mappers | route registrations | high | medium | info (hidden) | leak |
|---|---|---|---|---|---|---|---|
| order-mgmt | 396 | 9 sink | 159 | 83 | 136 | 300 | 67 |
| arcteryx-console | 59 | 3 (1 status, 2 sink) | 44 | 0 | 5 | 49 | 26 |
| kuzma-wms | 20 | 0 | 2 | 0 | 0 | 0 | 6 |

Detected mappers against the known list:

- order-mgmt (9): `handleStoreError` (addresses, sink, default 500), `mailboxRestoreError` (sink, no default), `mailboxViewerError` (sink, default 502), `systemMailboxPoolRevalidationError` and `systemMailboxPoolReplacementError` (sink, default 500), `respondUpstreamError` (mall, sink, default 502, branch ErrNotConfigured to 503), and the three `diagnostics*Error` (`diagnosticsMailClassifierAutomationError`, `diagnosticsGlobalMailRuleError`, `diagnosticsMailClassificationError`, sink, default 500). Every named mapper of the brief is found. The brief said "about 10"; 9 were found, and the run does not show a tenth.
- arcteryx-console (3): `mallErrorStatus` (status, no default), `mallSyncError` (sink, default 500), `mallTradeCouponError` (sink, default 500). All three are found.
- kuzma-wms: none expected (its Gin part is middleware and two routes).

Totals from the JSON: order-mgmt 286 records (83 high, 136 medium, 67 leak); 117 of them have a route. arcteryx 31 records (5 medium, 26 leak). kuzma 6 leaks (4 with status 500, one 502, one 409). The high records of order-mgmt are 63 `pgx.ErrNoRows`, 8 `mall.ErrInvalidMonth`, 5 `readonly.ErrGiftCardOwnerNotFound`, 4 `giftinquiries.ErrInference*`, 1 `orderimport.ErrMailboxNotFound`, 1 `wms.ErrShipmentDestinationUnsupported`, 1 `wms.ErrShipmentInventoryConflict`. The medium records of order-mgmt are led by `pgx.ErrTxClosed` (34) and `pgx.ErrTxCommitRollback` (34), which are the default 500 of a failed `Tx.Commit` (intended, not reviewed in the sample), then `mall.ErrNotConfigured` (11) and `readonly.ErrGiftCardOwnerForbidden` (10, not reviewed).

### Manual verification

Static evidence only; the services were not run. Samples: fixed seed 20260 (Python `random.Random(20260)` on the records sorted by position), 15 of the high records of order-mgmt and 5 leaks of each project. arcteryx and kuzma have no high record, so there is nothing to sample. Snippets are shortened and contain no secrets.

**order-mgmt, high (15 of 83).**

| # | site | variant, status | verdict | evidence |
|---|---|---|---|---|
| 1 | giftinquiries/handlers.go:66 | pgx.ErrNoRows, 500 | false-positive (flow reuse) | `owner, err := resolveOwner(...)` (l.53); `if err != nil { return }`; then `jobs, err := store.ListKindsForUser(...)` (l.64) reuses `err`; the ErrNoRows is resolveOwner's, which already returned. |
| 2 | giftinquiries/order_completion.go:122 | pgx.ErrNoRows, 500 | false-positive (stub) | `LoadOrderContactState` runs `SELECT count(*), ... FROM contacts`, an aggregate that always returns one row; the `(Row).Scan` stub still lists ErrNoRows. |
| 3 | mall/cart.go:177 | pgx.ErrNoRows, 502 | false-positive (flow reuse) | `row, err := cartRow(...)`; `if errors.Is(err, pgx.ErrNoRows)` returns 404 (l.144); later `out, err := revalidateCartRows(...)` (l.175) reuses `err`. |
| 4 | mall/cart.go:207 | pgx.ErrNoRows, 500 | false-positive (stub) | `HasConnectedPersonalMailbox`: `SELECT EXISTS (...)`, always one row. |
| 5 | mall/cart.go:89 | pgx.ErrNoRows, 502 | false-positive (flow reuse) | `listing, ok, err := service.findListing(...)` (l.87); `err = pool.QueryRow(...).Scan(&id)` (l.109) assigns the same `err` later in `addCartItem`. errset's witness for findListing has no ErrNoRows. |
| 6 | order_import/handlers.go:468 | pgx.ErrNoRows, 500 | false-positive (flow reuse) | `GetMailboxForUser` (pg_store.go:1509-1510) maps ErrNoRows to ErrMailboxNotFound; the raw `return item, err` (l.1514) cannot carry it. |
| 7 | order_import/handlers.go:468 | orderimport.ErrMailboxNotFound, 500 | **likely-bug** (narrow) | `account, err := store.GetMailboxForUser(...)` answers `httpapi.Detail(c, 500, "db error")` (l.467-468); the store returns ErrMailboxNotFound for a missing or disabled mailbox, and the same variant is 404 elsewhere. Reachable only if the mailbox is disabled or deleted between `MarkMailboxSyncing` and this read. |
| 8 | order_import/handlers.go:624 | pgx.ErrNoRows, 500 | false-positive (stub) | `GetMailboxQuota`: `FROM (VALUES ($1::BIGINT)) requested(user_id) LEFT JOIN user_memberships`, always one row. |
| 9 | orders/writes.go:1331 | pgx.ErrNoRows, 500 | false-positive (flow reuse) | `if !errors.Is(err, pgx.ErrNoRows) { return err }` (l.166), then `_, err = tx.Exec(...)` (l.169); the function's final `return err` is the Exec result. |
| 10 | orders/writes.go:1540 | pgx.ErrNoRows, 500 | false-positive (stub) | `ManagedOrderWarehouse`: `SELECT EXISTS (...)` (longinventory/guard.go:36-47). |
| 11 | orders/writes.go:285 | pgx.ErrNoRows, 500 | false-positive (stub) | `SELECT COUNT(*) FROM channels WHERE ...`, an aggregate. |
| 12 | readonly/handlers.go:101 | pgx.ErrNoRows, 500 | false-positive (stub) | `buildOrderCountQuery` returns `SELECT COUNT(*) FROM orders ...` (l.1212). |
| 13 | readonly/handlers.go:191 | pgx.ErrNoRows, 500 | false-positive (stub) | `LoadAICompletions`: `SELECT to_regclass(...) IS NOT NULL AND ...` has no FROM and always returns one row (ai_completion.go:47-55). |
| 14 | readonly/order_completion_store.go:347 | pgx.ErrNoRows, 500 | false-positive (flow reuse) | `ResolveGiftCardOwner` turns ErrNoRows into ErrGiftCardOwnerNotFound (gift_cards.go:207-208), which the caller answers 404 (order_completion_store.go:342-344); the raw `return 0, err` (gift_cards.go:211) cannot carry ErrNoRows. |
| 15 | wms/inventory_status.go:35 | pgx.ErrNoRows, 500 | false-positive (seeded row) | `SELECT ... FROM wms_inventory_sync_state WHERE instance_code='wms_long'`; the migration seeds the row (wms/schema.go:681), so ErrNoRows is not reachable after setup. A DB error answered as 500 is intended. |

Tally: 1 likely-bug, 14 false-positive: 6 flow reuse (1, 3, 5, 6, 9, 14), 7 aggregate/EXISTS/FROM-less stub (2, 4, 8, 10, 11, 12, 13), 1 seeded singleton (15).

**order-mgmt, leak (5 of 67).**

| # | site | status | verdict | evidence |
|---|---|---|---|---|
| 1 | order_import/handlers.go:952 | 400 | **likely-bug** | `httpapi.Detail(c, http.StatusBadRequest, err.Error())` on `store.Sync(...)`; `ReferenceSyncService.Sync` returns the raw DB error of `loadReferenceConfig` and the upstream error of `fetchSyncState` (reference_sync.go:204-217). Internal errors leave as a 400 body. |
| 2 | robot_orders/robot_orders.go:385 | 400 | intended | `c.JSON(400, Response{Message: err.Error()})` on `ShouldBindJSON`: a binding message for the client. |
| 3 | wms/handlers.go:309 | 500 | **likely-bug** | `c.JSON(500, gin.H{"message": err.Error()})` on `rows.Err()`: a raw database error. |
| 4 | wms/handlers.go:440 | 500 | **likely-bug** | `err` from `service.GetShipmentRequestDetail`, which returns the raw `QueryRow` scan error (service/shipment: `return nil, err`); its text goes to the client. |
| 5 | wms/handlers.go:549 | 500 | **likely-bug** | `c.SaveUploadedFile(fileHeader, target)` fails with an OS error that names `data/shipment-payment-proofs/...`: a server path. |

**arcteryx-console, leak (5 of 26).** No high record exists, so the sample is the leaks only.

| # | site | status | verdict | evidence |
|---|---|---|---|---|
| 1 | api/handlers/mall.go:214 | 4xx from mallErrorStatus (not constant) | intended | `if status, ok := mallErrorStatus(err); ok { c.JSON(status, gin.H{"error": err.Error()}) }`; the sentinels are returned bare (store/mall_listings.go:72), so the text is a fixed domain message. |
| 2 | api/handlers/mall.go:282 | 4xx from mallErrorStatus | intended | the same shape as 1. |
| 3 | api/handlers/mall_import.go:162 | 400 | intended | `c.JSON(400, gin.H{"error": err.Error()})` on `normalizeMallImportRows`: a validation message. |
| 4 | api/handlers/mall_import.go:191 | 4xx from mallErrorStatus | intended | the same shape as 1. |
| 5 | api/handlers/mall_trade_coupon.go:310 | 400 | intended | `mallTradeCouponError`: `errors.Is(err, store.ErrInvalidTradeType)` answered 400 with `err.Error()`; the sentinel is returned bare (store/mall_trade.go:68). |

**kuzma-wms, leak (5 of 6).**

| # | site | status | verdict | evidence |
|---|---|---|---|---|
| 1 | httpapi/backups.go:193 | 500 | **likely-bug** | `err` from `BackupService.FilePath`: `fmt.Errorf("read backup file path: %w", err)` (service/backup.go:507-516), the database error, sent in `gin.H{"error": err.Error()}`. |
| 2 | httpapi/shipment.go:159 | 500 | **likely-bug** | `err` from `ShipmentService.OMSFileProxyTarget`, which returns the raw `QueryRowContext(...).Scan` error (service/shipment.go:798-809). |
| 3 | httpapi/shipment.go:170 | 500 | **likely-bug** (low) | `http.NewRequestWithContext(..., endpoint, ...)`: the URL error repeats `endpoint`, built from the internal `baseURL` (shipment.go:162). |
| 4 | httpapi/shipment.go:178 | 502 | **likely-bug** | `omsFileProxyHTTPClient.Do(req)` returns a `*url.Error` with the internal OMS URL, sent to the client. |
| 5 | middleware/idempotency.go:80 | 409 | **likely-bug** | `registerPending` returns the raw `db.ExecContext` error (`return err`), sent as the body of a 409: a database error with the wrong status. |

Tally of the 15 leak samples: 9 likely-bug, 6 intended. By project: order-mgmt 4 likely-bug and 1 intended (the robot binding message); arcteryx 5 intended (fixed sentinel texts and validation messages); kuzma 5 likely-bug.

### False-positive causes (in the sample, and in the design)

1. *Flow-insensitive reuse* (6 of 15 high in the sample). errset's sets are the union over a declaration, so an `err` that is assigned again (`err =`, `:=` in the same scope) carries the variants of every assignment, including those that were handled by an earlier `if errors.Is(...)` return. Fix: an ordered (per-program-point) flow for the variable; not done.
2. *`(Row).Scan` stub is over-approximate* (7 of 15 high). The shipped `pgx.stubs` lists `pgx.ErrNoRows` for every `Row.Scan`, including aggregates (`count(*)`, `max`), `SELECT EXISTS`, `FROM`-less selects and single-row joins over `VALUES`, which always return a row. This accounts for 63 of the 83 order-mgmt high records. The fix belongs in the pgx stub, not here (shared; it changes the effects and errset results of other analyses); a per-query rule would need the SQL text.
3. *Seeded singletons* (1 of 15). A not-found from a row that the schema always seeds.

### Limitations of the Gin adapter

- [reading] Flow-insensitive per declaration under the AST engine (causes 1 above); under the SSA engine (default) each rule and default is read at its own use, so a guard above the sink removes what it tests and a reassigned variable carries only its current value. The precision problem of order-mgmt that this caused under the AST engine is measured in "Flow-sensitive error sets (P3)".
- [reading] Only sinks with constant status are checked; a status variable is unknown (status 0) and blocks the default for its variants, so no finding is made. The mapper status shape with a variable status is covered only through the status mapper.
- [reading] Project wrappers with a constant status inside (`httpapi.MissingField`, `ValueError`, ...) are not sinks: the sink verb takes an argument index. Only helpers that receive the status as an argument are stubbed. The 4xx validation helpers are therefore invisible, which is safe for the 5xx check.
- [reading] Handler literals at package level (a `var h = func(c *gin.Context)`) have no declaration flow, so their error variants are unknown and produce no reach findings; their leaks and obs are still computed.
- [reading] A mapper that calls another mapper, or a status returned by a helper, is not composed; the inner call is read as an opaque default.
- [reading] The leak check reads the body argument and the variadic tail of `c.String`; a body built by a helper function (`gin.H` returned by a function) is not seen. The variadic tail and the two composite levels are pinned by the fixture (`TestLeaks`).
- [reading] The `route` of a handler reached through a variable (`h := X.Handle; r.GET(p, h)`) is not resolved.
- [reading] The severity classification uses the explicit observations of the whole run: a variant mapped on one route and reaching 5xx on another is `high` even when the two cannot share the data path.
- Not reviewed in the sample: the 20 non-`pgx.ErrNoRows` high records of order-mgmt (in particular `readonly.ErrGiftCardOwnerNotFound` (5) and `mall.ErrInvalidMonth` (8), which could be real), and all medium records. These need the same review.

