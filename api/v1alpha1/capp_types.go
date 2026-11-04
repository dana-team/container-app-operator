/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kapis "knative.dev/pkg/apis"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
)

const (
	// CappStateEnabled is the enabled state value for a Capp.
	CappStateEnabled = "enabled"

	// CappStateDisabled is the disabled state value for a Capp.
	CappStateDisabled = "disabled"

	// CappConditionReady summarizes the capability conditions of the Capp.
	CappConditionReady = "Ready"

	// Capability condition types, set only when the capability is configured on a Capp.
	CappConditionVolumesReady  = "VolumesReady"
	CappConditionServingReady  = "ServingReady"
	CappConditionRouteReady    = "RouteReady"
	CappConditionEventingReady = "EventingReady"
	CappConditionLoggingReady  = "LoggingReady"

	// Rollout condition types, set on every Capp. They are not part of the Ready summary.
	// Available is True when the revisions serving traffic can serve it.
	// RolloutSucceeded is True when the latest revision has fully rolled out.
	CappConditionAvailable        = "Available"
	CappConditionRolloutSucceeded = "RolloutSucceeded"

	// Reasons for the Ready condition.
	CappReadyReasonReady            = "Ready"
	CappReadyReasonDisabled         = "Disabled"
	CappReadyReasonVolumesNotReady  = "VolumesNotReady"
	CappReadyReasonServingNotReady  = "ServingNotReady"
	CappReadyReasonRouteNotReady    = "RouteNotReady"
	CappReadyReasonEventingNotReady = "EventingNotReady"
	CappReadyReasonLoggingNotReady  = "LoggingNotReady"

	// Reasons for capability conditions. Otherwise, the child resource's reason is used.
	CapabilityReasonReady    = "Ready"
	CapabilityReasonPending  = "Pending"
	CapabilityReasonNotReady = "NotReady"

	// CapabilityReasonResourceSyncFailed indicates that a child resource could not be
	// created or updated (e.g. a webhook validation error).
	CapabilityReasonResourceSyncFailed = "ResourceSyncFailed"
)

// CappSpec defines the desired state of Capp.
type CappSpec struct {
	// State defines the state of capp
	// Possible values examples: "enabled", "disabled".
	// +optional
	// +kubebuilder:default:="enabled"
	// +kubebuilder:validation:Enum=enabled;disabled
	State string `json:"state,omitempty"`

	// ScaleSpec holds the Capp scaling configuration.
	ScaleSpec ScaleSpec `json:"scaleSpec"`

	// ConfigurationSpec holds the desired state of the Configuration (from the client).
	ConfigurationSpec knativev1.ConfigurationSpec `json:"configurationSpec"`

	// RouteSpec defines the route specification for the Capp.
	// +optional
	RouteSpec RouteSpec `json:"routeSpec,omitempty"`

	// LogSpec defines the configuration for shipping Capp logs.
	LogSpec LogSpec `json:"logSpec,omitempty"`

	// VolumesSpec defines the volumes specification for the Capp.
	VolumesSpec VolumesSpec `json:"volumesSpec,omitempty"`

	// EventSourcesSpec defines the event sources for the Capp.
	// +optional
	EventSourcesSpec EventSourcesSpec `json:"eventSourcesSpec,omitempty"`
}

// ScaleSpec defines the scale specification for the Capp.
type ScaleSpec struct {
	// Metric defines which metric type is watched by the Autoscaler.
	// Possible values examples: "concurrency", "rps", "cpu", "memory".
	// +kubebuilder:default:="concurrency"
	// +kubebuilder:validation:Enum=cpu;memory;rps;concurrency
	Metric string `json:"metric,omitempty"`

	// MinReplicas is the minimum number of replicas for the Capp.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the maximum number of replicas for the Capp.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// ScaleDelaySeconds is the delay in seconds before the Autoscaler scales down the Capp to zero.
	// +kubebuilder:validation:Minimum=0
	// +optional
	ScaleDelaySeconds *int32 `json:"scaleDelaySeconds,omitempty"`
}

// EventSourcesSpec defines all event sources for a Capp.
type EventSourcesSpec struct {
	// Sources is the list of event sources connected to the Capp's Knative Service.
	// +optional
	Sources []SourceConfiguration `json:"sources,omitempty"`
}

// PingSourceConfiguration defines the configuration for a Capp PingSource.
// This event source sends a specified payload to the capp at regular intervals defined by a cron expression schedule.
type PingSourceConfiguration struct {

	// Schedule is the cron expression that defines the schedule for the PingSource.
	// +kubebuilder:default:="* * * * *"
	Schedule string `json:"schedule"`
	// Data is the payload that the PingSource will send when it triggers.
	// Must be valid JSON.
	// +optional
	Data string `json:"data,omitempty"`
}

// KafkaSourceConfiguration defines the configuration for a Capp Kafka event source.
// +kubebuilder:validation:XValidation:rule="size(self.secretRef.name) > 0",message="secretRef.name is required"
type KafkaSourceConfiguration struct {
	// BootstrapServers is the list of Kafka broker addresses.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	BootstrapServers []string `json:"bootstrapServers"`

	// Topics is the list of Kafka topics to consume.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Topics []string `json:"topics"`

	// ConsumerGroup is the Kafka consumer group ID.
	// Optional; when unset the operator assigns a dedicated group for this source.
	// +optional
	ConsumerGroup string `json:"consumerGroup,omitempty"`

	// Consumers is the number of parallel KafkaSource consumers for this source.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Consumers *int32 `json:"consumers,omitempty"`

	// SecretRef references a Secret in the Capp namespace with credentials for the Kafka cluster.
	// +kubebuilder:validation:Required
	SecretRef corev1.LocalObjectReference `json:"secretRef"`
}

// SourceConfiguration defines a single Knative Eventing source connected to the Capp.
// +kubebuilder:validation:XValidation:rule="(has(self.pingSourceConfiguration) && !has(self.kafkaSourceConfiguration)) || (!has(self.pingSourceConfiguration) && has(self.kafkaSourceConfiguration))",message="exactly one of pingSourceConfiguration or kafkaSourceConfiguration must be set"
type SourceConfiguration struct {
	// Name is a unique logical identifier for this source within the Capp.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// URI is a relative path appended to the resolved Capp Knative Service address (e.g. "/path").
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self.startsWith('/')",message="uri must be a relative path"
	URI *kapis.URL `json:"uri,omitempty"`

	// Configuration for a PingSource.
	// +optional
	PingSourceConfiguration *PingSourceConfiguration `json:"pingSourceConfiguration,omitempty"`

	// Configuration for a KafkaSource.
	// +optional
	KafkaSourceConfiguration *KafkaSourceConfiguration `json:"kafkaSourceConfiguration,omitempty"`
}

// VolumesSpec defines the volumes specification for the Capp.
type VolumesSpec struct {
	// NFSVolumes is a list of NFS volumes to be mounted.
	NFSVolumes []NFSVolume `json:"nfsVolumes,omitempty"`
}

// NFSVolume defines the NFS volume specification for the Capp.
type NFSVolume struct {
	// Server is the hostname or IP address of the NFS server.
	// +kubebuilder:validation:MinLength=1
	Server string `json:"server"`

	// Path is the exported path on the NFS server.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self.startsWith('/')",message="path must start with '/'"
	Path string `json:"path"`

	// Name is the name of the volume.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// Capacity is the capacity of the volume.
	Capacity corev1.ResourceList `json:"capacity"`
}

// RouteSpec defines the route specification for the Capp.
// +kubebuilder:validation:XValidation:rule="!has(self.tlsEnabled) || !self.tlsEnabled || (has(self.hostname) && size(self.hostname) > 0)",message="hostname must be set when tlsEnabled is true"
type RouteSpec struct {
	// Hostname is the custom DNS name for the Capp route.
	// Required when tlsEnabled is true.
	// +optional
	Hostname string `json:"hostname,omitempty"`

	// TlsEnabled enables HTTPS and automatic certificate management for hostname.
	// +optional
	TlsEnabled bool `json:"tlsEnabled,omitempty"`

	// RouteTimeoutSeconds is the maximum duration in seconds
	// that the request instance is allowed to respond to a request.
	// +optional
	RouteTimeoutSeconds *int64 `json:"routeTimeoutSeconds,omitempty"`

	// RolloutStrategy defines how traffic is split between the stable and candidate revisions
	// of the Capp when a new revision is rolled out.
	// +optional
	RolloutStrategy *RolloutStrategy `json:"rolloutStrategy,omitempty"`
}

// RolloutStrategy defines a staged rollout of a new Capp revision.
type RolloutStrategy struct {
	// Stages is the ordered list of traffic stages the candidate revision goes through.
	// +kubebuilder:validation:MinItems=1
	Stages []RolloutStage `json:"stages"`

	// AutoRollback rolls traffic back to the stable revision when the candidate reports Ready=False or
	// misses the ProgressDeadline before it first becomes ready. When false, the rollout is marked Failed instead.
	// +optional
	AutoRollback bool `json:"autoRollback,omitempty"`

	// ProgressDeadline is the maximum duration the candidate revision may take to first become ready.
	// Defaults to 10m when unset.
	// +optional
	ProgressDeadline *metav1.Duration `json:"progressDeadline,omitempty"`
}

// RolloutStage defines a single traffic stage of a staged rollout.
type RolloutStage struct {
	// Percent is the percentage of traffic routed to the candidate revision during this stage.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Percent int64 `json:"percent"`

	// Duration is the minimum time the stage is held before advancing to the next stage. It is counted from when
	// the operator observes that Knative serves the stage's traffic split, so the stage can last longer.
	// Every stage except the last must set it. The last stage must not set it: a final stage at 100% promotes
	// the candidate as soon as it is reached, and a final stage below 100% holds until it is promoted or aborted.
	// +optional
	Duration *metav1.Duration `json:"duration,omitempty"`
}

// RolloutPhase is the current phase of a staged rollout.
type RolloutPhase string

const (
	// RolloutPhaseStable indicates that no rollout is in progress.
	RolloutPhaseStable RolloutPhase = "Stable"

	// RolloutPhaseProgressing indicates that the candidate revision is receiving traffic.
	RolloutPhaseProgressing RolloutPhase = "Progressing"

	// RolloutPhaseHolding indicates that the rollout is paused until the user promotes or aborts it. A rollout pauses
	// at a final stage below 100%, or when a strategy change leaves no stage at or below the share the candidate
	// serves. The second kind of hold can also be retried.
	RolloutPhaseHolding RolloutPhase = "Holding"

	// RolloutPhasePromoting indicates that the candidate revision receives all traffic and the operator is waiting
	// for Knative to confirm that split before the candidate becomes the stable revision.
	RolloutPhasePromoting RolloutPhase = "Promoting"

	// RolloutPhaseFailed indicates that the candidate revision failed to roll out.
	RolloutPhaseFailed RolloutPhase = "Failed"

	// RolloutPhaseRolledBack indicates that the candidate revision was rolled back to the stable revision.
	RolloutPhaseRolledBack RolloutPhase = "RolledBack"

	// RolloutPhaseAborted indicates that the rollout was aborted by a user.
	RolloutPhaseAborted RolloutPhase = "Aborted"
)

// RevisionTraffic describes the traffic share routed to a single revision.
type RevisionTraffic struct {
	// RevisionName is the name of the revision receiving traffic.
	RevisionName string `json:"revisionName"`

	// Percent is the percentage of traffic routed to the revision.
	Percent int64 `json:"percent"`

	// Tag is the tag assigned to the revision in the traffic routing table.
	Tag string `json:"tag"`
}

// RolloutStatus shows the observed state of a staged rollout.
type RolloutStatus struct {
	// StableRevision is the name of the revision currently considered stable.
	// +optional
	StableRevision string `json:"stableRevision,omitempty"`

	// CandidateRevision is the name of the revision being rolled out.
	// +optional
	CandidateRevision string `json:"candidateRevision,omitempty"`

	// PinnedRevision is set when the strategy was removed while the candidate was not promoted. Traffic stays on
	// the pinned stable revision until a newer revision is Ready. Empty otherwise.
	// +optional
	PinnedRevision string `json:"pinnedRevision,omitempty"`

	// Phase is the current phase of the rollout.
	// +optional
	Phase RolloutPhase `json:"phase,omitempty"`

	// StageIndex is the index of the current stage in RolloutStrategy.Stages.
	// +optional
	StageIndex int32 `json:"stageIndex,omitempty"`

	// HeldPercent is the candidate's share of traffic while the rollout is held because the strategy was changed
	// and no stage is at or below the share being served. The candidate keeps this share and never gets more
	// until the user promotes, aborts or retries the rollout. A promotion keeps it too, so a failed promotion
	// returns to this share. A failed promotion from a strategy whose only stage is 100% sets it to 0 instead, and
	// the hold is cleared once the candidate recovers, unless a strategy edit during that fallback moves the
	// rollout off the final 100% stage. A further strategy edit releases the hold only when a new stage is at or
	// below the held share: the rollout moves to the last such stage and continues. Otherwise the hold stays, so
	// traffic never increases as a direct result of the edit.
	// Nil when the rollout follows its stages.
	// +optional
	HeldPercent *int64 `json:"heldPercent,omitempty"`

	// PromotionFallback is true while HeldPercent is the 0% hold that a failed promotion from a single 100% stage set.
	// When the candidate recovers, the hold is lifted only if the rollout is still at the final 100% stage, so it
	// promotes again. Otherwise, a strategy edit during the fallback has moved the rollout off that stage, and the hold
	// stays until a user action or a further edit releases it, as described for HeldPercent. False otherwise.
	// +optional
	PromotionFallback bool `json:"promotionFallback,omitempty"`

	// StageStartTime is when the operator observed that Knative serves the current stage's traffic split.
	// Nil until then, and cleared when the rollout moves to another stage.
	// The stage duration is a minimum: the stage lasts at least that long, counted from this time.
	// Operator downtime can make a stage longer, never shorter.
	// +optional
	StageStartTime *metav1.Time `json:"stageStartTime,omitempty"`

	// CandidateSince is the start of the ProgressDeadline. It is the creation time of the candidate Revision,
	// or the time of the retry after a retry. When the Revision could not be read, it falls back to the time
	// the operator first saw the candidate, and moves to the creation time once the Revision is read.
	// +optional
	CandidateSince *metav1.Time `json:"candidateSince,omitempty"`

	// CandidateSinceFallback is true while CandidateSince is the time the operator first saw the candidate because
	// its Revision could not be read. It is false once CandidateSince is the Revision's creation time or a retry time,
	// so a later read of the Revision does not move a retry deadline back.
	// +optional
	CandidateSinceFallback bool `json:"candidateSinceFallback,omitempty"`

	// CandidateReadyOnce is true once the candidate revision has been Ready. The ProgressDeadline applies
	// only until then.
	// +optional
	CandidateReadyOnce bool `json:"candidateReadyOnce,omitempty"`

	// Traffic is the observed traffic routing table of the stable and candidate revisions.
	// +optional
	Traffic []RevisionTraffic `json:"traffic,omitempty"`

	// Message is a human-readable description of the rollout state.
	// +optional
	Message string `json:"message,omitempty"`
}

type LogType string

const (
	LogTypeElastic           LogType = "elastic"
	LogTypeElasticDataStream LogType = "elastic-datastream"
)

// LogSpec defines the configuration for shipping Capp logs.
// +kubebuilder:validation:XValidation:rule="!has(self.type) || (has(self.host) && size(self.host) > 0 && has(self.target) && size(self.target) > 0 && has(self.user) && size(self.user) > 0 && has(self.passwordSecret) && size(self.passwordSecret) > 0 && has(self.passwordKey) && size(self.passwordKey) > 0)",message="log configuration requires host, target, user, passwordSecret, and passwordKey"
// +kubebuilder:validation:XValidation:rule="(!has(self.host) || size(self.host) == 0) && (!has(self.target) || size(self.target) == 0) && (!has(self.user) || size(self.user) == 0) && (!has(self.passwordSecret) || size(self.passwordSecret) == 0) && (!has(self.passwordKey) || size(self.passwordKey) == 0) || (has(self.type) && (self.type == 'elastic' || self.type == 'elastic-datastream'))",message="type must be elastic or elastic-datastream when log configuration fields are set"
type LogSpec struct {
	// Type defines where to send the Capp logs
	// +kubebuilder:validation:Enum=elastic;elastic-datastream
	// +optional
	Type LogType `json:"type,omitempty"`

	// Host defines the hostname of the logs destination.
	// Must be a hostname only, without scheme, port or path (e.g. elasticsearch.example.com).
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	// +optional
	Host string `json:"host,omitempty"`

	// Target defines where to write the logs to:
	// the index name if type is set to "elastic",
	// or the data stream name if type is set to "elastic-datastream".
	// +optional
	Target string `json:"target,omitempty"`

	// User defines a User for authentication.
	// +optional
	User string `json:"user,omitempty"`

	// PasswordSecret defines the name of the secret
	// containing the password for authentication.
	// +optional
	PasswordSecret string `json:"passwordSecret,omitempty"`

	// PasswordKey defines the key inside PasswordSecret that holds the password.
	// +optional
	PasswordKey string `json:"passwordKey,omitempty"`
}

// RevisionInfo shows the revision information.
type RevisionInfo struct {
	// RevisionStatus communicates the observed state of the Revision (from the controller).
	// +optional
	RevisionStatus knativev1.RevisionStatus `json:"revisionsStatus,omitempty"`

	// RevisionName is the name of the revision.
	// +optional
	RevisionName string `json:"name,omitempty"`
}

type StateStatus struct {
	// State is actual enabled state of the capp
	// +optional
	State string `json:"state,omitempty"`

	// LastChange is the last time the state of capp changed
	// +optional
	LastChange metav1.Time `json:"lastChange,omitempty"`
}

// EventingStatus shows the observed state of all event sources linked to the Capp.
type EventingStatus struct {
	// EventSources lists the status of each owned event source resource.
	// +optional
	EventSources []EventSourceStatus `json:"eventSources,omitempty"`
}

// EventSourceStatus shows the observed state of a single event source resource.
type EventSourceStatus struct {
	// Name is the K8s name of the underlying source resource.
	// +optional
	Name string `json:"name,omitempty"`

	// Condition holds the readiness condition for the underlying source resource.
	Condition kapis.Condition `json:"condition"`
}

// LoggingStatus defines the state of the SyslogNGFlow and SyslogNGOutput objects linked to the Capp.
type LoggingStatus struct {
	// SyslogNGFlow represents the Status of the SyslogNGFlow used by the Capp.
	// +optional
	SyslogNGFlow loggingv1beta1.SyslogNGFlowStatus `json:"syslogngflow,omitempty"`

	// SyslogNGOutput represents the Status of the SyslogNGOutput used by the Capp.
	// +optional
	SyslogNGOutput loggingv1beta1.SyslogNGOutputStatus `json:"syslogngoutput,omitempty"`

	// Conditions contain details about the current state of the SyslogNGFlow and SyslogNGOutput used by the Capp.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// RouteStatus shows the state of the DomainMapping object linked to the Capp.
type RouteStatus struct {
	// DomainMappingObjectStatus is the status of the underlying DomainMapping object
	// +optional
	DomainMappingObjectStatus knativev1beta1.DomainMappingStatus `json:"domainMappingObjectStatus,omitempty"`

	// ARecordSetObjectStatus is the status of the underlying ARecordSet object
	// +optional
	DNSRecordObjectStatus DNSRecordObjectStatus `json:"dnsRecordObjectStatus,omitempty"`

	// CertificateObjectStatus is the status of the underlying Certificate object
	// +optional
	CertificateObjectStatus cmapi.CertificateStatus `json:"certificateObjectStatus,omitempty"`
}

type DNSRecordObjectStatus struct {
	// CNAMERecordObjectStatus is the status of the underlying ARecordSet object
	// +optional
	CNAMERecordObjectStatus dnsrecordv1alpha1.CNAMERecordStatus `json:"cnameRecordObjectStatus,omitempty"`
}

// VolumesStatus shows the state of the Volumes objects linked to the Capp.
type VolumesStatus struct {
	// NFSVolumeStatus is the status of the underlying NFSVolume objects.
	NFSVolumesStatus []NFSVolumeStatus `json:"nfsVolumesStatus,omitempty"`
}

type NFSVolumeStatus struct {
	// VolumeName is the name of the volume.
	VolumeName string `json:"volumeName,omitempty"`

	// NFSPVCStatus is the status of the underlying NfsPvc object.
	NFSPVCStatus nfspvcv1alpha1.NfsPvcStatus `json:"nfsPvcStatus,omitempty"`
}

// CappStatus defines the observed state of Capp.
type CappStatus struct {
	// KnativeObjectStatus represents the Status stanza of the Service resource.
	// +optional
	//
	// Deprecated: use the ServingReady condition. Will be removed in a future release.
	KnativeObjectStatus knativev1.ServiceStatus `json:"knativeObjectStatus,omitempty"`

	// RevisionInfo shows the revision information.
	// +optional
	//
	// Deprecated: use the ServingReady condition. Will be removed in a future release.
	RevisionInfo []RevisionInfo `json:"revisions,omitempty"`

	// StateStatus shows the current Capp state
	// +optional
	StateStatus StateStatus `json:"stateStatus,omitempty"`

	// LoggingStatus defines the state of the Flow and Output objects linked to the Capp.
	// +optional
	//
	// Deprecated: use the LoggingReady condition. Will be removed in a future release.
	LoggingStatus LoggingStatus `json:"loggingStatus,omitempty"`

	// RouteStatus shows the state of the DomainMapping object linked to the Capp.
	// +optional
	//
	// Deprecated: use the RouteReady condition. Will be removed in a future release.
	RouteStatus RouteStatus `json:"routeStatus,omitempty"`

	// VolumesStatus shows the state of the Volumes objects linked to the Capp.
	// +optional
	//
	// Deprecated: use the VolumesReady condition. Will be removed in a future release.
	VolumesStatus VolumesStatus `json:"volumesStatus,omitempty"`

	// EventingStatus shows the state of event sources linked to the Capp.
	// +optional
	EventingStatus EventingStatus `json:"eventingStatus,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions contain details about the current state of the Capp.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Rollout shows the state of the staged rollout of the Capp.
	// +optional
	Rollout *RolloutStatus `json:"rollout,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Custom URL",type="string",JSONPath=".spec.routeSpec.hostname",description="shorten url"
// +kubebuilder:printcolumn:name="AutoScale Type",type="string",JSONPath=".spec.scaleSpec.metric",description="autoscale metric"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status",description="whether the Capp and its child resources are ready"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].reason",description="reason for the current Ready status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].message",priority=1,description="message for the current Ready status"
// +kubebuilder:subresource:status

// Capp is the Schema for the capps API.
type Capp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// CappSpec defines the desired state of Capp.
	// +optional
	Spec CappSpec `json:"spec,omitempty"`

	// CappStatus defines the observed state of Capp.
	// +optional
	Status CappStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CappList contains a list of Capp.
type CappList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Capp `json:"items"`
}
