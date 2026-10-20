# CJob: event-driven batch workloads for RCS

| | |
|---|---|
| Status | Draft |
| Author | Dvir |
| Scope | `container-app-operator` only |
| Last updated | 2026-10-20 |

## 1. Motivation

A Capp is backed by a Knative Service. That makes it a good fit for request/response work that finishes while the HTTP request is open and that benefits from scale-to-zero. It is a poor fit for work that is triggered by an event but then runs to completion for minutes or hours: the request has to stay open for the whole run, retries happen at the HTTP layer, and there is no notion of a finished run with an exit code.

Knative Eventing ships a `JobSink` (`sinks.knative.dev/v1alpha1`) for exactly this case. A JobSink is an addressable sink that creates one Kubernetes `Job` per CloudEvent it receives, from a Job template, and hands the event to the pod as a file.

`CJob` is a new RCS custom resource that does for JobSink what `Capp` does for Knative Service. Users describe a batch workload and the events that trigger it, and the operator creates and owns the JobSink and the supporting resources (event sources, NFS volumes, log forwarding), enforces platform policy and reports run history in status.

## 2. Goals

- A new `CJob` kind in `rcs.dana.io/v1alpha1`, reconciled by its own controller inside `container-app-operator`.
- CJob embeds a `batch/v1` `JobSpec`, in the same way Capp embeds Knative's `ConfigurationSpec`.
- Reuse of the Capp capabilities that make sense for a Job: PingSource and KafkaSource triggers, NFS volumes and Elastic log forwarding.
- `state: enabled | disabled`, with the same meaning as on Capp.
- An opt-in `httpTrigger` flag that marks a CJob as one the CJob gateway may trigger over HTTP.
- Platform-enforced cleanup of finished Jobs through `ttlSecondsAfterFinished`, with a default and a maximum from config, and an admin bypass.
- CJob-specific defaults in CappConfig.
- Status that shows the JobSink address and the recent runs with their exit codes.
- Shared code between Capp and CJob. The existing resource managers, status helpers, and webhook validators are generalized rather than copied.

## 3. Non-goals

These are related and planned, but they are separate efforts and are not part of CJob. They are listed here only so readers know where they live.

- **External HTTP access (the "CJob gateway").** A JobSink only has a cluster-local address on the shared `job-sink` Service in `knative-eventing`. Exposing CJobs to callers outside the cluster will be done by a new, standalone project: an HTTP server, which may itself run as a Capp, that receives requests and forwards them to the right CJob. CJob does not create Routes, Ingresses, DomainMappings, DNS records or certificates. CJob's only part in this is the opt-in `httpTrigger` flag and the `status.address` the gateway reads (see 6.7).
- **Concurrency control with Kueue.** Limiting how many Jobs run at once will be done with Kueue. Kueue queues will be set up by `capp-backend` when it creates a namespace. CJob does not create Kueue objects, and the CJob webhook does not require or add the `kueue.x-k8s.io/queue-name` label. A label on the CJob passes through to every Job like any other label (see 6.2), so `capp-backend` can opt a CJob into its namespace's queue by setting the label when it creates the CJob.
- **capp-backend, cappctl, capp-mcp and capp-frontend support.** These will follow in their own designs.
- **Renaming CappConfig to RCSConfig.** CJob settings go in CappConfig. A future rename to something more generic, such as `RCSConfig`, is possible but is not part of this design.

## 4. Background: how JobSink behaves

These facts come from the Knative Eventing source (`cmd/jobsink/main.go`, `pkg/apis/sinks`), at the `knative.dev/eventing v0.50.x` line the operator already depends on. They drive several decisions below.

- **Spec.** `JobSink.spec.job` is a full `*batchv1.Job` (metadata and spec). When a JobSink is created or updated, its webhook dry-run creates a Job from the template, so an invalid template, or a quota or admission policy that rejects it, fails at JobSink admission time.
- **Address.** All JobSinks are served by one shared `job-sink` Deployment and Service in `knative-eventing`. A JobSink's address is `http://job-sink.knative-eventing.svc.cluster.local/<namespace>/<name>`. The receiver rejects any other path, including a sub-path or a query string, with `400 Malformed uri`.
- **One Job per event.** For each POSTed CloudEvent the receiver creates a Job named `<jobsink-name>-<md5(source+id)>`. The Job carries the labels `sinks.knative.dev/job-sink-name=<jobsink-name>` and `sinks.knative.dev/job-sink-id=<job-name>`, and an ownerReference to the JobSink.
- **Deduplication.** If a Job for the same `source` + `id` already exists, the receiver returns `202` without creating another. Once that Job has been deleted, a redelivery creates a new Job.
- **Event delivery to the pod.** The receiver stores the event JSON in an immutable Secret with the same name as the Job (key `event`), owned by the Job. It mounts the Secret as a volume named `jobsink-event` at `/etc/jobsink-event` in every container, and sets `K_EVENT_PATH`. If the template already defines a `jobsink-event` volume or mount, the receiver uses the user's mount path instead.
- **Cleanup.** The JobSink never deletes Jobs. Deleting a Job garbage-collects its pods and its event Secret. Deleting the JobSink garbage-collects all of its Jobs, including running ones.
- **Status.** `Ready` is made of `Addressable` and `EventPoliciesReady`. The address is in `status.address`.

## 5. API

### 5.1 CJob

```go
// CJobSpec defines the desired state of CJob.
type CJobSpec struct {
	// State defines the state of the CJob.
	// +optional
	// +kubebuilder:default:="enabled"
	// +kubebuilder:validation:Enum=enabled;disabled
	State string `json:"state,omitempty"`

	// JobSpec is the template for every Job created by the CJob's JobSink.
	JobSpec batchv1.JobSpec `json:"jobSpec"`

	// LogSpec defines the configuration for shipping CJob logs.
	// +optional
	LogSpec LogSpec `json:"logSpec,omitempty"`

	// VolumesSpec defines the volumes specification for the CJob.
	// +optional
	VolumesSpec VolumesSpec `json:"volumesSpec,omitempty"`

	// EventSourcesSpec defines the event sources that trigger the CJob.
	// +optional
	EventSourcesSpec EventSourcesSpec `json:"eventSourcesSpec,omitempty"`

	// HTTPTrigger controls whether the CJob gateway may trigger the CJob over HTTP.
	// +optional
	HTTPTrigger HTTPTrigger `json:"httpTrigger,omitempty"`
}

// HTTPTrigger defines whether the CJob can be triggered through the CJob gateway.
type HTTPTrigger struct {
	// Enabled opts the CJob in to being triggered by the CJob gateway.
	// +optional
	// +kubebuilder:default:=false
	Enabled bool `json:"enabled,omitempty"`
}
```

`LogSpec`, `VolumesSpec` and `EventSourcesSpec` are the existing Capp types, reused as-is. Capp's `ScaleSpec` and `RouteSpec` have no CJob equivalent.

```go
// CJobStatus defines the observed state of CJob.
type CJobStatus struct {
	// ObservedGeneration is the generation of the CJob last processed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions contain details about the current state of the CJob.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// StateStatus shows the current CJob state.
	// +optional
	StateStatus StateStatus `json:"stateStatus,omitempty"`

	// Address is the cluster-local URL that events for this CJob are sent to.
	// It is copied from the JobSink's status.address.url.
	// +optional
	Address string `json:"address,omitempty"`

	// EventingStatus shows the state of the event sources linked to the CJob.
	// +optional
	EventingStatus EventingStatus `json:"eventingStatus,omitempty"`

	// RecentRuns lists the CJob's existing Jobs, newest first,
	// capped at cjobConfig.runHistoryLimit.
	// +optional
	RecentRuns []CJobRun `json:"recentRuns,omitempty"`
}

// CJobRun is the observed state of a single Job created by the CJob.
type CJobRun struct {
	// JobName is the name of the Job.
	JobName string `json:"jobName"`

	// EventID and EventSource identify the CloudEvent that created the Job.
	// +optional
	EventID string `json:"eventID,omitempty"`
	// +optional
	EventSource string `json:"eventSource,omitempty"`

	// Phase is one of Pending, Running, Succeeded or Failed.
	Phase CJobRunPhase `json:"phase"`

	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// ExitCodes holds, for the most recently terminated pod of the Job,
	// the exit code of each of its containers.
	// +optional
	ExitCodes []ContainerExitCode `json:"exitCodes,omitempty"`
}

type ContainerExitCode struct {
	Container string `json:"container"`
	ExitCode  int32  `json:"exitCode"`
	// Reason is the container's terminated reason, e.g. Completed, Error, OOMKilled.
	// +optional
	Reason string `json:"reason,omitempty"`
}
```

Print columns: `Ready`, `Reason`, `State`, `Address`, and `Message` at priority 1, following Capp.

There is deliberately no cumulative failure count. Finished Jobs are deleted by the TTL, so a count would need the controller to record each failure before it disappears. The run list only reflects Jobs that still exist.

### 5.2 Example

```yaml
apiVersion: rcs.dana.io/v1alpha1
kind: CJob
metadata:
  name: report-generator
  namespace: team-a
spec:
  state: enabled
  jobSpec:
    backoffLimit: 2
    activeDeadlineSeconds: 3600
    ttlSecondsAfterFinished: 1800
    template:
      spec:
        restartPolicy: Never
        containers:
        - name: main
          image: registry.example.com/team-a/report:1.4.0
          volumeMounts:
          - name: reports
            mountPath: /reports
  volumesSpec:
    nfsVolumes:
    - name: reports
      server: nfs.example.com
      path: /exports/team-a
      capacity:
        storage: 50Gi
  logSpec:
    type: elastic
    host: elastic.example.com
    target: team-a-reports
    user: team-a
    passwordSecret: elastic-creds
    passwordKey: password
  eventSourcesSpec:
    sources:
    - name: nightly
      pingSourceConfiguration:
        schedule: "0 2 * * *"
        data: '{"report":"daily"}'
    - name: requests
      kafkaSourceConfiguration:
        bootstrapServers: ["kafka.example.com:9093"]
        topics: ["report-requests"]
        secretRef:
          name: kafka-creds
  httpTrigger:
    enabled: true
```

### 5.3 CappConfig changes

A new `cjobConfig` section holds the CJob-specific settings. The existing top-level `maxKafkaConsumers` applies to CJob Kafka sources too, since it is a limit on the Kafka source rather than on Capp.

```go
type CappConfigSpec struct {
	// ...existing fields...

	// CJobConfig holds the platform settings for CJobs.
	// +optional
	CJobConfig CJobConfig `json:"cjobConfig,omitempty"`
}

type CJobConfig struct {
	// DefaultResources is assigned to CJob containers that do not set their own.
	// +optional
	DefaultResources corev1.ResourceRequirements `json:"defaultResources,omitempty"`

	// DefaultTTLSecondsAfterFinished is set on the JobSpec when the user leaves it unset.
	// +kubebuilder:validation:Minimum=0
	DefaultTTLSecondsAfterFinished int32 `json:"defaultTTLSecondsAfterFinished"`

	// MaxTTLSecondsAfterFinished is the largest TTL a user may set.
	// +kubebuilder:validation:Minimum=0
	MaxTTLSecondsAfterFinished int32 `json:"maxTTLSecondsAfterFinished"`

	// RunHistoryLimit is the number of runs kept in CJob status.recentRuns.
	// +kubebuilder:default:=10
	// +kubebuilder:validation:Minimum=1
	RunHistoryLimit int `json:"runHistoryLimit,omitempty"`
}
```

A CEL rule on `CJobConfig` requires `defaultTTLSecondsAfterFinished <= maxTTLSecondsAfterFinished`. The default and max values themselves are left to whoever deploys the operator. The Helm chart's default CappConfig will need values.

## 6. Controller design

### 6.1 Resources created per CJob

| Resource | When | Name | Owner |
|---|---|---|---|
| `JobSink` | `state: enabled` | `<cjob>` | CJob |
| `PingSource` | per `pingSourceConfiguration` | same scheme as Capp (`GenerateEventSourceName`) | CJob |
| `KafkaSource` | per `kafkaSourceConfiguration` | same scheme as Capp | CJob |
| `NfsPvc` | per `nfsVolumes` entry | volume name | CJob |
| `SyslogNGOutput`, `SyslogNGFlow` | `logSpec.type` set | `<cjob>` | CJob |

Event sources set their sink to `{apiVersion: sinks.knative.dev/v1alpha1, kind: JobSink, name: <cjob>}`.

Every managed resource carries `rcs.dana.io/parent-cjob=<cjob>` and `rcs.dana.io/managed-by=cjob`, the CJob counterparts of `parent-capp` and `managed-by=capp`.

### 6.2 Building the JobSink

The JobSink's `spec.job` is built from the CJob:

- `spec` is a copy of `cjob.spec.jobSpec`, with the NFS volumes appended to the pod template as PVC volumes, as the Knative Service manager does for Capp.
- The Job's labels are the CJob's labels. The pod template also gets `rcs.dana.io/parent-cjob=<cjob>`, which the log flow matches on and which scopes the pod cache (see 6.5).
- The Job's annotations are the CJob's annotations, minus `rcs.dana.io/*` and `kubectl.kubernetes.io/*`, following the Capp filter.
- `enableServiceLinks` is set to `false`, as for Capp.
- `ttlSecondsAfterFinished` is always set by the time the controller sees it, because the mutating webhook defaults it.

### 6.3 Reconcile flow

The flow is the same as Capp's: get CappConfig, handle deletion through the cleanup finalizer, run each resource manager's `Manage`, then sync status. Managers run in this order: NfsPvc, JobSink, SyslogNGOutput, SyslogNGFlow, PingSource, KafkaSource. On deletion they are cleaned up in reverse order, as `finalizeCapp` does today.

### 6.4 State

- **enabled:** the JobSink exists.
- **disabled:** the JobSink is deleted. Through ownerReferences, Kubernetes then deletes every Job it created, including running ones, along with their pods and event Secrets. An event `CJobDisabled` is recorded.
- Going back to **enabled** recreates the JobSink. An event `CJobEnabled` is recorded.
- Event sources, NFS volumes and log forwarding are kept while disabled, matching Capp, where only the Knative Service is removed. Events that arrive while disabled fail delivery, since the sink is gone, and follow the source's retry and dead-letter settings. The sources report not-ready until the CJob is enabled again.

`status.stateStatus` is maintained the same way as Capp's.

### 6.5 Watches

- `CJob`: on generation, label or annotation changes.
- Owned resources (JobSink, PingSource, KafkaSource, NfsPvc, SyslogNGOutput, SyslogNGFlow): mapped back by label or owner, with the same status-change predicates Capp uses.
- `Job`s with the `sinks.knative.dev/job-sink-name` label: mapped to the CJob of the same name in the Job's namespace, on create, delete, and changes to `status`. These events drive `recentRuns`.
- `Pod`s with the `rcs.dana.io/parent-cjob` label: mapped to that CJob when a container terminates. The manager's cache for Pods is restricted with a label selector (`cache.Options.ByObject`), so the operator only caches CJob pods and not every pod in the cluster.
- `CappConfig`: requeues all CJobs on generation change, as for Capp.

### 6.6 Status

**Conditions.** These follow the Capp model of a `Ready` summary plus one condition per configured capability:

| Condition | Set when | Source |
|---|---|---|
| `SinkReady` | always while enabled | JobSink `Ready` |
| `VolumesReady` | `nfsVolumes` set | NfsPvc |
| `EventingReady` | sources set | PingSource and KafkaSource `Ready` |
| `LoggingReady` | `logSpec.type` set | SyslogNGOutput and SyslogNGFlow |
| `Ready` | always | summary, with reasons `Ready`, `Disabled`, `SinkNotReady`, `VolumesNotReady`, `EventingNotReady`, `LoggingNotReady` |

**Address.** This is copied from `JobSink.status.address.url` and cleared when disabled.

**Recent runs.** On each reconcile the controller:

1. Lists Jobs in the namespace with `sinks.knative.dev/job-sink-name=<cjob>`, sorted by `creationTimestamp` descending, and keeps the first `runHistoryLimit`.
2. Derives `phase` from Job conditions: `Complete` gives Succeeded, `Failed` gives Failed, `status.active > 0` gives Running, and anything else gives Pending. Pending covers a Job suspended by Kueue or one whose pods are not yet scheduled.
3. Reads `eventID` and `eventSource` from the Job's event Secret, which has the same name as the Job and holds the event under the `event` key. The Job itself only carries a hash of the two, so the Secret is the only place they exist. The operator already has `get` on Secrets.
4. Finds the Job's most recently terminated pod through `batch.kubernetes.io/job-name=<job>` and records each container's `state.terminated.exitCode` and `reason`. When the Job retried, this is the last attempt.

Because runs come only from existing Jobs, a Job deleted by its TTL drops out of status.

Status writes are skipped when nothing but volatile fields changed, as Capp does today. A busy CJob can still produce many status updates, since each Job and Pod transition triggers a reconcile.

### 6.7 HTTP trigger

`httpTrigger.enabled` is an opt-in contract between the CJob and the CJob gateway (see section 3). It defaults to `false`, so a CJob can only be triggered by its own event sources unless its owner asks otherwise.

- The operator creates no resources for it. The JobSink is the same whether the flag is on or off.
- The gateway watches CJobs and forwards a request only to a CJob that has `httpTrigger.enabled: true` and `Ready=True`. It sends the request as a CloudEvent to `status.address`, the same address event sources use.
- A CJob with the flag set and `state: disabled` has no address, so the gateway rejects requests for it.
- The flag is the gateway's allow-list, not a network control. The `job-sink` Service is still reachable from inside the cluster for every JobSink. Restricting senders to the gateway's identity, for example with a Knative `EventPolicy` once Eventing OIDC authentication is enabled, is left to the gateway design.

The exact request format, authentication and routing scheme belong to the gateway design.

### 6.8 Logging

The SyslogNGFlow for a CJob matches `json#kubernetes#labels#rcs.dana.io/parent-cjob` against the CJob name. Capp's flow matches on `serving.knative.dev/configuration`, which Job pods do not have. The SyslogNGOutput is identical to Capp's.

## 7. Admission webhooks

New `/mutate-cjob` and `/validate-cjob` endpoints are registered for `cjobs` on CREATE and UPDATE.

### 7.1 Mutating

- The last-updated-by annotation is set, as for Capp.
- `cjobConfig.defaultResources` is applied to containers that do not set requests or limits, using the same per-resource logic as `mutateResources`.
- `jobSpec.ttlSecondsAfterFinished` is set to `cjobConfig.defaultTTLSecondsAfterFinished` when it is unset.

### 7.2 Validating

A CJob is rejected when any of the following holds:

- `jobSpec.ttlSecondsAfterFinished` is greater than `cjobConfig.maxTTLSecondsAfterFinished`, unless the CJob has the annotation `rcs.dana.io/skip-ttl-check: "true"`. Only admins can set annotations in RCS, so the annotation is trusted as-is, following the existing `rcs.dana.io/skip-dns-check` precedent.
- `jobSpec.suspend` is set to `true`.
- `jobSpec.selector` or `jobSpec.manualSelector` is set.
- `jobSpec.template.spec.restartPolicy` is `Always`. Kubernetes rejects this anyway, but the webhook returns a clearer message.
- A volume or a container `volumeMount` is named `jobsink-event`. This would override where JobSink mounts the event.
- An event source sets `uri`. The JobSink receiver rejects any path other than `/<namespace>/<name>`.
- The shared Capp checks fail: NFS volume mounts must reference declared volumes, Ping schedule and data must be valid, the Kafka `secretRef` must have the required keys, Kafka consumers must be within `maxKafkaConsumers`, and the log spec must be complete.

Everything else in `JobSpec` is accepted. The JobSink's own dry-run then catches anything Kubernetes or a namespace policy would reject.

### 7.3 Name collisions with Capp

The SyslogNGOutput and SyslogNGFlow are named after their parent, and event sources are named `GenerateEventSourceName(parent, source)`. A Capp and a CJob with the same name in the same namespace would therefore manage the same child objects. To prevent this:

- The CJob validating webhook rejects a CJob on CREATE when a Capp with the same name exists in the namespace.
- The Capp validating webhook gains the reverse check, rejecting a Capp on CREATE when a CJob with the same name exists. This is the only change to Capp admission in this design.

Names are immutable, so the check only runs on CREATE.

## 8. Generalizing the Capp code

Today the resource managers, status helpers and webhook validators all take a `cappv1alpha1.Capp`. The goal is for CJob to reuse them rather than copy them.

- **Workload interface.** Introduce an interface implemented by both `*Capp` and `*CJob`:

  ```go
  type Workload interface {
  	client.Object
  	GetState() string
  	GetVolumesSpec() VolumesSpec
  	GetLogSpec() LogSpec
  	GetEventSourcesSpec() EventSourcesSpec
  	// SinkRef is the Knative Service (Capp) or JobSink (CJob) that event sources deliver to.
  	SinkRef() duckv1.KReference
  	// LogMatchLabel is the pod label the SyslogNGFlow matches on.
  	LogMatchLabel() string
  	// ManagedResourceLabels returns the parent and managed-by labels for child resources.
  	ManagedResourceLabels() map[string]string
  }
  ```

- **Shared resource managers.** `NFSPVCManager`, `SyslogNGOutputManager`, `SyslogNGFlowManager`, `PingSourceManager` and `KafkaSourceManager`, along with the helpers in `helpers.go` (`ensureOwnerReference`, `createManagedResource`, `deleteOwnedResources`, `listManagedResources`), move to a shared package and take a `Workload`. The `ResourceManager` interface becomes `Manage(ctx, Workload)`, `CleanUp(ctx, Workload)` and `IsRequired(Workload)`.
- **Kind-specific managers.** `KnativeServiceManager`, `DomainMappingManager`, `CertificateManager` and `DNSRecordManager` stay Capp-only. A new `JobSinkManager` is CJob-only.
- **Status.** The capability-condition machinery in `status/capp.go`, together with `eventing.go`, `logging.go` and `volumes.go`, is parameterized by the workload and its capability list. `route.go` and `serving.go` stay Capp-only.
- **Webhooks.** The validators for NFS mounts, event sources, the Kafka secret, the Ping schedule and the resource defaulting take the relevant sub-spec instead of a Capp.
- **Labels.** `cappmeta` gains `CJobResourceKey = rcs.dana.io/parent-cjob` and `CJobKey = cjob`. Watch map functions resolve the parent from whichever parent label is present.

The refactor lands first and changes no Capp behavior. Existing unit and e2e tests are the safety net.

## 9. RBAC

The controller needs the following, in addition to what it has today:

- `rcs.dana.io`: `cjobs`, `cjobs/status`, `cjobs/finalizers`
- `sinks.knative.dev`: `jobsinks`, with get, list, watch, create, update, patch and delete
- `batch`: `jobs`, with get, list and watch (read only, since JobSink creates them)
- `""`: `pods`, with get, list and watch

The operator already has Secret read access.

## 10. Prerequisites

- Knative Eventing with JobSink available. The `job-sink` component ships with Eventing core, and the operator's `knative.dev/eventing v0.50.1` dependency already includes the `sinks/v1alpha1` API. JobSink is still `v1alpha1` upstream.
- The other prerequisites (eventing-kafka-broker for KafkaSource, nfspvc-operator, logging-operator) are the same as for Capp.

## 11. Implementation plan

1. Generalize the shared managers, status helpers and webhook validators (section 8), with no change to Capp behavior.
2. Add the `CJob` and `CappConfig.cjobConfig` API types, CRDs, Helm chart updates and the default CappConfig values.
3. Add the CJob controller with the JobSink manager, conditions, address and state.
4. Add `recentRuns`, including the Job and Pod watches and the scoped pod cache.
5. Add the CJob mutating and validating webhooks, and the reverse name check in the Capp validator.
6. Add e2e tests and update `docs/user-guide.md` and the README.

## 12. Known limitations

- **NfsPvc names.** NfsPvcs are named after the volume, not the parent, so two workloads in a namespace that declare the same NFS volume name share one NfsPvc. This already applies between two Capps today. The name-collision check in 7.3 does not cover it.
- **Concurrent creates.** The name-collision check reads the API at admission time, so a Capp and a CJob with the same name created at the same instant could both be admitted. This is accepted as unlikely.
