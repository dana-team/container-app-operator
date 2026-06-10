package resourcemanagers

import (
	"context"
	"fmt"

	"github.com/cloudevents/sdk-go/v2/event"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
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
	for _, source := range capp.Spec.EventSourcesSpec.Sources {
		if source.PingSourceConfiguration != nil {
			return true
		}
	}
	return false
}

func (p PingSourceManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if p.IsRequired(capp) {
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

func (p PingSourceManager) createOrUpdate(ctx context.Context, capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) error {
	cfg := source.PingSourceConfiguration
	desired := &sourcesv1.PingSource{
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

func (p PingSourceManager) getPingSources(ctx context.Context, capp cappv1alpha1.Capp) (sourcesv1.PingSourceList, error) {
	list := sourcesv1.PingSourceList{}
	if err := listManagedResources(ctx, p.K8sclient, capp, &list, "PingSource", nil); err != nil {
		return list, err
	}
	return list, nil
}
