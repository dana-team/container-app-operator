package controllers

const (
	TypeReady = "Ready"

	ReasonBuildReconcileFailed = "BuildReconcileFailed"
	ReasonBuildConflict        = "BuildConflict"
	ReasonMissingPolicy        = "MissingPolicy"
	ReasonSourceAccessFailed   = "SourceAccessFailed"
)
