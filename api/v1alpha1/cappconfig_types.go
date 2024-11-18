package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CappConfigSpec defines the desired state of CappConfig
type CappConfigSpec struct {
	DNSConfig       DNSConfig       `json:"dnsConfig"`
	AutoscaleConfig AutoscaleConfig `json:"autoscalerConfig"`
}

type DNSConfig struct {
	Zone     string `json:"zone"`
	CNAME    string `json:"cname"`
	Provider string `json:"provider"`
	Issuer   string `json:"issuer"`
}

type AutoscaleConfig struct {
	RPS             string `json:"rps"`
	CPU             string `json:"cpu"`
	Memory          string `json:"memory"`
	Concurrency     string `json:"concurrency"`
	ActivationScale string `json:"activationScale"`
}

// CappConfigStatus defines the observed state of CappConfig
type CappConfigStatus struct{}

//+kubebuilder:object:root=true

// CappConfig is the Schema for the cappconfigs API
type CappConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CappConfigSpec   `json:"spec,omitempty"`
	Status CappConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CappConfigList contains a list of CappConfig
type CappConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CappConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CappConfig{}, &CappConfigList{})
}
