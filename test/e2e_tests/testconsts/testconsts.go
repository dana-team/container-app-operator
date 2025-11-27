package testconsts

import (
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
)

const (
	Timeout                         = 240 * time.Second
	TimeoutCapp                     = 90 * time.Second
	Interval                        = 2 * time.Second
	DefaultEventually               = 2 * time.Second
	DefaultConsistently             = 30 * time.Second
	ClientListLimit                 = 100
	CappKey                         = "capp"
	Charset                         = "abcdefghijklmnopqrstuvwxyz0123456789"
	RandStrLength                   = 10
	UnsupportedScaleMetric          = "storage"
	EnabledState                    = "enabled"
	DisabledState                   = "disabled"
	KnativeMetricAnnotation         = "autoscaling.knative.dev/metric"
	ImageExample                    = "danateam/autoscale-go"
	ExampleAppName                  = "new-app-name"
	NewSecretKey                    = "username"
	ExampleDanaAnnotation           = "rcs.dana.io/app-name"
	TestContainerName               = "capp-test-container"
	FirstRevisionSuffix             = "-00001"
	KnativeAutoscaleTargetKey       = "autoscaling.knative.dev/target"
	KnativeActivationScaleKey       = "autoscaling.knative.dev/activation-scale"
	TestIndex                       = "test"
	TestLabelKey                    = "e2e-test"
	CappConfigName                  = "capp-config"
	CappName                        = "capp-test"
	NSName                          = "capp-e2e-tests"
	RPSScaleMetric                  = "rps"
	SecretKey                       = "extra"
	SecretValue                     = "YmFyCg=="
	ControllerNS                    = "container-app-operator-system"
	ZoneValue                       = "capp-zone.com."
	CappBaseImage                   = "ghcr.io/dana-team/capp-gin-app:v0.2.0"
	PassEnvName                     = "PASSWORD"
	ElasticType                     = "elastic"
	ElasticHost                     = "1.2.3.4"
	MainIndex                       = "main"
	ElasticUserName                 = "elastic"
	ElasticSecretName               = "credentials"
	Server                          = "nfs-server"
	Path                            = "/nfs-path"
	Capacity                        = "1Gi"
	RouteHostname                   = "test.dev"
	RouteTLSSecret                  = "https-capp-secret"
	ServiceAccountNameFormat        = "system:serviceaccount:%s:%s"
	ServiceAccountName              = "test-user"
	ExcludedServiceAccountName      = "excluded-sa"
	ExcludedServiceAccountNamespace = "container-app-operator-system"
)

var (
	CappAPIGroup               = cappv1alpha1.GroupVersion.Group
	CappNamespaceKey           = CappAPIGroup + "/parent-capp-ns"
	CappResourceKey            = CappAPIGroup + "/parent-capp"
	ManagedByLabelKey          = CappAPIGroup + "/managed-by"
	LastUpdatedByAnnotationKey = CappAPIGroup + "/last-updated-by"
	CappNameLabelKey           = CappAPIGroup + "/cappName"
)
