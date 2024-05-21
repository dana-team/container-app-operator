# container-app-operator

The `container-app-operator` is an operator that reconciles `Capp` CRs.

`Capp` (or ContainerApp) provides a higher-level abstraction for deploying containerized Serverless workload, making it easier for end-users to deploy workloads on Kubernetes without being knowledgeable in Kubernetes concepts, while adhering to the standards required by the infrastructure and platform teams without any extra burden on the users.

The operator uses open-source projects, such as [`knative-serving`](https://github.com/knative/serving), [`logging-operator`](https://github.com/kube-logging/logging-operator), [`nfspvc-operator`](https://github.com/dana-team/nfspvc-operator) and [`external-dns`](https://github.com/kubernetes-sigs/external-dns) to create an abstraction for containerized workloads.

## Run Container Service

The `container-app-operator` project can work as a standalone solution, but is mostly used together with the [`rcs-ocm-deployer` project](https://github.com/dana-team/rcs-ocm-deployer), which allows deploying `Capp` workloads in a multi-cluster set-up, using the `OCM` (Open Cluster Management) open-source project.

## High Level Architecture

![Architecture](images/capp-architecture.svg)

1. The `capp controller` reconciles the `Capp` CRs in the cluster and creates (if needed) a `Knative Service` (`ksvc`) CR, a `DommainMapping` CR, and `Flow` & `Output` CRs for every Capp.

2. The `knative controller` reconciles the `ksvc` CRs in the cluster and controls the lifecycle an autoscaler and pods relevant to the `ksvc`.

3. The `nfspvc-operator controller` reconciles the `NFSPVC` CRs in the cluster and creates `PVC` and `PVs` with an external NFS storage configuration (bring your own NFS).

4. The `external-dns controller` reconciles the `DNSEndpoint` CRs in the cluster and creates DNS A Records in the pre-configured DNS provider (bring your own DNS provider).

5. The `logging-operator controller` reconciles the `Flow` and `Output` CRs in the cluster and collects logs from the pods' `stdout` and sends them to a pre-existing `Elasticsearch` or `Splunk` index (bring your own indexes).


## Feature Highlights

- [x] Support for autoscaler (`HPA` or `KPA`) according to the chosen `scaleMetric` (`concurrency`, `rps`, `cpu`, `memory`) with default settings.
- [x] Support for HTTP/HTTPS `DomainMapping` for accessing applications via `Ingress`/`Route`.
- [x] Support for `DNS A Records` lifecycle management based on the `hostname` API field.
- [x] Support for all `Knative Serving` configurations.
- [x] Support for exporting logs to an `Elasticsearch` index.
- [x] Support for changing the state of `Capp` from `enabled` (workload is in running state) to `disabled` (workload is not in running state).
- [x] Support for external NFS storage connected to `Capp` by using `volumeMounts`.
- [x] Support for `CappRevisions` to keep track of changes to `Capp` in a different CRD (up to 10 `CappRevisions` are saved for each `Capp`)

## Getting Started

### Prerequisites

1. A Kubernetes cluster (you can [use KinD](https://kind.sigs.k8s.io/docs/user/quick-start/)).

2. `Knative Serving` installed on the cluster (you can [use the quickstart](https://knative.dev/docs/getting-started/quickstart-install/)).

3. `NFSPVC Operator` installed on the cluster (you can [use the `install.yaml`](https://github.com/dana-team/nfspvc-operator/releases)).

4. `External DNS` installed on the cluster (you can [use the Helm Chart](https://artifacthub.io/packages/helm/external-dns/external-dns)).

5. `Logging Operator` installed on the cluster (you can [use the Helm Chart](https://kube-logging.dev/docs/install/#deploy-logging-operator-with-helm)).

`Knative Serving`, `Logging Operator`, `NFSPVC Operator` and `External DNS` can also be installed by running:

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

### Change target autoscaler default values

To change the target values a `configMap` with the name `autoscale-default` in the namespace `capp-operator-system` needs to be created.

The `configMap` should contain the scale metric types as keys and for the value the desired target values.

The `configMap` will affect the `ksvc` autoscale target value annotation `autoscaling.knative.dev/target`.

#### Example

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: autoscale-defaults
  namespace: capp-operator-system
data:
  rps: "200"
  cpu: "80"
  memory: "70"
  concurrency: "10"
```

### Enable Persistent Volume extension in Knative

In order to use `volumeMounts` in `Capp`, `Knative Serving` needs to be configured to support volumes. This is done by adding the following lines to the `ConfigMap` of name `config-features` in the `Knative Serving` namespace:

```yaml
kubernetes.podspec-persistent-volume-claim: enabled
kubernetes.podspec-persistent-volume-write: enabled
```

It's possible to use the following one-liner:

```bash
$ kubectl patch --namespace knative-serving configmap/config-features --type merge --patch '{"data":{"kubernetes.podspec-persistent-volume-claim": "enabled", "kubernetes.podspec-persistent-volume-write": "enabled"}}'
```

### Configuring External DNS

`Capp` is tested using the `RFC2136 provider`. After installing the operator, it is needed to :

1. Create a `ConfigMap` called `krb5-config` with the Kerebros Configuration, as explained the `external-dns` [docs](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/rfc2136.md#kerberos-configuration).
2. Modify the `external-dns` deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns
  namespace: external-dns-system
spec:
  # Removed for brevity
  template:
    spec:
      containers:
      - args:
        - --source=crd
        - --crd-source-apiversion=externaldns.k8s.io/v1alpha1
        - --crd-source-kind=DNSEndpoint
        - --provider=rfc2136
        - --rfc2136-gss-tsig
        - --rfc2136-host=<HOST>
        - --rfc2136-port=53
        - --rfc2136-zone=<ZONE>
        - --rfc2136-kerberos-username=<USERNAME>
        - --rfc2136-kerberos-password=<PASSWORD>
        - --rfc2136-kerberos-realm=<REALM>
        - --rfc2136-tsig-axfr
        - --events
        - --interval=10h
        - --txt-owner-id=dana-team
        - --txt-prefix=capp-
        volumeMounts:
          - mountPath: /etc/krb5.conf
            name: kerberos-config-volume
            subPath: krb5.conf
        # Removed for brevity
      volumes:
        - configMap:
            defaultMode: 420
            name: krb5-config
          name: kerberos-config-volume
      # Removed for brevity
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
            volumeMounts:
              - name: test-nfspvc
                mountPath: /data
  routeSpec:
    hostname: capp.dev
    tlsEnabled: true
    tlsSecret: cappTlsSecretName
  volumesSpec:
    nfsVolumes:
      - server: test
        path: /test
        name: test-nfspvc
        capacity:
          storage: 200Gi
  logSpec:
    type: elastic
    host: 10.11.12.13
    index: main
    user: elastic
    passwordSecret: es-elastic-user
  scaleMetric: concurrency
  state: enabled
```
