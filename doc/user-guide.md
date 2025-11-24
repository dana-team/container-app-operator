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

### `sources`
Configures Kafka event sources for event-driven applications:
- `name`: Source name
- `type`: `Kafka`
- `bootstrapServers`: Kafka broker addresses
- `topic`: Topics to consume from
- `kafkaAuth`: Username and password reference

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

### Step 6: Connect Kafka Event Sources

```yaml
spec:
  sources:
    - name: kafka-events
      type: Kafka
      bootstrapServers:
        - kafka-broker-1:9092
      topic:
        - user-events
      kafkaAuth:
        username: kafka-user
        passwordKey:
          name: kafka-secret
          key: password
```

Create the secret:
```bash
kubectl create secret generic kafka-secret --from-literal=password='your-password' -n my-namespace
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

The status section includes: `knativeObjectStatus`, `routeStatus`, `loggingStatus`, `volumesStatus`, and `conditions`.

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

### Example 2: Event-Driven Application with Full Features

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
  sources:
    - name: user-events
      type: Kafka
      bootstrapServers:
        - kafka-1.internal:9092
        - kafka-2.internal:9092
        - kafka-3.internal:9092
      topic:
        - user-activity
        - user-transactions
      kafkaAuth:
        username: analytics-consumer
        passwordKey:
          name: kafka-analytics-secret
          key: password
```

Event-driven processor with Kafka sources, CPU-based autoscaling, NFS persistent storage, and Elasticsearch logging. Create required secrets before applying:

```bash
# Elasticsearch secret
kubectl create secret generic es-analytics-secret \
  --from-literal=password='es-password' \
  -n analytics

# Kafka secret
kubectl create secret generic kafka-analytics-secret \
  --from-literal=password='kafka-password' \
  -n analytics
```

