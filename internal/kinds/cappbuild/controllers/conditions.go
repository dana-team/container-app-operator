package controllers

const (
	TypeReady = "Ready"

	ReasonBuildReconcileFailed  = "BuildReconcileFailed"
	ReasonBuildConflict         = "BuildConflict"
	ReasonBuildStrategyNotFound = "BuildStrategyNotFound"
	ReasonMissingPolicy         = "MissingPolicy"
	ReasonSourceAccessFailed    = "SourceAccessFailed"
)
