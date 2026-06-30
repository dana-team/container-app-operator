package resourcemanagers

import (
	"context"
	"fmt"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	"github.com/cisco-open/operator-tools/pkg/secret"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/output"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	SyslogNGOutput                        = "syslogNGOutput"
	eventCappSyslogNGOutputCreationFailed = "SyslogNGOutputCreationFailed"
	eventCappSyslogNGOutputCreated        = "SyslogNGOutputCreated"
	elasticSSLVersion                     = "tlsv1_2"
	elasticTemplate                       = "$(format-json --subkeys json# --key-delimiter #)"
	elasticDataStreamTemplate             = "--subkeys json# --key-delimiter # --exclude DATE --key ISODATE @timestamp=${ISODATE}"
	elasticSecretKey                      = "elastic"
)

type SyslogNGOutputManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
}

// syslogNGOutputCreators is a map that associates log types with their corresponding SyslogNGOutput creation functions.
var syslogNGOutputCreators = map[cappv1alpha1.LogType]func(cappv1alpha1.LogSpec) loggingv1beta1.SyslogNGOutputSpec{
	cappv1alpha1.LogTypeElastic:           createElasticsearchOutput,
	cappv1alpha1.LogTypeElasticDataStream: createElasticDataStreamOutput,
}

func isSupportedLogType(logType cappv1alpha1.LogType) bool {
	_, ok := syslogNGOutputCreators[logType]
	return ok
}

func isLogSpecRequired(capp cappv1alpha1.Capp) bool {
	if capp.Spec.LogSpec == (cappv1alpha1.LogSpec{}) {
		return false
	}
	return isSupportedLogType(capp.Spec.LogSpec.Type)
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

// createElasticDataStreamOutput creates an Elasticsearch Data Stream SyslogNGOutput object based on the provided logSpec.
func createElasticDataStreamOutput(logSpec cappv1alpha1.LogSpec) loggingv1beta1.SyslogNGOutputSpec {
	peerVerify := false

	syslogNGOutput := loggingv1beta1.SyslogNGOutputSpec{
		ElasticsearchDatastream: &output.ElasticsearchDatastreamOutput{
			Record: elasticDataStreamTemplate,
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
	return syslogNGOutput
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
				Labels:    utils.ManagedResourceLabels(capp.Name),
			},
			Spec: syslogNGOutputSpec,
		}
		return syslogNGOutput
	}

	return loggingv1beta1.SyslogNGOutput{}
}

// CleanUp attempts to delete the associated SyslogNGOutput for a given Capp resource.
func (o SyslogNGOutputManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	var syslogNGOutput loggingv1beta1.SyslogNGOutput
	if err := o.K8sClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &syslogNGOutput); err != nil {
		return client.IgnoreNotFound(err)
	}
	if capp.DeletionTimestamp != nil {
		if ok, err := controllerutil.HasOwnerReference(syslogNGOutput.OwnerReferences, &capp, o.K8sClient.Scheme()); err != nil || ok {
			return err
		}
	}
	return client.IgnoreNotFound(o.DeleteResource(ctx, &syslogNGOutput))
}

// IsRequired is responsible to determine if resource logging operator is required.
func (o SyslogNGOutputManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return isLogSpecRequired(capp)
}

// Manage creates or updates a SyslogNGOutput resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (o SyslogNGOutputManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if o.IsRequired(capp) {
		return o.createOrUpdate(ctx, capp)
	}

	return o.CleanUp(ctx, capp)
}

// createOrUpdate creates or updates a SyslogNGOutput resource.
func (o SyslogNGOutputManager) createOrUpdate(ctx context.Context, capp cappv1alpha1.Capp) error {
	syslogNGOutputFromCapp := o.prepareResource(capp)
	if !isSupportedLogType(capp.Spec.LogSpec.Type) {
		return fmt.Errorf("unsupported log type %q", capp.Spec.LogSpec.Type)
	}
	syslogNGOutput := loggingv1beta1.SyslogNGOutput{}

	if err := o.K8sClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: syslogNGOutputFromCapp.Name}, &syslogNGOutput); err != nil {
		if errors.IsNotFound(err) {
			return createManagedResource(ctx, o.K8sClient, o.CreateResource, o.EventRecorder, &capp, &syslogNGOutputFromCapp,
				"SyslogNGOutput", eventCappSyslogNGOutputCreated, eventCappSyslogNGOutputCreationFailed)
		}
		return fmt.Errorf("failed to get SyslogNGOutput %q: %w", syslogNGOutputFromCapp.Name, err)
	}

	orig := syslogNGOutput.DeepCopy()
	syslogNGOutput.Spec = *syslogNGOutputFromCapp.Spec.DeepCopy()
	if err := ensureOwnerReference(o.K8sClient, &capp, &syslogNGOutput, "SyslogNGOutput"); err != nil {
		return err
	}
	return updateManagedResourceIfNeeded(ctx, o.UpdateResource, &syslogNGOutput, orig.Spec, syslogNGOutput.Spec, orig.OwnerReferences)
}
