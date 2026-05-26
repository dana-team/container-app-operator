package resourcemanagers

import (
	"context"
	"fmt"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/go-logr/logr"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/filter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	SyslogNGFlow                        = "syslogNGFlow"
	eventCappSyslogNGFlowCreationFailed = "SyslogNGFlowCreationFailed"
	eventCappSyslogNGFlowCreated        = "SyslogNGFlowCreated"
	knativeConfiguration                = "serving.knative.dev/configuration"
)

type SyslogNGFlowManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder events.EventRecorder
}

// prepareResource prepares a SyslogNGFlow resource based on the provided Capp.
func (f SyslogNGFlowManager) prepareResource(capp cappv1alpha1.Capp) loggingv1beta1.SyslogNGFlow {
	syslogNGFlowName := capp.GetName()
	syslogNGOutputName := capp.GetName()

	syslogNGFlow := loggingv1beta1.SyslogNGFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      syslogNGFlowName,
			Namespace: capp.GetNamespace(),
			Labels:    utils.ManagedResourceLabels(capp.Name),
		},
		Spec: loggingv1beta1.SyslogNGFlowSpec{
			Match: &loggingv1beta1.SyslogNGMatch{
				Regexp: &filter.RegexpMatchExpr{
					Pattern: capp.GetName(),
					Type:    "string",
					Value:   fmt.Sprintf("json#kubernetes#labels#%s", knativeConfiguration),
				},
			},
			LocalOutputRefs: []string{syslogNGOutputName},
		},
	}
	return syslogNGFlow
}

// CleanUp attempts to delete the associated SyslogNGFlow for a given Capp resource.
func (f SyslogNGFlowManager) CleanUp(capp cappv1alpha1.Capp) error {
	var syslogNGFlow loggingv1beta1.SyslogNGFlow
	if err := f.K8sclient.Get(f.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &syslogNGFlow); err != nil {
		return client.IgnoreNotFound(err)
	}
	if capp.DeletionTimestamp != nil {
		if ok, err := controllerutil.HasOwnerReference(syslogNGFlow.OwnerReferences, &capp, f.K8sclient.Scheme()); err != nil || ok {
			return err
		}
	}
	resourceManager := rclient.ResourceManagerClient{Ctx: f.Ctx, K8sclient: f.K8sclient, Log: f.Log}
	return client.IgnoreNotFound(resourceManager.DeleteResource(&syslogNGFlow))
}

// IsRequired is responsible to determine if resource logging operator SyslogNGFlow is required.
func (f SyslogNGFlowManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return isLogSpecRequired(capp)
}

// Manage creates or updates a SyslogNGFlow resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (f SyslogNGFlowManager) Manage(capp cappv1alpha1.Capp) error {
	if f.IsRequired(capp) {
		return f.createOrUpdate(capp)
	}

	return f.CleanUp(capp)
}

// createOrUpdate creates or updates a SyslogNGFlow resource.
func (f SyslogNGFlowManager) createOrUpdate(capp cappv1alpha1.Capp) error {
	syslogNGFlowFromCapp := f.prepareResource(capp)
	syslogNGFlow := loggingv1beta1.SyslogNGFlow{}
	resourceManager := rclient.ResourceManagerClient{Ctx: f.Ctx, K8sclient: f.K8sclient, Log: f.Log}

	if err := f.K8sclient.Get(f.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: syslogNGFlowFromCapp.Name}, &syslogNGFlow); err != nil {
		if errors.IsNotFound(err) {
			return f.createSyslogNGFlow(syslogNGFlowFromCapp, capp, resourceManager)
		} else {
			return fmt.Errorf("failed to get SyslogNGFlow %q: %w", syslogNGFlow.Name, err)
		}
	}

	return f.updateSyslogNGFlow(&capp, &syslogNGFlow, &syslogNGFlowFromCapp, resourceManager)
}

// createSyslogNGFlow creates a new SyslogNGFlow and emits an event.
func (f SyslogNGFlowManager) createSyslogNGFlow(syslogNGFlowFromCapp loggingv1beta1.SyslogNGFlow, capp cappv1alpha1.Capp, resourceManager rclient.ResourceManagerClient) error {
	if err := controllerutil.SetOwnerReference(&capp, &syslogNGFlowFromCapp, f.K8sclient.Scheme()); err != nil {
		return fmt.Errorf("set SyslogNGFlow owner reference: %w", err)
	}
	if err := resourceManager.CreateResource(&syslogNGFlowFromCapp); err != nil {
		f.EventRecorder.Eventf(&capp, nil, corev1.EventTypeWarning, eventCappSyslogNGFlowCreationFailed, eventCappSyslogNGFlowCreationFailed,
			fmt.Sprintf("Failed to create SyslogNGFlow %s", syslogNGFlowFromCapp.Name))
		return err
	}

	f.EventRecorder.Eventf(&capp, nil, corev1.EventTypeNormal, eventCappSyslogNGFlowCreated, eventCappSyslogNGFlowCreated,
		fmt.Sprintf("Created SyslogNGFlow %s", syslogNGFlowFromCapp.Name))

	return nil
}

// updateSyslogNGFlow checks if an update to the SyslogNGFlow is necessary and performs the update to match desired state.
func (f SyslogNGFlowManager) updateSyslogNGFlow(capp *cappv1alpha1.Capp, syslogNGFlow, syslogNGFlowFromCapp *loggingv1beta1.SyslogNGFlow, resourceManager rclient.ResourceManagerClient) error {
	orig := syslogNGFlow.DeepCopy()
	if err := controllerutil.SetOwnerReference(capp, syslogNGFlow, f.K8sclient.Scheme()); err != nil {
		return fmt.Errorf("set SyslogNGFlow owner reference: %w", err)
	}
	syslogNGFlow.Spec = *syslogNGFlowFromCapp.Spec.DeepCopy()

	if equality.Semantic.DeepEqual(orig.Spec, syslogNGFlow.Spec) &&
		equality.Semantic.DeepEqual(orig.OwnerReferences, syslogNGFlow.OwnerReferences) {
		return nil
	}

	return resourceManager.UpdateResource(syslogNGFlow)
}
