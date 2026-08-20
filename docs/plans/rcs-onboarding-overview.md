# RCS Platform — Architecture Overview

## Table of Contents

- [What is RCS?](#what-is-rcs)
- [The Problem RCS Solves](#the-problem-rcs-solves)
- [Who Uses It?](#who-uses-it)
- [Platform Components](#platform-components)
- [How It All Fits Together](#how-it-all-fits-together)
- [What a Capp Gives You](#what-a-capp-gives-you)
- [Platform Configuration](#platform-configuration)
- [Authentication](#authentication)
- [Deployment Overview](#deployment-overview)
- [Design Principles](#design-principles)
- [Glossary](#glossary)

## What is RCS?

RCS (Run Container Service) is a Container-as-a-Service platform. It lets users deploy and manage containerized applications on Kubernetes **without needing to know Kubernetes**.

Users interact with a single concept — the **Capp** (Container Application). Behind the scenes, the platform handles everything: running the container, scaling it up and down, giving it a URL, securing it with HTTPS, collecting logs, connecting storage, and wiring up event triggers.

## The Problem RCS Solves

Deploying a containerized application on Kubernetes today requires deep expertise. A developer who just wants to run their app must deal with:

- **Infrastructure complexity** — writing and maintaining multiple configuration files that describe how the app runs, how it's exposed, and how it connects to other services
- **Autoscaling configuration** — choosing a scaling strategy, tuning thresholds, and handling scale-to-zero
- **Networking and TLS** — configuring DNS records, obtaining certificates, setting up traffic routing
- **Observability** — setting up log pipelines, connecting to centralized storage, filtering per-app logs
- **Storage** — provisioning persistent volumes, configuring network file mounts, managing capacity
- **Event-driven patterns** — wiring up message queues or scheduled triggers to an application
- **Consistency** — every team solving these problems differently, leading to drift and maintenance burden

RCS eliminates this by providing **one resource** (Capp) that expresses intent ("run this image, scale on CPU, give it this hostname, ship logs here") and a platform that translates that intent into fully managed infrastructure. The developer never touches the underlying Kubernetes resources.

For platform teams, the challenge is the opposite: how to give developers self-service without losing control over resource limits, security policies, and naming standards. RCS solves this with **CappConfig** — a single policy resource that enforces guardrails across all workloads.

## Who Uses It?

- **Application developers** — deploy and manage their workloads via the web console or CLI without touching command-line Kubernetes tools or YAML
- **Platform teams** — configure cluster-wide policies (scaling limits, allowed domains, resource defaults) once, and the platform enforces them automatically

## Platform Components

| Service | What it does |
|---------|-------------|
| **Web Console** (`capp-frontend`) | A browser-based UI where users sign in, create apps, and monitor their status |
| **API Server** (`capp-backend`) | The central service that handles user authentication, connects to clusters, and exposes a REST API |
| **Operator** (`container-app-operator`) | Runs inside the Kubernetes cluster and automatically provisions all infrastructure for each Capp |
| **CLI** (`cappctl`) | A command-line tool offering the same capabilities as the web console |

## How It All Fits Together

> **Note:** The diagram below is a text representation. When importing to Confluence, replace it with a draw.io or image-based diagram for best rendering.

```
  User
   │
   ▼
┌──────────────────┐         ┌──────────────────┐         ┌────────────────────────┐
│   Web Console    │────────▶│    API Server    │────────▶│   Kubernetes Cluster   │
│   or CLI         │         │                  │         │                        │
└──────────────────┘         └──────────────────┘         │   Operator watches     │
                                                          │   for Capp changes     │
                                                          │        │               │
                                                          │        ▼               │
                                                          │   Provisions:          │
                                                          │   • Running container  │
                                                          │   • Public URL + HTTPS │
                                                          │   • Log collection     │
                                                          │   • Storage            │
                                                          │   • Event triggers     │
                                                          └────────────────────────┘
```

### Step by Step

1. A user signs into the **web console** (or uses the CLI)
2. They create a Capp — specifying an image, scaling preferences, and optional features (custom domain, logging, etc.)
3. The **API server** authenticates the user and sends the Capp definition to the cluster
4. The **operator** detects the new Capp and automatically creates everything needed to run it
5. The application is live — with autoscaling, a URL, log collection, and any other requested features
6. Status and health information flows back to the user through the same path

## What a Capp Gives You

When a user creates a Capp, the platform manages all of the following automatically:

| Feature | Description | Problem it solves |
|---------|-------------|-------------------|
| **Container Runtime** | Runs the application container with automatic restarts and health monitoring | No manual infrastructure setup or lifecycle management |
| **Autoscaling** | Scales the number of running instances up or down based on traffic or resource usage — can even scale to zero when idle | No need to configure scaling rules or worry about cost during idle periods |
| **Custom Domain + HTTPS** | Assigns a public URL with a custom hostname and automatically provisions a TLS certificate for secure access | No manual DNS, certificate, or traffic routing setup |
| **Log Collection** | Captures application output and ships it to a centralized log store for searching and debugging | No need to build or maintain a logging pipeline per application |
| **Persistent Storage** | Mounts shared network storage for applications that need to read/write files | No manual storage provisioning or mount configuration |
| **Event Triggers** | Sends scheduled messages (cron-like) or consumes messages from a message queue, delivering them to the application as HTTP requests | No need to write consumer code or manage message infrastructure |
| **State Control** | Pause or resume the application without deleting its configuration | Safely stop workloads during maintenance without losing setup |
| **Revision History** | Keeps a history of configuration changes, so you can see what was changed and when | Built-in audit trail without external tooling |

## Platform Configuration

Platform administrators define cluster-wide policies through a configuration resource. This controls:

- **Scaling limits** — maximum replicas, default scaling thresholds
- **Domain rules** — which hostnames are allowed, DNS zone, certificate issuer
- **Resource defaults** — default CPU and memory allocations for new Capps
- **Operational limits** — e.g. maximum consumers per message queue source

This means users get sensible defaults without needing to know about infrastructure details, while the platform team maintains guardrails.

### How Policies Help

| Scenario | Without RCS | With RCS |
|----------|-------------|----------|
| A developer requests too many replicas | Manual review and back-and-forth | Automatically capped by the configured max |
| A team uses an unauthorized domain | Discovered after deployment, causes issues | Rejected immediately with a clear error |
| New Capps have no resource limits | Risk of noisy neighbors | Defaults applied automatically from policy |

## Authentication

The platform supports multiple ways to authenticate users, depending on the environment:

| Mode | When to use |
|------|-------------|
| **OpenShift OAuth** | When running on OpenShift — users sign in with their existing corporate credentials (SSO) |
| **Dex (Identity Provider)** | When running on standard Kubernetes — username/password login via an external identity provider |
| **Token-based** | For automation, CI/CD pipelines, or service-to-service communication |

## Deployment Overview

The platform runs inside a Kubernetes cluster:

- The **operator** is deployed as a package in its own namespace
- The **API server** runs as a container, configured with cluster connection details
- The **web console** is a static web application served by any HTTP server
- **User workloads** (Capps) run in their own namespaces, isolated from the platform

The operator relies on several infrastructure components being present in the cluster (serverless runtime, certificate management, log collection, DNS management). These are all installable as a bundle provided with the platform.

## Design Principles

| Principle | How RCS applies it |
|-----------|-------------------|
| **Single abstraction** | One resource (Capp) replaces 5–10 Kubernetes resources a developer would normally manage |
| **Separation of concerns** | Users define *what* they want; the platform decides *how* to achieve it |
| **Policy as configuration** | Platform teams set rules once; enforcement is automatic and consistent |
| **Bring your own infrastructure** | RCS integrates with your existing DNS, certificate, storage, and logging systems — it doesn't replace them |
| **Progressive disclosure** | Simple Capps need only an image name; advanced features are opt-in |
| **Status transparency** | Every provisioned subsystem reports its health back to the user through a unified status |

## Glossary

| Term | Meaning |
|------|---------|
| **Capp** | Container Application — the single resource a user creates to deploy a workload |
| **Operator** | A program running inside Kubernetes that watches for Capp changes and provisions infrastructure automatically |
| **API Server** | The backend service that handles authentication and forwards requests to clusters |
| **CappConfig** | The cluster-wide configuration that defines defaults and limits for all Capps |
| **cappctl** | The command-line tool for interacting with the platform |
| **Namespace** | A Kubernetes concept for isolating resources — think of it as a project or team folder |
| **Autoscaling** | Automatically adjusting the number of running instances based on load |
| **Scale to zero** | Shutting down all instances when there's no traffic, and starting them again on demand |
