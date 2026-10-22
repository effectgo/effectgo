---
title: Releasing effectgo
description: "Run the complete Go, runtime and Effect TypeScript release gate, then bind reviewed source and runtime snapshots to the candidate."
---

Run `scripts/release-check.sh` for changes to analyzer, generator, runtime, test tooling, or dependency inputs. It checks the current checkout and exits on the first failure; it does not install tools, change dependency versions, commit, tag, push or publish. For a release changing only version declarations and documentation, reuse the recorded full pass for unchanged implementation/dependency inputs, run the complete CLI race-test package, check the actual version and behavior of both release binaries, and validate documentation. Record the source diff and distinguish reused evidence from checks executed for the release.

Current stable release: `v0.4.0` ([GitHub Release](https://github.com/effectgo/effectgo/releases/tag/v0.4.0)). The canonical module paths remain `github.com/effectgo/effectgo` and `github.com/effectgo/effectgo/rt`, introduced in v0.3.0. Consumer locks must identify the exact tag, source tree digest, runtime manifest, and validated binary. Applications retain their independently validated pins until an explicit consumer upgrade; publishing this release does not repin or redeploy them. Historical tags remain unchanged.

## Prepare the toolchain

Use Go **1.26.x**, Node.js **22 or newer** with `fetch` and `File`, Python **3**, and npm. Routine documentation checks also require pnpm **10.11.0** (the site uses lockfile version 9 and has no `packageManager` field). Race tests require the Go race detector's supported platform and C toolchain. Keep the committed versions in `examples/web/ts/package.json` and `package-lock.json`: Effect and `@effect/platform-node` **4.0.4**, TypeScript **7.0.2**, and tsx **4.23.15**. A global `tsc` or dependencies installed in another project do not satisfy the gate.

Install the example's locked dependencies explicitly before checking a candidate:

```sh
npm ci --prefix examples/web/ts
```

The gate never runs `npm install`, `npx` or a global package install. Missing Node.js/npm, missing or mismatched package metadata, a missing compiler, or lockfile drift produce a nonzero exit with a repair command. Review an intentional dependency update before regenerating its lockfile; Effect remains pinned to 4.0.4.

For routine feedback, install the site with `pnpm --dir site install --frozen-lockfile`, then run `bash scripts/check.sh`. This runs the Node tooling tests, Python benchmark parser regression, root Go build/vet, documentation lint, public build and public-output scan. `SITE_URL` defaults to the documented public origin and can be overridden. Pull requests and pushes to `main` run this routine check in `.github/workflows/check.yml`; they do not run the complete analyzer race suite. Keep the full gate below as a separate manual check.

Both check scripts default `GOMAXPROCS=2` and `GOMEMLIMIT=2GiB` when unset, preserve explicit values, and append `-p=1` to caller `GOFLAGS` so other options survive while Go package work stays serial. The isolated runner applies the same limits when invoked directly. These are process defaults, not an aggregate memory cap: concurrent package execution previously exhausted a 6 GiB scope, while serial package work with these settings passed. Preserve the external scope and shared lock on this host.

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

Historical full-gate runs took roughly 67–72 minutes on this four-CPU host. Retain the per-test timeout; an outer 2400-second cutoff would kill a known passing run. Reuse successful checks only for the exact same relevant source/dependency inputs, and record any checks supplied by exact-commit CI separately from remaining local checks. Documentation-only changes do not require repeating analyzer race tests; run the routine check instead.

The script runs these existing consumers:

| Area | Checks |
|---|---|
| Release tooling | Node release/source/isolated-runner regressions and `python3 scripts/benchmark_test.py` (real benchmark parser regression) |
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

Update `VERSION` and the corresponding `CHANGELOG.md` entry deliberately. Record the candidate source revision, platform, commands and exit statuses alongside the release review. Changes to implementation, test tooling, or dependencies require the full gate; version-only promotion follows the evidence-reuse boundary above.

Use the [source pinning tools](https://github.com/effectgo/effectgo/blob/main/scripts/verify-source.mjs) to validate the actual consumer lock, generator source, runtime snapshot and optional generator binary. Their [behavior tests](https://github.com/effectgo/effectgo/blob/main/scripts/source-tools.test.mjs) run in the gate. Keep each application's source/runtime lock tied to the exact candidate it consumes; a successful gate in this checkout does not itself update or validate a different application's installed snapshot.

From the checked framework source, update an existing application with:

```sh
node scripts/pin-source.mjs /path/to/app --binary linux-arm64=bin/effectgo
node /path/to/app/scripts/effectgo-source.mjs runtime
node /path/to/app/scripts/effectgo-source.mjs source /path/to/effectgo
node /path/to/app/scripts/effectgo-source.mjs binary /path/to/effectgo/bin/effectgo
```

The updater verifies the old runtime before replacing its owned files and copies the verifier without application-specific changes. It records exact source/runtime inventories and hashes, retains the runtime's MIT `LICENSE` in the pinned copy, and checks the runtime `SOURCE.json` descriptor against the lock. Extra Go/stub build inputs and symbolic links are rejected. Binary inputs must have the declared Linux ELF64 architecture; their hashes are recorded, so release preparation must also run the native CLI and compare its version. Cross-built binaries require their target-platform smoke check before promotion.

An untagged source records `snapshot_kind: "release-candidate"`. Use `--release v0.4.0` only from the clean checkout whose HEAD matches that tag and `VERSION`; this records `snapshot_kind: "release"`. The source-tree digest remains a separate identity from the version and commit. Re-generate and check each consumer's contracts after pinning; application auth, query conversion, CI policies and deployment remain application-owned.

Tagging, publishing artifacts and changing application locks are separate, explicit release actions. This check produces no release and provides no deployment or production signoff.

## Publish the Go modules and documentation

The repository is public at `https://github.com/effectgo/effectgo`. Keep the approved MIT license text in both root `LICENSE` and `rt/LICENSE`, including in the release revision and runtime source pins. pkg.go.dev uses recognized license files to decide whether to display package documentation; without a recognized license it shows limited information. Public source access alone is insufficient for durable proxy caching when no suitable license is detected ([pkg.go.dev license policy](https://pkg.go.dev/license-policy), [Go module services FAQ](https://proxy.golang.org/)).

1. After review and the release checks, publish root tag `v0.4.0` and nested-module tag `rt/v0.4.0` for the same approved source revision. The `rt/` prefix belongs to the Git tag; callers still request `github.com/effectgo/effectgo/rt@v0.4.0`. Root module archives exclude nested modules, so release both independently. Never move historical tags ([version-to-commit mapping](https://go.dev/ref/mod#vcs-version), [module directories](https://go.dev/ref/mod#vcs-dir)).
2. Once the tags are public, request both versions through the public proxy and record successful downloads from a fresh module cache:

   ```sh
   curl -fsS https://proxy.golang.org/github.com/effectgo/effectgo/@v/v0.4.0.info
   curl -fsS https://proxy.golang.org/github.com/effectgo/effectgo/rt/@v/v0.4.0.info
   GOMODCACHE="$(mktemp -d)" GOWORK=off GOPROXY=https://proxy.golang.org \
     go mod download -json github.com/effectgo/effectgo@v0.4.0 github.com/effectgo/effectgo/rt@v0.4.0
   ```

   Keep the download JSON as evidence, remove the temporary cache afterward, and separately record the public CLI install and version smoke check. A warm local cache does not establish that the public proxy served a version. Requesting a tag before pushing it can leave a negative cache entry for up to 30 minutes ([adding a package](https://pkg.go.dev/about#adding-a-package), [proxy protocol](https://go.dev/ref/mod#goproxy-protocol), [module downloads](https://go.dev/ref/mod#go-mod-download), [proxy FAQ](https://proxy.golang.org/)).
3. pkg.go.dev monitors the public module index. Visit the [root module](https://pkg.go.dev/github.com/effectgo/effectgo@v0.4.0) and [runtime module](https://pkg.go.dev/github.com/effectgo/effectgo/rt@v0.4.0); if either is missing, use **Request** on its page. Confirm the version and license display after indexing. A GitHub Release or proxy response does not itself prove documentation is visible, and indexing has no fixed completion deadline in the documented procedure ([adding a package](https://pkg.go.dev/about#adding-a-package)).

This is the publication procedure, not a record that it has run. Record observed download, install, documentation, plugin-build and editor checks separately; success in one route does not establish the others.
