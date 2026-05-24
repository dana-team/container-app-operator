package eventsources

import (
	"context"
	"encoding/json"
	"fmt"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/sources"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	cronparser "github.com/robfig/cron/v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	applicationJSONContentType = "application/json"
)

func init() {
	sources.Register(sources.PingSourceKindName, &PingSourceKind{})
}

type PingSourceKind struct {
	sources.EventSourceKind
}

// Generate creates a new Knative PingSource object based on the provided Capp and SourceConfiguration.
func (k *PingSourceKind) Generate(capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) client.Object {
	var schedule, data string
	if source.PingSourceConfiguration != nil {
		schedule = source.PingSourceConfiguration.Schedule
		data = source.PingSourceConfiguration.Data
	}
	pingSource := &sourcesv1.PingSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", capp.Name, source.Name),
			Namespace: capp.Namespace,
			Labels:    utils.ManagedResourceLabels(capp.Name),
		},
		Spec: sourcesv1.PingSourceSpec{
			Schedule:    schedule,
			Data:        data,
			ContentType: applicationJSONContentType,
			SourceSpec: duckv1.SourceSpec{
				Sink: duckv1.Destination{
					Ref: &duckv1.KReference{
						Name:       capp.Name,
						Namespace:  capp.Namespace,
						Kind:       "Service",
						APIVersion: servingv1.SchemeGroupVersion.String(),
					},
					URI: source.URI,
				},
			},
		},
	}
	return pingSource
}

func (k *PingSourceKind) CreateOrUpdate(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) error {
	desired := k.Generate(capp, source).(*sourcesv1.PingSource)

	existing := &sourcesv1.PingSource{}
	err := rm.K8sclient.Get(ctx, client.ObjectKey{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		if err := controllerutil.SetOwnerReference(&capp, desired, rm.K8sclient.Scheme()); err != nil {
			return fmt.Errorf("set PingSource owner reference: %w", err)
		}
		log.Info("Creating PingSource", "Name", desired.Name)
		return rm.K8sclient.Create(ctx, desired)
	}

	orig := existing.DeepCopy()
	if err := controllerutil.SetOwnerReference(&capp, existing, rm.K8sclient.Scheme()); err != nil {
		return fmt.Errorf("set PingSource owner reference: %w", err)
	}
	existing.Spec = desired.Spec

	if equality.Semantic.DeepEqual(orig.Spec, existing.Spec) &&
		equality.Semantic.DeepEqual(orig.OwnerReferences, existing.OwnerReferences) {
		return nil
	}
	log.Info("Updating PingSource", "Name", existing.Name)
	return rm.K8sclient.Update(ctx, existing)
}

func (k *PingSourceKind) List(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp) ([]client.Object, error) {
	pingSourceList := &sourcesv1.PingSourceList{}
	listOpts := []client.ListOption{
		client.InNamespace(capp.Namespace),
		client.MatchingLabels(utils.ManagedResourceLabels(capp.Name)),
	}
	if err := rm.K8sclient.List(ctx, pingSourceList, listOpts...); err != nil {
		return nil, err
	}
	result := make([]client.Object, len(pingSourceList.Items))
	for i := range pingSourceList.Items {
		result[i] = &pingSourceList.Items[i]
	}
	return result, nil
}

func (k *PingSourceKind) Delete(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) error {
	pingSourceObj := k.Generate(capp, source)
	existingPingSource := &sourcesv1.PingSource{}
	err := rm.K8sclient.Get(ctx, client.ObjectKey{Name: pingSourceObj.GetName(), Namespace: pingSourceObj.GetNamespace()}, existingPingSource)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	log.Info("Deleting PingSource", "Name", pingSourceObj.GetName())
	return rm.K8sclient.Delete(ctx, existingPingSource)
}

func (k *PingSourceKind) GetStatus(ctx context.Context, rm rclient.ResourceManagerClient, log logr.Logger, capp cappv1alpha1.Capp) ([]cappv1alpha1.EventSourceStatus, error) {
	sourceList, err := k.List(ctx, rm, log, capp)
	if err != nil {
		return nil, err
	}

	result := make([]cappv1alpha1.EventSourceStatus, 0, len(sourceList))
	for _, source := range sourceList {
		condition := metav1.Condition{
			Type:               string(apis.ConditionReady),
			Status:             metav1.ConditionUnknown,
			Reason:             "Pending",
			Message:            "Source readiness not known",
			LastTransitionTime: metav1.Now(),
		}
		status := source.(*sourcesv1.PingSource).Status
		if sourceCondition := status.GetCondition(apis.ConditionReady); sourceCondition != nil {
			condition.Status = metav1.ConditionStatus(sourceCondition.Status)
			condition.Message = sourceCondition.Message
			if sourceCondition.Reason != "" {
				condition.Reason = sourceCondition.Reason
			}
			condition.LastTransitionTime = sourceCondition.LastTransitionTime.Inner
		}
		result = append(result, cappv1alpha1.EventSourceStatus{
			Name:      source.GetName(),
			Condition: condition,
		})
	}
	return result, nil
}

func (k *PingSourceKind) Validate(capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) error {
	if source.PingSourceConfiguration == nil {
		return fmt.Errorf("source %q must specify pingSourceConfiguration", source.Name)
	}
	cfg := source.PingSourceConfiguration
	if cfg.Schedule != "" {
		parser := cronparser.NewParser(cronparser.Minute | cronparser.Hour | cronparser.Dom | cronparser.Month | cronparser.Dow)
		if _, err := parser.Parse(cfg.Schedule); err != nil {
			return fmt.Errorf("source %q has invalid cron schedule %q: %w", source.Name, cfg.Schedule, err)
		}
	}
	if cfg.Data != "" && !json.Valid([]byte(cfg.Data)) {
		return fmt.Errorf("source %q has invalid JSON in data field", source.Name)
	}
	return nil
}
