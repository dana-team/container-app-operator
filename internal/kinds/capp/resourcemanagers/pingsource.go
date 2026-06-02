package resourcemanagers

import (
	"context"
	"fmt"
	"sort"

	"github.com/cloudevents/sdk-go/v2/event"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/events"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	kapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	PingSource                    = "pingSource"
	eventPingSourceCreationFailed = "PingSourceCreationFailed"
	eventPingSourceCreated        = "PingSourceCreated"
)

type PingSourceManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder events.EventRecorder
}

func (p PingSourceManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return len(capp.Spec.EventSourcesSpec.Sources) > 0
}

func (p PingSourceManager) Manage(capp cappv1alpha1.Capp) error {
	if p.IsRequired(capp) {
		return p.reconcilePingSources(capp)
	}

	return p.CleanUp(capp)
}

func (p PingSourceManager) CleanUp(capp cappv1alpha1.Capp) error {
	pingSources, err := p.getPingSources(capp)
	if err != nil {
		return err
	}
	for i := range pingSources.Items {
		ps := &pingSources.Items[i]
		if err := p.K8sclient.Delete(p.Ctx, ps); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to delete PingSource %q: %w", ps.Name, err)
		}
	}
	return nil
}

func (p PingSourceManager) GetStatus(capp cappv1alpha1.Capp) (cappv1alpha1.EventingStatus, error) {
	pingSources, err := p.getPingSources(capp)
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

func (p PingSourceManager) reconcilePingSources(capp cappv1alpha1.Capp) error {
	for _, source := range capp.Spec.EventSourcesSpec.Sources {
		if source.PingSourceConfiguration == nil {
			continue
		}
		if err := p.createOrUpdate(capp, source); err != nil {
			return fmt.Errorf("failed to create or update PingSource %q: %w", source.Name, err)
		}
	}
	return p.cleanUpOrphans(capp)
}

func (p PingSourceManager) createOrUpdate(capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) error {
	desired := p.preparePingSource(capp, source)
	existing := &sourcesv1.PingSource{}
	err := p.K8sclient.Get(p.Ctx, client.ObjectKey{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get PingSource %q: %w", desired.Name, err)
		}
		return p.createPingSource(&capp, desired)
	}
	return p.updatePingSource(&capp, existing, desired)
}

func (p PingSourceManager) createPingSource(capp *cappv1alpha1.Capp, ps *sourcesv1.PingSource) error {
	if err := controllerutil.SetOwnerReference(capp, ps, p.K8sclient.Scheme()); err != nil {
		return fmt.Errorf("set PingSource owner reference: %w", err)
	}
	p.Log.Info("Creating PingSource", "Name", ps.Name)
	if err := p.K8sclient.Create(p.Ctx, ps); err != nil {
		p.EventRecorder.Eventf(capp, nil, corev1.EventTypeWarning, eventPingSourceCreationFailed, eventPingSourceCreationFailed,
			"Failed to create PingSource %s", ps.Name)
		return err
	}
	p.EventRecorder.Eventf(capp, nil, corev1.EventTypeNormal, eventPingSourceCreated, eventPingSourceCreated,
		"Created PingSource %s", ps.Name)
	return nil
}

func (p PingSourceManager) updatePingSource(capp *cappv1alpha1.Capp, existing, desired *sourcesv1.PingSource) error {
	orig := existing.DeepCopy()
	if err := controllerutil.SetOwnerReference(capp, existing, p.K8sclient.Scheme()); err != nil {
		return fmt.Errorf("set PingSource owner reference: %w", err)
	}
	existing.Spec = desired.Spec
	if equality.Semantic.DeepEqual(orig.Spec, existing.Spec) &&
		equality.Semantic.DeepEqual(orig.OwnerReferences, existing.OwnerReferences) {
		return nil
	}
	p.Log.Info("Updating PingSource", "Name", existing.Name)
	return p.K8sclient.Update(p.Ctx, existing)
}

func (p PingSourceManager) cleanUpOrphans(capp cappv1alpha1.Capp) error {
	desired := make(map[string]struct{})
	for _, source := range capp.Spec.EventSourcesSpec.Sources {
		if source.PingSourceConfiguration != nil {
			desired[fmt.Sprintf("%s-%s", capp.Name, source.Name)] = struct{}{}
		}
	}
	owned, err := p.getPingSources(capp)
	if err != nil {
		return err
	}
	for i := range owned.Items {
		ps := &owned.Items[i]
		if _, keep := desired[ps.Name]; !keep {
			if err := p.K8sclient.Delete(p.Ctx, ps); client.IgnoreNotFound(err) != nil {
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

func (p PingSourceManager) getPingSources(capp cappv1alpha1.Capp) (sourcesv1.PingSourceList, error) {
	list := sourcesv1.PingSourceList{}
	set := labels.Set{utils.CappResourceKey: capp.Name}
	listOpts := utils.GetListOptions(set)
	listOpts.Namespace = capp.Namespace
	if err := p.K8sclient.List(p.Ctx, &list, &listOpts); err != nil {
		return list, fmt.Errorf("unable to list PingSources of Capp %q: %w", capp.Name, err)
	}
	return list, nil
}
