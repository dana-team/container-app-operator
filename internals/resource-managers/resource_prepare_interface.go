package resourceprepares

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
)

type ResourceManager interface {
	CreateOrUpdateObject(capp cappv1alpha1.Capp) error
	CleanUp(capp cappv1alpha1.Capp) error
	IsRequired(capp cappv1alpha1.Capp) bool
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
	eventNFSPVCCreationFailed             = "NfsPvcCreationFailed"
	eventNFSPVCCreated                    = "NfsPvcCreated"
	DomainMapping                         = "domainMapping"
	KnativeServing                        = "knativeServing"
	Flow                                  = "flow"
	Output                                = "output"
	NFSPVC                                = "NfsPvc"
)
