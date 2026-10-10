---
title: Development journal
description: Index of the development history of effectgo, by phase, with the section of the former NOTES.md that each file holds.
---

This is the record of how effectgo was built: the prototypes, the phases P0 to P4, the integrations between branches, and the real-world evaluations with their numbers. It is kept for the people who want the evidence behind a claim, not as a guide to using the tool. The reference is in [reference](../reference/directives.md), and the design rationale is in [design](../design/architecture.md).

The text is moved from the former `NOTES.md`, lightly edited for readability. No new claims were added. A reference in these files to "the README" or to `NOTES.md` means the former document; the current pages are the ones linked from the index. Where the journal and the reference disagree, the reference is current, and the journal shows what was true at the time.

## Phases

| File | Phase | What it holds |
|---|---|---|
| [01-prototypes.md](01-prototypes.md) | prototypes | the deviations from the two prototypes, and the open items that were not done |
| [02-p1-driver-stubs.md](02-p1-driver-stubs.md) | P1 | driver stubs, closure attribution, the measurements of the real-world snapshots |
| [03-p1-determinism-vet-identity.md](03-p1-determinism-vet-identity.md) | P1 | the stub-configuration determinism finding, zero configuration, the vet cache identity and its proof |
| [04-p1-report-tooling.md](04-p1-report-tooling.md) | P1 | witness lines, test-file handling, fresh errors, foreign collapse and their tests |
| [05-p1-openapi.md](05-p1-openapi.md) | P1 | OpenAPI error documentation: status inference, route discovery, the subcommand, the drift check, the 500 fix |
| [06-p1-integration.md](06-p1-integration.md) | P1 | the merge of the P0 and P1 branches, the commands and results |
| [07-p2-huma-adapter.md](07-p2-huma-adapter.md) | P2 | the Huma adapter, its kuzma-wms measurements and the manual verification |
| [08-p2-gin-adapter.md](08-p2-gin-adapter.md) | P2 | the Gin adapter, its results per project and the manual verification |
| [09-p2-integration.md](09-p2-integration.md) | P2 | the integration of the Huma and Gin adapters |
| [10-p3-flow-sensitive.md](10-p3-flow-sensitive.md) | P3 | the SSA-based flow-sensitive error sets: design, corpus, oracle, metrics, performance |
| [11-p4a-mustuse-exhaustive.md](11-p4a-mustuse-exhaustive.md) | P4a | the mustuse and exhaustive analyzers, their prototype defects and their corpus results |
| [12-p4b-editor.md](12-p4b-editor.md) | P4b | the golangci-lint plugin and the editor recipes: mechanism, verification matrix, latency |
| [13-p4-integration.md](13-p4-integration.md) | P4 | the integration of P4a and P4b: conflicts, wiring, corpus and sanity results |
| [14-p5-dogfood-fixes.md](14-p5-dogfood-fixes.md) | P5 | the fixes from the reservations dogfood run: named results and defers, test files and package defaults, net/http stubs, stub duplicates |
| [15-p6-output-contract.md](15-p6-output-contract.md) | P6 | the output contract: root-only reporting of contract violations, test files in the standalone run, the summary line |
| [16-p7-writer-effects.md](16-p7-writer-effects.md) | P7 | writer and reader effects: stub via items, stored values resolved from their constructor, the page on what effectgo does not check |
| [17-p7b-body-fields-memory.md](17-p7b-body-fields-memory.md) | P7b | the memory of the standalone run (the inspector requirement removed), and field sources for HTTP bodies |
| [18-p8-no-silent-directives.md](18-p8-no-silent-directives.md) | P8 | the directive analyzer for placements and spacing, one keyword rule for trailing comments and arguments, type-level contracts without methods, B4 closed |

## Where the former NOTES.md sections went

| Former `NOTES.md` section | Now |
|---|---|
| Tags used in the limitation lists | [design/limitations.md](../design/limitations.md) (the tags are explained there) |
| Layout | [design/architecture.md](../design/architecture.md) |
| Shared pieces | [reference/directives.md](../reference/directives.md), [reference/stubs.md](../reference/stubs.md), [design/architecture.md](../design/architecture.md) |
| Effects (R) | [reference/effects.md](../reference/effects.md) |
| Driver stubs & closure attribution | rules in [reference/stubs.md](../reference/stubs.md) and [reference/effects.md](../reference/effects.md); history in [02](02-p1-driver-stubs.md) |
| Error sets (E): Model, Directives and checks, Stubs | [reference/errors.md](../reference/errors.md), [reference/directives.md](../reference/directives.md), [reference/stubs.md](../reference/stubs.md) |
| Report tooling & error variants | rules in [reference/errors.md](../reference/errors.md); history in [04](04-p1-report-tooling.md) |
| Huma adapter | verb rules in [reference/stubs.md](../reference/stubs.md); the showcase in [guide/http-checks.md](../guide/http-checks.md); history in [07](07-p2-huma-adapter.md) |
| Flow-sensitive error sets (P3) | rules in [reference/errors.md](../reference/errors.md); history in [10](10-p3-flow-sensitive.md) |
| mustuse & exhaustive | [reference/analyzers.md](../reference/analyzers.md); history in [11](11-p4a-mustuse-exhaustive.md) |
| Signature (`effectgo sig`) | [reference/analyzers.md](../reference/analyzers.md#signature) |
| Stub flags and environment | [reference/cli.md](../reference/cli.md), [reference/stubs.md](../reference/stubs.md) |
| Determinism, Vet cache identity | [reference/cli.md](../reference/cli.md#go-vet-and-the-cache); history in [03](03-p1-determinism-vet-identity.md) |
| HTTP responses: Gin (adapter) | [guide/http-checks.md](../guide/http-checks.md); history in [08](08-p2-gin-adapter.md) |
| Limitations | [design/limitations.md](../design/limitations.md) (merged, de-duplicated, current) |
| Deviations from the prototypes, Not done | [01](01-prototypes.md) |
| Merge | [06](06-p1-integration.md) |
| OpenAPI | [reference/cli.md](../reference/cli.md) and [guide/http-checks.md](../guide/http-checks.md); history in [05](05-p1-openapi.md) |
| Integration (P0+P1a+P1b) | [06](06-p1-integration.md) |
| Integration (P2: Huma + Gin) | [09](09-p2-integration.md) |
| Editor and tooling: Editor integration | [guide/editor.md](../guide/editor.md); history in [12](12-p4b-editor.md) |
| Integration (P4: mustuse/exhaustive + editor) | [13](13-p4-integration.md) |

The design decisions and their reasons are in [design/decisions.md](../design/decisions.md).

## Tags in the phase files

The phase files keep the tags of the original notes: `[test]` (pinned by a test), `[run]` (checked by running a command) and `[reading]` (followed from the code, not run). In the current design pages these are **[verified]** and **[by reading]**.

## Measurements

The numbers in these files are the ones recorded at the time, on the snapshots of the real-world evaluation. They were produced by scripts outside the repository, and some of them were later changed by the flow-sensitive engine. Numbers recorded before the flow-sensitive engine (P3) belong to the earlier engine; [10](10-p3-flow-sensitive.md) gives the comparison.
