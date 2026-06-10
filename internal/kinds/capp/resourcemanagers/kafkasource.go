package resourcemanagers

import (
	"cmp"
	"context"
	"fmt"
	"sort"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	bindingsv1 "knative.dev/eventing-kafka-broker/control-plane/pkg/apis/bindings/v1"
	kafkasourcev1 "knative.dev/eventing-kafka-broker/control-plane/pkg/apis/sources/v1"
	kafkasecurity "knative.dev/eventing-kafka-broker/control-plane/pkg/security"
	kapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	eventKafkaSourceCreationFailed = "KafkaSourceCreationFailed"
	eventKafkaSourceCreated        = "KafkaSourceCreated"
)

type KafkaSourceManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
}

func (k KafkaSourceManager) IsRequired(capp cappv1alpha1.Capp) bool {
	for _, source := range capp.Spec.EventSourcesSpec.Sources {
		if source.KafkaSourceConfiguration != nil {
			return true
		}
	}
	return false
}

func (k KafkaSourceManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if k.IsRequired(capp) {
		for _, source := range capp.Spec.EventSourcesSpec.Sources {
			if source.KafkaSourceConfiguration == nil {
				continue
			}
			if err := k.createOrUpdate(ctx, capp, source); err != nil {
				return fmt.Errorf("failed to create or update KafkaSource %q: %w", source.Name, err)
			}
		}
		return k.cleanUpOrphans(ctx, capp)
	}

	return k.CleanUp(ctx, capp)
}

func (k KafkaSourceManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	kafkaSources, err := k.getKafkaSources(ctx, capp)
	if err != nil {
		return err
	}
	for i := range kafkaSources.Items {
		ks := &kafkaSources.Items[i]
		if err := client.IgnoreNotFound(k.DeleteResource(ctx, ks)); err != nil {
			return fmt.Errorf("failed to delete KafkaSource %q: %w", ks.Name, err)
		}
	}
	return nil
}

func (k KafkaSourceManager) GetStatus(ctx context.Context, capp cappv1alpha1.Capp) (cappv1alpha1.EventingStatus, error) {
	kafkaSources, err := k.getKafkaSources(ctx, capp)
	if err != nil {
		return cappv1alpha1.EventingStatus{}, err
	}
	if len(kafkaSources.Items) == 0 {
		return cappv1alpha1.EventingStatus{}, nil
	}
	statuses := make([]cappv1alpha1.EventSourceStatus, 0, len(kafkaSources.Items))
	for _, ks := range kafkaSources.Items {
		statuses = append(statuses, newEventSourceStatus(ks.Name, ks.Status.GetCondition(kapis.ConditionReady)))
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
	return cappv1alpha1.EventingStatus{EventSources: statuses}, nil
}

func (k KafkaSourceManager) createOrUpdate(ctx context.Context, capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) error {
	kafkaSourceFromCapp := k.prepareResource(capp, source)
	existing := &kafkasourcev1.KafkaSource{}
	err := k.K8sclient.Get(ctx, client.ObjectKey{Name: kafkaSourceFromCapp.Name, Namespace: kafkaSourceFromCapp.Namespace}, existing)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get KafkaSource %q: %w", kafkaSourceFromCapp.Name, err)
		}
		return createManagedResource(ctx, k.K8sclient, k.CreateResource, k.EventRecorder, &capp, &kafkaSourceFromCapp,
			"KafkaSource", eventKafkaSourceCreated, eventKafkaSourceCreationFailed)
	}

	orig := existing.DeepCopy()
	existing.Spec = kafkaSourceFromCapp.Spec
	existing.Spec.ConsumerGroup = orig.Spec.ConsumerGroup
	if err := ensureOwnerReference(k.K8sclient, &capp, existing, "KafkaSource"); err != nil {
		return err
	}
	if managedResourceNeedsUpdate(orig.Spec, existing.Spec, orig.OwnerReferences, existing.OwnerReferences) {
		k.Log.Info("Updating KafkaSource", "Name", existing.Name)
	}
	return updateManagedResourceIfNeeded(ctx, k.UpdateResource, existing, orig.Spec, existing.Spec, orig.OwnerReferences)
}

// prepareResource prepares a KafkaSource resource based on the provided Capp and source entry.
func (k KafkaSourceManager) prepareResource(capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) kafkasourcev1.KafkaSource {
	cfg := source.KafkaSourceConfiguration
	name := fmt.Sprintf("%s-%s", capp.Name, source.Name)

	consumers := int32(1)
	if capp.Spec.State == cappv1alpha1.CappStateDisabled {
		consumers = 0
	}

	return kafkasourcev1.KafkaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: capp.Namespace,
			Labels:    utils.ManagedResourceLabels(capp.Name),
		},
		Spec: kafkasourcev1.KafkaSourceSpec{
			Consumers: &consumers,
			KafkaAuthSpec: bindingsv1.KafkaAuthSpec{
				BootstrapServers: cfg.BootstrapServers,
				Net: bindingsv1.KafkaNetSpec{
					SASL: bindingsv1.KafkaSASLSpec{
						Enable:   true,
						User:     newSecretValueFromSource(cfg.SecretRef.Name, kafkasecurity.SaslUserKey),
						Password: newSecretValueFromSource(cfg.SecretRef.Name, kafkasecurity.SaslPasswordKey),
						Type:     newSecretValueFromSource(cfg.SecretRef.Name, kafkasecurity.SaslMechanismKey),
					},
					TLS: bindingsv1.KafkaTLSSpec{Enable: false},
				},
			},
			Topics:        cfg.Topics,
			ConsumerGroup: cmp.Or(cfg.ConsumerGroup, name),
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

func (k KafkaSourceManager) cleanUpOrphans(ctx context.Context, capp cappv1alpha1.Capp) error {
	desired := make(map[string]struct{})
	for _, source := range capp.Spec.EventSourcesSpec.Sources {
		if source.KafkaSourceConfiguration != nil {
			desired[fmt.Sprintf("%s-%s", capp.Name, source.Name)] = struct{}{}
		}
	}
	owned, err := k.getKafkaSources(ctx, capp)
	if err != nil {
		return err
	}
	for i := range owned.Items {
		ks := &owned.Items[i]
		if _, keep := desired[ks.Name]; !keep {
			if err := client.IgnoreNotFound(k.DeleteResource(ctx, ks)); err != nil {
				return fmt.Errorf("failed to delete orphaned KafkaSource %q: %w", ks.Name, err)
			}
		}
	}
	return nil
}

func (k KafkaSourceManager) getKafkaSources(ctx context.Context, capp cappv1alpha1.Capp) (kafkasourcev1.KafkaSourceList, error) {
	list := kafkasourcev1.KafkaSourceList{}
	if err := listManagedResources(ctx, k.K8sclient, capp, &list, "KafkaSource", nil); err != nil {
		return list, err
	}
	return list, nil
}

func newSecretValueFromSource(secretName, key string) bindingsv1.SecretValueFromSource {
	return bindingsv1.SecretValueFromSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
			Key:                  key,
		},
	}
}
