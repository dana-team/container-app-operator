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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CappSpec defines the desired state of Capp.
type CappSpec struct {
	// ScaleMetric defines which metric type is watched by the Autoscaler.
	// Possible values examples: "concurrency", "rps", "cpu", "memory".
	// +optional
	ScaleMetric string `json:"scaleMetric,omitempty"`

	// Site defines where to deploy the Capp.
	// It can be a specific cluster or a placement name.
	// +optional
	Site string `json:"site,omitempty"`

	// ConfigurationSpec holds the desired state of the Configuration (from the client).
	ConfigurationSpec knativev1.ConfigurationSpec `json:"configurationSpec"`

	// RouteSpec defines the route specification for the Capp.
	// +optional
	RouteSpec RouteSpec `json:"routeSpec,omitempty"`

	// LogSpec defines the configuration for shipping Capp logs.
	LogSpec LogSpec `json:"logSpec,omitempty"`
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

// ApplicationLinks contains relevant information about
// the cluster that the Capp is deployed in.
type ApplicationLinks struct {
	// ConsoleLink holds a link for the Openshift cluster console.
	// +optional
	ConsoleLink string `json:"consoleLink,omitempty"`

	// Site holds the cluster name that the Capp is deployed on.
	// +optional
	Site string `json:"site,omitempty"`

	// ClusterSegment holds the segment of the cluster
	// that the Capp is deployed on.
	// +optional
	ClusterSegment string `json:"clusterSegment,omitempty"`
}

// LogSpec defines the configuration for shipping Capp logs.
type LogSpec struct {
	// Type defines where to send the Capp logs
	// Possible values : "elastic" and "splunk".
	// +optional
	Type string `json:"type,omitempty"`

	// Host defines Elasticsearch or Splunk host.
	// +optional
	Host string `json:"host,omitempty"`

	// SSLVerify determines whether to skip ssl verification.
	// +optional
	SSLVerify bool `json:"sslVerify,omitempty"`

	// Index defines the index name to write events to.
	// +optional
	Index string `json:"index,omitempty"`

	// UserName defines a User for authentication.
	// +optional
	UserName string `json:"username,omitempty"`

	// PasswordSecretName defines the name of the secret
	// containing the password for authentication.
	// +optional
	PasswordSecretName string `json:"passwordSecretName,omitempty"`

	// HecTokenSecretName defines the name of the secret
	// containing the Splunk Hec token.
	// +optional
	HecTokenSecretName string `json:"hecTokenSecretName,omitempty"`
}

// RevisionInfo shows the revision information.
type RevisionInfo struct {
	// RevisionStatus communicates the observed state of the Revision (from the controller).
	// +optional
	RevisionStatus knativev1.RevisionStatus `json:"RevisionsStatus,omitempty"`

	// RevisionName is the name of the revision.
	// +optional
	RevisionName string `json:"name,omitempty"`
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
	RevisionInfo []RevisionInfo `json:"Revisions,omitempty"`

	// Conditions contain details about the current state of the Capp.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Site",type="string",JSONPath=".status.applicationLinks.site",description="cluster of the resource"
// +kubebuilder:printcolumn:name="Custom URL",type="string",JSONPath=".spec.routeSpec.hostname",description="shorten url"
// +kubebuilder:printcolumn:name="AutoScale Type",type="string",JSONPath=".spec.scaleMetric",description="autoscale metric"
//+kubebuilder:subresource:status

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

//+kubebuilder:object:root=true

// CappList contains a list of Capp.
type CappList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Capp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Capp{}, &CappList{})
}
