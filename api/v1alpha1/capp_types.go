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

// CappSpec defines the desired state of Capp
type CappSpec struct {
	ScaleMetric       string                      `json:"scaleMetric,omitempty"`
	Site              string                      `json:"site,omitempty"`
	ConfigurationSpec knativev1.ConfigurationSpec `json:"configurationSpec"`
	RouteSpec         RouteSpec                   `json:"routeSpec,omitempty"`
}

type RouteSpec struct {
	Hostname string `json:"hostname,omitempty"`
	//TrafficTarget knativev1.TrafficTarget `json:"trafficTarget,omitempty"`
}

type ApplicationLinks struct {
	ConsoleLink    string `json:"consoleLink,omitempty"`
	Site           string `json:"site,omitempty"`
	ClusterSegment string `json:"clusterSegment,omitempty"`
}

type RevisionInfo struct {
	RevisionStatus knativev1.RevisionStatus `json:"RevisionsStatus,omitempty"`
	RevisionName   string                   `json:"name,omitempty"`
}

// CappStatus defines the observed state of Capp
type CappStatus struct {
	ApplicationLinks    ApplicationLinks        `json:"applicationLinks,omitempty"`
	KnativeObjectStatus knativev1.ServiceStatus `json:"knativeObjectStatus,omitempty"`
	RevisionInfo        []RevisionInfo          `json:"Revisions,omitempty"`
	Conditions          []metav1.Condition      `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Site",type="string",JSONPath=".status.applicationLinks.site",description="cluster of the resource"
// +kubebuilder:printcolumn:name="Custom URL",type="string",JSONPath=".spec.routeSpec.hostname",description="shorten url"
// +kubebuilder:printcolumn:name="AutoScale Type",type="string",JSONPath=".spec.scaleMetric",description="autoscale metric"
//+kubebuilder:subresource:status

// Capp is the Schema for the capps API
type Capp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CappSpec   `json:"spec,omitempty"`
	Status CappStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CappList contains a list of Capp
type CappList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Capp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Capp{}, &CappList{})
}
