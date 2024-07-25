package resourcemanagers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
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
	EventRecorder record.EventRecorder
}

// prepareResource prepares a SyslogNGFlow resource based on the provided Capp.
func (f SyslogNGFlowManager) prepareResource(capp cappv1alpha1.Capp) loggingv1beta1.SyslogNGFlow {
	syslogNGFlowName := capp.GetName()
	syslogNGOutputName := capp.GetName()

	syslogNGFlow := loggingv1beta1.SyslogNGFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      syslogNGFlowName,
			Namespace: capp.GetNamespace(),
			Labels: map[string]string{
				utils.CappResourceKey:   capp.Name,
				utils.ManagedByLabelKey: utils.CappKey,
			},
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
	resourceManager := rclient.ResourceManagerClient{Ctx: f.Ctx, K8sclient: f.K8sclient, Log: f.Log}
	syslogNGFlow := rclient.GetBareSyslogNGFlow(capp.Name, capp.Namespace)

	if err := resourceManager.DeleteResource(&syslogNGFlow); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return nil
}

// IsRequired is responsible to determine if resource logging operator SyslogNGFlow is required.
func (f SyslogNGFlowManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.LogSpec != cappv1alpha1.LogSpec{}
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

	return f.updateSyslogNGFlow(&syslogNGFlow, &syslogNGFlowFromCapp, resourceManager)
}

// createSyslogNGFlow creates a new SyslogNGFlow and emits an event.
func (f SyslogNGFlowManager) createSyslogNGFlow(syslogNGFlowFromCapp loggingv1beta1.SyslogNGFlow, capp cappv1alpha1.Capp, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(&syslogNGFlowFromCapp); err != nil {
		f.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappSyslogNGFlowCreationFailed,
			fmt.Sprintf("Failed to create SyslogNGFlow %s", syslogNGFlowFromCapp.Name))
		return err
	}

	f.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventCappSyslogNGFlowCreated,
		fmt.Sprintf("Created SyslogNGFlow %s", syslogNGFlowFromCapp.Name))

	return nil
}

// updateSyslogNGFlow checks if an update to the SyslogNGFlow is necessary and performs the update to match desired state.
func (f SyslogNGFlowManager) updateSyslogNGFlow(syslogNGFlow, syslogNGFlowFromCapp *loggingv1beta1.SyslogNGFlow, resourceManager rclient.ResourceManagerClient) error {
	if !reflect.DeepEqual(syslogNGFlow.Spec, syslogNGFlowFromCapp.Spec) {
		syslogNGFlow.Spec = syslogNGFlowFromCapp.Spec
		return resourceManager.UpdateResource(syslogNGFlow)
	}

	return nil
}
