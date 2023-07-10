package resourceprepares

import (
	"context"
	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internals/wrappers"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/filter"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type FlowManager struct {
	Ctx       context.Context
	K8sclient client.Client
	Log       logr.Logger
}

// prepareResource prepares a flow resource based on the provided capp.
func (f FlowManager) prepareResource(capp rcsv1alpha1.Capp) loggingv1beta1.Flow {
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
func (f FlowManager) CleanUp(capp rcsv1alpha1.Capp) error {
	flowName := capp.GetName() + "-flow"
	resourceManager := rclient.ResourceBaseManager{Ctx: f.Ctx, K8sclient: f.K8sclient, Log: f.Log}
	flow := loggingv1beta1.Flow{}
	if err := resourceManager.DeleteResource(&flow, flowName, capp.Namespace); err != nil {
		return err
	}
	return nil
}

// CreateOrUpdateObject creates or updates a flow object based on the provided capp.
// It returns an error if any operation fails.
func (f FlowManager) CreateOrUpdateObject(capp rcsv1alpha1.Capp) error {
	flowName := capp.GetName() + "-flow"
	logger := log.FromContext(f.Ctx).WithValues("CappName", capp.Name, "CappNamespace", capp.Namespace, "flowName", flowName)
	if capp.Spec.LogSpec.Type == LogTypeElastic || capp.Spec.LogSpec.Type == LogTypeSplunk {
		generatedFlow := f.prepareResource(capp)
		// get instance of current flow
		currentFlow := loggingv1beta1.Flow{}
		resourceManager := rclient.ResourceBaseManager{Ctx: f.Ctx, K8sclient: f.K8sclient, Log: f.Log}
		logger.Info("trying to fetch existing flow")
		switch err := f.K8sclient.Get(f.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: flowName}, &currentFlow); {
		case errors.IsNotFound(err):
			logger.Error(err, "didn't find existing flow")
			if err := resourceManager.CreateResource(&generatedFlow); err != nil {
				logger.Error(err, "failed to create flow")
				return err
			}
			logger.Info("created flow successfully")
		case err != nil:
			logger.Error(err, "failed to fetch existing flow")
			return err
		}
		if !reflect.DeepEqual(currentFlow.Spec, generatedFlow.Spec) {
			currentFlow.Spec = generatedFlow.Spec
			logger.Info("trying to update the current flow")
			if err := resourceManager.UpdateResource(&currentFlow); err != nil {
				logger.Error(err, "failed to update the current flow")
				return err
			}
			logger.Info("current flow successfully updated")
		}
	}
	return nil
}
