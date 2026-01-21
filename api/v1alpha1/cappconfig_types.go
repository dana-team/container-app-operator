package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CappConfigSpec defines the desired state of CappConfig
type CappConfigSpec struct {
	// +kubebuilder:validation:Required
	DNSConfig DNSConfig `json:"dnsConfig"`

	// +kubebuilder:validation:Required
	AutoscaleConfig AutoscaleConfig `json:"autoscaleConfig"`

	// DefaultResources is the default resources to be assigned to Capp.
	// If other resources are specified then they override the default values.
	DefaultResources corev1.ResourceRequirements `json:"defaultResources"`

	// AllowedHostnamePatterns is an optional slice of regex patterns to be used to validate the hostname of the Capp.
	// If the Capp hostname matches a pattern, it is allowed to be created.
	// +kubebuilder:default:={}
	AllowedHostnamePatterns []string `json:"allowedHostnamePatterns"`

	// +optional
	// CappBuild holds platform defaults/policy for the CappBuild feature.
	CappBuild *CappBuildConfig `json:"cappBuild,omitempty"`
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

type CappBuildConfig struct {
	// +optional
	// Output holds defaults for deriving the build output image.
	Output *CappBuildOutputConfig `json:"output,omitempty"`

	// ClusterBuildStrategy holds platform defaults for selecting a build strategy.
	ClusterBuildStrategy CappBuildClusterStrategyConfig `json:"clusterBuildStrategy"`

	// +optional
	// OnCommit configures webhook-triggered rebuilds.
	OnCommit *CappBuildOnCommitConfig `json:"onCommit,omitempty"`
}

type CappBuildOnCommitConfig struct {
	// Enabled enables webhook-triggered rebuilds.
	// +kubebuilder:default:=false
	Enabled bool `json:"enabled"`
}

type CappBuildOutputConfig struct {
	// +optional
	// DefaultImageRepo is the default OCI image repo (no tag/digest) for build outputs.
	// +kubebuilder:validation:MinLength=1
	DefaultImageRepo string `json:"defaultImageRepo,omitempty"`
}

type CappBuildClusterStrategyConfig struct {
	// BuildFile holds strategy defaults for selecting a strategy based on whether a
	// build file indicator is present (e.g. Dockerfile, Containerfile).
	BuildFile CappBuildFileStrategyConfig `json:"buildFile"`
}

type CappBuildFileStrategyConfig struct {
	// Present is the strategy name to use when the source indicates a file-based build.
	// +kubebuilder:validation:MinLength=1
	Present string `json:"present"`

	// Absent is the strategy name to use when the source does not indicate a file-based build.
	// +kubebuilder:validation:MinLength=1
	Absent string `json:"absent"`
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
