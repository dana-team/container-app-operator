package resourceprepares

import (
	"context"
	"fmt"
	"reflect"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/wrappers"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/filter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FlowManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// prepareResource prepares a flow resource based on the provided capp.
func (f FlowManager) prepareResource(capp cappv1alpha1.Capp) loggingv1beta1.Flow {
	flowName := capp.GetName() + "-flow"
	outputName := capp.GetName() + "-output"
	flow := loggingv1beta1.Flow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      flowName,
			Namespace: capp.GetNamespace(),
		},
		Spec: loggingv1beta1.FlowSpec{
			Filters: []loggingv1beta1.Filter{
				{
					TagNormaliser: &filter.TagNormaliser{},
				},
				{
					Parser: &filter.ParserConfig{
						RemoveKeyNameField: true,
						ReserveData:        true,
						Parse: filter.ParseSection{
							Type: NginxPraser,
						},
					},
				},
			},
			Match: []loggingv1beta1.Match{
				{
					Select: &loggingv1beta1.Select{
						Labels: map[string]string{
							KnativeConfiguration: capp.GetName(),
						},
					},
				},
			},
			LocalOutputRefs: []string{outputName},
		},
	}
	return flow
}

// CleanUp deletes the flow resource associated with the Capp object.
// The flow resource is deleted by calling the DeleteResource method of the resourceManager object.
func (f FlowManager) CleanUp(capp cappv1alpha1.Capp) error {
	if f.IsRequired(capp) {
		flowName := capp.GetName() + "-flow"
		resourceManager := rclient.ResourceBaseManagerClient{Ctx: f.Ctx, K8sclient: f.K8sclient, Log: f.Log}
		flow := loggingv1beta1.Flow{}
		if err := resourceManager.DeleteResource(&flow, flowName, capp.Namespace); err != nil {
			return fmt.Errorf("unable to delete flow %q: %w", flowName, err)
		}

	}
	return nil
}

// IsRequired is responsible to determine if resource logging operator flow is required.
func (f FlowManager) IsRequired(capp cappv1alpha1.Capp) bool {
	if capp.Spec.LogSpec != (cappv1alpha1.LogSpec{}) {
		return capp.Spec.LogSpec.Type == LogTypeElastic || capp.Spec.LogSpec.Type == LogTypeSplunk
	}
	return false
}

// CreateOrUpdateObject creates or updates a flow object based on the provided capp.
// It returns an error if any operation fails.
func (f FlowManager) CreateOrUpdateObject(capp cappv1alpha1.Capp) error {
	flowName := capp.GetName() + "-flow"
	logger := f.Log.WithValues("FlowName", flowName, "FlowNamespace", capp.Namespace)

	if f.IsRequired(capp) {
		generatedFlow := f.prepareResource(capp)
		// get instance of current flow
		currentFlow := loggingv1beta1.Flow{}
		resourceManager := rclient.ResourceBaseManagerClient{Ctx: f.Ctx, K8sclient: f.K8sclient, Log: f.Log}
		logger.Info("Trying to fetch existing flow")
		switch err := f.K8sclient.Get(f.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: flowName}, &currentFlow); {
		case errors.IsNotFound(err):
			logger.Info("didn't find flow")

			if err := resourceManager.CreateResource(&generatedFlow); err != nil {
				f.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappFlowCreationFailed, fmt.Sprintf("Failed to create flow %s for Capp %s", flowName, capp.Name))
				return fmt.Errorf("failed to create flow %q: %w", flowName, err)
			}
			logger.Info("Created flow successfully")
			f.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappFlowCreated, fmt.Sprintf("Created flow %s for Capp %s", flowName, capp.Name))
		case err != nil:
			return fmt.Errorf("failed to fetch existing flow %q: %w", flowName, err)
		}
		if !reflect.DeepEqual(currentFlow.Spec, generatedFlow.Spec) {
			currentFlow.Spec = generatedFlow.Spec
			logger.Info("Trying to update the current flow")
			if err := resourceManager.UpdateResource(&currentFlow); err != nil {
				return fmt.Errorf("failed to update the current flow %q: %w", flowName, err)
			}
			logger.Info("Current flow successfully updated")
		}
	}
	return nil
}
