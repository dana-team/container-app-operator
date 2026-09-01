# Capp Platform SLO — Gaps & Action Items

Actionable gaps identified during SLO definition. Each item can be addressed by the platform team.

---

## 1. Capp Readiness Time

| Item | Mitigation |
|---|---|
| Probe parameter validation | Enforce upper bounds on readiness probe timing via admission control |
| Image size enforcement | Limit maximum container image size via admission control |

## 2. Request Routing Latency

No gaps — platform networking components (ingress, Knative networking, DomainMapping, TLS termination) are infrastructure-managed.

## 3. Scale Lag

| Item | Mitigation |
|---|---|
| No per-Capp control over scale-up speed | Consider exposing autoscaler responsiveness tuning if tighter scale lag targets are needed |

## 4. Service Dependencies Readiness

| Item | Mitigation |
|---|---|
| No per-subsystem readiness metrics | Expose per-subsystem transition time as Prometheus metrics for independent alerting |

## 5. Backend & Frontend Availability

No gaps — backend exposes `/healthz`, `/readyz`, Prometheus metrics, and optional OTLP tracing. Frontend is a static bundle with no server-side state.
