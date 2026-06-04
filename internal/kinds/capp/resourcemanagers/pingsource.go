package resourcemanagers

import (
	"context"
	"fmt"
	"sort"

	"github.com/cloudevents/sdk-go/v2/event"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/events"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	kapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PingSource                    = "pingSource"
	eventPingSourceCreationFailed = "PingSourceCreationFailed"
	eventPingSourceCreated        = "PingSourceCreated"
)

type PingSourceManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
}

func (p PingSourceManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return len(capp.Spec.EventSourcesSpec.Sources) > 0
}

func (p PingSourceManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if p.IsRequired(capp) {
		return p.reconcilePingSources(ctx, capp)
	}

	return p.CleanUp(ctx, capp)
}

func (p PingSourceManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	pingSources, err := p.getPingSources(ctx, capp)
	if err != nil {
		return err
	}
	for i := range pingSources.Items {
		ps := &pingSources.Items[i]
		if err := client.IgnoreNotFound(p.DeleteResource(ctx, ps)); err != nil {
			return fmt.Errorf("failed to delete PingSource %q: %w", ps.Name, err)
		}
	}
	return nil
}

func (p PingSourceManager) GetStatus(ctx context.Context, capp cappv1alpha1.Capp) (cappv1alpha1.EventingStatus, error) {
	pingSources, err := p.getPingSources(ctx, capp)
	if err != nil {
		return cappv1alpha1.EventingStatus{}, err
	}
	if len(pingSources.Items) == 0 {
		return cappv1alpha1.EventingStatus{}, nil
	}
	statuses := make([]cappv1alpha1.EventSourceStatus, 0, len(pingSources.Items))
	for _, ps := range pingSources.Items {
		condition := kapis.Condition{
			Type:               kapis.ConditionReady,
			Status:             corev1.ConditionUnknown,
			Message:            "Source readiness not known",
			LastTransitionTime: kapis.VolatileTime{Inner: metav1.Now()},
		}
		if sourceCondition := ps.Status.GetCondition(kapis.ConditionReady); sourceCondition != nil {
			condition.Status = sourceCondition.Status
			condition.Message = sourceCondition.Message
			if sourceCondition.Reason != "" {
				condition.Reason = sourceCondition.Reason
			}
			condition.LastTransitionTime = sourceCondition.LastTransitionTime
		}
		statuses = append(statuses, cappv1alpha1.EventSourceStatus{
			Name:      ps.Name,
			Condition: condition,
		})
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
	return cappv1alpha1.EventingStatus{EventSources: statuses}, nil
}

func (p PingSourceManager) reconcilePingSources(ctx context.Context, capp cappv1alpha1.Capp) error {
	for _, source := range capp.Spec.EventSourcesSpec.Sources {
		if source.PingSourceConfiguration == nil {
			continue
		}
		if err := p.createOrUpdate(ctx, capp, source); err != nil {
			return fmt.Errorf("failed to create or update PingSource %q: %w", source.Name, err)
		}
	}
	return p.cleanUpOrphans(ctx, capp)
}

func (p PingSourceManager) createOrUpdate(ctx context.Context, capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) error {
	desired := p.preparePingSource(capp, source)
	existing := &sourcesv1.PingSource{}
	err := p.K8sclient.Get(ctx, client.ObjectKey{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get PingSource %q: %w", desired.Name, err)
		}
		return createManagedResource(ctx, p.K8sclient, p.CreateResource, p.EventRecorder, &capp, desired,
			"PingSource", eventPingSourceCreated, eventPingSourceCreationFailed)
	}

	orig := existing.DeepCopy()
	existing.Spec = desired.Spec
	if err := ensureOwnerReference(p.K8sclient, &capp, existing, "PingSource"); err != nil {
		return err
	}
	if managedResourceNeedsUpdate(orig.Spec, existing.Spec, orig.OwnerReferences, existing.OwnerReferences) {
		p.Log.Info("Updating PingSource", "Name", existing.Name)
	}
	return updateManagedResourceIfNeeded(ctx, p.UpdateResource, existing, orig.Spec, existing.Spec, orig.OwnerReferences)
}

func (p PingSourceManager) cleanUpOrphans(ctx context.Context, capp cappv1alpha1.Capp) error {
	desired := make(map[string]struct{})
	for _, source := range capp.Spec.EventSourcesSpec.Sources {
		if source.PingSourceConfiguration != nil {
			desired[fmt.Sprintf("%s-%s", capp.Name, source.Name)] = struct{}{}
		}
	}
	owned, err := p.getPingSources(ctx, capp)
	if err != nil {
		return err
	}
	for i := range owned.Items {
		ps := &owned.Items[i]
		if _, keep := desired[ps.Name]; !keep {
			if err := client.IgnoreNotFound(p.DeleteResource(ctx, ps)); err != nil {
				return fmt.Errorf("failed to delete orphaned PingSource %q: %w", ps.Name, err)
			}
		}
	}
	return nil
}

func (p PingSourceManager) preparePingSource(capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) *sourcesv1.PingSource {
	cfg := source.PingSourceConfiguration
	return &sourcesv1.PingSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", capp.Name, source.Name),
			Namespace: capp.Namespace,
			Labels:    utils.ManagedResourceLabels(capp.Name),
		},
		Spec: sourcesv1.PingSourceSpec{
			Schedule:    cfg.Schedule,
			Data:        cfg.Data,
			ContentType: event.ApplicationJSON,
			SourceSpec: duckv1.SourceSpec{
				Sink: duckv1.Destination{
					Ref: &duckv1.KReference{
						Name:       capp.Name,
						Namespace:  capp.Namespace,
						Kind:       knativeServiceKind,
						APIVersion: servingv1.SchemeGroupVersion.String(),
					},
					URI: source.URI,
				},
			},
		},
	}
}

func (p PingSourceManager) getPingSources(ctx context.Context, capp cappv1alpha1.Capp) (sourcesv1.PingSourceList, error) {
	list := sourcesv1.PingSourceList{}
	set := labels.Set{utils.CappResourceKey: capp.Name}
	listOpts := utils.GetListOptions(set)
	listOpts.Namespace = capp.Namespace
	if err := p.K8sclient.List(ctx, &list, &listOpts); err != nil {
		return list, fmt.Errorf("unable to list PingSources of Capp %q: %w", capp.Name, err)
	}
	return list, nil
}
