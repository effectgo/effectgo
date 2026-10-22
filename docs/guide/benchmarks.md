---
title: Benchmarks and related tools
description: Source-linked feature comparisons and reproducible benchmarks for effectgo, Go vet, Staticcheck, errcheck, and exhaustive, with explicit scope and cache limitations.
---

Several Go analyzers overlap with parts of effectgo. errcheck targets ignored errors; exhaustive and go-sumtype target incomplete switches; Staticcheck and Go vet cover many other correctness checks; NilAway specializes in nil flows. These are useful alternatives or complements for particular checks. This comparison is a selected set, not an exhaustive market survey or a claim that effectgo has no competitors.

The capability descriptions below come from upstream documentation and source reviewed on **2026-10-22**. They are distinct from the measurements under [Results](#results). Upstream performance claims, including NilAway's build-overhead claim, are not benchmark results from this repository.

## Capability overlap

**N/A** means the named property is outside the selected tool/version/configuration's scope. It is not a missed bug. A partial overlap does not mean two analyzers enforce the same contract.

| Property | effectgo | Related tools and scope |
|---|---|---|
| Purity and transitive effects | Infers named effects through calls and checks `pure`/`allow` contracts, with witnesses. Its taxonomy and stubs limit what “pure” proves. [Effects](../reference/effects.md), [limitations](../design/limitations.md#effects). | Staticcheck uses inferred purity for **SA4017**, reporting discarded results of eligible side-effect-free functions. It does not enforce effectgo's named effect budgets or declaration contracts. [SA4017 source][sa4017], [purity source][purity]. |
| Allowed error sets and HTTP mappings | Infers error variants, checks `errors` contracts and handler coverage, and offers framework-specific HTTP mapping checks. These are approximations, including unresolved calls and mapping limitations. [Errors](../reference/errors.md), [HTTP checks](http-checks.md), [limitations](../design/limitations.md#error-sets). | The documented checks of the tools surveyed here do not provide equivalent effectgo error-set/HTTP-status contracts. Go vet's `errorsas` checks misuse of `errors.As`; it does not enumerate allowed return errors. [Vet checks][vet]. |
| Ignored errors | `mustuse` reports bare calls discarding error results unless the callee is proved never to fail or is allowed; `_` is an explicit discard. [Rules](../reference/analyzers.md#mustuse). | **errcheck** directly overlaps. By default it permits `_`; `-blank` also checks blank assignments. It does no subsequent analysis of assigned errors. Staticcheck's **SA4006** can find assigned values overwritten before use, including errors; that is a different check. [errcheck][errcheck], [SA4006][sa4006]. |
| Custom must-use values | A directive marks a named **type**, so bare calls returning that type must use or explicitly discard their result. [Rules](../reference/analyzers.md#mustuse). | Staticcheck **SA4017** can catch an unused result when the callee qualifies as pure; it has no equivalent type marker. Go vet's **unusedresult** checks a configured set of functions and string-returning methods; its `funcs` flag can name user functions, but does not impose a type-wide rule. [SA4017][sa4017], [vet unusedresult][unusedresult]. |
| Integer/string enums | `sealed` opts a named integer/string type into switch coverage checking. A default satisfies coverage. [Rules](../reference/analyzers.md#exhaustive). | **exhaustive** discovers enumerated types and checks expression switches; it also supports map literals. A default does **not** satisfy coverage unless `-d` is set. [exhaustive documentation][exhaustive]. |
| Sealed interfaces/type switches | Checks same-package implementations of interfaces with unexported methods; `sealed` also validates the declaration. [Rules](../reference/analyzers.md#exhaustive). | **go-sumtype** uses `//go-sumtype:decl` on sealed interfaces. A default normally satisfies coverage, except an always-panicking default. **exhaustive v0.16.1** additionally supports `-check=typeswitch`, finding known implementations in the current package/import graph; this is not proof of a globally closed interface, and generic implementations have documented limits. [go-sumtype][sumtype], [exhaustive][exhaustive]. |
| Nil flows/guards | N/A: effectgo's error narrowing does not provide general pointer-nilness checking. [Limitations](../design/limitations.md#error-sets). | **NilAway** tracks nil flows across functions and packages, with inference and documented false positives/false negatives. **Staticcheck v0.8.2 disables SA5011**; other checks such as SA4023 concern specific nil-related mistakes, not general guard coverage. Go vet's `nilfunc` concerns function comparisons, not pointer dereferences. [NilAway][nilaway], [Staticcheck release notes][sc-release], [vet][vet]. |
| Printf format/argument consistency | N/A. Recording an I/O effect does not validate its format arguments. | **Go vet printf** checks format/argument consistency. **Staticcheck SA5009** checks invalid calls to its supported formatting functions; its source notes that printf wrappers are not detected by that check. [Vet][vet], [SA5009 source][sa5009]. |

### What SA4017 actually proves

SA4017 is **not restricted to a standard-library allowlist**. Its prerequisite purity analyzer includes such a list, but also examines user-defined function bodies, follows eligible static callees, and exports/imports purity facts across packages. Its inference is deliberately constrained: for example, it rejects unsupported parameter shapes, interface/dynamic calls, heap allocation and nonlocal stores. SA4017 then reports a statically resolved call whose result has no uses; it excludes certain benchmark code and does not support anonymous callees. [Pinned purity implementation][purity], [pinned SA4017 implementation][sa4017].

Consequently, a discarded result of a user-defined pure function can overlap with custom must-use enforcement. A call to an impure function inside `//effectgo:pure`, with its result used, tests a different property: a transitive effect contract. Treating SA4017 as an equivalent purity-contract checker would misstate its purpose. Their definitions also differ: Staticcheck's purity table includes `time.Now`, whereas effectgo records the `time` effect.

## Pinned tools and output

The third-party pins below were the Go module proxy's latest stable versions on the research date. Their exact `.mod` files all declare **Go 1.26.0**, which Go **1.26.10** satisfies; installation and execution evidence belongs with the run metadata. Pinning avoids silently changing a comparison when `@latest` moves.

| Tool | Exact installation/version | Output consumed by a runner |
|---|---|---|
| effectgo | Public release `v0.3.0`; record the installed binary's module metadata. | Native `-json`: Go analysis JSON for the six default analyzers. HTTP adapter reports are outside this benchmark. [CLI](../reference/cli.md). |
| Go vet | Included with `go1.26.10`; no separate module install. | `go vet -json`: analysis JSON; default is diagnostic text. A nonzero exit can mean findings or a tool/invocation error. [Pinned command documentation][vet]. |
| Staticcheck | `go install honnef.co/go/tools/cmd/staticcheck@v0.8.2` (release **2026.2.2**). [Proxy pin][sc-pin], [module requirements][sc-mod]. | `-f=json`: one JSON object per finding, including `code`, `location` and `message`; it is a stream, not an array. [Formatters][sc-format]. |
| errcheck | `go install github.com/kisielk/errcheck@v1.30.0`. [Proxy pin][ec-pin], [module requirements][ec-mod]. | Plain `file:line:column:` plus source text on stdout; no JSON flag in this CLI. Exit 1 means findings, 2 means failure. [Pinned command source][ec-cli]. |
| exhaustive | `go install github.com/nishanths/exhaustive/cmd/exhaustive@v0.16.1`. [Proxy pin][ex-pin], [module requirements][ex-mod]. | Plain analysis text on stderr; this version's custom driver has **no `-json` flag**. Exit 3 means findings, 1 means analysis/loading failure. Use `-check=switch,typeswitch` to enable both switch categories. [Pinned driver][ex-driver], [options][exhaustive]. |

Go-sumtype and NilAway are researched comparators, **not measured tools** in this run. Their absence from timing/results tables says nothing about their speed or detection quality. NilAway's upstream documentation recommends a modular driver for large projects and describes its standalone checker primarily as an evaluation entry point; driver selection materially changes cache/memory behavior. [NilAway][nilaway].

The API/schema side has other comparators: [Huma](https://github.com/danielgtaylor/huma) builds typed HTTP APIs with JSON Schema/OpenAPI and SDK integrations; [ogen](https://github.com/ogen-go/ogen) generates Go clients, servers and validation from OpenAPI; [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) generates Go client/server/model code. Those overlap with parts of `rt/web` and generation, but this measured suite evaluates static analyzers. It supplies no HTTP throughput or cross-language generator ranking for those projects.

## Prespecified benchmark method

The runner and machine-readable tool pins live in [`scripts/benchmark.py`](../../scripts/benchmark.py) and [`benchmarks/toolchain.json`](../../benchmarks/toolchain.json). The hand-authored corpus lives in [`benchmarks/corpus`](../../benchmarks/corpus). The runner installs exact versions into its new output directory, preserves raw outputs/build metadata and never overwrites an earlier run.

Requirements: Go **1.26.10**, Python **3**, and GNU `/usr/bin/time` on Linux (`apt-get install time` on Debian). From the repository root:

```sh
python3 scripts/benchmark_test.py
mkdir -p "$HOME/.cache/effectgo"
flock "$HOME/.cache/effectgo/heavy.lock" systemd-run --scope -q -p MemoryMax=6G -p MemorySwapMax=0 \
  python3 scripts/benchmark.py --output /tmp/effectgo-comparison --samples 5
```

Choose a new output path for each run. `results.json` contains all observations, `report.md` the summary, and per-tool files the unmodified stdout/stderr and build information. The source/tool pins are independent of the application's dependencies.

The comparison separates **targeted detection** from **end-to-end cost**:

1. Run nine bad/fixed pairs: `ignored_error`, `enum_exhaustiveness`, `pure_transitive`, `error_contract`, `mustuse_value`, `sealed_types`, `nil_guard`, `typed_nil`, and `printf_format`. The bad package contains an intentional target violation; the fixed package makes the corresponding repair. `typed_nil` exercises Staticcheck's active SA4023; `nil_guard` remains an unsupported scope control. Read the corpus expectations and command configuration before looking at results.
2. For each applicable pair/tool, report whether a diagnostic identifies the intended rule and location in the bad member, and whether the fixed member avoids that diagnostic. Preserve unrelated findings separately. Do not treat any nonzero exit as a detection or any load/analysis failure as a clean result. Do not count unsupported cases as false negatives.
3. Report applicability per capability/configuration. In particular, exhaustive's type-switch support must be explicitly enabled, effectgo's enum opt-in must be present, and the pinned Staticcheck has no active SA5011. The nil-guard pair is a scope control for this selected measured set; NilAway's documented capability is not a measured hit.
4. Also analyze the public module **`github.com/google/uuid@v1.6.0`** as a real-source workload for startup/loading/analysis cost and cache behavior. Copy its production-source snapshot with `_test.go` files excluded identically for every tool; record the source/module checksums and analyze `./...`. No reference bug labels are supplied for that module: diagnostic counts are observations, **not** precision, recall or bug totals. [Pinned UUID source][uuid].
5. Keep installation/download time separate from measured analyzer invocation time. Record exact arguments, exits, source and tool versions, Go version, OS/architecture, available CPUs, concurrency settings, repetitions and raw stdout/stderr. Run timed tools sequentially to reduce contention. Report individual samples and median wall time rather than selecting a fastest run.
6. Give each tool fresh `GOCACHE` and `XDG_CACHE_HOME` directories for its one fresh-cache measurement, then run one excluded warmup and five warm measurements. Rotate the first tool each warm round, preserve every sample, and compare medians/ranges. Remove inherited `GOCACHEPROG` and `STATICCHECK_CACHE` overrides. Tool-specific caches differ: a fast warm run can reflect cached findings rather than repeated full analysis.

### Interpretation limits

These eighteen small packages are deliberately constructed examples, **not a representative sample of production bugs**. Their category selection favors the properties being demonstrated. A bad/fixed result establishes behavior for that fixture and command, not population accuracy, soundness, completeness, or a universal winner. A silent repaired example is evidence for that repair, not a production false-positive rate. HTTP mapping is described from sources and is not separately tested by these pairs.

Fresh `GOCACHE` does **not** mean a cold filesystem: the kernel page cache, downloaded modules, executable pages and other system caches may remain warm. Warm-cache behavior also differs because the tools cache different amounts of analysis. Times include each CLI's loading/driver work and enabled checks; broader check sets can legitimately cost more. Loading tests or excluding them must be disclosed in the recorded arguments.

On Linux, GNU `time`'s maximum RSS is a resource-usage high-water mark. It is **not the sum of simultaneously resident sibling processes** or a sampled peak for the whole process tree. A tool that spawns `go`/compiler workers may therefore have a larger total memory footprint than this number shows. Record the metric as reported maximum RSS, with units, rather than total tree memory. [GNU time manual][time], [Linux `ru_maxrss` semantics][rusage].

## Results

Observed on Linux/aarch64, Go **1.26.10**, four available logical CPUs, `GOMAXPROCS=2`, `GOMEMLIMIT=2GiB`, `GOFLAGS="-buildvcs=false -p=1"`, inside a 6 GiB systemd scope. Five warm samples per tool; the host was not dedicated benchmark hardware. The UUID snapshot contains sixteen production `.go` files before platform/build-tag selection. All five tools emitted zero findings on that snapshot; this is a cost workload without bug labels.

| Case | effectgo | Go vet | Staticcheck | errcheck | exhaustive |
|---|---|---|---|---|---|
| ignored_error | Detected; fixed silent | N/A | N/A | Detected; fixed silent | N/A |
| enum_exhaustiveness | Detected; fixed silent | N/A | N/A | N/A | Detected; fixed silent |
| pure_transitive | Detected; fixed silent | N/A | N/A | N/A | N/A |
| error_contract | Detected; fixed silent | N/A | N/A | N/A | N/A |
| mustuse_value | Detected; fixed silent | N/A | N/A | N/A | N/A |
| sealed_types | Detected; fixed silent | N/A | N/A | N/A | Detected; fixed silent |
| nil_guard | N/A | N/A | N/A | N/A | N/A |
| typed_nil | N/A | N/A | Detected; fixed silent | N/A | N/A |
| printf_format | N/A | Detected; fixed silent | Detected; fixed silent | N/A | N/A |

| Tool | Applicable bad cases detected | Controls with findings | Fresh cache (s) | Warm median (s) | Warm range (s) | Warm max RSS (MiB) |
|---|---:|---:|---:|---:|---:|---:|
| effectgo | 6/6 | 0 | 5.814 | 3.197 | 2.680–3.271 | 468.8 |
| go-vet | 1/1 | 0 | 19.228 | 0.123 | 0.106–0.150 | 27.0 |
| staticcheck | 2/2 | 0 | 22.461 | 0.158 | 0.144–0.206 | 25.1 |
| errcheck | 1/1 | 0 | 18.792 | 0.145 | 0.118–0.146 | 25.5 |
| exhaustive | 2/2 | 0 | 2.614 | 1.244 | 1.095–1.441 | 277.2 |

All applicable bad cases were detected, their repaired controls were silent, and there were no other fixture findings. These denominators differ because tool scopes differ. The `nil_guard` case is N/A for every measured version/configuration.

On this workload, effectgo has materially higher warm invocation cost and reported RSS than the other tools. Go vet, Staticcheck and errcheck benefit from their warm caching/loading paths; this experiment does not isolate which phase causes the difference. The fresh-cache column also includes different compilation/loading work. These figures do not establish that any tool is generally faster, more accurate, or a replacement for the others.

The observations, individual samples, binary hashes, UUID module/source hashes and executed input hashes are saved in [the portable run record](../../benchmarks/results/2026-10-22-linux-arm64.json). Raw stdout/stderr, build metadata and the original JSON remain in the output directory. Installation time and the excluded warmup are outside the warm sample medians.

## v0.4.0 standard-library optimization

Profiling the standalone driver found that `errset` built SSA and inferred error facts for standard-library bodies even though its consumers deliberately ignore those facts and use stubs or opaque values. `effects` already skipped those bodies. The implementation now returns a typed empty `errset.Result` for packages recognized by the existing source-layout detector, before SSA construction. User and third-party dependency analysis remains unchanged. No new persistent result cache is involved.

The same UUID production snapshot was measured in six alternating before/after rounds, excluding the first sample per binary. Go 1.26.10, `GOMAXPROCS=2`, `GOMEMLIMIT=2GiB` and serial package work were held constant; profiling was disabled for timing. These are new before/after observations, separate from the released v0.3.0 competitor table above.

| Metric | Before | Optimized | Reduction |
|---|---:|---:|---:|
| Warm standalone median | 2.819 s | 1.257 s | 55.4% |
| Maximum reported RSS across five warm runs | 465.9 MiB | 238.8 MiB | 48.7% |

Native diagnostics and exit codes, contract JSON, generated Web TypeScript and the layer/todo examples were byte-identical in six end-to-end comparisons. A regression checks that standard-library analysis exports no body-derived function/error-free sets. Sampled heap profiles reduced cumulative allocations from approximately 828 MiB to 331 MiB; these allocation totals are not retained heap or RSS. The remaining CPU profile is dominated by package loading and Go type checking, so a further optimization needs separate evidence at that stage.

An independent entry-point baseline measured released standalone at 3.192 s versus an already-cached `go vet -vettool=...` at 0.168 s on this workload. That cache hit is useful for repeated CI/editor checks, but it is not the algorithmic improvement in the table. [Portable samples, binary hashes and semantic comparisons](../../benchmarks/results/2026-10-22-stdlib-optimization.json) retain both measurements.

To collect CPU and heap evidence on a source build with the new standalone flags:

```sh
go build -o bin/effectgo ./cmd/effectgo
# Run from the module you want to analyze; keep profiles outside its source tree.
/path/to/effectgo/bin/effectgo -cpuprofile=/tmp/effectgo.cpu -memprofile=/tmp/effectgo.heap ./...
go tool pprof -top /path/to/effectgo/bin/effectgo /tmp/effectgo.cpu
go tool pprof -top -alloc_space /path/to/effectgo/bin/effectgo /tmp/effectgo.heap
```



[sa4017]: https://github.com/dominikh/go-tools/blob/v0.8.2/staticcheck/sa4017/sa4017.go
[purity]: https://github.com/dominikh/go-tools/blob/v0.8.2/analysis/facts/purity/purity.go
[sa4006]: https://staticcheck.dev/docs/checks/#SA4006
[sa5009]: https://github.com/dominikh/go-tools/blob/v0.8.2/staticcheck/sa5009/sa5009.go
[sc-release]: https://staticcheck.dev/changes/2026.2/
[vet]: https://github.com/golang/go/blob/go1.26.10/src/cmd/vet/doc.go
[unusedresult]: https://github.com/golang/go/blob/go1.26.10/src/cmd/vendor/golang.org/x/tools/go/analysis/passes/unusedresult/unusedresult.go
[errcheck]: https://github.com/kisielk/errcheck/blob/v1.30.0/README.md
[ec-cli]: https://github.com/kisielk/errcheck/blob/v1.30.0/main.go
[exhaustive]: https://github.com/nishanths/exhaustive/blob/v0.16.1/passes/exhaustive/doc.go
[ex-driver]: https://github.com/nishanths/exhaustive/blob/v0.16.1/internal/driver/driver.go
[sumtype]: https://github.com/BurntSushi/go-sumtype/blob/master/README.md
[nilaway]: https://github.com/uber-go/nilaway/blob/main/README.md
[sc-format]: https://staticcheck.dev/docs/running-staticcheck/cli/formatters/
[sc-pin]: https://proxy.golang.org/honnef.co/go/tools/@latest
[sc-mod]: https://proxy.golang.org/honnef.co/go/tools/@v/v0.8.2.mod
[ec-pin]: https://proxy.golang.org/github.com/kisielk/errcheck/@latest
[ec-mod]: https://proxy.golang.org/github.com/kisielk/errcheck/@v/v1.30.0.mod
[ex-pin]: https://proxy.golang.org/github.com/nishanths/exhaustive/@latest
[ex-mod]: https://proxy.golang.org/github.com/nishanths/exhaustive/@v/v0.16.1.mod
[uuid]: https://github.com/google/uuid/tree/v1.6.0
[time]: https://www.gnu.org/software/time/manual/
[rusage]: https://man7.org/linux/man-pages/man2/getrusage.2.html
