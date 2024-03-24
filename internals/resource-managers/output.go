package resourceprepares

import (
	"context"
	"fmt"
	"reflect"

	"github.com/cisco-open/operator-tools/pkg/secret"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internals/wrappers"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/output"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OutputManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

const (
	LogTypeElastic       string = "elastic"
	LogTypeSplunk        string = "splunk"
	KnativeConfiguration        = "serving.knative.dev/configuration"
	NginxPraser                 = "nginx"
	ElasticPort                 = 9200
	SplunkHecPort               = 8088
	ElasticSSLVersion           = "TLSv1_2"
	BufferTimekey               = "1m"
	BufferTimekeyWait           = "30s"
	BufferTimekeyUseUtc         = true
)

// createElasticsearchOutput creates an Elasticsearch output object based on the provided logSpec.
// It constructs the Elasticsearch output which is returned as an OutputSpec.
func createElasticsearchOutput(logSpec cappv1alpha1.LogSpec) loggingv1beta1.OutputSpec {
	protocol := "http"
	falseVar := false
	if logSpec.SSLVerify {
		protocol = "https"
	}
	outputSpec := loggingv1beta1.OutputSpec{
		ElasticsearchOutput: &output.ElasticsearchOutput{
			Host:       logSpec.Host,
			Port:       ElasticPort,
			Scheme:     protocol,
			SslVerify:  &falseVar,
			SslVersion: ElasticSSLVersion,
			User:       logSpec.UserName,
			Password: &secret.Secret{
				ValueFrom: &secret.ValueFrom{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: logSpec.PasswordSecretName},
						Key:                  "elastic",
					},
				},
			},
			IndexName: logSpec.Index,
			Buffer: &output.Buffer{
				Timekey:       BufferTimekey,
				TimekeyWait:   BufferTimekeyWait,
				TimekeyUseUtc: BufferTimekeyUseUtc,
			},
		},
	}
	return outputSpec
}

// createSplunkHecOutput creates a splunk output object based on the provided logSpec.
// It constructs the splunk output which is returned as an OutputSpec.
func createSplunkHecOutput(logSpec cappv1alpha1.LogSpec) loggingv1beta1.OutputSpec {
	protocol := "http"
	if logSpec.SSLVerify {
		protocol = "https"
	}
	insecureSSL := !logSpec.SSLVerify
	outputSpec := loggingv1beta1.OutputSpec{
		SplunkHecOutput: &output.SplunkHecOutput{
			HecHost:     logSpec.Host,
			HecPort:     SplunkHecPort,
			InsecureSSL: &insecureSSL,
			HecToken: &secret.Secret{
				ValueFrom: &secret.ValueFrom{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: logSpec.HecTokenSecretName},
						Key:                  "SplunkHecToken",
					},
				},
			},
			Index:    logSpec.Index,
			Protocol: protocol,
			Format:   &output.Format{Type: "json"},
			Buffer: &output.Buffer{
				Timekey:       BufferTimekey,
				TimekeyWait:   BufferTimekeyWait,
				TimekeyUseUtc: BufferTimekeyUseUtc,
			},
		},
	}
	return outputSpec
}

// outputCreators is a map that associates log types with their corresponding output creation functions.
var outputCreators = map[string]func(cappv1alpha1.LogSpec) loggingv1beta1.OutputSpec{
	LogTypeElastic: createElasticsearchOutput,
	LogTypeSplunk:  createSplunkHecOutput,
}

// prepareResource prepares an output resource based on the provided capp.
func (o OutputManager) prepareResource(capp cappv1alpha1.Capp) loggingv1beta1.Output {
	outputName := capp.GetName() + "-output"
	if createFunc, ok := outputCreators[capp.Spec.LogSpec.Type]; ok {
		outputSpec := createFunc(capp.Spec.LogSpec)
		output := loggingv1beta1.Output{
			ObjectMeta: metav1.ObjectMeta{
				Name:      outputName,
				Namespace: capp.GetNamespace(),
			},
			Spec: outputSpec,
		}
		return output
	}
	return loggingv1beta1.Output{}
}

// CleanUp deletes the output resource associated with the Capp object.
// The output resource is deleted by calling the DeleteResource method of the resourceManager object.
func (o OutputManager) CleanUp(capp cappv1alpha1.Capp) error {
	if o.IsRequired(capp) {
		outputName := capp.GetName() + "-output"
		resourceManager := rclient.ResourceBaseManagerClient{Ctx: o.Ctx, K8sclient: o.K8sclient, Log: o.Log}
		outputObject := loggingv1beta1.Output{}
		if err := resourceManager.DeleteResource(&outputObject, outputName, capp.Namespace); err != nil {
			return fmt.Errorf("unable to delete output %q: %w ", outputName, err)
		}
	}

	return nil
}

// IsRequired is responsible to determine if resource logging operator is required.
func (o OutputManager) IsRequired(capp cappv1alpha1.Capp) bool {
	if capp.Spec.LogSpec != (cappv1alpha1.LogSpec{}) {
		return capp.Spec.LogSpec.Type == LogTypeElastic || capp.Spec.LogSpec.Type == LogTypeSplunk
	}
	return false
}

// CreateOrUpdateObject creates or updates an output object based on the provided Capp.
// It returns an error if any operation fails.
func (o OutputManager) CreateOrUpdateObject(capp cappv1alpha1.Capp) error {
	outputName := capp.GetName() + "-output"
	logger := o.Log.WithValues("OutputName", outputName, "OutputNamespace", capp.Namespace)
	if o.IsRequired(capp) {
		generatedOutput := o.prepareResource(capp)
		// get instance of current output
		currentOutput := loggingv1beta1.Output{}
		resourceManager := rclient.ResourceBaseManagerClient{Ctx: o.Ctx, K8sclient: o.K8sclient, Log: o.Log}
		logger.Info("Trying to fetch existing output")
		switch err := o.K8sclient.Get(o.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: outputName}, &currentOutput); {
		case errors.IsNotFound(err):
			logger.Error(err, "didn't find existing output")
			if err := resourceManager.CreateResource(&generatedOutput); err != nil {
				logger.Error(err, "failed to create output")
				o.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappOutputCreationFailed, fmt.Sprintf("Failed to create output %s for Capp %s", outputName, capp.Name))
				return err
			}
			logger.Info("Created output successfully")
			o.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappOutputCreated, fmt.Sprintf("Created output %s for Capp %s", outputName, capp.Name))
		case err != nil:
			logger.Error(err, "failed to fetch existing output")
			return err
		}
		if !reflect.DeepEqual(currentOutput.Spec, generatedOutput.Spec) {
			currentOutput.Spec = generatedOutput.Spec
			logger.Info("Trying to update the current")
			if err := resourceManager.UpdateResource(&currentOutput); err != nil {
				return fmt.Errorf("failed to update the current output %q: %w ", currentOutput.Name, err)
			}
			logger.Info("Current output successfully updated")
		}
	}
	return nil
}
