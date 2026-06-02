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
	// +kubebuilder:default:=0
	// +kubebuilder:validation:Minimum=0
	// +optional
	MinReplicas int `json:"minReplicas,omitempty"`

	// ScaleDelaySeconds is the delay in seconds before the Autoscaler scales down the Capp to zero.
	// +kubebuilder:default:=0
	// +kubebuilder:validation:Minimum=0
	// +optional
	ScaleDelaySeconds int `json:"scaleDelaySeconds,omitempty"`
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

// SourceConfiguration defines a single Knative Eventing source connected to the Capp.
type SourceConfiguration struct {
	// Name is a unique logical identifier for this source within the Capp.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// URI is a relative path appended to the resolved Capp Knative Service address (e.g. "/path").
	// +kubebuilder:validation:Optional
	// +kubebuilder:xValidation:rule="self == null || self.startsWith('/')",message="uri must be a relative path"
	URI *kapis.URL `json:"uri,omitempty"`

	// Configuration for a PingSource.
	// +optional
	PingSourceConfiguration *PingSourceConfiguration `json:"pingSourceConfiguration,omitempty"`
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

	// TrafficTarget holds a single entry of the routing table for the Capp route.
	// +optional
	TrafficTarget knativev1.TrafficTarget `json:"trafficTarget,omitempty"`

	// RouteTimeoutSeconds is the maximum duration in seconds
	// that the request instance is allowed to respond to a request.
	// +optional
	RouteTimeoutSeconds *int64 `json:"routeTimeoutSeconds,omitempty"`
}

type LogType string

const (
	LogTypeElastic           LogType = "elastic"
	LogTypeElasticDataStream LogType = "elastic-datastream"
)

// LogSpec defines the configuration for shipping Capp logs.
// +kubebuilder:validation:XValidation:rule="!has(self.type) || self.type != 'elastic' || (has(self.host) && size(self.host) > 0 && has(self.index) && size(self.index) > 0 && has(self.user) && size(self.user) > 0 && has(self.passwordSecret) && size(self.passwordSecret) > 0)",message="elastic log configuration requires host, index, user, and passwordSecret"
// +kubebuilder:validation:XValidation:rule="!has(self.type) || self.type != 'elastic-datastream' || (has(self.host) && size(self.host) > 0 && has(self.user) && size(self.user) > 0 && has(self.passwordSecret) && size(self.passwordSecret) > 0)",message="elastic-datastream log configuration requires host, user, and passwordSecret"
// +kubebuilder:validation:XValidation:rule="(!has(self.host) || size(self.host) == 0) && (!has(self.index) || size(self.index) == 0) && (!has(self.user) || size(self.user) == 0) && (!has(self.passwordSecret) || size(self.passwordSecret) == 0) || (has(self.type) && (self.type == 'elastic' || self.type == 'elastic-datastream'))",message="type must be elastic or elastic-datastream when log configuration fields are set"
type LogSpec struct {
	// Type defines where to send the Capp logs
	// +kubebuilder:validation:Enum=elastic;elastic-datastream
	// +optional
	Type LogType `json:"type,omitempty"`

	// Host defines Elasticsearch or Splunk host.
	// Should include full URL with protocol and port (e.g. https://elasticsearch:9200/_bulk).
	// +optional
	Host string `json:"host,omitempty"`

	// Index defines the index name to write events to.
	// Ignored if type is set to "elastic-datastream".
	// +optional
	Index string `json:"index,omitempty"`

	// User defines a User for authentication.
	// +optional
	User string `json:"user,omitempty"`

	// PasswordSecret defines the name of the secret
	// containing the password for authentication.
	// +optional
	PasswordSecret string `json:"passwordSecret,omitempty"`
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
	KnativeObjectStatus knativev1.ServiceStatus `json:"knativeObjectStatus,omitempty"`

	// RevisionInfo shows the revision information.
	// +optional
	RevisionInfo []RevisionInfo `json:"revisions,omitempty"`

	// StateStatus shows the current Capp state
	// +optional
	StateStatus StateStatus `json:"stateStatus,omitempty"`

	// LoggingStatus defines the state of the Flow and Output objects linked to the Capp.
	// +optional
	LoggingStatus LoggingStatus `json:"loggingStatus,omitempty"`

	// RouteStatus shows the state of the DomainMapping object linked to the Capp.
	// +optional
	RouteStatus RouteStatus `json:"routeStatus,omitempty"`

	// VolumesStatus shows the state of the Volumes objects linked to the Capp.
	// +optional
	VolumesStatus VolumesStatus `json:"volumesStatus,omitempty"`

	// EventingStatus shows the state of event sources linked to the Capp.
	// +optional
	EventingStatus EventingStatus `json:"eventingStatus,omitempty"`

	// Conditions contain details about the current state of the Capp.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Custom URL",type="string",JSONPath=".spec.routeSpec.hostname",description="shorten url"
// +kubebuilder:printcolumn:name="AutoScale Type",type="string",JSONPath=".spec.scaleSpec.metric",description="autoscale metric"
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
