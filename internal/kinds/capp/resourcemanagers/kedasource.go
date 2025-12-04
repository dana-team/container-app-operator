package resourcemanagers

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	scaledObjectCreated        = "ScaledObjectCreated"
	scaledObjectCreationFailed = "ScaledObjectCreationFailed"
	triggerAuthCreationFailed  = "TriggerAuthenticationFailed"
	triggerAuthCreated         = "TriggerAuthenticationCreated"
)

type KedaSourceManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// prepareResource generates a KedaSource resource from a given Capp resource.
func (k KedaSourceManager) prepareResource(capp cappv1alpha1.Capp) ([]kedav1alpha1.ScaledObject, []kedav1alpha1.TriggerAuthentication) {
	//nolint:prealloc
	var scaledObjects []kedav1alpha1.ScaledObject
	//nolint:prealloc
	var triggerAuthentications []kedav1alpha1.TriggerAuthentication

	for _, source := range capp.Spec.Sources {
		scaledObject := k.prepareScaledObject(capp, source)
		scaledObjects = append(scaledObjects, scaledObject)

		triggerAuthentication := k.prepareTriggerAuthentication(capp, source)
		if triggerAuthentication != nil {

			triggerAuthentications = append(triggerAuthentications, *triggerAuthentication)
		}
	}

	return scaledObjects, triggerAuthentications
}

// prepareScaledObject prepares and returns a scaled object from a given capp
func (k KedaSourceManager) prepareScaledObject(capp cappv1alpha1.Capp, source cappv1alpha1.KedaSource) kedav1alpha1.ScaledObject {
	scaledObject := kedav1alpha1.ScaledObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ScaledObject",
			APIVersion: "keda.sh/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.Name,
			Namespace: capp.Namespace,
			Labels: map[string]string{
				utils.CappResourceKey:   capp.Name,
				utils.ManagedByLabelKey: utils.CappKey,
			},
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			MinReplicaCount: source.MinReplicas,
			MaxReplicaCount: source.MaxReplicas,
			ScaleTargetRef: &kedav1alpha1.ScaleTarget{
				Kind: "Service",
				Name: capp.Name,
			},
			Triggers: []kedav1alpha1.ScaleTriggers{
				{
					Type:     source.ScalarType,
					Metadata: source.ScalarMetadata,
				},
			},
		},
	}

	if source.TriggerAuth != nil {
		scaledObject.Spec.Triggers[0].AuthenticationRef = &kedav1alpha1.AuthenticationRef{
			Name: source.TriggerAuth.Name,
		}
	}

	return scaledObject
}

// prepareTriggerAuthentication prepares and returns a trigger authentication from a given capp
func (k KedaSourceManager) prepareTriggerAuthentication(capp cappv1alpha1.Capp, source cappv1alpha1.KedaSource) *kedav1alpha1.TriggerAuthentication {
	if source.TriggerAuth == nil {
		return nil
	}
	triggerAuthentication := &kedav1alpha1.TriggerAuthentication{
		TypeMeta: metav1.TypeMeta{
			Kind:       strings.Title(source.TriggerAuth.Type),
			APIVersion: "keda.sh/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.TriggerAuth.Name,
			Namespace: capp.Namespace,
		},
	}

	for _, secret := range source.TriggerAuth.SecretTargets {
		triggerAuthentication.Spec.SecretTargetRef = append(triggerAuthentication.Spec.SecretTargetRef, kedav1alpha1.AuthSecretTargetRef{
			Parameter: secret.Parameter,
			Name:      secret.SecretRef.Name,
			Key:       secret.SecretRef.Key,
		})
	}

	for _, env := range source.TriggerAuth.EnvTargets {
		triggerAuthentication.Spec.Env = append(triggerAuthentication.Spec.Env, kedav1alpha1.AuthEnvironment{
			Parameter: env.Parameter,
			Name:      env.Name,
		})
	}

	if source.TriggerAuth.PodIdentity != nil {
		triggerAuthentication.Spec.PodIdentity = &kedav1alpha1.AuthPodIdentity{
			Provider: kedav1alpha1.PodIdentityProvider(source.TriggerAuth.PodIdentity.Provider),
		}
	}
	return triggerAuthentication
}

// CleanUp attempts to delete the associated scaled object for a given Capp resource.
func (k KedaSourceManager) CleanUp(capp cappv1alpha1.Capp) error {
	resourceManager := rclient.ResourceManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}

	for _, source := range capp.Spec.Sources {
		scaledObject := rclient.GetBareScaledObject(capp.Name, capp.Namespace)

		if err := resourceManager.DeleteResource(&scaledObject); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}

		if source.TriggerAuth != nil {
			triggerAuth := rclient.GetBareTriggerAuth(capp.Name, capp.Namespace)
			if err := resourceManager.DeleteResource(&triggerAuth); err != nil {
				if errors.IsNotFound(err) {
					return nil
				}
				return err
			}

		}
	}
	return nil
}

// IsRequired is responsible to determine if resource KafkaSource is required.
func (k KedaSourceManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return len(capp.Spec.Sources) > 0
}

// Manage creates or updates a Keda source resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (k KedaSourceManager) Manage(capp cappv1alpha1.Capp) error {
	if k.IsRequired(capp) {
		k.disableKPA(&capp)
		return k.createOrUpdate(capp)
	}

	k.enableKPA(&capp)
	return k.CleanUp(capp)
}

func (k KedaSourceManager) enableKPA(capp *cappv1alpha1.Capp) {
	annotations := capp.Spec.ConfigurationSpec.Template.ObjectMeta.Annotations
	if annotations == nil {
		return
	}

	delete(annotations, "autoscaling.knative.dev/class")
	delete(annotations, "autoscaling.knative.dev/metric")

	capp.Spec.ConfigurationSpec.Template.ObjectMeta.Annotations = annotations
}

func (k KedaSourceManager) disableKPA(capp *cappv1alpha1.Capp) {
	annotations := capp.Spec.ConfigurationSpec.Template.ObjectMeta.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}

	annotations["autoscaling.knative.dev/class"] = "hpa.autoscaling.knative.dev"
	annotations["autoscaling.knative.dev/metric"] = "disabled"

	capp.Spec.ConfigurationSpec.Template.ObjectMeta.Annotations = annotations
}

// createOrUpdate creates or updates a KafkaSource resource.
func (k KedaSourceManager) createOrUpdate(capp cappv1alpha1.Capp) error {
	scaledObjects, triggerAuth := k.prepareResource(capp)
	resourceManager := rclient.ResourceManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}

	for _, source := range scaledObjects {

		existingScaledObject := kedav1alpha1.ScaledObject{}
		if err := k.K8sclient.Get(k.Ctx, client.ObjectKey{Namespace: capp.Namespace, Name: source.Name}, &existingScaledObject); err != nil {
			if errors.IsNotFound(err) {
				if err := k.createScaledObject(&capp, &source, resourceManager); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("failed to get Kafka Source %q: %w", source.Name, err)
			}
		} else if err := k.updateScaledObject(existingScaledObject, source, resourceManager); err != nil {
			return err
		}
	}

	for _, source := range triggerAuth {
		existingTriggerAuth := kedav1alpha1.TriggerAuthentication{}
		if err := k.K8sclient.Get(k.Ctx, client.ObjectKey{Namespace: capp.Namespace, Name: source.Name}, &existingTriggerAuth); err != nil {
			if errors.IsNotFound(err) {
				if err := k.createTriggerAuth(&capp, &source, resourceManager); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("failed to get Kafka Source %q: %w", source.Name, err)
			}
		} else if err := k.updateTriggerAuth(existingTriggerAuth, source, resourceManager); err != nil {
			return err
		}

	}

	return nil
}

// createTriggerAuth creates a new Trigger Authentication and emits an event.
func (k KedaSourceManager) createTriggerAuth(capp *cappv1alpha1.Capp, triggerAuth *kedav1alpha1.TriggerAuthentication, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(triggerAuth); err != nil {
		k.EventRecorder.Event(capp, corev1.EventTypeWarning, triggerAuthCreationFailed,
			fmt.Sprintf("Failed to create Trigger Authentication %s", triggerAuth.Name))
		return err
	}

	k.EventRecorder.Event(capp, corev1.EventTypeNormal, triggerAuthCreated,
		fmt.Sprintf("Created Trigger Authentication %s", triggerAuth.Name))

	return nil
}

// updateTriggerAuth checks if an update to the scaled object is necessary and performs the update to match desired state.
func (k KedaSourceManager) updateTriggerAuth(existingtriggerAuth, triggerAuth kedav1alpha1.TriggerAuthentication, resourceManager rclient.ResourceManagerClient) error {
	if !reflect.DeepEqual(existingtriggerAuth.Spec, triggerAuth.Spec) {
		existingtriggerAuth.Spec = triggerAuth.Spec
		return resourceManager.UpdateResource(&existingtriggerAuth)
	}

	return nil
}

// createScaledObject creates a new Scaled Object and emits an event.
func (k KedaSourceManager) createScaledObject(capp *cappv1alpha1.Capp, scaledObject *kedav1alpha1.ScaledObject, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(scaledObject); err != nil {
		k.EventRecorder.Event(capp, corev1.EventTypeWarning, scaledObjectCreationFailed,
			fmt.Sprintf("Failed to create Scaled Object %s", scaledObject.Name))
		return err
	}

	k.EventRecorder.Event(capp, corev1.EventTypeNormal, scaledObjectCreated,
		fmt.Sprintf("Created Scaled Object %s", scaledObject.Name))

	return nil
}

// updateKafkaSource checks if an update to the scaled object is necessary and performs the update to match desired state.
func (k KedaSourceManager) updateScaledObject(existingScaledObject, scaledObject kedav1alpha1.ScaledObject, resourceManager rclient.ResourceManagerClient) error {
	if !reflect.DeepEqual(existingScaledObject.Spec, scaledObject.Spec) {
		existingScaledObject.Spec = scaledObject.Spec
		return resourceManager.UpdateResource(&existingScaledObject)
	}

	return nil
}
