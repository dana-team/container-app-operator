package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HostnamePattern defines a regex pattern for validating Capp hostnames.
type HostnamePattern struct {
	// Match is a regex used to match Capp hostnames.
	// +kubebuilder:validation:MinLength=1
	Match string `json:"match"`
	// Explanation is a human-readable description shown in webhook error messages.
	// +kubebuilder:validation:MaxLength=100
	// +optional
	Explanation string `json:"explanation,omitempty"`
}

// CappConfigSpec defines the desired state of CappConfig
type CappConfigSpec struct {
	// +kubebuilder:validation:Required
	DNSConfig DNSConfig `json:"dnsConfig"`

	// +kubebuilder:validation:Required
	AutoscaleConfig AutoscaleConfig `json:"autoscaleConfig"`

	// DefaultResources is the default resources to be assigned to Capp.
	// If other resources are specified then they override the default values.
	DefaultResources corev1.ResourceRequirements `json:"defaultResources"`

	// AllowedHostnamePatterns is a list of hostname patterns used to validate Capp hostnames.
	// If the Capp hostname matches a pattern, it is allowed to be created.
	// Defaults to an empty list (all hostnames denied) if not specified.
	// +kubebuilder:default:={}
	AllowedHostnamePatterns []HostnamePattern `json:"allowedHostnamePatterns"`

	// RevisionHistoryLimit defines how many CappRevisions will be retained
	// +kubebuilder:default:=10
	// +kubebuilder:validation:Minimum=1
	RevisionHistoryLimit int `json:"revisionHistoryLimit,omitempty"`
}

// IssuerRef identifies a cert-manager issuer by name, kind, and API group.
type IssuerRef struct {
	// Name is the name of the certificate issuer.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Kind is the kind of the certificate issuer (e.g. ClusterIssuer).
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`
	// Group is the API group of the certificate issuer (e.g. cert-manager.io).
	// +kubebuilder:validation:MinLength=1
	Group string `json:"group"`
}

type DNSConfig struct {
	// Zone defines the DNS zone for Capp Hostnames.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self.endsWith('.')",message="zone must end with '.'"
	Zone string `json:"zone"`
	// CNAME defines the CNAME record that will be used for Capp Hostnames.
	// +kubebuilder:validation:MinLength=1
	CNAME string `json:"cname"`
	// Provider defines the DNS provider.
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`
	// IssuerRef identifies the cert-manager issuer used to issue certificates.
	IssuerRef IssuerRef `json:"issuerRef"`
}

type AutoscaleConfig struct {
	// RPS is the desired requests per second to trigger upscaling.
	// +kubebuilder:validation:Minimum=1
	RPS int `json:"rps"`
	// CPU is the desired CPU utilization to trigger upscaling.
	// +kubebuilder:validation:Minimum=1
	CPU int `json:"cpu"`
	// Memory is the desired memory utilization to trigger upscaling.
	// +kubebuilder:validation:Minimum=1
	Memory int `json:"memory"`
	// Concurrency is the maximum concurrency of a Capp.
	// +kubebuilder:validation:Minimum=1
	Concurrency int `json:"concurrency"`
	// ActivationScale is the default number of replicas used when a scale-to-zero Capp scales up from idle.
	// +kubebuilder:validation:Minimum=2
	ActivationScale int `json:"activationScale"`
	// MinReplicasLimit is the global minimum scale. (maximum allowed value for minReplicas).
	// +kubebuilder:validation:Minimum=1
	MinReplicasLimit int `json:"minReplicasLimit"`
	// MaxScaleDelay is the maximum delay in seconds before the Autoscaler scales down the Capp to zero.
	// +kubebuilder:default:=3600
	// +kubebuilder:validation:Minimum=0
	MaxScaleDelay int `json:"maxScaleDelay"`
}

// CappConfigStatus defines the observed state of CappConfig
type CappConfigStatus struct{}

// +kubebuilder:object:root=true

// CappConfig is the Schema for the cappconfigs API
type CappConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CappConfigSpec   `json:"spec,omitempty"`
	Status CappConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CappConfigList contains a list of CappConfig
type CappConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CappConfig `json:"items"`
}
