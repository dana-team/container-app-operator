package resourceprepares

import (
	"context"
	"fmt"
	"reflect"

	"github.com/cisco-open/operator-tools/pkg/secret"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/wrappers"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/output"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SyslogNGOutputManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

const (
	logTypeElastic    = "elastic"
	elasticSSLVersion = "tlsv1_2"
	elasticTemplate   = "$(format-json --subkeys json# --key-delimiter #)"
	elasticSecretKey  = "elastic"
)

// syslogNGOutputCreators is a map that associates log types with their corresponding SyslogNGOutput creation functions.
var syslogNGOutputCreators = map[string]func(cappv1alpha1.LogSpec) loggingv1beta1.SyslogNGOutputSpec{
	logTypeElastic: createElasticsearchOutput,
}

// createElasticsearchOutput creates an Elasticsearch SyslogNGOutput object based on the provided logSpec.
// It constructs the Elasticsearch SyslogNGOutput which is returned as a SyslogNGOutputSpec.
func createElasticsearchOutput(logSpec cappv1alpha1.LogSpec) loggingv1beta1.SyslogNGOutputSpec {
	peerVerify := false

	syslogNGOutputSpec := loggingv1beta1.SyslogNGOutputSpec{
		Elasticsearch: &output.ElasticsearchOutput{
			Index:    logSpec.Index,
			Template: elasticTemplate,
			HTTPOutput: output.HTTPOutput{
				URL:  logSpec.Host,
				User: logSpec.User,
				Password: secret.Secret{
					ValueFrom: &secret.ValueFrom{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: logSpec.PasswordSecret},
							Key:                  elasticSecretKey,
						},
					},
				},
				TLS: &output.TLS{
					PeerVerify: &peerVerify,
					SslVersion: elasticSSLVersion,
				},
			},
		},
	}

	return syslogNGOutputSpec
}

// prepareResource prepares a SyslogNGOutput resource based on the provided Capp.
func (o SyslogNGOutputManager) prepareResource(capp cappv1alpha1.Capp) loggingv1beta1.SyslogNGOutput {
	syslogNGOutputName := capp.GetName()

	if createFunc, ok := syslogNGOutputCreators[capp.Spec.LogSpec.Type]; ok {
		syslogNGOutputSpec := createFunc(capp.Spec.LogSpec)

		syslogNGOutput := loggingv1beta1.SyslogNGOutput{
			ObjectMeta: metav1.ObjectMeta{
				Name:      syslogNGOutputName,
				Namespace: capp.GetNamespace(),
			},
			Spec: syslogNGOutputSpec,
		}
		return syslogNGOutput
	}

	return loggingv1beta1.SyslogNGOutput{}
}

// CleanUp deletes the SyslogNGOutput resource associated with the Capp object.
// The SyslogNGOutput resource is deleted by calling the DeleteResource method of the resourceManager object.
func (o SyslogNGOutputManager) CleanUp(capp cappv1alpha1.Capp) error {
	if o.IsRequired(capp) {
		syslogNGOutputName := capp.GetName()
		resourceManager := rclient.ResourceBaseManagerClient{Ctx: o.Ctx, K8sclient: o.K8sclient, Log: o.Log}
		syslogNGOutputObject := loggingv1beta1.SyslogNGOutput{}

		if err := resourceManager.DeleteResource(&syslogNGOutputObject, syslogNGOutputName, capp.Namespace); err != nil {
			return fmt.Errorf("unable to delete SyslogNGOutput %q: %w", syslogNGOutputName, err)
		}
	}

	return nil
}

// IsRequired is responsible to determine if resource logging operator is required.
func (o SyslogNGOutputManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.LogSpec != cappv1alpha1.LogSpec{}
}

// CreateOrUpdateObject creates or updates a SyslogNGOutput object based on the provided Capp.
// It returns an error if any operation fails.
func (o SyslogNGOutputManager) CreateOrUpdateObject(capp cappv1alpha1.Capp) error {
	syslogNGOutputName := capp.GetName()
	logger := o.Log.WithValues("SyslogNGOutputName", syslogNGOutputName, "SyslogNGOutputNamespace", capp.Namespace)

	if o.IsRequired(capp) {
		generatedSyslogNGOutput := o.prepareResource(capp)
		currentSyslogNGOutput := loggingv1beta1.SyslogNGOutput{}
		resourceManager := rclient.ResourceBaseManagerClient{Ctx: o.Ctx, K8sclient: o.K8sclient, Log: o.Log}

		logger.Info("Trying to fetch existing SyslogNGOutput")
		if err := o.K8sclient.Get(o.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: syslogNGOutputName}, &currentSyslogNGOutput); err != nil {
			if errors.IsNotFound(err) {
				if err := resourceManager.CreateResource(&generatedSyslogNGOutput); err != nil {
					logger.Error(err, "failed to create SyslogNGOutput")
					o.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappSyslogNGOutputCreationFailed,
						fmt.Sprintf("Failed to create SyslogNGOutput %s for Capp %s", syslogNGOutputName, capp.Name))
					return err
				}

				logger.Info("Created SyslogNGOutput successfully")
				o.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappSyslogNGlSOutputCreated,
					fmt.Sprintf("Created SyslogNGOutput %s for Capp %s", syslogNGOutputName, capp.Name))
			} else {
				logger.Error(err, "failed to fetch existing SyslogNGOutput")
				return err
			}
		}

		if !reflect.DeepEqual(currentSyslogNGOutput.Spec, generatedSyslogNGOutput.Spec) {
			currentSyslogNGOutput.Spec = generatedSyslogNGOutput.Spec
			if err := resourceManager.UpdateResource(&currentSyslogNGOutput); err != nil {
				logger.Error(err, "failed to update the SyslogNGOutput")
			}
			logger.Info("Successfully updated SyslogNGOutput")
		}
	}

	return nil
}
