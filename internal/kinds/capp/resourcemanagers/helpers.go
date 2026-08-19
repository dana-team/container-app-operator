package resourcemanagers

import (
	"context"
	"fmt"
	"strings"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func ensureOwnerReference(k8s client.Client, capp *cappv1alpha1.Capp, obj client.Object, kind string) error {
	if err := controllerutil.SetOwnerReference(capp, obj, k8s.Scheme()); err != nil {
		return fmt.Errorf("set %s owner reference: %w", kind, err)
	}
	return nil
}

func createManagedResource(
	ctx context.Context,
	k8s client.Client,
	create func(context.Context, client.Object) error,
	recorder events.EventRecorder,
	capp *cappv1alpha1.Capp,
	obj client.Object,
	kind, eventCreated, eventFailed string,
) error {
	if err := ensureOwnerReference(k8s, capp, obj, kind); err != nil {
		return err
	}
	if err := create(ctx, obj); err != nil {
		recorder.Eventf(capp, nil, corev1.EventTypeWarning, eventFailed, eventFailed,
			fmt.Sprintf("Failed to create %s %s", kind, obj.GetName()))
		return err
	}
	recorder.Eventf(capp, nil, corev1.EventTypeNormal, eventCreated, eventCreated,
		fmt.Sprintf("Created %s %s", kind, obj.GetName()))
	return nil
}

func managedResourceNeedsUpdate(origSpec, newSpec any, origOwners, newOwners []metav1.OwnerReference) bool {
	return !equality.Semantic.DeepEqual(origSpec, newSpec) ||
		!equality.Semantic.DeepEqual(origOwners, newOwners)
}

func updateManagedResourceIfNeeded(
	ctx context.Context,
	update func(context.Context, client.Object) error,
	obj client.Object,
	origSpec, newSpec any,
	origOwners []metav1.OwnerReference,
	recorder events.EventRecorder,
	capp *cappv1alpha1.Capp,
	kind, eventFailed string,
) error {
	if !managedResourceNeedsUpdate(origSpec, newSpec, origOwners, obj.GetOwnerReferences()) {
		return nil
	}
	if err := update(ctx, obj); err != nil {
		recorder.Eventf(capp, nil, corev1.EventTypeWarning, eventFailed, eventFailed,
			fmt.Sprintf("Failed to update %s %s", kind, obj.GetName()))
		return err
	}
	return nil
}

func deleteOwnedResources[T client.Object](ctx context.Context, c client.Client, capp *cappv1alpha1.Capp, resources []T) error {
	for i := range resources {
		resource := resources[i]
		if capp.DeletionTimestamp != nil {
			ok, err := controllerutil.HasOwnerReference(resource.GetOwnerReferences(), capp, c.Scheme())
			if err != nil {
				return err
			}
			if ok {
				continue
			}
		}
		if err := client.IgnoreNotFound(c.Delete(ctx, resource)); err != nil {
			return fmt.Errorf("failed to delete %s %q: %w",
				resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName(), err)
		}
	}
	return nil
}

// GenerateResourceName appends suffix (minus its trailing dot) to hostname
// when hostname does not already end with that suffix.
func GenerateResourceName(hostname, suffix string) string {
	bare := strings.TrimSuffix(suffix, ".")
	if !strings.HasSuffix(hostname, bare) {
		return hostname + "." + bare
	}

	return hostname
}

// GenerateRecordName returns hostname with the zone suffix stripped when
// present, or the original hostname unchanged otherwise.
func GenerateRecordName(hostname, suffix string) string {
	bare := strings.TrimSuffix(suffix, ".")
	if !strings.HasSuffix(hostname, bare) {
		return hostname
	}

	return hostname[:len(hostname)-len(suffix)]
}

func generateTLSSecretName(resourceName string) string {
	return fmt.Sprintf("%s-tls", resourceName)
}

// GetCappConfig fetches and returns the singleton CappConfig resource.
func GetCappConfig(ctx context.Context, k8sClient client.Client) (*cappv1alpha1.CappConfig, error) {
	cappConfig := &cappv1alpha1.CappConfig{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: cappmeta.CappConfigName, Namespace: cappmeta.CappNS}, cappConfig); err != nil {
		return nil, err
	}
	return cappConfig, nil
}

func listManagedResources(
	ctx context.Context,
	k8s client.Client,
	capp cappv1alpha1.Capp,
	list client.ObjectList,
	kind string,
	extraLabels labels.Set,
) error {
	set := labels.Set{cappmeta.CappResourceKey: capp.Name}
	for key, value := range extraLabels {
		set[key] = value
	}
	listOptions := client.ListOptions{
		LabelSelector: labels.SelectorFromSet(set),
		Namespace:     capp.Namespace,
	}
	if err := k8s.List(ctx, list, &listOptions); err != nil {
		return fmt.Errorf("unable to list %ss of Capp %q: %w", kind, capp.Name, err)
	}
	return nil
}
