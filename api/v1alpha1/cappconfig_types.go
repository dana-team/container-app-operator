package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CappConfigSpec defines the desired state of CappConfig
type CappConfigSpec struct {
	// +kubebuilder:validation:Required
	DNSConfig DNSConfig `json:"dnsConfig"`
	// +kubebuilder:validation:Required
	AutoscaleConfig AutoscaleConfig `json:"autoscaleConfig"`
}

type DNSConfig struct {
	// Zone defines the DNS zone for Capp Hostnames
	Zone string `json:"zone"`
	// CNAME defines the CNAME record that will be used for Capp Hostnames
	CNAME string `json:"cname"`
	// Provider defines the DNS provider
	Provider string `json:"provider"`
	// Issuer defines the certificate issuer
	Issuer string `json:"issuer"`
}

type AutoscaleConfig struct {
	// RPS is the desired requests per second to trigger upscaling.
	RPS int `json:"rps"`
	// CPU is the desired CPU utilization to trigger upscaling.
	CPU int `json:"cpu"`
	// Memory is the desired memory utilization to trigger upscaling.
	Memory int `json:"memory"`
	// Concurrency is the maximum concurrency of a Capp.
	Concurrency int `json:"concurrency"`
	// ActivationScale is the default scale.
	ActivationScale int `json:"activationScale"`
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

func init() {
	SchemeBuilder.Register(&CappConfig{}, &CappConfigList{})
}
