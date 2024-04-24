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
)

// CappRevisionSpec defines the desired state of CappRevision
type CappRevisionSpec struct {
	// RevisionNumber represent the revision number of Capp
	RevisionNumber int `json:"revisionNumber"`

	// CappTemplate holds the manifest of a specific Capp corresponding to a particular RevisionNumber
	CappTemplate CappTemplate `json:"cappTemplate"`
}

// CappRevisionStatus defines the observed state of CappRevision
type CappRevisionStatus struct {
}

// CappTemplate template of Capp.
type CappTemplate struct {
	// Spec is the related Capp spec
	Spec CappSpec `json:"cappSpec,omitempty"`

	// Labels is a map of string keys and values which are the actual labels of the related Capp
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations is a map of string keys and values which are the actual annotations of the related Capp
	// +optional
	Annotations map[string]string `json:"Annotations,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CappRevision is the Schema for the CappRevisions API
type CappRevision struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CappRevisionSpec   `json:"spec,omitempty"`
	Status CappRevisionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CappRevisionList contains a list of CappRevision
type CappRevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CappRevision `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CappRevision{}, &CappRevisionList{})
}
