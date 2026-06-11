# Capp User Guide

## Introduction

**Capp** (Container Application) is a Kubernetes Custom Resource that provides a simplified abstraction for deploying containerized serverless workloads. It allows users to deploy applications without requiring deep knowledge of Kubernetes concepts, while automatically managing autoscaling, routing, logging, and storage. For architecture details and operator installation instructions, please refer to the [main README](../README.md).

## Capp CR Specification

### `scaleMetric`
Defines which metric the autoscaler uses. Options: `concurrency` (default, best for HTTP services), `rps` (requests per second), `cpu`, or `memory`. The operator creates an appropriate HPA or KPA autoscaler based on this value.

### `state`
Controls application state: `enabled` (running, default) or `disabled` (suspended but preserves configuration). Use `disabled` for temporary suspension during maintenance or cost savings.

### `configurationSpec`
Defines container specifications including image, environment variables, and resource requirements. Based on Knative's ConfigurationSpec with a `template.spec` containing:
- `containers`: Container definitions (name, image, env, resources, volumeMounts)

At least one container with a valid image is required. Follows standard Kubernetes pod specifications.

### `routeSpec`
Configures custom DNS routing and TLS:
- `hostname`: Custom DNS name (e.g., `myapp.example.com`)
- `tlsEnabled`: Enable HTTPS with automatic certificate management
- `trafficTarget`: Advanced traffic routing for canary/A/B testing
- `routeTimeoutSeconds`: Request timeout duration

When `hostname` is set, the operator creates DomainMapping, CNAMERecord, and optionally a Certificate resource.

### `logSpec`
Configures automatic log shipping to Elasticsearch:
- `type`: Log destination (currently only `elastic`)
- `host`: Elasticsearch host address
- `index`: Elasticsearch index name
- `user`: Username for authentication
- `passwordSecret`: Secret name containing the password

Creates SyslogNGFlow and SyslogNGOutput resources to collect logs from stdout.

### `volumesSpec`
Defines NFS persistent storage volumes with:
- `name`: Volume name (must match `volumeMounts` in container spec)
- `server`: NFS server address
- `path`: Export path
- `capacity`: Storage size (e.g., `200Gi`)

### `eventSourcesSpec`
Attaches Knative Eventing sources to the Capp. Each source in `sources` requires:
- `name`: Unique identifier for this source within the Capp
- **Exactly one** of `pingSourceConfiguration` or `kafkaSourceConfiguration`

**`pingSourceConfiguration`** — trigger on a cron schedule:
- `schedule` (optional, default `"* * * * *"`): Cron expression (e.g., `"*/5 * * * *"`)
- `data` (optional): JSON payload sent with each trigger (e.g., `'{"key":"value"}'`)
- `uri` (optional, on the source entry): Relative HTTP path on the Capp Knative Service (e.g. `"/events/ping"`)

**`kafkaSourceConfiguration`** — consume from Kafka topics:
- `bootstrapServers` (required): Kafka broker addresses
- `topics` (required): Topic names to consume
- `secretRef` (required): Secret in the same namespace with Kafka cluster credentials
- `consumerGroup` (optional): Consumer group ID; defaults to `{capp-name}-{source-name}` when omitted
- `consumers` (optional, default `1`): Number of parallel KafkaSource consumers
- `uri` (optional, on the source entry): Relative HTTP path on the Capp Knative Service (e.g. `"/events/orders"`)

Required Secret keys: `user`, `password`, `sasl.mechanism` (same namespace as the Capp).

Source readiness is reported in `status.eventingStatus.eventSources`.

## How to Use Capp

Step-by-step instructions for common scenarios (assumes the operator is installed).

### Step 1: Create a Basic Capp

Create a simple Capp with just a container and default settings:

```yaml
apiVersion: rcs.dana.io/v1alpha1
kind: Capp
metadata:
  name: my-app
  namespace: my-namespace
spec:
  configurationSpec:
    template:
      spec:
        containers:
          - name: my-app
            image: ghcr.io/myorg/my-app:v1.0.0
  state: enabled
```

Apply it with: `kubectl apply -f my-app.yaml`

### Step 2: Configure Autoscaling

Set `spec.scaleMetric` to: `rps` (high-traffic APIs), `cpu` (CPU-intensive), `memory` (memory-intensive), or `concurrency` (default, concurrent requests).

### Step 3: Add a Custom Domain with TLS

To expose your application with a custom domain and HTTPS:

```yaml
spec:
  routeSpec:
    hostname: myapp.example.com
    tlsEnabled: true
```

### Step 4: Enable Elasticsearch Logging

```yaml
spec:
  logSpec:
    type: elastic
    host: elasticsearch.example.com
    index: my-app-logs
    user: elastic
    passwordSecret: es-password-secret
```

Create the secret first:
```bash
kubectl create secret generic es-password-secret --from-literal=password='your-password' -n my-namespace
```

### Step 5: Mount NFS Volumes

```yaml
spec:
  configurationSpec:
    template:
      spec:
        containers:
          - name: my-app
            image: ghcr.io/myorg/my-app:v1.0.0
            volumeMounts:
              - name: data-volume
                mountPath: /data
  volumesSpec:
    nfsVolumes:
      - name: data-volume
        server: nfs.example.com
        path: /exports/my-app-data
        capacity:
          storage: 100Gi
```

### Step 6: Attach an Event Source

**Ping:**

```yaml
spec:
  eventSourcesSpec:
    sources:
      - name: hourly
        pingSourceConfiguration:
          schedule: "0 * * * *"
          data: '{"trigger":"hourly"}'
```

**Kafka:**

Create a credentials Secret first:

```bash
kubectl create secret generic kafka-creds -n my-namespace \
  --from-literal=user=my-user \
  --from-literal=password=my-password \
  --from-literal=sasl.mechanism=SCRAM-SHA-256
```

```yaml
spec:
  eventSourcesSpec:
    sources:
      - name: orders
        uri: /events/orders
        kafkaSourceConfiguration:
          bootstrapServers:
            - kafka.example:9092
          topics:
            - orders
            - payments
          secretRef:
            name: kafka-creds
```

```bash
kubectl get capp my-app -n my-namespace -o jsonpath='{.status.eventingStatus}'
```

### Step 7: Manage State and Check Status

**Disable/enable application**:
```bash
kubectl patch capp my-app -n my-namespace --type=merge -p '{"spec":{"state":"disabled"}}'  # suspend
kubectl patch capp my-app -n my-namespace --type=merge -p '{"spec":{"state":"enabled"}}'   # resume
```

**Check status**:
```bash
kubectl get capp my-app -n my-namespace              # basic status
kubectl describe capp my-app -n my-namespace         # detailed status
```

The status section includes: `knativeObjectStatus`, `routeStatus`, `loggingStatus`, `volumesStatus`, `eventingStatus`, and `conditions`.

## Practical Examples

### Example 1: Web Application with Custom Domain

```yaml
apiVersion: rcs.dana.io/v1alpha1
kind: Capp
metadata:
  name: web-app
  namespace: production
spec:
  scaleMetric: rps
  state: enabled
  configurationSpec:
    template:
      spec:
        containers:
          - name: web-app
            image: ghcr.io/mycompany/web-app:v2.1.0
            env:
              - name: ENVIRONMENT
                value: production
              - name: PORT
                value: "8080"
            resources:
              requests:
                memory: "256Mi"
                cpu: "200m"
              limits:
                memory: "512Mi"
                cpu: "500m"
  routeSpec:
    hostname: web.mycompany.com
    tlsEnabled: true
    routeTimeoutSeconds: 60
```

Deploys a web application with RPS-based autoscaling, custom HTTPS domain, and resource limits.

### Example 2: Application with Logging, NFS, and Custom Domain

```yaml
apiVersion: rcs.dana.io/v1alpha1
kind: Capp
metadata:
  name: event-processor
  namespace: analytics
spec:
  scaleMetric: cpu
  state: enabled
  configurationSpec:
    template:
      spec:
        containers:
          - name: processor
            image: ghcr.io/mycompany/event-processor:v1.5.0
            env:
              - name: DATA_DIR
                value: /data
              - name: BATCH_SIZE
                value: "1000"
            resources:
              requests:
                memory: "1Gi"
                cpu: "500m"
              limits:
                memory: "2Gi"
                cpu: "1000m"
            volumeMounts:
              - name: processed-data
                mountPath: /data
  routeSpec:
    hostname: processor.analytics.mycompany.com
    tlsEnabled: true
  volumesSpec:
    nfsVolumes:
      - name: processed-data
        server: nfs-storage.internal
        path: /exports/analytics/processed
        capacity:
          storage: 500Gi
  logSpec:
    type: elastic
    host: elasticsearch.monitoring.svc.cluster.local
    index: event-processor-logs
    user: analytics-user
    passwordSecret: es-analytics-secret
```

Application with CPU-based autoscaling, NFS persistent storage, Elasticsearch logging, and a custom HTTPS route. Create the Elasticsearch secret before applying:

```bash
kubectl create secret generic es-analytics-secret \
  --from-literal=password='es-password' \
  -n analytics
```

