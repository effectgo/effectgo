---
title: "P7: writer and reader effects (via items, stored values, the scope page)"
description: "A pure-declared function that writes through an io.Writer was not reported. Stub via items charge a method of an argument at each call site, stored writers and readers are resolved from their constructor, and the page on what effectgo does not check is added."
---

### Writer and reader effects (P7)

Branch `feat/writer-effects`, from `feat/output-contract`. The second dogfood run found a soundness hole: `fmt.Fprintf(w, ...)` in a function declared `//effectgo:pure` was not reported, while a direct `w.Write` was. The stdlib body is not analysed, so the write through `w` disappeared.

**The mechanism.** A stub value may now contain `via:<argIndex>.<Method>`: the call performs that method on the argument. The effects of the method are evaluated at each call site from the argument's static type: an interface argument is charged as a direct call of the method (its contract, else `unknown`); a concrete argument is charged with the effects of its method (`*bytes.Buffer` is pure, `*os.File` is `fs`, `os.Stdout` is `stdio`). The witness keeps the stub's step: `ToWriter → fmt.Fprintf → io.Writer.Write`. The shipped stubs use it for `fmt.Fprint*`, `fmt.Fscan*`, `io.WriteString`, `io.Copy*`, `io.ReadAll`, `io.ReadFull`, `io.ReadAtLeast`, `log.New`, `log.SetOutput`, `text/template`, `html/template` and `encoding/binary`. The blanket `unknown` of `io.Copy` and its relatives is replaced by these rules.

**Stored values.** A reader or writer that a constructor keeps (`json.NewEncoder(w)`, `csv.NewReader(f)`, `bufio.NewReader(r)`, the gob and xml codecs) is resolved when the method is called on the constructor's result, or on a local variable that has exactly one assignment, from that constructor, and is used only through selectors (other than `Reset` and `Init`, which re-point a bufio reader or writer). The syntax is `stores:<argIndex>` on the constructor and `via:stored.<Method>` on the method. Any other receiver (a field, a parameter, an escaped or reassigned variable) makes the method `unknown`. A tabwriter is not tracked, because its `Init` re-points the writer. A `log.Logger` keeps its taxonomy label `stdio`.

**Errors.** `fmt.Fprintf` and `io.Copy` keep the opaque error set `?`, which is sound. A writer's own error set would need the errset analyzer to evaluate via items; that is not done. `context.Cause` gets the set `context.Canceled | context.DeadlineExceeded | ?`, from its source (`context/context.go:288-306`).

**Scope.** [What effectgo does not check](../guide/what-it-does-not-check.md) lists the properties outside the analysis (races, leaks, deadlocks, cancellation, timing), with the tool for each, and states how the effects of a goroutine and the errors of a callback are attributed.

**Measurements.** The disciplined corpus oracle stays at TP 17, FN 0, FP 0, with the same 17 diagnostics as P6. The probe `ToWriter` is reported with the witness above.

Dogfood, P6 binary against the final binary (the new findings only; every mutant still caught):

- `dogfood2/importer`: `report.Watch` and `report.Final` are reported, both `allow spawn,time` with an `io.Writer` parameter written through `fmt.Fprintf`. These are true findings, the case the dogfood author suspected. `source.ReadRows` (a `csv.NewReader` over an `*os.File`, `allow fs`) is no longer reported.
- `dogfood/reservations`: `httpapi.writeJSON` (`json.NewEncoder(w)` over an `http.ResponseWriter`, `allow net`) is no longer reported. `httpapi.decode` is still reported, as `unknown via json.(*Decoder).Decode → io.Reader.Read`: its reader is `r.Body`, an `io.ReadCloser`, and the analysis does not know it is a network connection. A stub for the request body would be needed to clear it.
- Mutant 8 (the control) keeps its findings: the `decode` finding in `reservations`, and the `Watch`/`Final` findings in `importer`. The P6 binary was silent on them only because `json.Decoder` and `csv.Reader` were modelled as pure.

Real-world metrics on the final binary: `order-mgmt` `pct_unknown` 30.3 to 30.6 (6 functions changed); `kuzma-wms` 40.7 to 40.9 (9 functions changed). Samples: `decodeJSONBody` and `PIRunner.Run` gain `unknown` from an interface reader (`json.Decoder` over `Body`, `bufio.Scanner` over a pipe); `giftreplay.render` gains it from `fmt.Fprintf` to an `io.Writer`; `logging.prettyHandler.Handle` gains it from a stored `io.Writer` field; `middleware.Recovery`, `httpapi.New` and `app.Bootstrap` lose `stdio`, because `log.New` no longer carries the taxonomy label and the writes of the returned logger closure were never charged. `arcteryx-console` could not be measured: the analysis is killed at the 6 GB cap, for both binaries.
