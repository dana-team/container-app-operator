package resourcemanagers

import (
	"context"
	"strconv"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	kautoscaling "knative.dev/serving/pkg/apis/autoscaling"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func newKsvcScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(knativev1.AddToScheme(s))
	return s
}

func newKsvcManager(k8sClient client.Client) (KnativeServiceManager, *events.FakeRecorder) {
	recorder := events.NewFakeRecorder(10)
	return KnativeServiceManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         recorder,
	}, recorder
}

func newKsvcCapp() cappv1alpha1.Capp {
	capp := newBaseCapp()
	capp.Spec.State = cappEnabledState
	capp.Spec.ScaleSpec = cappv1alpha1.ScaleSpec{Metric: kautoscaling.CPU}
	capp.Spec.ConfigurationSpec = knativev1.ConfigurationSpec{
		Template: knativev1.RevisionTemplateSpec{
			Spec: knativev1.RevisionSpec{
				PodSpec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "example.com/app:v1",
					}},
				},
			},
		},
	}
	return capp
}

func TestKnativeServiceManagerPrepareResource(t *testing.T) {
	ctx := context.Background()

	t.Run("filters internal annotations and propagates allowed metadata", func(t *testing.T) {
		const (
			allowedAnnotationKey    = "example.com/propagate"
			allowedAnnotationValue  = "propagated"
			strippedAnnotationValue = "drop-me"
		)
		internalAnnotationKey := utils.CappAPIGroup + "/internal"

		km, _ := newKsvcManager(newFakeClient(newKsvcScheme(), newCappConfig()))
		capp := newKsvcCapp()
		capp.Annotations = map[string]string{
			allowedAnnotationKey:  allowedAnnotationValue,
			internalAnnotationKey: strippedAnnotationValue,
			kubectlKubernetesIOAnnotationPrefix + "last-applied-configuration": strippedAnnotationValue,
		}

		got := km.prepareResource(capp, ctx)

		require.Equal(t, allowedAnnotationValue, got.Annotations[allowedAnnotationKey])
		require.NotContains(t, got.Annotations, internalAnnotationKey)
		require.NotContains(t, got.Annotations, kubectlKubernetesIOAnnotationPrefix+"last-applied-configuration")
		require.Equal(t, allowedAnnotationValue, got.Spec.Template.Annotations[allowedAnnotationKey])
	})

	t.Run("propagates user labels and sets capp resource key on template", func(t *testing.T) {
		const (
			userLabelKey   = "team"
			userLabelValue = "platform"
		)

		km, _ := newKsvcManager(newFakeClient(newKsvcScheme(), newCappConfig()))
		capp := newKsvcCapp()
		capp.Labels = map[string]string{
			userLabelKey:          userLabelValue,
			utils.CappResourceKey: "user-override",
		}

		got := km.prepareResource(capp, ctx)

		require.Equal(t, userLabelValue, got.Spec.Template.Labels[userLabelKey])
		require.Equal(t, cappName, got.Spec.Template.Labels[utils.CappResourceKey])
		require.Equal(t, cappName, got.Labels[utils.CappResourceKey])
		require.Equal(t, utils.CappKey, got.Labels[utils.ManagedByLabelKey])
	})

	t.Run("sets route timeout on template", func(t *testing.T) {
		km, _ := newKsvcManager(newFakeClient(newKsvcScheme(), newCappConfig()))
		capp := newKsvcCapp()
		routeTimeout := int64(30)
		capp.Spec.RouteSpec.RouteTimeoutSeconds = &routeTimeout

		got := km.prepareResource(capp, ctx)

		require.NotNil(t, got.Spec.Template.Spec.TimeoutSeconds)
		require.Equal(t, routeTimeout, *got.Spec.Template.Spec.TimeoutSeconds)
	})

	t.Run("appends nfs volumes as pvc volume sources", func(t *testing.T) {
		const nfsVolumeName = "data-vol"

		km, _ := newKsvcManager(newFakeClient(newKsvcScheme(), newCappConfig()))
		capp := newKsvcCapp()
		capp.Spec.VolumesSpec.NFSVolumes = []cappv1alpha1.NFSVolume{{
			Name:     nfsVolumeName,
			Server:   "nfs.example.com",
			Path:     "/export",
			Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
		}}

		got := km.prepareResource(capp, ctx)

		require.Len(t, got.Spec.Template.Spec.Volumes, 1)
		require.Equal(t, nfsVolumeName, got.Spec.Template.Spec.Volumes[0].Name)
		require.Equal(t, nfsVolumeName, got.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName)
	})

	t.Run("merges autoscale annotations from capp and cappConfig", func(t *testing.T) {
		cappConfig := newCappConfig()

		km, _ := newKsvcManager(newFakeClient(newKsvcScheme(), cappConfig))
		capp := newKsvcCapp()

		got := km.prepareResource(capp, ctx)

		require.Equal(t, kautoscaling.HPA, got.Spec.Template.Annotations[kautoscaling.ClassAnnotationKey])
		require.Equal(t, kautoscaling.CPU, got.Spec.Template.Annotations[kautoscaling.MetricAnnotationKey])
		require.Equal(t, strconv.Itoa(cappConfig.Spec.AutoscaleConfig.CPU), got.Spec.Template.Annotations[kautoscaling.TargetAnnotationKey])
		require.Equal(t, strconv.Itoa(cappConfig.Spec.AutoscaleConfig.ActivationScale), got.Spec.Template.Annotations[kautoscaling.ActivationScaleKey])
	})
}

func TestKnativeServiceManagerManage(t *testing.T) {
	ctx := context.Background()

	t.Run("creates ksvc with owner reference when enabled", func(t *testing.T) {
		km, _ := newKsvcManager(newFakeClient(newKsvcScheme(), newCappConfig()))
		capp := newKsvcCapp()

		require.NoError(t, km.Manage(ctx, capp))

		got := &knativev1.Service{}
		require.NoError(t, km.K8sclient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got))
		require.Len(t, got.OwnerReferences, 1)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
		require.Equal(t, cappName, got.Labels[utils.CappResourceKey])
	})

	t.Run("updates ksvc when spec changes", func(t *testing.T) {
		const updatedContainerImage = "example.com/app:v2"

		km, _ := newKsvcManager(newFakeClient(newKsvcScheme(), newCappConfig()))
		capp := newKsvcCapp()
		require.NoError(t, km.Manage(ctx, capp))

		capp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Image = updatedContainerImage
		require.NoError(t, km.Manage(ctx, capp))

		got := &knativev1.Service{}
		require.NoError(t, km.K8sclient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got))
		require.Equal(t, updatedContainerImage, got.Spec.Template.Spec.Containers[0].Image)
	})

	t.Run("skips update when unchanged", func(t *testing.T) {
		km, _ := newKsvcManager(newFakeClient(newKsvcScheme(), newCappConfig()))
		capp := newKsvcCapp()
		require.NoError(t, km.Manage(ctx, capp))

		before := &knativev1.Service{}
		require.NoError(t, km.K8sclient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, before))
		beforeRV := before.ResourceVersion

		require.NoError(t, km.Manage(ctx, capp))

		after := &knativev1.Service{}
		require.NoError(t, km.K8sclient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, after))
		require.Equal(t, beforeRV, after.ResourceVersion)
	})

	t.Run("deletes ksvc and emits disabled event when state is disabled", func(t *testing.T) {
		km, recorder := newKsvcManager(newFakeClient(newKsvcScheme(), newCappConfig()))
		capp := newKsvcCapp()
		require.NoError(t, km.Manage(ctx, capp))
		require.Contains(t, <-recorder.Events, eventCappKnativeServiceCreated)

		capp.Spec.State = cappDisabledState
		require.NoError(t, km.Manage(ctx, capp))

		got := &knativev1.Service{}
		getErr := km.K8sclient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got)
		require.Error(t, getErr)
		require.True(t, errors.IsNotFound(getErr))

		require.Contains(t, <-recorder.Events, eventCappDisabled)
	})

	t.Run("recreates ksvc and emits enabled event when resuming from disabled", func(t *testing.T) {
		km, recorder := newKsvcManager(newFakeClient(newKsvcScheme(), newCappConfig()))
		capp := newKsvcCapp()
		capp.Status.StateStatus = cappv1alpha1.StateStatus{
			State:      cappDisabledState,
			LastChange: metav1.Now(),
		}

		require.NoError(t, km.Manage(ctx, capp))

		got := &knativev1.Service{}
		require.NoError(t, km.K8sclient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got))
		require.Len(t, got.OwnerReferences, 1)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)

		require.Contains(t, <-recorder.Events, eventCappKnativeServiceCreated)
		require.Contains(t, <-recorder.Events, eventCappEnabled)
	})
}

func TestKnativeServiceManagerCleanUp(t *testing.T) {
	ctx := context.Background()

	t.Run("succeeds when none exist", func(t *testing.T) {
		km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
		require.NoError(t, km.CleanUp(ctx, newKsvcCapp()))
	})

	t.Run("skips delete when deleting and has owner reference", func(t *testing.T) {
		capp := cappWithDeletionTimestamp(newKsvcCapp())

		ksvc := &knativev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cappName,
				Namespace: cappNamespace,
			},
		}
		require.NoError(t, controllerutil.SetOwnerReference(&capp, ksvc, newKsvcScheme()))

		km, _ := newKsvcManager(newFakeClient(newKsvcScheme(), ksvc))
		require.NoError(t, km.CleanUp(ctx, capp))

		got := &knativev1.Service{}
		require.NoError(t, km.K8sclient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got))
	})
}
