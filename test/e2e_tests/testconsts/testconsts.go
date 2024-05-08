package testconsts

import "time"

const (
	Timeout           = 300 * time.Second
	Interval          = 2 * time.Second
	DefaultEventually = 2 * time.Second
)

const (
	UnsupportedScaleMetric    = "storage"
	EnabledState              = "enabled"
	DisabledState             = "disabled"
	KnativeMetricAnnotation   = "autoscaling.knative.dev/metric"
	ImageExample              = "danateam/autoscale-go"
	NonExistingImageExample   = "example-python-app:v1"
	ExampleAppName            = "new-app-name"
	NewSecretKey              = "username"
	ExampleDanaAnnotation     = "rcs.dana.io/app-name"
	TestContainerName         = "capp-test-container"
	FirstRevisionSuffix       = "-00001"
	KnativeAutoscaleTargetKey = "autoscaling.knative.dev/target"
	TestIndex                 = "test"
	TestLabelKey              = "e2e-test"
	CappResourceKey           = "rcs.dana.io/parent-capp"
)
