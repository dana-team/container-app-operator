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
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
)

// AuthSecretTarget maps a secret into the TriggerAuth
type AuthSecretTarget struct {
	Parameter string                   `json:"parameter"`
	SecretRef corev1.SecretKeySelector `json:"secretRef"`
}

// AuthEnvTarget maps an environment variable into TriggerAuth
type AuthEnvTarget struct {
	Parameter string `json:"parameter"`
	Name      string `json:"name"`
}

// PodIdentity config for cloud identity bindings
type PodIdentity struct {
	// +kubebuilder:validation:Enum=none;azure;aws;gcp
	Provider string `json:"provider"`
}

// CappSpec defines the desired state of Capp.
type CappSpec struct {
	// ScaleMetric defines which metric type is watched by the Autoscaler.
	// Possible values examples: "concurrency", "rps", "cpu", "memory", "external".
	// +kubebuilder:default:="concurrency"
	// +kubebuilder:validation:Enum=cpu;memory;rps;concurrency;external
	ScaleMetric string `json:"scaleMetric,omitempty"`
	// State defines the state of capp
	// Possible values examples: "enabled", "disabled".
	// +optional
	// +kubebuilder:default:="enabled"
	// +kubebuilder:validation:Enum=enabled;disabled
	State string `json:"state,omitempty"`

	// ConfigurationSpec holds the desired state of the Configuration (from the client).
	ConfigurationSpec knativev1.ConfigurationSpec `json:"configurationSpec"`

	// RouteSpec defines the route specification for the Capp.
	// +optional
	RouteSpec RouteSpec `json:"routeSpec,omitempty"`

	// LogSpec defines the configuration for shipping Capp logs.
	LogSpec LogSpec `json:"logSpec,omitempty"`

	// VolumesSpec defines the volumes specification for the Capp.
	VolumesSpec VolumesSpec `json:"volumesSpec,omitempty"`

	// Sources define the configuration and status of event sources
	Sources []KedaSource `json:"sources,omitempty"`
}

// VolumesSpec defines the volumes specification for the Capp.
type VolumesSpec struct {
	// NFSVolumes is a list of NFS volumes to be mounted.
	NFSVolumes []NFSVolume `json:"nfsVolumes,omitempty"`
}

// NFSVolume defines the NFS volume specification for the Capp.
type NFSVolume struct {
	// Server is the hostname or IP address of the NFS server.
	Server string `json:"server"`

	// Path is the exported path on the NFS server.
	Path string `json:"path"`

	// Name is the name of the volume.
	Name string `json:"name"`

	// Capacity is the capacity of the volume.
	Capacity corev1.ResourceList `json:"capacity"`
}

// RouteSpec defines the route specification for the Capp.
type RouteSpec struct {
	// Hostname is a custom DNS name for the Capp route.
	// +optional
	Hostname string `json:"hostname,omitempty"`

	// TlsEnabled determines whether to enable TLS for the Capp route.
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

// LogSpec defines the configuration for shipping Capp logs.
type LogSpec struct {
	// Type defines where to send the Capp logs
	// +kubebuilder:validation:Enum=elastic
	// +optional
	Type string `json:"type,omitempty"`

	// Host defines Elasticsearch or Splunk host.
	// +optional
	Host string `json:"host,omitempty"`

	// Index defines the index name to write events to.
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

// ApplicationLinks contains relevant information about
// the cluster that the Capp is deployed in.
type ApplicationLinks struct {
	// ConsoleLink holds a link for the Openshift cluster console.
	// +optional
	ConsoleLink string `json:"consoleLink,omitempty"`

	// Site holds the cluster name that the Capp is deployed on.
	// +optional
	Site string `json:"site,omitempty"`
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

// KedaSource defines the configuration of a Keda sources
type KedaSource struct {

	// Name is the name of the Keda source
	Name string `json:"name"`

	// ScalarType defines the type of the scalar used
	ScalarType string `json:"scalarType"`

	// ScalarMetadata defines the data passed directly to the scalar
	ScalarMetadata map[string]string `json:"scalarMetadata,omitempty"`

	// TriggerAuth defines the authentication for the trigger (if needed)
	TriggerAuth *TriggerAuth `json:"triggerAuth,omitempty"`

	// MinReplicas is the minimum of replicas allowed
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the maximum of replicas allowed
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`
}

// TriggerAuth defines the authentication info needed for a scalar trigger
type TriggerAuth struct {

	// Type defines if the authentication object is "triggerAuthentication" or "clusterTriggerAuthentication"
	// +kubebuilder:validation:Enum=triggerAuthentication;clusterTriggerAuthentication
	Type string `json:"type"`

	// Name of the authentication object
	Name string `json:"name"`

	// SecretTargets are secrets the scalar may need (e.g Kafka scalar)
	SecretTargets []AuthSecretTarget `json:"secretTargets,omitempty"`

	// EnvTargets are environment variable the scalar may need
	EnvTargets []AuthEnvTarget `json:"envTargets,omitempty"`

	// PodIdentity information (AWS IAM, Azure AD, etc.)
	PodIdentity *PodIdentity `json:"podIdentity,omitempty"`
}

// KedaStatus defines the status of a Keda source
type KedaStatus struct {
	// Conditions contain details about the current state of the Keda source.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// CappStatus defines the observed state of Capp.
type CappStatus struct {
	// ApplicationLinks contains relevant information about
	// the cluster that the Capp is deployed in.
	// +optional
	ApplicationLinks ApplicationLinks `json:"applicationLinks,omitempty"`

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

	// Conditions contain details about the current state of the Capp.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// SourceStatus contains details about the current state of a source.
	SourceStatus []KedaStatus `json:"sourceStatus,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Site",type="string",JSONPath=".status.applicationLinks.site",description="cluster of the resource"
// +kubebuilder:printcolumn:name="Custom URL",type="string",JSONPath=".spec.routeSpec.hostname",description="shorten url"
// +kubebuilder:printcolumn:name="AutoScale Type",type="string",JSONPath=".spec.scaleMetric",description="autoscale metric"
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

func init() {
	SchemeBuilder.Register(&Capp{}, &CappList{})
}
