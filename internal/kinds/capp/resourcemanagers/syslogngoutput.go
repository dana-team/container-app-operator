package resourcemanagers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/cisco-open/operator-tools/pkg/secret"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
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

const (
	SyslogNGOutput                        = "syslogNGOutput"
	eventCappSyslogNGOutputCreationFailed = "SyslogNGOutputCreationFailed"
	eventCappSyslogNGlSOutputCreated      = "SyslogNGOutputCreated"
	logTypeElastic                        = "elastic"
	elasticSSLVersion                     = "tlsv1_2"
	elasticTemplate                       = "$(format-json --subkeys json# --key-delimiter #)"
	elasticSecretKey                      = "elastic"
)

type SyslogNGOutputManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

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

// CleanUp attempts to delete the associated SyslogNGOutput for a given Capp resource.
func (o SyslogNGOutputManager) CleanUp(capp cappv1alpha1.Capp) error {
	resourceManager := rclient.ResourceManagerClient{Ctx: o.Ctx, K8sclient: o.K8sclient, Log: o.Log}
	syslogNGOutput := rclient.GetBareSyslogNGOutput(capp.Name, capp.Namespace)

	if err := resourceManager.DeleteResource(&syslogNGOutput); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return nil
}

// IsRequired is responsible to determine if resource logging operator is required.
func (o SyslogNGOutputManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.LogSpec != cappv1alpha1.LogSpec{}
}

// Manage creates or updates a SyslogNGOutput resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (o SyslogNGOutputManager) Manage(capp cappv1alpha1.Capp) error {
	if o.IsRequired(capp) {
		return o.createOrUpdate(capp)
	}

	return o.CleanUp(capp)
}

// createOrUpdate creates or updates a SyslogNGOutput resource.
func (o SyslogNGOutputManager) createOrUpdate(capp cappv1alpha1.Capp) error {
	syslogNGOutputFromCapp := o.prepareResource(capp)
	syslogNGOutput := loggingv1beta1.SyslogNGOutput{}
	resourceManager := rclient.ResourceManagerClient{Ctx: o.Ctx, K8sclient: o.K8sclient, Log: o.Log}

	if err := o.K8sclient.Get(o.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: syslogNGOutputFromCapp.Name}, &syslogNGOutput); err != nil {
		if errors.IsNotFound(err) {
			return o.createSyslogNGOutput(syslogNGOutputFromCapp, capp, resourceManager)
		} else {
			return fmt.Errorf("failed to get SyslogNGOutput %q: %w", syslogNGOutputFromCapp.Name, err)
		}
	}

	return o.updateSyslogNGOutput(&syslogNGOutput, &syslogNGOutputFromCapp, resourceManager)
}

// createSyslogNGOutput creates a new SyslogNGOutput and emits an event.
func (o SyslogNGOutputManager) createSyslogNGOutput(syslogNGOutputFromCapp loggingv1beta1.SyslogNGOutput, capp cappv1alpha1.Capp, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(&syslogNGOutputFromCapp); err != nil {
		o.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappSyslogNGOutputCreationFailed,
			fmt.Sprintf("Failed to create SyslogNGOutput %s", syslogNGOutputFromCapp.Name))
		return err
	}

	o.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappSyslogNGlSOutputCreated,
		fmt.Sprintf("Created SyslogNGOutput %s", syslogNGOutputFromCapp.Name))

	return nil
}

// updateSyslogNGOutput checks if an update to the SyslogNGOutput is necessary and performs the update to match desired state.
func (o SyslogNGOutputManager) updateSyslogNGOutput(syslogNGOutput, syslogNGOutputFromCapp *loggingv1beta1.SyslogNGOutput, resourceManager rclient.ResourceManagerClient) error {
	if !reflect.DeepEqual(syslogNGOutput.Spec, syslogNGOutputFromCapp.Spec) {
		syslogNGOutput.Spec = syslogNGOutputFromCapp.Spec
		return resourceManager.UpdateResource(syslogNGOutput)
	}

	return nil
}
