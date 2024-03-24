package resourceprepares

import (
	"context"
	"fmt"
	"reflect"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internals/wrappers"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CappResourceKey         = "rcs.dana.io/parent-capp"
	eventCappSecretNotFound = "SecretNotFound"
)

type KnativeDomainMappingManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// PrepareKnativeDomainMapping creates a new DomainMapping for a Knative service.
// Takes a context.Context object, and a cappv1alpha1.Capp object as input.
// Returns a knativev1alphav1.DomainMapping object.
func (k KnativeDomainMappingManager) prepareResource(capp cappv1alpha1.Capp) knativev1beta1.DomainMapping {
	knativeDomainMapping := &knativev1beta1.DomainMapping{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      capp.Spec.RouteSpec.Hostname,
			Namespace: capp.Namespace,
			Labels: map[string]string{
				CappResourceKey: capp.Name,
			},
			Annotations: map[string]string{
				CappResourceKey: capp.Name,
			},
		},
		Spec: knativev1beta1.DomainMappingSpec{
			Ref: duckv1.KReference{
				APIVersion: knativev1.SchemeGroupVersion.String(),
				Name:       capp.Name,
				Kind:       "Service",
			},
		},
	}
	resourceManager := rclient.ResourceBaseManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	k.setHttpsKnativeDomainMapping(capp, knativeDomainMapping, resourceManager)
	return *knativeDomainMapping
}

func (k KnativeDomainMappingManager) CleanUp(capp cappv1alpha1.Capp) error {
	if capp.Spec.RouteSpec.Hostname == "" {
		return nil
	}
	resourceManager := rclient.ResourceBaseManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	DomainMapping := knativev1beta1.DomainMapping{}
	if err := resourceManager.DeleteResource(&DomainMapping, capp.Spec.RouteSpec.Hostname, capp.Namespace); err != nil {
		return fmt.Errorf("unable to delete DomainMapping %q: %w", capp.Spec.RouteSpec.Hostname, err)
	}
	return nil
}

// IsRequired is responsible to determine if resource knative domain mapping is required.
func (k KnativeDomainMappingManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return capp.Spec.RouteSpec.Hostname != ""
}

func (k KnativeDomainMappingManager) CreateOrUpdateObject(capp cappv1alpha1.Capp) error {
	if err := k.HandleIrrelevantDomainMapping(capp); err != nil {
		k.Log.Error(err, "failed to handle irrelevant DomainMappings")
		return err
	}
	if k.IsRequired(capp) {
		cappDomainMapping := k.prepareResource(capp)
		knativeDomainMapping := knativev1beta1.DomainMapping{}
		resourceManager := rclient.ResourceBaseManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
		if err := k.K8sclient.Get(k.Ctx, types.NamespacedName{Namespace: capp.Namespace, Name: capp.Spec.RouteSpec.Hostname}, &knativeDomainMapping); err != nil {
			if errors.IsNotFound(err) {
				if err := resourceManager.CreateResource(&cappDomainMapping); err != nil {
					k.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappDomainMappingCreationFailed, fmt.Sprintf("Failed to create DomainMapping %s for Capp %s", capp.Spec.RouteSpec.Hostname, capp.Name))
					return fmt.Errorf("unable to create DomainMapping: %w", err)
				}
			} else {
				return err
			}
			return nil
		}
		if !reflect.DeepEqual(knativeDomainMapping.Spec, cappDomainMapping.Spec) {
			knativeDomainMapping.Spec = cappDomainMapping.Spec
			if err := resourceManager.UpdateResource(&knativeDomainMapping); err != nil {
				return fmt.Errorf("unable to update DomainMapping: %w", err)
			}
		}
	}
	return nil
}

func (k KnativeDomainMappingManager) HandleIrrelevantDomainMapping(capp cappv1alpha1.Capp) error {
	logger := k.Log
	requirement, err := labels.NewRequirement(CappResourceKey, selection.Equals, []string{capp.Name})
	if err != nil {
		return fmt.Errorf("unable to create label requirement for Capp: %w", err)
	}
	labelSelector := labels.NewSelector().Add(*requirement)
	listOptions := client.ListOptions{
		LabelSelector: labelSelector,
	}
	knativeDomainMappings := knativev1beta1.DomainMappingList{}
	if err := k.K8sclient.List(k.Ctx, &knativeDomainMappings, &listOptions); err != nil {
		return fmt.Errorf("unable to list DomainMappings of Capp %q: %w", capp.Name, err)
	}
	resourceManager := rclient.ResourceBaseManagerClient{Ctx: k.Ctx, K8sclient: k.K8sclient, Log: k.Log}
	for _, domainMapping := range knativeDomainMappings.Items {
		if domainMapping.Name != capp.Spec.RouteSpec.Hostname {
			DomainMapping := knativev1beta1.DomainMapping{}
			if err := resourceManager.DeleteResource(&DomainMapping, domainMapping.Name, capp.Namespace); err != nil {
				logger.Error(err, fmt.Sprintf("unable to delete irrelevant DomainMapping %q", domainMapping.Name))
				return err
			}
		}
	}
	return nil
}

// SetHttpsKnativeDomainMapping takes a Capp, Knative Domain Mapping and
// a ResourceBaseManager Client and sets the Knative Domain Mapping Tls based on the Capp's Https field.
func (k KnativeDomainMappingManager) setHttpsKnativeDomainMapping(capp cappv1alpha1.Capp, knativeDomainMapping *knativev1beta1.DomainMapping, resourceManager rclient.ResourceBaseManagerClient) {
	isHttps := capp.Spec.RouteSpec.TlsEnabled
	if isHttps {
		tlsSecret := corev1.Secret{}
		if err := resourceManager.K8sclient.Get(resourceManager.Ctx, types.NamespacedName{Name: capp.Spec.RouteSpec.TlsSecret, Namespace: capp.Namespace}, &tlsSecret); err != nil {
			if errors.IsNotFound(err) {
				resourceManager.Log.Error(err, fmt.Sprintf("the tls secret %s for DomainMapping does not exist", capp.Spec.RouteSpec.TlsSecret))
				k.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventCappSecretNotFound, fmt.Sprintf("Secret %s for DomainMapping %s does not exist", capp.Spec.RouteSpec.TlsSecret, knativeDomainMapping.Name))
				return
			}
			resourceManager.Log.Error(err, fmt.Sprintf("unable to get tls secret %s for DomainMapping", capp.Spec.RouteSpec.TlsSecret))
		} else {
			knativeDomainMapping.Spec.TLS = &knativev1beta1.SecretTLS{
				SecretName: capp.Spec.RouteSpec.TlsSecret,
			}
		}
	}
}
