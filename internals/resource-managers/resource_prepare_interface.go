package resourceprepares

import (
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
)

type ResourceManager interface {
	CreateOrUpdateObject(capp rcsv1alpha1.Capp) error
	CleanUp(capp rcsv1alpha1.Capp) error
	IsRequired(capp rcsv1alpha1.Capp) bool
}

const (
	eventCappFlowCreationFailed           = "FlowCreationFailed"
	eventCappFlowCreated                  = "FlowCreated"
	eventCappDomainMappingCreationFailed  = "DomainMappingCreationFailed"
	eventCappKnativeServiceCreationFailed = "KnativeServiceCreationFailed"
	eventCappOutputCreationFailed         = "OutputCreationFailed"
	eventCappOutputCreated                = "OutputCreated"
	eventCappDisabled                     = "CappDisabled"
	eventCappEnabled                      = "CappEnabled"
	DomainMapping                         = "domainMapping"
	KnativeServing                        = "knativeServing"
	Flow                                  = "flow"
	Output                                = "output"
)
