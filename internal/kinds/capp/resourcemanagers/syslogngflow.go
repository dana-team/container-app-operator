package resourcemanagers

import (
	"context"
	"fmt"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/kube-logging/logging-operator/pkg/sdk/logging/model/syslogng/filter"
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
	rclient.ResourceManagerClient
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
func (f SyslogNGFlowManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	var syslogNGFlow loggingv1beta1.SyslogNGFlow
	if err := f.K8sclient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Name}, &syslogNGFlow); err != nil {
		return client.IgnoreNotFound(err)
	}
	if capp.DeletionTimestamp != nil {
		if ok, err := controllerutil.HasOwnerReference(syslogNGFlow.OwnerReferences, &capp, f.K8sclient.Scheme()); err != nil || ok {
			return err
		}
	}
	return client.IgnoreNotFound(f.DeleteResource(ctx, &syslogNGFlow))
}

// IsRequired is responsible to determine if resource logging operator SyslogNGFlow is required.
func (f SyslogNGFlowManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return isLogSpecRequired(capp)
}

// Manage creates or updates a SyslogNGFlow resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (f SyslogNGFlowManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if f.IsRequired(capp) {
		return f.createOrUpdate(ctx, capp)
	}

	return f.CleanUp(ctx, capp)
}

// createOrUpdate creates or updates a SyslogNGFlow resource.
func (f SyslogNGFlowManager) createOrUpdate(ctx context.Context, capp cappv1alpha1.Capp) error {
	syslogNGFlowFromCapp := f.prepareResource(capp)
	syslogNGFlow := loggingv1beta1.SyslogNGFlow{}

	if err := f.K8sclient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: syslogNGFlowFromCapp.Name}, &syslogNGFlow); err != nil {
		if errors.IsNotFound(err) {
			return createManagedResource(ctx, f.K8sclient, f.CreateResource, f.EventRecorder, &capp, &syslogNGFlowFromCapp,
				"SyslogNGFlow", eventCappSyslogNGFlowCreated, eventCappSyslogNGFlowCreationFailed)
		}
		return fmt.Errorf("failed to get SyslogNGFlow %q: %w", syslogNGFlow.Name, err)
	}

	orig := syslogNGFlow.DeepCopy()
	syslogNGFlow.Spec = *syslogNGFlowFromCapp.Spec.DeepCopy()
	if err := ensureOwnerReference(f.K8sclient, &capp, &syslogNGFlow, "SyslogNGFlow"); err != nil {
		return err
	}
	return updateManagedResourceIfNeeded(ctx, f.UpdateResource, &syslogNGFlow, orig.Spec, syslogNGFlow.Spec, orig.OwnerReferences)
}
