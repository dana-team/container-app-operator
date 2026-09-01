# Capp Platform — Service Level Objectives

## Overview

This document defines the Service Level Objectives (SLOs) for the Capp platform. Each SLO specifies a measurable target and how it is measured.

---

## 1. Capp Readiness Time

Time from Capp creation (or update that triggers a new revision) until the Capp is ready and serving traffic.

### Target

| Percentile | Scenario | Target |
|---|---|---|
| p95 | Image cached on node | ≤ 60s |
| p95 | Image pull required | ≤ 120s |

### Measurement

Time from Capp resource creation until the `Ready` condition becomes `True`. The Ready condition reflects the health of all child resources — Knative Service, logging, routing, certificates, volumes, and event sources.

---

## 2. Request Routing Latency

Platform-owned overhead added to each HTTP request, from client initiation until the request reaches the application container. This excludes application processing time.

### Target

| Percentile | Target |
|---|---|
| p95 | ≤ 50ms |

### Measurement

Time spent in the platform networking path before the request enters the application container. This includes:

- Ingress controller routing
- Knative networking layer
- DomainMapping resolution
- TLS termination (when `tlsEnabled: true`)
- Knative activator queue time (when pods are running; scale-from-zero latency is covered in section 3)

Measured as the difference between ingress-reported request duration and application-reported request duration.

---

## 3. Scale Lag

Time from an autoscaling metric threshold breach until additional replicas are ready to serve traffic.

### Target

| Percentile | Autoscaler | Target |
|---|---|---|
| p95 | KPA (concurrency, rps) | ≤ 30s |
| p95 | HPA (cpu, memory) | ≤ 60s |

### Measurement

Time from when the autoscaler decides to scale up until the new pod(s) pass their readiness probe and begin receiving traffic. KPA targets are tighter because Knative's pod autoscaler reacts at request level, while HPA relies on metrics polling intervals.

