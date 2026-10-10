---
title: Backlog
description: Known gaps that were found in use and deliberately not fixed in the documentation round, each with the evidence, the proposed change and why it is not a small fix.
---

Each item came from a first-time user, a probe run, or the evaluation of the dogfood project. The evidence is what was observed; the proposal is a direction, not a decided design. An item is removed when it is fixed, and the fix is recorded in the [journal](../journal/index.md), not here.

The three output-contract items (root-only reporting, test files by default, the summary line) were fixed in the [P6 journal entry](../journal/15-p6-output-contract.md) and are no longer listed here.

## B5. Timing on a small module is several seconds, and the cache hides the cost of `go vet`

**Evidence.** On a six-package module (about 1.8k lines), `effectgo ./...` took 3.9 to 4.7 s of wall time, and `effectgo sig ./...` about 3.7 s. `go vet -vettool` took about 0.2 s once its result was cached, so the first run after an edit is the only honest timing.

**Proposal.** None yet. Measure on the disciplined corpus and on a large service before choosing between caching facts, sharing the SSA build between analyzers, or narrowing the packages loaded. This is an observation, not a defect.
