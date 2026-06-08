# HLD: Knative KafkaSource for Capp

## Summary

- Add `kafkaSourceConfiguration` to `eventSourcesSpec.sources` (same pattern as `pingSourceConfiguration`).
- Operator owns `KafkaSource` `{capp-name}-{source-name}` → sink to Capp Knative Service (+ per-entry `uri`).
- Customer target: external Kafka, **SASL_PLAINTEXT** + **SCRAM-SHA-256**, no TLS in v1.
- User supplies `bootstrapServers`, `topics`, same-ns Secret; optional `consumerGroup` (default `{capp-name}-{source-name}`); Kafka lifecycle is user-owned.
- Mixed ping + kafka in one Capp: **yes** — separate managers, merged `eventingStatus`.
- Disabled Capp: kafka pauses via `spec.consumers: 0` (CR kept); ping unchanged (known gap).

## Goals / Non-goals

**Goals:** kafka fields on Capp; reconcile KafkaSource (owner ref, labels, orphan cleanup); mixed sources; admission validation (incl. Secret); merged status; pause kafka on disable.

**Non-goals (v1):** TLS/SASL_SSL; cross-ns secrets; `initialOffset`/filters/`consumers` tuning; OAUTHBEARER/MSK IAM; installing Knative Kafka; pausing PingSource on disable.

## Architecture

- **PingSourceManager** / **KafkaSourceManager** — each reconciles only its entries; same `{capp-name}-{source-name}` pattern as ping today.
- **Webhook** — structure + Secret checks + Knative `Validate()`.
- **Status** — merge both managers' `GetStatus`, sort by name → `eventingStatus.eventSources`.
- **Controller** — watch PingSource + KafkaSource Ready → requeue Capp.
- **Prerequisite:** Knative Kafka eventing installed in cluster.

```text
Capp → [PingSourceManager | KafkaSourceManager] → CRs → Knative Service (HTTP)
KafkaSource → SASL_PLAINTEXT → external Kafka
```

**Invariants:** unique `sources[].name` (all types); one type block per entry; URI collisions not blocked.

## Data model / APIs

New block on existing `SourceConfiguration`:

```yaml
kafkaSourceConfiguration:
  bootstrapServers: ["kafka.example:9092"]
  topics: ["orders", "payments"]
  consumerGroup: my-team-orders-consumer  # optional; default: {capp-name}-{source-name}
  secretRef:
    name: kafka-creds
```

| Capp | KafkaSource |
|------|-------------|
| `{capp}-{source}` name | `metadata.name` |
| `bootstrapServers`, `topics` | same |
| `consumerGroup` if set | same |
| `consumerGroup` if omitted | operator sets `{capp-name}-{source-name}` (not Knative UUID) |
| `secretRef.name` | `net.sasl` key refs |
| `uri` | `sink.uri` + `sink.ref` → ksvc |
| `state: disabled` / `enabled` | `consumers: 0` / `1` |

**consumerGroup:** optional on Capp; immutable on KafkaSource after create (Knative rule). Operator always writes an explicit group — never leaves empty for Knative UUID default.

**Secret** (same ns, validated at admission): keys `protocol`=`SASL_PLAINTEXT`, `sasl.mechanism` (v1: `SCRAM-SHA-256`), `user`, `password` — all non-empty. No live broker check at admission.

## Validation (admission)

- Unique names; exactly one of ping \| kafka config per entry; mixed types OK.
- Kafka: non-empty `bootstrapServers`, `topics`; relative `uri`; `consumerGroup` optional.
- Secret: exists, required keys present, values valid; errors cite `sources[i]` + key.
- Knative delegate validation where applicable.

## Workflow

1. Apply → webhook → reconcile both managers → merged status.
2. Disable → ksvc deleted; kafka `consumers: 0`; ping keeps firing.
3. Re-enable → ksvc restored; kafka `consumers: 1`.
4. Remove source / delete Capp → manager orphan cleanup + owner-ref GC.

## Failure modes & Recovery

| Case | Signal |
|------|--------|
| Bad spec / Secret at apply | Webhook deny |
| Secret drift / Kafka auth / broker down | `eventingStatus` Not Ready |
| Disabled Capp | Sink errors (expected) |
| Kafka CRD missing | Kafka entries Not Ready; ping OK |

Fix spec/Secret/cluster; operator re-reconciles.

## Security

- Creds in Secret ref only (same ns). SASL_PLAINTEXT = creds on wire — accepted for v1.
- RBAC: `kafkasources.sources.knative.dev` (mirror pingsources).

## Rollout

Additive CRD field; no migration. `SyncStatus` aggregates both managers (ping-only Capps unchanged in output).

## Alternatives (rejected)

- Delete KafkaSource on disable → use `consumers: 0`.
- Block mixed ping+kafka in v1.
- Knative UUID consumer group default → use `{capp-name}-{source-name}` instead.
- TLS in v1.

## Open questions (LLD)

- Full `sasl.mechanism` enum beyond SCRAM-SHA-256.
- `type` field on `EventSourceStatus`.
- PingSource pause on disable (follow-up).
