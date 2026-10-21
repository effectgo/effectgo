---
title: Releasing effectgo
description: "Run the complete Go, runtime and Effect TypeScript release gate, then bind reviewed source and runtime snapshots to the candidate."
---

A release candidate must pass `scripts/release-check.sh`. The script checks the current checkout and exits on the first failure. It does not install tools, change dependency versions, commit, tag, push or publish a release.

Current candidate: `v0.2.0-rc.1` ([GitHub Release](https://github.com/FaintGhost/effectgo/releases/tag/v0.2.0-rc.1)). Consumer locks must identify the exact tag, source tree digest, runtime manifest, and validated binary; the deployed `v0.1.0` remains the production baseline until consumer signoff completes.

## Prepare the toolchain

Use Go **1.26.x**, Node.js **22 or newer** with `fetch` and `File`, and npm. Race tests also require the Go race detector's supported platform and C toolchain. Keep the committed versions in `examples/web/ts/package.json` and `package-lock.json`: Effect and `@effect/platform-node` **4.0.4**, TypeScript **7.0.2**, and tsx **4.23.15**. A global `tsc` or dependencies installed in another project do not satisfy the gate.

Install the example's locked dependencies explicitly before checking a candidate:

```sh
npm ci --prefix examples/web/ts
```

The gate never runs `npm install`, `npx` or a global package install. Missing Node.js/npm, missing or mismatched package metadata, a missing compiler, or lockfile drift produce a nonzero exit with a repair command. Review an intentional dependency update before regenerating its lockfile; Effect remains pinned to 4.0.4.

## Run the gate

From the repository root:

```sh
bash scripts/release-check.sh
```

In the shared evaluation workspace, use the repository's heavy-command lock and memory limit:

```sh
mkdir -p "$HOME/.cache/effectgo"
flock "$HOME/.cache/effectgo/heavy.lock" systemd-run --scope -q -p MemoryMax=6G -p MemorySwapMax=0 \
  bash -c 'cd /path/to/effectgo && env -u GOROOT GOFLAGS=-buildvcs=false bash scripts/release-check.sh'
```

The reference full-gate run took 4005.75 seconds on this four-CPU host. Retain the per-test timeout; an outer 2400-second cutoff would kill a known passing run. Reuse successful checks only for the exact same relevant source/dependency inputs, and record any checks supplied by exact-commit CI separately from remaining local checks. Documentation-only changes do not require repeating analyzer race tests; validate the documentation build instead.

The script runs these existing consumers:

| Area | Checks |
|---|---|
| Release tooling | `node --test scripts/release-check.test.mjs scripts/source-tools.test.mjs scripts/test-isolated.test.mjs` |
| Root module | `go build ./...`, `go vet ./...`, `node scripts/test-isolated.mjs --root` (all root tests with `-race`) |
| Runtime module (`rt/`) | `go vet ./...`, `go test -race ./... -count=1` |
| Separate example modules | `go vet ./...` and `go test ./... -count=1` in `examples/layers`, `todo`, `editor` and `web` |
| Analyzer and generated example acceptance | `bash examples/run.sh`, including comparison of freshly generated TypeScript with the committed client |
| Example TypeScript | `npm --prefix examples/web/ts run typecheck` |

Root tests include **all** `tsgen` checks, not a selected name list: freshly generated mapping fixtures compiled by the locked compiler, native schema decoding, HTTP clients, external middleware errors, JSON request contracts, multipart binary bytes and response contracts. This includes new cross-language tests as they enter the package. `examples/run.sh` intentionally prints diagnostics for its fault examples; use its exit status and the individual test results when assessing the gate.
The isolated runner also executes the OpenAPI and signature packages one top-level test per fresh race-enabled process; these packages retain analyzer facts across loads and must not share the ordinary root test process.

The gate exports `GOWORK=off` and `EFFECTGO_STRICT_CROSS_LANGUAGE=1`. Every cross-language `tsgen` dependency check uses `requireTypeScript`: missing Node.js, missing TypeScript, or a missing/mismatched Effect dependency fails the test in this mode. Ordinary `go test` keeps its dependency skip behavior for Go-only development. An ordinary Go-only pass is therefore not release evidence. The gate disables test result reuse; root and runtime tests run with the race detector, including real decoder tests and `tsc`. The CLI and generator test binaries execute each discovered top-level test in a fresh process because repeated analysis loads otherwise retain race-detector metadata beyond the shared 6 GiB cap. All top-level tests, examples and fuzz seeds are discovered from the compiled binaries; no selected-name subset replaces the suite.

To investigate one cross-language test while preserving release dependency requirements:

```sh
EFFECTGO_STRICT_CROSS_LANGUAGE=1 go test ./tsgen -run TestGoldenCompiles -count=1 -v
```

A targeted run diagnoses a failure; the complete release gate must still pass after the fix. When adding a TypeScript consumer test, call `requireTypeScript(t)` before running it so the same dependency policy applies.

## Bind the candidate to consumers

Update `VERSION` and the corresponding `CHANGELOG.md` entry deliberately. Record the candidate source revision, platform, gate command and exit status alongside the release review. Re-run the gate after changing the candidate's source or dependencies.

Use the [source pinning tools](../../scripts/verify-source.mjs) to validate the actual consumer lock, generator source, runtime snapshot and optional generator binary. Their [behavior tests](../../scripts/source-tools.test.mjs) run in the gate. Keep each application's source/runtime lock tied to the exact candidate it consumes; a successful gate in this checkout does not itself update or validate a different application's installed snapshot.

From the checked framework source, update an existing application with:

```sh
node scripts/pin-source.mjs /path/to/app --binary linux-arm64=bin/effectgo
node /path/to/app/scripts/effectgo-source.mjs runtime
node /path/to/app/scripts/effectgo-source.mjs source /path/to/effectgo
node /path/to/app/scripts/effectgo-source.mjs binary /path/to/effectgo/bin/effectgo
```

The updater verifies the old runtime before replacing its owned files and copies the verifier without application-specific changes. It records exact source/runtime inventories and hashes, and checks the runtime `SOURCE.json` descriptor against the lock. Extra Go/stub build inputs and symbolic links are rejected. Binary inputs must have the declared Linux ELF64 architecture; their hashes are recorded, so release preparation must also run the native CLI and compare its version. Cross-built binaries require their target-platform smoke check before promotion.

An untagged source records `snapshot_kind: "release-candidate"`. Use `--release v0.1.0` only from the clean checkout whose HEAD matches that tag and `VERSION`; this records `snapshot_kind: "release"`. The source-tree digest remains a separate identity from the version and commit. Re-generate and check each consumer's contracts after pinning; application auth, query conversion, CI policies and deployment remain application-owned.

Tagging, publishing artifacts and changing application locks are separate, explicit release actions. This check produces no release and provides no deployment or production signoff.
