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
- `rolloutStrategy`: Staged traffic splitting between the stable and the newest revision (see [Traffic splitting & rollouts](#traffic-splitting--rollouts))
- `routeTimeoutSeconds`: Request timeout duration

When `hostname` is set, the operator creates DomainMapping, CNAMERecord, and optionally a Certificate resource.

### `logSpec`
Configures automatic log shipping to Elasticsearch:
- `type`: Log destination (`elastic` or `elastic-datastream`)
- `host`: Hostname of the logs destination, without scheme, port or path (e.g. `elasticsearch.example.com`)
- `target`: Index name when `type` is `elastic`, or data stream name when `type` is `elastic-datastream`
- `user`: Username for authentication
- `passwordSecret`: Secret name containing the password
- `passwordKey`: Key inside `passwordSecret` that holds the password

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
    target: my-app-logs
    user: elastic
    passwordSecret: es-password-secret
    passwordKey: password
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
kubectl get capp my-app -n my-namespace              # Ready status and reason
kubectl get capp my-app -n my-namespace -o wide      # adds the Ready message
kubectl describe capp my-app -n my-namespace         # all conditions
```

#### Status conditions

Each configured capability in Capp spec (Serving, Route, Logging...) has its own condition. A condition is removed when its spec section is removed.

| Condition | Set when | Checks |
|-----------|----------|--------|
| `VolumesReady` | `volumesSpec.nfsVolumes` is set | each NfsPvc |
| `ServingReady` | `state` is `enabled` | the Knative Service and its latest created revision |
| `RouteReady` | `routeSpec.hostname` is set | CNAMERecord, Certificate (if TLS is enabled), DomainMapping |
| `EventingReady` | an event source is set | each PingSource / KafkaSource |
| `LoggingReady` | `logSpec` is set | SyslogNGOutput, SyslogNGFlow |
| `Ready` | always | summary of the conditions above |
| `Available` | always | the revisions that serve traffic (see below) |
| `RolloutSucceeded` | the Knative Service has a created revision, with or without a strategy | the latest revision has fully rolled out (see below) |

A capability condition is:
- `False` with reason `ResourceSyncFailed` when the operator can't apply one of its resources.
- `Unknown` with reason `Pending` while a resource is missing or not yet reconciled.
- Otherwise, the status and reason of the first resource that isn't ready, or `True` with reason `Ready`.

`Ready` points to the root cause:
- `state: disabled` gives `Ready=True` with reason `Disabled`.
- A capability whose dependency isn't ready is a symptom and is skipped. `ServingReady` depends on `VolumesReady`, and `RouteReady` and `EventingReady` depend on `ServingReady`.
- Otherwise the reason is `<Capability>NotReady` (`VolumesNotReady`, `ServingNotReady`, `RouteNotReady`, `EventingNotReady`, `LoggingNotReady`) and the message is `<capability reason>: <capability message>`. `False` wins over `Unknown`. Ties follow the table order.

For example, when the image can't be pulled:

```
Ready         False  ServingNotReady   ContainerMissing: Revision my-app-00001: Unable to fetch image "..."
ServingReady  False  ContainerMissing  Revision my-app-00001: Unable to fetch image "..."
RouteReady    False  ...               ← symptom of ServingReady
```

`Available` and `RolloutSucceeded` are not part of the `Ready` summary, and `Ready` keeps its meaning. `Ready` stays False while the latest revision fails its Knative check, even after a rollback. Use the two conditions below to tell whether traffic is served and whether the rollout finished:

- `Available` is **True** when the Knative Service routes are ready and every revision that gets traffic (percent above 0) is Ready. Revisions that get 0% are ignored, so a rolled-back candidate does not make the Capp unavailable. It is **False** when any revision that gets traffic is not Ready, or the routes are not ready, even if another serving revision is still Unknown. It is **Unknown** while routing is being updated, while a revision that gets traffic has not reported Ready yet and none has failed, or when the Knative Service does not exist yet. A failed apply of the Knative Service does not change `Available`, because the revision that was serving keeps serving. For a disabled Capp, `Available` is **Unknown** with reason `Pending` because the Knative Service is absent, while `Ready` is True with reason `Disabled`.
- `RolloutSucceeded` is **True** only when the phase is `Stable`, the Knative Service holds every field set in the desired revision template, its configurations are ready, and Knative confirms that the latest revision serves 100% of the traffic. Until Knative has picked up the desired template, and while its configurations are not ready, the condition is **Unknown** with reason `Pending`, even if the old revision still serves 100%. If the stable revision fails while its configurations are not ready, the condition is **False** with reason `RevisionFailed`. Removing any field or trailing list element from the template is not detected by this check, so **True** can briefly remain until Knative's generation and configuration status catches up. It is **False** when the phase is `Failed`, `RolledBack` or `Aborted`. It is also **False** with reason `ResourceSyncFailed` when the operator fails to apply the Knative Service. The previous revision keeps serving, and the rollout status can be ahead of what Knative serves, for example when only the traffic split failed to apply. The condition clears on the next successful apply. `ResourceSyncFailed` is therefore transient, and a pipeline should treat it as retriable rather than abort on it immediately. It is **Unknown** while the phase is `Progressing`, `Holding` or `Promoting`, and its reason is the phase. A Capp without a `rolloutStrategy` gets the same condition, based on its latest created revision instead: **True** when that revision is Ready and Knative serves it 100% of the traffic, **False** when it failed (Ready=False, reason `RevisionFailed`), and **Unknown** while it is still progressing. The wait below therefore works without a strategy too. On the first deploy, before any stable revision exists, the condition is also **Unknown** with reason `Pending` while the latest revision is not Ready yet, and **False** with reason `RevisionFailed` if that revision fails. It is absent while there is no Knative Service or no created revision, unless the apply of the Knative Service failed. A failed create or update is then reported as **False** with reason `ResourceSyncFailed`.

What to wait on:
- A deploy pipeline waits for `RolloutSucceeded=True` and fails on `False`:
  ```bash
  kubectl wait capp/my-app -n my-namespace --for=condition=RolloutSucceeded --timeout=30m
  ```
- Availability monitoring and alerting use `Available`.
- A blue-green Capp stays `Unknown` (`Holding`) until it is promoted, so its pipeline waits after the promote step, not after the deploy.

The status also includes `observedGeneration`, `eventingStatus` and `stateStatus`. The fields `knativeObjectStatus`, `revisions`, `routeStatus`, `volumesStatus` and `loggingStatus` are deprecated and will be removed in a later release; use the conditions instead.

## Traffic splitting & rollouts

By default, a Capp sends 100% of its traffic to the latest revision. Set `routeSpec.rolloutStrategy` to move traffic to a new revision in stages instead. You describe the rollout once; every time a new revision is created, the operator walks it through the stages on its own.

The strategy refers to roles, not revision names:
- **stable**: the last revision that finished a rollout (reached 100%).
- **candidate**: the newest revision, while it differs from stable.

Each stage gives the candidate `percent` of the traffic, and stable gets the rest, for at least `duration`, counted from when Knative serves that split (see below). The webhook validates the stages:
- `percent` values are strictly increasing. Equal or decreasing values are rejected.
- Every stage except the last needs a `duration`, and it must be positive.
- The last stage must not set a `duration`. A last stage of `percent: 100` starts promotion (`Promoting`) when it is reached, and the candidate becomes stable once Knative confirms it serves 100% of the traffic. A last stage below 100 holds at that percent until you promote or abort.

Rollouts start only for changes that create a new Knative revision. The first revision of a Capp gets 100% of the traffic and becomes stable once it is Ready.

`routeSpec.trafficTarget` has been removed. It never had an effect, and the operator drops any stored value. A manifest that still sets it must remove the field, because strict field validation can reject it.

### Canary

10% of the traffic for 5 minutes, then 50% for 10 minutes, then 100%:

```yaml
spec:
  routeSpec:
    rolloutStrategy:
      stages:
        - percent: 10
          duration: 5m
        - percent: 50
          duration: 10m
        - percent: 100
      autoRollback: true       # optional, default false
      progressDeadline: 10m    # optional, default 10m
```

The `progressDeadline` is measured from the creation of the candidate revision. If the candidate is not Ready within that time, or reports Ready=False, then with `autoRollback: true` 100% of the traffic goes back to stable and the phase becomes `RolledBack`. With `autoRollback: false` the phase becomes `Failed`, the traffic stays at the current stage's split (0% if the candidate never became Ready), and stages stop advancing. If the candidate becomes Ready again, the rollout resumes at the same stage. Otherwise you decide what to do using the annotation below.

Stage timers start only after Knative serves the stage's split. The operator sends the traffic for a stage, then waits until the Knative Service reports that split and its routes are ready. Routing updates take time, and that time does not count toward the stage. A stage's `duration` is a **minimum**: the candidate serves that percent for at least `duration`, counted from when the operator observes the confirmation. Operator downtime can make a stage last longer, never shorter. The operator moves at most one stage per confirmed split.

### Steady split

Hold 50/50 between stable and the latest revision indefinitely:

```yaml
spec:
  routeSpec:
    rolloutStrategy:
      stages:
        - percent: 50
```

The phase is `Holding` until you promote or abort.

### Blue-green

The candidate gets no traffic. Test it on its tag URL, then promote it:

```yaml
spec:
  routeSpec:
    rolloutStrategy:
      stages:
        - percent: 0
```

### Promote, abort and retry

Set the `rcs.dana.io/rollout` annotation on the Capp. The value names the action and the candidate revision it is meant for, in the form `promote:<revision>`, `abort:<revision>` or `retry:<revision>`. The operator acts on it and then removes it:

```bash
# my-app-00002 is the status.rollout.candidateRevision
kubectl annotate capp my-app -n my-namespace rcs.dana.io/rollout=promote:my-app-00002   # candidate goes to 100% and becomes stable
kubectl annotate capp my-app -n my-namespace rcs.dana.io/rollout=abort:my-app-00002     # 100% back to stable, phase Aborted
kubectl annotate capp my-app -n my-namespace rcs.dana.io/rollout=retry:my-app-00002     # runs the stages again from stage 0
```

The revision binds the action to one rollout. If it is not the current `candidateRevision`, for example because a newer revision has replaced it, the action is rejected with a Warning event and the rollout is left alone. A bare `promote`, `abort` or `retry`, or any other value, is refused by the webhook. If the webhook is bypassed, the operator ignores an invalid value and emits a `RolloutInvalidAction` Warning event.

`promote` is refused (with a `RolloutPromoteRejected` Warning event) if the candidate is not Ready or the phase is `RolledBack` or `Aborted`.

A rollout action on a Capp without a `rolloutStrategy` has no rollout to act on. The operator ignores it, emits a Warning event with reason `RolloutInvalidAction`, and removes the annotation. The same happens to an action that arrives before there is a Knative Service with a created revision, or before any stable revision exists, for example during the first deploy.

Replaying an action after the operator applied it, for example after an operator restart between applying it and removing the annotation, is safe, but the outcome depends on the action:
- A replayed `promote` for a revision that is already stable, or a replayed `abort` for a rollout that is already `Aborted`, is a no-op and emits no Warning event.
- A replayed `retry` is rejected with a `RolloutRetryRejected` Warning event if the rollout is no longer in a phase that allows a retry. It runs again if the candidate has failed again since.
- A stale `abort` for a revision that is no longer the candidate is rejected with a Warning event and leaves the rollout alone.

### Retry

A `Failed`, `RolledBack` or `Aborted` rollout can run again for the same candidate revision with `retry:<revision>`. So can a `Holding` rollout that was held because the strategy was changed (see below). The rollout restarts at stage 0 and gets a full `progressDeadline` again, counted from the retry, and a `RolloutRetried` event is emitted. A retry for any other phase, or for a revision that is not the candidate, is rejected with a `RolloutRetryRejected` Warning event. To roll out a different build, change the Capp template instead: that creates a new revision, and a new rollout starts for it.

### Promoting

A candidate reaches 100% through its last stage, through `promote`, or through removing the strategy. The phase then becomes `Promoting`, and the candidate receives all of the traffic. The candidate becomes stable and the phase `Stable` only after Knative confirms that split. `RolloutSucceeded` stays `Unknown` while the phase is `Promoting`. Health checks still apply: if the candidate turns Ready=False during this time, the rollout is rolled back or fails, as it does during a stage.

With `autoRollback: false`, a failing candidate never keeps 100% of the traffic. The traffic goes back to the split the candidate had before the promotion, and the phase becomes `Failed`:
- After a manual `promote`, the current stage's split.
- After the rollout reached a final 100% stage, the previous stage's split. If the final stage is the only stage, the traffic goes back to 0%, and the candidate is left out of the traffic (stable keeps 100%).
- If the rollout was held at a share because the strategy changed, that held share.

Once the candidate is Ready again, the rollout resumes. A failed promotion from the final 100% stage does not promote again straight away. It falls back to the previous stage, and on recovery that stage runs again for its full `duration`, counted from when Knative confirms its split. Only then does it promote. For example, with `[{10, 5m}, {50, 10m}, {100}]`, a candidate that fails while promoting drops to 50%, and after it recovers it serves 50% for another 10 minutes before it is promoted. A single-stage `[{100}]` strategy has no previous stage, so its candidate promotes again as soon as it is Ready, unless a strategy edit made while the candidate was failed moves the rollout off that stage. Then the 0% hold stays until `promote`, `abort` or `retry`, or until an edit adds a stage at or below 0%. A promotion from any other stage resumes at the same stage, and that stage's timer restarts once Knative confirms the split. A rollout held at a share stays held at that share. Otherwise a retry or abort works as usual.

### Changing or removing the strategy

The strategy can be changed or removed while a rollout is running. Neither change raises the candidate's traffic on its own.

**Changing the strategy.** The rollout keeps its place when the stage at its current index has the same percent, so duration-only edits do not restart the timer. The exception is a rollout held at a stage that had no duration: if the edit gives that stage a duration, its timer restarts once Knative confirms the split, so the time it already spent holding does not count. The percent being served is the lower of the candidate's requested share and the share Knative reports for it. A higher split that Knative has not applied yet does not count, so an edit never raises the traffic. Otherwise the rollout moves to the last stage whose percent is at most the percent being served, and that stage's timer starts again once Knative confirms its split. For example, at the 50% stage of `[10, 50, 100]`, changing the strategy to `[10, 100]` moves the rollout to 10%, not to 100%. A stale or rejected action annotation does not stop this from happening.

If every new stage is above the percent being served, the candidate stays at its current percent, the phase becomes `Holding`, and a `RolloutStagesChanged` Warning event is emitted. For example, at 10%, changing to `[50, 100]` or `[100]` holds at 10% and does not promote. Continue with `promote`, `abort` or `retry`.

While the rollout is `Holding` after a strategy change, a further edit releases the hold if one of the new stages is at or below the held share. The rollout then moves onto the last such stage and continues from there, and that stage's timer starts once Knative confirms the split. For example, held at 10%, an edit to `[{5, 1m}, {100}]` moves the candidate to 5%, and after 1 minute it promotes. Traffic never rises as a direct result of an edit. If every new stage is still above the held share, the hold stays.

**Removing the strategy.** Removing `rolloutStrategy` never sends traffic to a rejected revision:
- If the candidate is Ready and the phase is `Progressing`, `Holding` or `Promoting`, and no newer revision has been created since, the removal works like `promote`. The candidate goes to 100% (through `Promoting`) and becomes stable, and the Capp goes back to the default routing. A candidate that a newer revision has replaced is never promoted; the rules below apply instead.
- Otherwise, the candidate is rejected because it is not Ready, or the phase is `Failed`, `RolledBack` or `Aborted`. The traffic is pinned to stable at 100%, and `status.rollout.pinnedRevision` names the stable revision. A rollback to the split before a failed promotion does not apply here, because no strategy is left to fall back to. A Progressing, Holding or Promoting phase becomes `Failed`. A `RolloutPinned` Warning event explains the pin. The pin is released only when a newer revision is Ready. A newer revision that has only been created does not release it. After that, the Capp goes back to the default routing.

### Tag URLs

The stable and candidate revisions are tagged `stable` and `candidate`, so each can be reached on its own URL, for example `candidate-my-app.my-namespace.<knative-domain>`. Tag URLs exist only on the Knative default cluster domain; they are not available on the Capp's custom `hostname`.

### Checking rollout progress

```bash
kubectl get capp my-app -n my-namespace -o jsonpath='{.status.rollout}'
```

`status.rollout` shows `stableRevision`, `candidateRevision`, `phase` (`Stable`, `Progressing`, `Holding`, `Promoting`, `Failed`, `RolledBack`, `Aborted`), `stageIndex`, `traffic` and a `message`, along with these fields:
- `pinnedRevision`: the stable revision that traffic is pinned to after a strategy removal rejected the candidate. Empty otherwise.
- `stageStartTime`: when Knative confirmed the current stage's traffic split. Unset until then.
- `heldPercent`: the candidate's share while a strategy change holds the rollout. Unset when the rollout follows its stages.
- `promotionFallback`: true while `heldPercent` is the 0% hold that a failed promotion from a single 100% stage left. The hold is lifted when the candidate recovers. If a strategy edit during the fallback moves the rollout off that stage, the hold stays until a user action or a later edit releases it.
- `candidateSince`: the start of the `progressDeadline`, which is the candidate's creation time, or the time of the last retry.
- `candidateSinceFallback`: true while `candidateSince` is the time the operator first saw the candidate, because its Revision could not be read.
- `candidateReadyOnce`: true once the candidate has been Ready. The `progressDeadline` applies only until then. Rollout events are emitted on the Capp: `RolloutCompleted`, `RolloutRestarted`, `RolloutRetried`, `RolloutRolledBack`, `RolloutFailed`, `RolloutAborted`, `RolloutPinned`, `RolloutStagesChanged`, `RolloutPromoteRejected`, `RolloutAbortRejected`, `RolloutRetryRejected` and `RolloutInvalidAction`. `RolloutCompleted`, `RolloutRetried` and `RolloutAborted` are Normal events, and the rejections are Warnings. `RolloutRestarted`, `RolloutRolledBack`, `RolloutFailed` and `RolloutPinned` are Warnings too. `RolloutStagesChanged` is a Normal event when the rollout moves to another stage, and a Warning when it holds. `RolloutInvalidAction` is emitted when the annotation value is invalid (the webhook refuses it, so this happens only when the webhook is bypassed), when any action is set on a Capp without a `rolloutStrategy`, or when an action arrives before there is a stable revision or a Knative Service.

After a rollback or abort, the Capp template no longer matches the revision that receives traffic. `RolloutSucceeded=False` says so until you change the template. `Ready` can also stay False while `Available` is True, because the failed revision is not serving traffic.

### Behaviour to know: a new revision restarts the rollout

> **This is deliberate, and it may surprise you.** If a new revision appears while a rollout is running, the running rollout is aborted and a new one starts.

Say rev1 is stable and rev2 is being rolled out (mid-stage, or holding). When rev3 is created:

1. The rev1 -> rev2 rollout is aborted. Rev2 stops receiving traffic right away and never becomes stable.
2. A new rollout from rev1 to rev3 starts at stage 0.
3. A `RolloutRestarted` Warning event is emitted.

Stable stays rev1 throughout. This means that a steady split is always between stable and the **latest** revision, not between the two latest revisions, and that pushing changes quickly can keep a rollout from ever finishing. The upside is that an urgent fix never waits behind a slow canary.

**Capacity:** while rev3 is not Ready yet, rev1 (stable) receives 100% of the traffic, up from its share of the split. Knative scales rev1 up to match, but a sudden burst can reach cold pods. Keep enough `scaleSpec.minReplicas` on the Capp to absorb the full load while a new revision starts.

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
    target: event-processor-logs
    user: analytics-user
    passwordSecret: es-analytics-secret
    passwordKey: password
```

Application with CPU-based autoscaling, NFS persistent storage, Elasticsearch logging, and a custom HTTPS route. Create the Elasticsearch secret before applying:

```bash
kubectl create secret generic es-analytics-secret \
  --from-literal=password='es-password' \
  -n analytics
```

