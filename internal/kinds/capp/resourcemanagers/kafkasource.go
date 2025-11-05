package resourcemanagers

import (
	"context"
	"fmt"
	"reflect"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bindingsv1beta1 "knative.dev/eventing-kafka/pkg/apis/bindings/v1beta1"
	kafkasourcesv1 "knative.dev/eventing-kafka/pkg/apis/sources/v1beta1"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const kafka = "kafka"

type KafkaSourceManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// prepareResource generates a KafkaSource resource from a given Capp resource.
func (k KafkaSourceManager) prepareResource(capp cappv1alpha1.Capp) []kafkasourcesv1.KafkaSource {
	//nolint:prealloc
	var kafkaSources []kafkasourcesv1.KafkaSource

	for _, source := range capp.Spec.Sources {

		if source.Type != kafka {
			continue
		}

		kafkaSource := kafkasourcesv1.KafkaSource{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      source.Name,
				Namespace: capp.Namespace,
				Labels: map[string]string{
					utils.CappResourceKey:   capp.Name,
					utils.ManagedByLabelKey: utils.CappKey,
				},
			},
			Spec: kafkasourcesv1.KafkaSourceSpec{
				Topics: source.Topic,
				KafkaAuthSpec: bindingsv1beta1.KafkaAuthSpec{
					BootstrapServers: source.BootstrapServers,
					Net: bindingsv1beta1.KafkaNetSpec{
						SASL: bindingsv1beta1.KafkaSASLSpec{
							Enable: true,
							User: bindingsv1beta1.SecretValueFromSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									Key: source.KafkaAuth.Username,
								},
							},
							Password: bindingsv1beta1.SecretValueFromSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									Key: source.KafkaAuth.PasswordKeyRef.Key,
								},
							},
						},
					},
				},
			},
		}

		kafkaSources = append(kafkaSources, kafkaSource)
	}

	return kafkaSources

}

// CleanUp attempts to delete the associated KafkaSource for a given Capp resource.
func (k KafkaSourceManager) CleanUp(capp cappv1alpha1.Capp) error {
	resourceManager := rclient.ResourceManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}

	for _, source := range capp.Spec.Sources {

		if source.Type != kafka {
			continue
		}
		kafkaSource := rclient.GetBareKafkaSource(capp.Name, capp.Namespace)

		if err := resourceManager.DeleteResource(&kafkaSource); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
	}
	return nil
}

// IsRequired is responsible to determine if resource KafkaSource is required.
func (k KafkaSourceManager) IsRequired(capp cappv1alpha1.Capp) bool {
	for _, source := range capp.Spec.Sources {
		if source.Type == kafka {
			return true
		}
	}
	return false
}

// Manage creates or updates a KafkaSource resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (k KafkaSourceManager) Manage(capp cappv1alpha1.Capp) error {
	if k.IsRequired(capp) {
		return k.createOrUpdate(capp)
	}

	return k.CleanUp(capp)
}

// createOrUpdate creates or updates a KafkaSource resource.
func (k KafkaSourceManager) createOrUpdate(capp cappv1alpha1.Capp) error {
	generateKafkaSource := k.prepareResource(capp)
	resourceManager := rclient.ResourceManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}

	for _, source := range generateKafkaSource {

		existingKafkaSource := kafkasourcesv1.KafkaSource{}
		if err := k.K8sclient.Get(k.Ctx, client.ObjectKey{Namespace: capp.Namespace, Name: source.Name}, &existingKafkaSource); err != nil {
			if errors.IsNotFound(err) {
				if err := k.createKafkaSource(&capp, &source, resourceManager); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("failed to get Kafka Source %q: %w", source.Name, err)
			}
		} else if err := k.updateKafkaSource(existingKafkaSource, source, resourceManager); err != nil {
			return err
		}
	}

	return nil
}

// createKafkaSource creates a new Kafka Source and emits an event.
func (k KafkaSourceManager) createKafkaSource(capp *cappv1alpha1.Capp, kafkaSource *kafkasourcesv1.KafkaSource, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(kafkaSource); err != nil {
		k.EventRecorder.Event(capp, corev1.EventTypeWarning, eventNFSPVCCreationFailed,
			fmt.Sprintf("Failed to create Kafka Source %s", kafkaSource.Name))
		return err
	}

	k.EventRecorder.Event(capp, corev1.EventTypeNormal, eventNFSPVCCreated,
		fmt.Sprintf("Created Kafka Source %s", kafkaSource.Name))

	return nil
}

// updateKafkaSource checks if an update to the Kafka Source is necessary and performs the update to match desired state.
func (k KafkaSourceManager) updateKafkaSource(existingKafka, kafkaSource kafkasourcesv1.KafkaSource, resourceManager rclient.ResourceManagerClient) error {
	if !reflect.DeepEqual(existingKafka.Spec, kafkaSource.Spec) {
		existingKafka.Spec = kafkaSource.Spec
		return resourceManager.UpdateResource(&existingKafka)
	}

	return nil
}
