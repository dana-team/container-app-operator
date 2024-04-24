package resourceprepares

import (
	"context"
	"fmt"
	"reflect"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/wrappers"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/filter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SyslogNGFlowManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

const knativeConfiguration = "serving.knative.dev/configuration"

// prepareResource prepares a SyslogNGFlow resource based on the provided Capp.
func (f SyslogNGFlowManager) prepareResource(capp cappv1alpha1.Capp) loggingv1beta1.SyslogNGFlow {
	syslogNGFlowName := capp.GetName()
	syslogNGOutputName := capp.GetName()

	syslogNGFlow := loggingv1beta1.SyslogNGFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      syslogNGFlowName,
			Namespace: capp.GetNamespace(),
		},
		Spec: loggingv1beta1.SyslogNGFlowSpec{
			Filters: []loggingv1beta1.SyslogNGFilter{
				{
					Match: &filter.MatchConfig{
						Regexp: &filter.RegexpMatchExpr{
							Pattern: capp.GetName(),
							Type:    "string",
							Value:   fmt.Sprintf("json#kubernetes#json#%s", knativeConfiguration),
						},
					},
				},
			},
			LocalOutputRefs: []string{syslogNGOutputName},
		},
	}
	return syslogNGFlow
}

// CleanUp deletes the SyslogNGFlow resource associated with the Capp object.
// The SyslogNGFlow resource is deleted by calling the DeleteResource method of the resourceManager object.
func (f SyslogNGFlowManager) CleanUp(capp cappv1alpha1.Capp) error {
	if f.IsRequired(capp) {
		syslogNGFlowName := capp.GetName()
		resourceManager := rclient.ResourceBaseManagerClient{Ctx: f.Ctx, K8sclient: f.K8sclient, Log: f.Log}

		syslogNGFlow := loggingv1beta1.SyslogNGFlow{}
		if err := resourceManager.DeleteResource(&syslogNGFlow, syslogNGFlowName, capp.Namespace); err != nil {
			return fmt.Errorf("unable to delete SyslogNGFlow %q: %w", syslogNGFlowName, err)
		}

	}

	return nil
}

// IsRequired is responsible to determine if resource logging operator SyslogNGFlow is required.
func (f SyslogNGFlowManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.LogSpec != cappv1alpha1.LogSpec{}
}

// CreateOrUpdateObject creates or updates a SyslogNGFlow object based on the provided Capp.
// It returns an error if any operation fails.
func (f SyslogNGFlowManager) CreateOrUpdateObject(capp cappv1alpha1.Capp) error {
	syslogNGFlowName := capp.GetName()
	logger := f.Log.WithValues("SyslogNGFlowName", syslogNGFlowName, "SyslogNGFlowNamespace", capp.Namespace)

	if f.IsRequired(capp) {
		generatedSyslogNGFlow := f.prepareResource(capp)
		currentSyslogNGFlow := loggingv1beta1.SyslogNGFlow{}

		resourceManager := rclient.ResourceBaseManagerClient{Ctx: f.Ctx, K8sclient: f.K8sclient, Log: f.Log}

		if err := f.K8sclient.Get(f.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: syslogNGFlowName}, &currentSyslogNGFlow); err != nil {
			if errors.IsNotFound(err) {
				if err := resourceManager.CreateResource(&generatedSyslogNGFlow); err != nil {
					f.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappSyslogNGFlowCreationFailed,
						fmt.Sprintf("Failed to create SyslogNGFlow %s for Capp %s", syslogNGFlowName, capp.Name))
					return fmt.Errorf("failed to create SyslogNGFlow %q: %w", syslogNGFlowName, err)
				}

				logger.Info("Successfully created SyslogNGFlow")
				f.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappSyslogNGFlowCreated,
					fmt.Sprintf("Created SyslogNGFlow %s for Capp %s", syslogNGFlowName, capp.Name))
			} else {
				logger.Error(err, "failed to fetch existing SyslogNGFlow")
				return err
			}
		}

		if !reflect.DeepEqual(currentSyslogNGFlow.Spec, generatedSyslogNGFlow.Spec) {
			currentSyslogNGFlow.Spec = generatedSyslogNGFlow.Spec
			logger.Info("Trying to update the current SyslogNGFlow")

			if err := resourceManager.UpdateResource(&currentSyslogNGFlow); err != nil {
				logger.Error(err, "failed to update the SyslogNGFlow")
			}
			logger.Info("Successfully updated SyslogNGFlow")
		}
	}

	return nil
}
