package resourcemanagers

import (
	"context"
	"fmt"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	EventSources                  = "eventSources"
	pingSourceType                = "pingsource"
	eventPingSourceCreationFailed = "PingSourceCreationFailed"
	eventPingSourceCreated        = "PingSourceCreated"
)

// EventSourceManager manages all Knative Eventing sources for a Capp.
// To add a new source type: add manage*Source + cleanOrphaned*Sources methods,
// call them in createOrUpdateSources and CleanUp respectively.
type EventSourceManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// IsRequired returns true when at least one event source is declared in the spec.
func (e EventSourceManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return len(capp.Spec.EventSourcesSpec.Sources) > 0
}

// Manage creates/updates sources when required, or cleans all up when not.
func (e EventSourceManager) Manage(capp cappv1alpha1.Capp) error {
	if e.IsRequired(capp) {
		return e.createOrUpdateSources(capp)
	}
	return e.CleanUp(capp)
}

// CleanUp deletes all source resources owned by the Capp, across all source types.
func (e EventSourceManager) CleanUp(capp cappv1alpha1.Capp) error {
	return e.cleanOrphanedPingSources(capp, nil)
}

// createOrUpdateSources syncs spec sources and removes orphaned ones.
func (e EventSourceManager) createOrUpdateSources(capp cappv1alpha1.Capp) error {
	desiredPingSourceNames := make(map[string]bool)

	for i, source := range capp.Spec.EventSourcesSpec.Sources {
		if source.PingSource != nil {
			name := e.sourceName(capp.Name, source.Name, pingSourceType, i)
			desiredPingSourceNames[name] = true
			if err := e.managePingSource(capp, source, i); err != nil {
				return err
			}
		}
	}

	return e.cleanOrphanedPingSources(capp, desiredPingSourceNames)
}

// sourceName returns the K8s object name for an event source.
// If the user provided an explicit name, use <capp-name>-<name>.
// Otherwise derive a stable name from <capp-name>-<type>-<index>.
func (e EventSourceManager) sourceName(cappName, explicitName, sourceType string, index int) string {
	if explicitName != "" {
		return fmt.Sprintf("%s-%s", cappName, explicitName)
	}
	return fmt.Sprintf("%s-%s-%d", cappName, sourceType, index)
}

// managePingSource creates or updates a single PingSource.
func (e EventSourceManager) managePingSource(capp cappv1alpha1.Capp, source cappv1alpha1.EventSource, index int) error {
	desired := e.preparePingSource(capp, source, index)
	rm := rclient.ResourceManagerClient{Ctx: e.Ctx, K8sclient: e.K8sclient, Log: e.Log}

	existing := sourcesv1.PingSource{}
	if err := e.K8sclient.Get(e.Ctx, client.ObjectKey{Namespace: desired.Namespace, Name: desired.Name}, &existing); err != nil {
		if errors.IsNotFound(err) {
			return e.createPingSource(&capp, &desired, rm)
		}
		return fmt.Errorf("failed to get PingSource %q: %w", desired.Name, err)
	}
	return e.updatePingSource(existing, desired, rm)
}

// preparePingSource builds the desired PingSource from a Capp and EventSource.
// The sink is always set to the Capp's Knative Service.
func (e EventSourceManager) preparePingSource(capp cappv1alpha1.Capp, source cappv1alpha1.EventSource, index int) sourcesv1.PingSource {
	ps := source.PingSource
	return sourcesv1.PingSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      e.sourceName(capp.Name, source.Name, pingSourceType, index),
			Namespace: capp.Namespace,
			Labels: map[string]string{
				utils.CappResourceKey:   capp.Name,
				utils.ManagedByLabelKey: utils.CappKey,
			},
		},
		Spec: sourcesv1.PingSourceSpec{
			SourceSpec: duckv1.SourceSpec{
				Sink: duckv1.Destination{
					Ref: &duckv1.KReference{
						APIVersion: "serving.knative.dev/v1",
						Kind:       "Service",
						Name:       capp.Name,
						Namespace:  capp.Namespace,
					},
				},
			},
			Schedule:    ps.Schedule,
			ContentType: ps.ContentType,
			Data:        ps.Data,
			Timezone:    ps.Timezone,
		},
	}
}

// createPingSource creates a new PingSource and emits a Kubernetes event.
func (e EventSourceManager) createPingSource(capp *cappv1alpha1.Capp, ps *sourcesv1.PingSource, rm rclient.ResourceManagerClient) error {
	if err := rm.CreateResource(ps); err != nil {
		e.EventRecorder.Event(capp, corev1.EventTypeWarning, eventPingSourceCreationFailed,
			fmt.Sprintf("Failed to create PingSource %s", ps.Name))
		return err
	}
	e.EventRecorder.Event(capp, corev1.EventTypeNormal, eventPingSourceCreated,
		fmt.Sprintf("Created PingSource %s", ps.Name))
	return nil
}

// updatePingSource updates an existing PingSource if its spec has drifted.
func (e EventSourceManager) updatePingSource(existing, desired sourcesv1.PingSource, rm rclient.ResourceManagerClient) error {
	if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		existing.Spec = desired.Spec
		return rm.UpdateResource(&existing)
	}
	return nil
}

// getPreviousPingSources lists all PingSources owned by the Capp via label selector.
func (e EventSourceManager) getPreviousPingSources(capp cappv1alpha1.Capp) (sourcesv1.PingSourceList, error) {
	list := sourcesv1.PingSourceList{}
	set := labels.Set{utils.CappResourceKey: capp.Name}
	listOptions := utils.GetListOptions(set)
	if err := e.K8sclient.List(e.Ctx, &list, &listOptions); err != nil {
		return list, fmt.Errorf("unable to list PingSources of Capp %q: %w", capp.Name, err)
	}
	return list, nil
}

// cleanOrphanedPingSources deletes PingSources not present in desiredNames.
// Pass nil to delete all PingSources owned by the Capp.
func (e EventSourceManager) cleanOrphanedPingSources(capp cappv1alpha1.Capp, desiredNames map[string]bool) error {
	existing, err := e.getPreviousPingSources(capp)
	if err != nil {
		return err
	}
	rm := rclient.ResourceManagerClient{Ctx: e.Ctx, K8sclient: e.K8sclient, Log: e.Log}
	for _, ps := range existing.Items {
		if desiredNames != nil && desiredNames[ps.Name] {
			continue
		}
		bare := rclient.GetBarePingSource(ps.Name, ps.Namespace)
		if err := rm.DeleteResource(&bare); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
