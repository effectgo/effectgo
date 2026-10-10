---
title: "P7b: the memory of the standalone run, and field sources for HTTP bodies"
description: "The standalone run was killed at a 6 GiB cap on arcteryx-console. Two analyzers that required the inspector kept one inspector per dependency package alive; removing the requirement brings the peak under the cap with identical output. A field source stub form makes a read of http.Request.Body or http.Response.Body a network read."
---

### Memory of the standalone run (P7b, part A)

Branch `feat/body-fields-memory`, from `feat/writer-effects` (P7). Two items: the standalone run `effectgo -effects.report -errset.report ./...` was killed at a 6 GiB cap on the arcteryx-console snapshot, with the P6 and the P7 binaries, and a field source rule for HTTP bodies.

**Measurements.** Peak memory is the cgroup `memory.peak` of the run's scope, in GiB (2^30 bytes), and the wall time in seconds. The runs were one at a time under the shared lock, with `MemoryMax=10G` for diagnosis. The P3 binary is built from commit `4763d2f` (its stub set is its own five files, given with `-stubs`). P6 is commit `3d3a67c` and P7 is `eb99b74`. The `-2a` binaries are P6 or P7 with the standalone analyzer list cut to `effects` and `errset` (the list that P3 ran), built from those commits.

| binary | analyzers | `-test` | project | peak GiB | wall s |
|---|---|---|---|---|---|
| P3 `4763d2f` | effects, errset | default (no effect, see below) | arcteryx-console | 4.95 | 23.6 |
| P3 `4763d2f` | effects, errset | false | arcteryx-console | 4.78 | 17.5 |
| P6 `3d3a67c` | all four | default (true) | arcteryx-console | 6.43 | 23.4 |
| P6 `3d3a67c` | all four | false | arcteryx-console | 6.31 | 25.9 |
| P6 `3d3a67c`, P3 stubs | all four | false | arcteryx-console | 6.48 | 21.5 |
| P6 `3d3a67c` | effects, errset | default | arcteryx-console | 5.17 | 23.4 |
| P6 `3d3a67c` | effects, errset | false | arcteryx-console | 4.95 | 17.8 |
| P6 `3d3a67c` | effects, errset, mustuse | false | arcteryx-console | 5.83 | 25.4 |
| P6 `3d3a67c` | effects, errset, exhaustive | false | arcteryx-console | 6.06 | 17.7 |
| P7 `eb99b74` | all four | default | arcteryx-console | 6.52 | 25.0 |
| P7 `eb99b74` | all four | false | arcteryx-console | 6.19 | 32.1 |
| P7 `eb99b74` | effects, errset | default | arcteryx-console | 5.16 | 27.1 |
| fix `2176200` | all four | default | arcteryx-console | 5.35 | 19.6 |
| fix `2176200` | all four | false | arcteryx-console | 5.17 | 26.1 |
| P3 `4763d2f` | effects, errset | false | kuzma-wms | 4.09 | 22.8 |
| P7 `eb99b74` | all four | default | kuzma-wms | 5.88 | 19.3 |
| fix `2176200` | all four | default | kuzma-wms | 4.58 | 22.8 |
| fix `2176200` | all four | false | kuzma-wms | 4.20 | 24.7 |
| P7 `eb99b74` | all four | default | order-mgmt | 5.58 | 18.8 |
| P7 `eb99b74` | all four | false | order-mgmt | 4.48 | 15.2 |
| fix `2176200` | all four | default | order-mgmt | 4.20 | 26.3 |
| fix `2176200` | all four | false | order-mgmt | 3.40 | 14.8 |

The P6 and P7 peaks are above the 6 GiB cap, so the cap kills them; the arcteryx-console P7 default run completed under the 10 GiB diagnostic cap only. The wall times are noisy: other runs share the machine, and the lock serializes only the heavy jobs.

**The P3 comparison is not like for like.** The P3 standalone run was the multichecker with `effects` and `errset` only. P6 added `mustuse` and `exhaustive` to the standalone run. The `-test` flag of the P3 binary is listed as true, but its output is identical with `-test=false`, so P3 never analysed test variants; its default is effectively test-off.

**Root cause.** The standalone run is `checker.Analyze` over all four analyzers, and the graph it builds is kept until the diagnostics are printed. The checker runs an analyzer that has facts on every package in the dependency closure, the standard library and third-party packages included, and each action keeps the `ResultOf` of its pass (x/tools `go/analysis/checker`, `Analyze` and `Action.pass`). `mustuse` and `exhaustive` both have facts and both required `inspect`, so one `inspector` was built and retained for each package of the closure. The effect is measured, not profiled: the two analyzers alone cost +0.9 GiB (`mustuse`) and +1.1 GiB (`exhaustive`), and the two together cost +1.4 GiB. The `effects` and `errset` analyzers have no `Requires`, so the P3 run never built an inspector.

The other suspects were measured and are small. Test variants are 21 more packages on arcteryx-console and cost 0.2 GiB there, 0.4 GiB on kuzma-wms and 0.8 GiB on order-mgmt. The new driver is not the cause: the two-analyzer P6 build (5.17 GiB default, 4.95 GiB with `-test=false`) is within 0.2 GiB of P3. The P7 via and stored evaluation adds nothing measurable (P6 and P7 with the two analyzers both peak at 5.16 to 5.17 GiB). The stub set does not matter (P6 with P3's stubs peaks at 6.48 GiB against 6.31 GiB with its own).

**Fix.** `mustuse` and `exhaustive` no longer require `inspect`. Each walks `pass.Files` with `ast.Inspect` and the statements it checks come in the same order as the inspector gave. `mustuse` keeps its `errset` requirement, which it reads through `ResultOf`. The per-statement body of `mustuse` is now the named function `checkDiscarded`, so the same code runs in both paths. Commit `2176200`.

**Results before and after.** The diagnostics are the same: arcteryx-console is byte for byte identical to P7 (`cmp` on stdout and stderr), and kuzma-wms and order-mgmt are identical after sorting the lines. Arcteryx-console peaks at 5.35 GiB (from 6.52 GiB), kuzma-wms at 4.58 GiB (from 5.88), and order-mgmt at 4.20 GiB (from 5.58). Against P3 on the same test setting, the fixed binary is within the target of 20 percent: arcteryx-console 5.35 GiB default against 4.95 GiB for P3 (+8 percent), and 5.17 GiB with `-test=false` against 4.78 GiB (+8 percent); kuzma-wms 4.20 GiB with `-test=false` against 4.09 GiB (+3 percent).

**Tests.** The `mustuse` and `exhaustive` package tests pass, and so does the full suite (see the verification section).

### Field sources (P7b, part B)

**The problem.** In the dogfood project `reservations`, `httpapi.decode` reads a request body with `json.NewDecoder(r.Body).Decode(dst)`. The field `Body` has type `io.ReadCloser`, so the reader is an interface and the analysis reported `unknown via json.(*Decoder).Decode → io.Reader.Read`. By the net/http contract, the body of a request, and the response body of a client, is network data (`net/http/request.go:174-188` and `net/http/response.go:55-72`). A rule that says so is a stub about a field, not about a function.

**The form.** A stub rule `effects field:(pkgpath.Type).Field <effects>` gives the fixed effects of the field of a named struct type. The shipped rules are

```text
effects field:(net/http.Request).Body net
effects field:(net/http.Response).Body net
```

in `stubs/stdlib.stubs`, with the doc lines cited. The parser (`internal/stubs`) reads the target with `ParseFieldTarget`, and rejects the other verbs, pointers, the predeclared error type, a missing field, and via or stores items on a field target. Each error names the file and the line.

**Where it applies.** The rule applies in three places, and nowhere else:

- a method called on the field value, such as `r.Body.Read(p)`, which takes the fixed effects instead of the interface contract or `unknown` (`methodOfField` in `effects/fieldsource.go`, called from `collectCall`);
- an argument of a via item, or the stored value of a constructor, when the argument is the field load: `io.ReadAll(r.Body)`, `json.NewDecoder(r.Body)` (`argViaHits`, which is also the path of `via:stored`);
- the same, through a local variable with exactly one assignment, from the field load, that is not address-taken and has an interface or pointer type: `body := r.Body`.

The field owner is the struct that declares the field, found through the embedding path, so a promoted field of an embedded `*http.Request` matches the rule of `Request`. A field of another struct type with the same name does not match, and a local that is reassigned or whose address is taken is not trusted.

**What it does not do.** A write to the field is not tracked: a body assigned in the same function and then read is still charged with the rule, which over-approximates an in-memory reader. The rule is the source's claim about the value; a body that holds a file would be charged `net` too. These are recorded in [limitations](../design/limitations.md#effects) and in [the stub reference](../reference/stubs.md#field-sources).

**Effects on the snapshots.** Compared with the P7 binary, with the part A fix already in:

| project | unknown in effects | changed effect lines | notes |
|---|---|---|---|
| arcteryx-console | 27.6 to 26.4 percent | 9 | `HTTPSender.Send`, `Client.post` and the sitemap fetchers lose `unknown` |
| kuzma-wms | 40.9 to 40.3 percent | 18 | `readBody` in tests and `uploadWebDAVFile` lose `unknown` |
| order-mgmt | 30.6 to 30.2 percent | 31 | `decodeJSONBody` is `{net}` now (was `{net, unknown}`); several `readBody` helpers too |

Some lines gain `net` where the code reads or closes a response body: `connectViaHTTPProxy` calls `resp.Body.Close()` (`order_import/simple_proxy_mailbox.go:170`), and its callers `support_mail` `connect` and `Send`, and `completion_discord` `RunOnce`, inherit it. Several test helpers gain `net` where a fake client returns an in-memory body. The second kind is the over-approximation that the net/http contract allows.

### Verification

The acceptance checks, with the commands and results:

- The unit tests of the field source: `effects/testdata/src/fieldsrc` (`TestFieldSources`), covering a direct read (`ReadAll`, `DecodeDirect`), a local (`DecodeLocal`), a stored decoder (`DecodeStored`, `DecodeStoredLocal`), a response body (`DecodeResponse`), a promoted field (`DecodePromoted`), a field of another type (`DecodeOther`, unknown), a reassigned local (`DecodeReassigned`, unknown), an address-taken local (`DecodeAddr`, unknown) and an allowed function (`DecodeAllowed`, its fact is `{net}`).
- The parser tests: `TestParseFieldSourceTargets` and `TestParseFieldSourceErrors`, with the file and line of each malformed form.
- The disciplined corpus, read-only: TP 17, FN 0, FP 0.
- The reservations dogfood: no findings (P7 reported `decode`). Its mutants 1 to 7 are caught (exit 3) with the diagnostics of the P7 run, and mutant 8 is silent (exit 0).
- The importer dogfood: exactly `report.Watch` and `report.Final`, both `allow spawn,time` with an `io.Writer` written through `fmt.Fprintf`. Mutants 1, 2, 3, 5, 6 and 7 are caught; mutant 4 is not caught, as before, since it is a `math/rand` source the stub set does not name.
- The real-world runs under the 6 GiB cap, default flags: arcteryx-console 5.22 GiB and 21.8 s, kuzma-wms 4.50 GiB and 27.1 s, order-mgmt 4.30 GiB and 25.5 s. All three exit 3 with their findings.

The findings count is the same as in P7 on each snapshot (1631, 2843 and 5382). The changed lines are the effect lines listed above.
