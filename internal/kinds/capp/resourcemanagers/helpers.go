package resourcemanagers

import (
	"fmt"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	k8s client.Client,
	create func(client.Object) error,
	recorder events.EventRecorder,
	capp *cappv1alpha1.Capp,
	obj client.Object,
	kind, eventCreated, eventFailed string,
) error {
	if err := ensureOwnerReference(k8s, capp, obj, kind); err != nil {
		return err
	}
	if err := create(obj); err != nil {
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

func updateManagedResourceIfNeeded(update func(client.Object) error, obj client.Object, origSpec, newSpec any, origOwners []metav1.OwnerReference) error {
	if !managedResourceNeedsUpdate(origSpec, newSpec, origOwners, obj.GetOwnerReferences()) {
		return nil
	}
	return update(obj)
}
