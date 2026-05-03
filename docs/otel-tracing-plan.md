# OpenTelemetry traces — plan

**Program goal:** Better visibility into **Capp reconciliation churn** using traces (per-reconcile duration, **where time goes inside a reconcile**, errors, sampling).

**Milestone 1 (first delivery)** below is **work to implement** (steps 1–9). The **outcome** and **Elastic UI** sections describe what exists **after** that work is done—not the current repo state. **Further tracing work** is under *Next steps* (reference only).

---

## Milestone 1 — first delivery

### After implementation — outcome

OTLP export when configured; **root + child spans** for Capp reconcile (see steps **4–8**); safe default sampling; validated on **kind** with a **real Elastic** stack; short operator notes.

### After implementation — Elastic UI (one sampled reconcile)

Trace waterfall is a **tree**: each line is a span; **indent = parent/child**; bar length in the UI ≈ **duration** (illustrative ASCII below).

```
span (example durations — illustrative only)          bar ( ∝ time )
────────────────────────────────────────────────────────────────────
capp.reconcile                               120 ms    ██████████████████████████
  capp.sync_application                     95 ms     ████████████████████
    capp.manage.knativeServing              12 ms     ████
    capp.manage.DNSRecord                    8 ms     ███
    capp.manage.certificate                 22 ms     ██████
    …                                        …       …
    capp.sync_status                        18 ms     ██████
```

In Elastic, **each row’s duration** is the real measured value for that sampled trace; the **ms numbers above are only an example** of how the waterfall reads. Elastic also shows **attributes** (e.g. namespace, Capp name) and **errors** on failed spans.

### Implementation steps

1. **Step 1 — Deps.** Add OpenTelemetry Go SDK + OTLP HTTP exporter; align versions; `go mod tidy`, `go build ./...`.
2. **Step 2 — Bootstrap.** Small `internal/telemetry` helper: global tracer provider + propagator when an OTLP endpoint env is set; otherwise no-op. Batch export, parent-based ratio sampler (tunable), shutdown hook.
3. **Step 3 — Wire `main`.** Init after logging setup; defer graceful shutdown.
4. **Step 4 — Capp reconcile (root).** In `Reconcile`: start/end **root** span (e.g. `capp.reconcile`), low-cardinality attributes (namespace + Capp name), span status from returned error; pass **`ctx`** into downstream calls so child spans attach.
5. **Step 5 — Child: sync application.** Under the same `ctx`, wrap **`SyncApplication`** in a **child span** (e.g. `capp.sync_application`) so Elastic shows **time in sync** vs the rest of reconcile.
6. **Step 6 — Child: resource managers.** Inside the sync loop, start/end a **child span per** `Manage` call, named from the map key (e.g. `capp.manage.knativeServing`, `capp.manage.DNSRecord`, … — keys are the fixed set: `knativeServing`, `DNSRecord`, `certificate`, `domainMapping`, `syslogNGFlow`, `syslogNGOutput`, `nfsPvc`). Shows **which manager** dominates time each pass.
7. **Step 7 — Child: Capp status.** Under the same `ctx`, wrap **`status.SyncStatus`** in a **child span** (e.g. `capp.sync_status`) so Elastic shows **time in status** separately from sync.
8. **Step 8 — Validate.** Local `go build` / tests; then on **kind** with a **real Elastic** stack: OTLP reaches Elastic, traces show **root + full child tree**. Confirms wiring, auth, sampling, networking.
9. **Step 9 — Operator note.** Short README/runbook: env vars to enable OTLP; how to raise sampling briefly during incidents.

**Out of scope for milestone 1:** CappRevision traces, log–trace correlation, per-`client` / per-API-call spans, webhooks, spans **inside** a single `Manage` implementation (helpers, every sub-call)—add only if still blind after manager-level spans.

---

## Next steps (reference — later work)

| Topic | Notes |
|--------|--------|
| **CappRevision** | Same root (± child) pattern if revision churn must match Capp. |
| **Trace ↔ log** | Put trace/span context on log records (e.g. Elastic) so you can jump from logs to traces. |
| **Finer than managers** | Optional spans inside a hot `Manage` (only if manager-level still isn’t enough). |

---

## Defaults

Tracing **off** without an OTLP endpoint. Prefer **low sampling** in steady state; **full sampling** only briefly when debugging.
