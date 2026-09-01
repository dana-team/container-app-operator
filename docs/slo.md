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

