---
title: P2: integration of Huma and Gin
description: The integration of the Huma and Gin adapters and the shared HTTP schema.
---

### Integration (P2: Huma + Gin)

Branch `integrate/p2`, created from `integrate/p1` (PR #1). Merges, in order, with `--no-ff`: `feat/http-gin` (3cdf6b1) and `feat/http-huma` (04afcd9).

**Conflicts and resolutions** (in `04afcd9`; the gin merge was clean).

- `internal/stubs/stubs.go`: the header lists the verbs of both branches (`status` with code or `arg:N`, `route` with `@N.Field`, `errdefault`, `sink`). The const block has `VerbSink` and `VerbErrdefault`. `sink` is read before the arity table (as in the gin branch); the unknown-verb message names all verbs. Both parsers survive.
- `errset/errset.go`: `Result` has both `Flows` (gin, read-only views) and `Mappers` (huma, every `func(error) error`). `run` builds both.
- `cmd/effectgo/main.go`: usage lines and dispatch for `http-gin` and `http-huma`.
- `README.md`: both usage lines.
- Build fix: both cmd files defined `relativize` with different signatures; the huma one is renamed `relativizeHuma`.

**Interaction tests** (all pass; fixture module `cmd/effectgo/testdata/integrate`, whose `fakes/` are the adapters' fake Huma and gin packages, via `replace`):

- (a) `TestInteractionCarriedThroughLocals` (cmd/effectgo/integrate_test.go). `Find` returns `e2`, two assignments after `huma.Error404NotFound`. The errset flow view (`Result.Flows`) keeps `Carried=true, Status=404`. The gin adapter reports `ThroughLocal` (a local `err` through `e2` into `c.JSON(500)`) with variant `huma.Error404NotFound`, route `GET /through`, status 500. The huma adapter sees the carried 404 with a known status and reports no 5xx finding.
- (b) `TestOneFileAllVerbs` and `TestMalformedLinesNameFileAndLine` (internal/stubs/integrate_test.go). One file holds sink (3 and 4 fields), `status` with a code, `arg:N`, and a type target, route with literal and `@N.Field` method and path, `errdefault`, `effects` and `errors`. Eleven malformed lines are each rejected with `bad.stubs:2:` in the error.
- (c) `TestShippedStubsLoadTogether`. The five shipped `stubs/*.stubs` load with `stubs.Load` and through `EFFECTGO_STUBS`. `effects`, `errset`, gin and huma then analyze the fixture with no error.

**Stub fix: `Rows.Scan` (no rule change was needed).** The brief expected `(*database/sql.Rows).Scan` to be modeled as returning `sql.ErrNoRows`. The evidence does not support that:

- Go 1.26.9 `database/sql/sql.go` (`go list -f '{{.Dir}}' database/sql`): `Rows.Scan` is at line 3365. It returns `rs.lasterr`, the closed-rows error, or a conversion error (`scanLocked`); it never returns `ErrNoRows`. `Row.Scan` is at line 3486, and its `return ErrNoRows` is at line 3513.
- `stubs/stdlib.stubs` has no errors rule for `(*database/sql.Rows).Scan`, so its set is opaque (`?`). The `(*database/sql.Row).Scan` rule (`ErrNoRows,?`) is correct and is unchanged.
- pgx v5.9.2 (module cache): `connRow.Scan` (the `pgx.Row` implementation) returns `ErrNoRows` at rows.go:111. `baseRows.Scan` (`pgx.Rows.Scan`) at rows.go:235 never does. `stubs/pgx.stubs` has the errors rule on `pgx.Row` only, so it is also correct.
- The ErrNoRows that the adapter notes attributed to `Rows.Scan` is the `SELECT COUNT(*)` `QueryRowContext(...).Scan(&total)` at kuzma-wms `internal/service/backup.go:74-76` (witness `sql.ErrNoRows via (Row).Scan [stub]`). That is `Row.Scan` on an aggregate, the documented limitation, and it stays. The Huma verification row 12 (GET /api/v1/backups) cites `Rows.Scan`; its verdict (false positive) holds, but the reason is the aggregate.
- Changes: a citation comment in `stubs/stdlib.stubs` (comment only). Tests: `TestRowsScanStubFix` (real `database/sql`, module mode: `RowsScan` has no `sql.ErrNoRows`, `RowScan` has it) and `TestPgxRowsStubRules` (parses `pgx.stubs`: the `pgx.Row` rule names ErrNoRows, `pgx.Rows.Scan` has no errors rule).

**`effectgo http`** (cmd/effectgo/http.go). One `packages.Load` and one `checker.Analyze` with both analyzers. The findings use the shared schema, sorted by severity (high, medium, leak, info), then position (route, variant break ties); info is hidden unless `-all`. `http-gin` and `http-huma` keep their output; their load code moved into `analyzeHTTP`, `ginFromGraph` and `humaFromGraph`. Tests: `TestHTTPUnionOfAdapters`, `TestRankOf`.

**Regression proof on the three snapshots** (`runs/integrate2/`, all runs under the lock; stubs = each branch's own four stub files plus the project stub file; pre-merge binaries built from `git archive` of `feat/http-gin` and `feat/http-huma`).

- Gin, `-json -all`: `order-mgmt` 586 rows, `kuzma-wms` 6, `arcteryx-console` 80. Pre-merge (`effectgo-pre-gin http-gin -json -all`) and merged (`http-gin -json -all`) outputs are identical as sorted sets: 0 differing lines on all three. The pre-merge summary lines match too.
- Huma, `-json -all`: kuzma-wms 355 rows, identical to pre-merge and to `runs/p2a/kuzma.http-huma.jsonl` (0 differing lines). order-mgmt and arcteryx-console have no huma routes.
- **Removed rows: none.** No row was removed; the stub fix changes nothing because no rule changed. The list is therefore empty.
- Union: `effectgo http -json -all` equals the sorted union of `http-gin -json -all` and `http-huma -json -all` on all three (order-mgmt 586, kuzma-wms 361, arcteryx-console 80; 0 differing lines).
- A caveat on the pre-merge gin baseline: an earlier pre-gin run produced `{}`, because the runner invoked the binary without the `http-gin` subcommand. Those files were discarded and re-run correctly.

**Metrics** (`metrics.py` on `effectgo -effects.report -errset.report ./...` with the four shipped stubs, no project stubs; baseline `runs/fix-stdlib/<project>.integrated-B.report`):

- order-mgmt and arcteryx-console: identical on every metric.
- kuzma-wms: `pct_opaque` 87.5 to 86.3, `pct_only_opaque` 16.3 to 14.7, `avg_named_variants` 2.59 to 2.61; all other metrics identical. Ten functions in `httpapi/` and `modules/pallet/` change from `?` to named huma carried variants (`resolveAuthForMe`, `requireBoundInboundSession`, `requireBoundInboundCommitToken`, `checkBoundInboundOperator`, `parseInventoryFilters`, `parseInventoryExportFilters`, `parseInventorySkuSummaryFilters`, `inventoryFiltersFromFields`, `sourceChannelError`, `palletError`).
- Cause: the huma branch's `stubs/huma.stubs` adds 43 `status` rules for the Huma constructors (`huma.Error4xx/5xx…`), so these calls carry statuses. `integrated-B` predates them: `git show integrate/p1:stubs/huma.stubs` has 0 `status` rules. The merged `stubs/huma.stubs` is identical to `feat/http-huma` (0 diff lines). This is the intended carried-status behavior, not a Rows.Scan change and not a merge bug. The brief expected only Rows.Scan changes; this difference is reported in full here.

**Verification.** `go build ./...`, `go vet ./...`, `go test ./... -count=1` (9 packages ok, under the lock), `gofmt -l .` (empty), `bash examples/run.sh` (exit 0, under the lock).

**Limitations of this integration.** The regression compares each pre-merge branch with the merged tree on the same snapshots; the stub files differ between the two branches (the merged `huma.stubs` and `gin.stubs` carry the sink and route forms), so each branch runs with its own stubs. The Huma fixture and the gin fixture are the adapters' fakes; they do not replace the real module packages.

