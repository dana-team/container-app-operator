<<<<<<< HEAD
# KUTTL

<img src="https://kuttl.dev/images/kuttl-horizontal-logo.png" width="256">

[![lint](https://github.com/kudobuilder/kuttl/actions/workflows/lint.yml/badge.svg?branch=main)](https://github.com/kudobuilder/kuttl/actions)
[![unit test](https://github.com/kudobuilder/kuttl/actions/workflows/unittest.yml/badge.svg?branch=main)](https://github.com/kudobuilder/kuttl/actions)
[![integration-test](https://github.com/kudobuilder/kuttl/actions/workflows/integration-test.yml/badge.svg?branch=main)](https://github.com/kudobuilder/kuttl/actions)
[![e2e](https://github.com/kudobuilder/kuttl/actions/workflows/e2e.yml/badge.svg?branch=main)](https://github.com/kudobuilder/kuttl/actions)

KUbernetes Test TooL (KUTTL) provides a declarative approach to test Kubernetes Operators.

KUTTL is designed for testing operators, however it can declaratively test any kubernetes objects.

## Getting Started

Please refer to the [getting started guide](https://kuttl.dev/docs/) documentation.

## Resources

Initially Built under the KUDO project, we continue to use that channel for KUTTL.

* Slack Channel: [#kudo](https://kubernetes.slack.com/archives/CG3HTFCMV)
* Google Group: [kudobuilder@googlegroups.com](https://groups.google.com/forum/#!forum/kudobuilder)

## Community Meetings

We have open community meetings every 2nd and 4th Wednesday of the month at 9:00 a.m. PST. (17:00 UTC)

* Agenda and Notes: https://docs.google.com/document/d/1UqgtCMUHSsOohZYF8K7zX8WcErttuMSx7NbvksIbZgg
* Zoom Meeting: https://d2iq.zoom.us/j/443128842
=======
# container-app-operator

The `container-app-operator` is an operator that reconciles `Capp` CRs.
>>>>>>> d0c06005e9644db890b7358c1628ada5c21ecd7a

`Capp` (or ContainerApp) provides a higher-level abstraction for deploying containerized workload, making it easier for end-users to deploy workloads on Kubernetes without being knowledgeable in Kubernetes concepts, while adhering to the standards required by the infrastructure and platform teams without any extra burden on the users.

<<<<<<< HEAD
## Community, Events, Discussion, Contribution, and Support

Learn more on how to engage with the KUDO community on the [community page](https://kudo.dev/community/).

## Contributions

Please read the [contributing guide](https://github.com/kudobuilder/kuttl/blob/main/CONTRIBUTING.md) for details around:

1. Code of Conduct
1. Code Culture
1. Details on how to contribute
=======
The operator uses open-source projects, such as [`Knative Serving`](https://github.com/knative/serving) and [`logging-operator`](https://github.com/kube-logging/logging-operator) to create an abstraction for containerized workloads.

## Run Container Service

The `container-app-operator` project can work as a standalone solution, but is mostly used together with the [`rcs-ocm-deployer` project](https://github.com/dana-team/rcs-ocm-deployer), which allows deploying `Capp` workloads in a multi-cluster set-up, using the `OCM` (Open Cluster Management) open-source project.

## High Level Architecture

![Architecture](images/capp-architecture.svg)

1. The `capp controller` reconciles the `Capp` CRs in the cluster and creates (if needed) a `Knative Service` (`ksvc`) CR, a `DommainMapping` CR, and `Flow` & `Output` CRs for every Capp.

2. The `knative controller` reconciles the `ksvc` CRs in the cluster and controls the lifecycle an autoscaler and pods relevant to the `ksvc`.

3. The `logging-operator controller` reconciles the `Flow` and `Output` CRs in the cluster and collects logs from the pods' `stdout` and sends them to a pre-existing `Elasticsearch` or `Splunk` index.

## Feature Highlights

- [x] Support for autoscaler (`HPA` or `KPA`) according to the chosen `scaleMetric` (`concurrency`, `rps`, `cpu`, `memory`) with default setting of `autoscaling.knative.dev/activation-scale: "3"`.
- [x] Support for HTTP/HTTPS `DomainMapping` for accessing applications via `Ingress`/`Route`.
- [x] Support for all `Knative Serving` configurations.
- [x] Support for exporting logs to `Elasticsearch` and `Splunk` indexes.

## Getting Started

### Prerequisites

1. A Kubernetes cluster (you can [use KinD](https://kind.sigs.k8s.io/docs/user/quick-start/)).

2. `Knative Serving` installed on the cluster (you can [use the quickstart](https://knative.dev/docs/getting-started/quickstart-install/))

3. `Logging Operator` installed on the cluster (you can [use the Helm Chart](https://kube-logging.dev/docs/install/#deploy-logging-operator-with-helm))

`Knative Serving` and `Logging Operator` can also be installed by running:
```bash
$ make prereq
```

### Deploying the controller

```bash
$ make deploy IMG=ghcr.io/dana-team/container-app-operator:<release>
```

#### Build your own image

```bash
$ make docker-build docker-push IMG=<registry>/container-app-operator:<tag>
```

## Example Capp

```yaml
apiVersion: rcs.dana.io/v1alpha1
kind: Capp
metadata:
  name: capp-sample
  namespace: capp-sample
spec:
  configurationSpec:
    template:
      spec:
        containers:
          - env:
              - name: APP_NAME
                value: capp-env-var
            image: 'quay.io/danateamorg/example-python-app:v1-flask'
            name: capp-sample
  routeSpec:
    hostname: capp.dev
    tlsEnabled: true
    tlsSecret: cappTlsSecretName
  logSpec:
    type: elastic
    host: 10.11.12.13
    index: main
    username: elastic
    passwordSecretName: es-elastic-user
    sslVerify: false
  scaleMetric: cpu
```
>>>>>>> d0c06005e9644db890b7358c1628ada5c21ecd7a
