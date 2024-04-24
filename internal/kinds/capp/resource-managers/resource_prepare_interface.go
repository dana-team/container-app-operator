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
	eventCappDisabled = "CappDisabled"
	eventCappEnabled  = "CappEnabled"

	KnativeServing                        = "knativeServing"
	eventCappKnativeServiceCreationFailed = "KnativeServiceCreationFailed"

	DomainMapping                        = "domainMapping"
	eventCappDomainMappingCreationFailed = "DomainMappingCreationFailed"

	NFSPVC                    = "NfsPvc"
	eventNFSPVCCreationFailed = "NfsPvcCreationFailed"
	eventNFSPVCCreated        = "NfsPvcCreated"

	SyslogNGFlow                          = "syslogNGFlow"
	SyslogNGOutput                        = "syslogNGOutput"
	eventCappSyslogNGFlowCreationFailed   = "SyslogNGFlowCreationFailed"
	eventCappSyslogNGFlowCreated          = "SyslogNGFlowCreated"
	eventCappSyslogNGOutputCreationFailed = "SyslogNGOutputCreationFailed"
	eventCappSyslogNGlSOutputCreated      = "SyslogNGOutputCreated"
)
