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

## 2. Service Dependencies Readiness

Each Capp child resource must become ready for the Capp to be fully operational. Each subsystem is an independent failure domain that can block the overall Capp Ready condition.

### Target

All subsystems must report a healthy status for the Capp Ready condition to be `True`:

- Knative Service
- Logging (SyslogNG flow/output)
- DNS record
- DomainMapping
- Certificate (cert-manager)
- NFS volumes
- Event sources (Kafka, PingSource)

### Measurement

Capp Ready condition status and reason. When a subsystem is not ready, the reason identifies the blocking subsystem.

### Exclusions

Failures caused by user misconfiguration or unavailability of user-managed external services are excluded.

---

## 3. Request Routing Latency

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
- Knative activator queue time (when pods are running)

Measured as the difference between ingress-reported request duration and application-reported request duration.

---

## 4. Scale Lag

Time from an autoscaling metric threshold breach until additional replicas are ready to serve traffic.

### Target

| Percentile | Autoscaler | Target |
|---|---|---|
| p95 | KPA (concurrency, rps) | ≤ 30s |
| p95 | HPA (cpu, memory) | ≤ 60s |

### Measurement

Time from when the autoscaler decides to scale up until the new pod(s) pass their readiness probe and begin receiving traffic. KPA targets are tighter because Knative's pod autoscaler reacts at request level, while HPA relies on metrics polling intervals.

---

## 5. Backend & Frontend Availability

Availability of the Capp management plane — the backend API and web console.

### Target

| Component | Target |
|---|---|
| capp-backend | 99.9% availability |
| capp-frontend | 99.9% availability |

### Measurement

- **Backend:** Percentage of responses with HTTP 2xx status codes.
- **Frontend:** Percentage of responses with HTTP 2xx or 3xx status codes.
