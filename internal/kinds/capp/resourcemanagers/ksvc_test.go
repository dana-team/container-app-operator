package resourcemanagers

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/rollout"
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
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	kautoscaling "knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/config"
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
		ResourceManagerClient: rclient.ResourceManagerClient{K8sClient: k8sClient, Log: logr.Discard()},
		EventRecorder:         recorder,
		CappConfig:            newCappConfig(),
	}, recorder
}

func newKsvcCapp() cappv1alpha1.Capp {
	capp := newBaseCapp()
	capp.Spec.State = cappv1alpha1.CappStateEnabled
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
		internalAnnotationKey := cappmeta.CappAPIGroup + "/internal"

		km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
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

		km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
		capp := newKsvcCapp()
		capp.Labels = map[string]string{
			userLabelKey:             userLabelValue,
			cappmeta.CappResourceKey: "user-override",
		}

		got := km.prepareResource(capp, ctx)

		require.Equal(t, userLabelValue, got.Spec.Template.Labels[userLabelKey])
		require.Equal(t, cappName, got.Spec.Template.Labels[cappmeta.CappResourceKey])
		require.Equal(t, cappName, got.Labels[cappmeta.CappResourceKey])
		require.Equal(t, cappmeta.CappKey, got.Labels[cappmeta.ManagedByLabelKey])
	})

	t.Run("leaves traffic to knative defaults without rollout strategy", func(t *testing.T) {
		km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
		capp := newKsvcCapp()
		capp.Status.Rollout = &cappv1alpha1.RolloutStatus{Traffic: []cappv1alpha1.RevisionTraffic{
			{RevisionName: "r1", Percent: 100, Tag: rollout.TagStable},
		}}

		got := km.prepareResource(capp, ctx)

		want := knativev1.Service{}
		want.Spec.RouteSpec.SetDefaults(ctx)
		require.Equal(t, want.Spec.Traffic, got.Spec.Traffic)
	})

	t.Run("pins traffic to the stable revision after the strategy was removed", func(t *testing.T) {
		km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
		capp := newKsvcCapp()
		capp.Status.Rollout = &cappv1alpha1.RolloutStatus{
			StableRevision:    "r1",
			CandidateRevision: "r2",
			PinnedRevision:    "r1",
			Traffic:           []cappv1alpha1.RevisionTraffic{{RevisionName: "r2", Percent: 50, Tag: rollout.TagCandidate}},
		}

		got := km.prepareResource(capp, ctx)

		require.Equal(t, []knativev1.TrafficTarget{
			{RevisionName: "r1", Percent: ptr.To(int64(100)), Tag: rollout.TagStable, LatestRevision: ptr.To(false)},
		}, got.Spec.Traffic)
	})

	t.Run("applies rollout traffic split from status", func(t *testing.T) {
		km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
		capp := newKsvcCapp()
		capp.Spec.RouteSpec.RolloutStrategy = &cappv1alpha1.RolloutStrategy{}
		capp.Status.Rollout = &cappv1alpha1.RolloutStatus{Traffic: []cappv1alpha1.RevisionTraffic{
			{RevisionName: "r1", Percent: 90, Tag: rollout.TagStable},
			{RevisionName: "r2", Percent: 10, Tag: rollout.TagCandidate},
		}}

		got := km.prepareResource(capp, ctx)

		require.Equal(t, []knativev1.TrafficTarget{
			{RevisionName: "r1", Percent: ptr.To(int64(90)), Tag: rollout.TagStable, LatestRevision: ptr.To(false)},
			{RevisionName: "r2", Percent: ptr.To(int64(10)), Tag: rollout.TagCandidate, LatestRevision: ptr.To(false)},
		}, got.Spec.Traffic)
	})

	t.Run("sets route timeout on template", func(t *testing.T) {
		km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
		capp := newKsvcCapp()
		routeTimeout := int64(30)
		capp.Spec.RouteSpec.RouteTimeoutSeconds = &routeTimeout

		got := km.prepareResource(capp, ctx)

		require.NotNil(t, got.Spec.Template.Spec.TimeoutSeconds)
		require.Equal(t, routeTimeout, *got.Spec.Template.Spec.TimeoutSeconds)
	})

	t.Run("appends nfs volumes as pvc volume sources", func(t *testing.T) {
		const nfsVolumeName = "data-vol"

		km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
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
		km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
		capp := newKsvcCapp()

		got := km.prepareResource(capp, ctx)

		require.Equal(t, kautoscaling.HPA, got.Spec.Template.Annotations[kautoscaling.ClassAnnotationKey])
		require.Equal(t, kautoscaling.CPU, got.Spec.Template.Annotations[kautoscaling.MetricAnnotationKey])
		require.Equal(t, strconv.Itoa(km.CappConfig.Spec.AutoscaleConfig.CPU), got.Spec.Template.Annotations[kautoscaling.TargetAnnotationKey])
		require.Equal(t, strconv.Itoa(km.CappConfig.Spec.AutoscaleConfig.ActivationScale), got.Spec.Template.Annotations[kautoscaling.ActivationScaleKey])
	})
}

func TestKnativeServiceManagerManage(t *testing.T) {
	ctx := context.Background()

	t.Run("creates ksvc with owner reference when enabled", func(t *testing.T) {
		km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
		capp := newKsvcCapp()

		require.NoError(t, km.Manage(ctx, capp))

		got := &knativev1.Service{}
		require.NoError(t, km.K8sClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got))
		require.Len(t, got.OwnerReferences, 1)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
		require.Equal(t, cappName, got.Labels[cappmeta.CappResourceKey])
	})

	t.Run("updates ksvc when spec changes", func(t *testing.T) {
		const updatedContainerImage = "example.com/app:v2"

		km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
		capp := newKsvcCapp()
		require.NoError(t, km.Manage(ctx, capp))

		capp.Spec.ConfigurationSpec.Template.Spec.Containers[0].Image = updatedContainerImage
		require.NoError(t, km.Manage(ctx, capp))

		got := &knativev1.Service{}
		require.NoError(t, km.K8sClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got))
		require.Equal(t, updatedContainerImage, got.Spec.Template.Spec.Containers[0].Image)
	})

	t.Run("skips update when unchanged", func(t *testing.T) {
		km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
		capp := newKsvcCapp()
		require.NoError(t, km.Manage(ctx, capp))

		before := &knativev1.Service{}
		require.NoError(t, km.K8sClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, before))
		beforeRV := before.ResourceVersion

		require.NoError(t, km.Manage(ctx, capp))

		after := &knativev1.Service{}
		require.NoError(t, km.K8sClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, after))
		require.Equal(t, beforeRV, after.ResourceVersion)
	})

	t.Run("deletes ksvc and emits disabled event when state is disabled", func(t *testing.T) {
		km, recorder := newKsvcManager(newFakeClient(newKsvcScheme()))
		capp := newKsvcCapp()
		require.NoError(t, km.Manage(ctx, capp))
		require.Contains(t, <-recorder.Events, eventCappKnativeServiceCreated)

		capp.Spec.State = cappv1alpha1.CappStateDisabled
		require.NoError(t, km.Manage(ctx, capp))

		got := &knativev1.Service{}
		getErr := km.K8sClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got)
		require.Error(t, getErr)
		require.True(t, errors.IsNotFound(getErr))

		require.Contains(t, <-recorder.Events, eventCappDisabled)
	})

	t.Run("recreates ksvc and emits enabled event when resuming from disabled", func(t *testing.T) {
		km, recorder := newKsvcManager(newFakeClient(newKsvcScheme()))
		capp := newKsvcCapp()
		capp.Status.StateStatus = cappv1alpha1.StateStatus{
			State:      cappv1alpha1.CappStateDisabled,
			LastChange: metav1.Now(),
		}

		require.NoError(t, km.Manage(ctx, capp))

		got := &knativev1.Service{}
		require.NoError(t, km.K8sClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got))
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
		require.NoError(t, km.K8sClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got))
	})

	t.Run("deletes when deleting and lacks owner reference", func(t *testing.T) {
		capp := cappWithDeletionTimestamp(newKsvcCapp())

		ksvc := &knativev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cappName,
				Namespace: cappNamespace,
			},
		}
		km, _ := newKsvcManager(newFakeClient(newKsvcScheme(), ksvc))
		require.NoError(t, km.CleanUp(ctx, capp))

		got := &knativev1.Service{}
		getErr := km.K8sClient.Get(ctx, types.NamespacedName{Name: cappName, Namespace: cappNamespace}, got)
		require.True(t, errors.IsNotFound(getErr))
	})
}

func TestSetAutoScaler(t *testing.T) {
	defaults := cappv1alpha1.AutoscaleConfig{
		RPS: 200, CPU: 80, Memory: 70, Concurrency: 10, ActivationScale: 3,
	}

	t.Run("uses HPA class and cpu target from autoscale config", func(t *testing.T) {
		capp := cappv1alpha1.Capp{
			ObjectMeta: metav1.ObjectMeta{Name: cappName},
			Spec:       cappv1alpha1.CappSpec{ScaleSpec: cappv1alpha1.ScaleSpec{Metric: kautoscaling.CPU}},
		}
		got := setAutoScaler(capp, defaults)
		require.Equal(t, kautoscaling.HPA, got[kautoscaling.ClassAnnotationKey])
		require.Equal(t, kautoscaling.CPU, got[kautoscaling.MetricAnnotationKey])
		require.Equal(t, "80", got[kautoscaling.TargetAnnotationKey])
		require.Equal(t, "3", got[kautoscaling.ActivationScaleKey])
	})

	t.Run("uses KPA class and rps target from autoscale config", func(t *testing.T) {
		capp := cappv1alpha1.Capp{
			ObjectMeta: metav1.ObjectMeta{Name: cappName},
			Spec:       cappv1alpha1.CappSpec{ScaleSpec: cappv1alpha1.ScaleSpec{Metric: kautoscaling.RPS}},
		}
		got := setAutoScaler(capp, defaults)
		require.Equal(t, kautoscaling.KPA, got[kautoscaling.ClassAnnotationKey])
		require.Equal(t, kautoscaling.RPS, got[kautoscaling.MetricAnnotationKey])
		require.Equal(t, "200", got[kautoscaling.TargetAnnotationKey])
		require.Equal(t, "3", got[kautoscaling.ActivationScaleKey])
	})

	t.Run("uses HPA class and memory target from autoscale config", func(t *testing.T) {
		capp := cappv1alpha1.Capp{
			ObjectMeta: metav1.ObjectMeta{Name: cappName},
			Spec:       cappv1alpha1.CappSpec{ScaleSpec: cappv1alpha1.ScaleSpec{Metric: kautoscaling.Memory}},
		}
		got := setAutoScaler(capp, defaults)
		require.Equal(t, kautoscaling.HPA, got[kautoscaling.ClassAnnotationKey])
		require.Equal(t, kautoscaling.Memory, got[kautoscaling.MetricAnnotationKey])
		require.Equal(t, "70", got[kautoscaling.TargetAnnotationKey])
		require.Equal(t, "3", got[kautoscaling.ActivationScaleKey])
	})

	t.Run("sets min-scale and omits activation-scale when MinReplicas is set", func(t *testing.T) {
		capp := cappv1alpha1.Capp{
			ObjectMeta: metav1.ObjectMeta{Name: cappName},
			Spec:       cappv1alpha1.CappSpec{ScaleSpec: cappv1alpha1.ScaleSpec{Metric: kautoscaling.CPU, MinReplicas: ptr.To(int32(5))}},
		}
		got := setAutoScaler(capp, defaults)
		require.Equal(t, "5", got[kautoscaling.MinScaleAnnotationKey])
		require.NotContains(t, got, kautoscaling.ActivationScaleKey)
	})

	t.Run("omits min-scale and sets activation-scale from config when MinReplicas is unset", func(t *testing.T) {
		capp := cappv1alpha1.Capp{
			ObjectMeta: metav1.ObjectMeta{Name: cappName},
			Spec:       cappv1alpha1.CappSpec{ScaleSpec: cappv1alpha1.ScaleSpec{Metric: kautoscaling.CPU}},
		}
		got := setAutoScaler(capp, defaults)
		require.NotContains(t, got, kautoscaling.MinScaleAnnotationKey)
		require.Equal(t, "3", got[kautoscaling.ActivationScaleKey])
	})

	t.Run("sets max-scale annotation when MaxReplicas is set", func(t *testing.T) {
		capp := cappv1alpha1.Capp{
			ObjectMeta: metav1.ObjectMeta{Name: cappName},
			Spec:       cappv1alpha1.CappSpec{ScaleSpec: cappv1alpha1.ScaleSpec{Metric: kautoscaling.CPU, MaxReplicas: ptr.To(int32(10))}},
		}
		got := setAutoScaler(capp, defaults)
		require.Equal(t, "10", got[kautoscaling.MaxScaleAnnotationKey])
		require.Equal(t, "3", got[kautoscaling.ActivationScaleKey])
	})

	t.Run("returns empty map when metric is empty", func(t *testing.T) {
		capp := cappv1alpha1.Capp{
			ObjectMeta: metav1.ObjectMeta{Name: cappName},
			Spec:       cappv1alpha1.CappSpec{ScaleSpec: cappv1alpha1.ScaleSpec{Metric: ""}},
		}
		got := setAutoScaler(capp, defaults)
		require.Empty(t, got)
	})
}

func TestPrepareResourceTimeoutSeconds(t *testing.T) {
	ctx := context.Background()
	km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))

	tests := map[string]struct {
		routeTimeout *int64
		want         *int64
	}{
		"unset stays nil so the webhook applies the cluster default": {routeTimeout: nil, want: nil},
		"zero stays nil because Knative treats it as unset":          {routeTimeout: ptr.To[int64](0), want: nil},
		"set value is kept": {routeTimeout: ptr.To[int64](60), want: ptr.To[int64](60)},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			capp := newKsvcCapp()
			capp.Spec.RouteSpec.RouteTimeoutSeconds = test.routeTimeout
			ksvc := km.prepareResource(capp, ctx)
			require.Equal(t, test.want, ksvc.Spec.Template.Spec.TimeoutSeconds)
		})
	}
}

func TestKnativeServiceManagerTemplateApplied(t *testing.T) {
	ctx := context.Background()
	km, _ := newKsvcManager(newFakeClient(newKsvcScheme()))
	capp := newKsvcCapp()

	t.Run("true when the Knative Service holds the desired template", func(t *testing.T) {
		ksvc := km.prepareResource(capp, ctx)
		require.True(t, km.TemplateApplied(ctx, capp, &ksvc))
	})

	t.Run("false when the image differs from the desired template", func(t *testing.T) {
		ksvc := km.prepareResource(capp, ctx)
		ksvc.Spec.Template.Spec.Containers[0].Image = "example.com/app:v0"
		require.False(t, km.TemplateApplied(ctx, capp, &ksvc))
	})

	t.Run("true when only the traffic differs from the desired traffic", func(t *testing.T) {
		ksvc := km.prepareResource(capp, ctx)
		ksvc.Spec.Traffic = []knativev1.TrafficTarget{{RevisionName: "rev-1", Percent: ptr.To(int64(50))}}
		require.True(t, km.TemplateApplied(ctx, capp, &ksvc))
	})

	t.Run("against a Knative Service defaulted by the webhook", func(t *testing.T) {
		base := newKsvcCapp()
		base.Annotations = map[string]string{"a": "1"}
		base.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env = []corev1.EnvVar{{Name: "A", Value: "1"}}
		cfg := newWebhookConfig(ctx, t)

		cases := []struct {
			name        string
			storedCapp  func(*cappv1alpha1.Capp)
			desiredCapp func(*cappv1alpha1.Capp)
			storedKsvc  func(*knativev1.Service)
			want        bool
		}{
			{
				name: "true when no timeout is set on the Capp",
				want: true,
			},
			{
				name:        "true when routeTimeoutSeconds is 600 on both sides",
				storedCapp:  func(c *cappv1alpha1.Capp) { c.Spec.RouteSpec.RouteTimeoutSeconds = ptr.To(int64(600)) },
				desiredCapp: func(c *cappv1alpha1.Capp) { c.Spec.RouteSpec.RouteTimeoutSeconds = ptr.To(int64(600)) },
				want:        true,
			},
			{
				name:        "true when routeTimeoutSeconds is 0 on both sides",
				storedCapp:  func(c *cappv1alpha1.Capp) { c.Spec.RouteSpec.RouteTimeoutSeconds = ptr.To(int64(0)) },
				desiredCapp: func(c *cappv1alpha1.Capp) { c.Spec.RouteSpec.RouteTimeoutSeconds = ptr.To(int64(0)) },
				want:        true,
			},
			{
				name: "true when the webhook adds a template annotation",
				storedKsvc: func(s *knativev1.Service) {
					s.Spec.Template.Annotations["webhook"] = "x"
				},
				want: true,
			},
			{
				name: "true when the webhook appends a sidecar",
				storedKsvc: func(s *knativev1.Service) {
					s.Spec.Template.Spec.Containers = append(s.Spec.Template.Spec.Containers, corev1.Container{Name: "sidecar", Image: "example.com/sidecar:v1"})
				},
				want: true,
			},
			{
				name: "false when the image differs",
				desiredCapp: func(c *cappv1alpha1.Capp) {
					c.Spec.ConfigurationSpec.Template.Spec.Containers[0].Image = "example.com/app:v2"
				},
				want: false,
			},
			{
				name: "false when an env value differs",
				desiredCapp: func(c *cappv1alpha1.Capp) {
					c.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env[0].Value = "2"
				},
				want: false,
			},
			{
				name: "false when an env var is added to the desired template",
				desiredCapp: func(c *cappv1alpha1.Capp) {
					c.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env = append(c.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env, corev1.EnvVar{Name: "B", Value: "2"})
				},
				want: false,
			},
			{
				name: "false when an annotation is added to the desired template",
				desiredCapp: func(c *cappv1alpha1.Capp) {
					c.Annotations["b"] = "2"
				},
				want: false,
			},
			{
				name:        "false when routeTimeoutSeconds differs",
				desiredCapp: func(c *cappv1alpha1.Capp) { c.Spec.RouteSpec.RouteTimeoutSeconds = ptr.To(int64(600)) },
				want:        false,
			},
			{
				name: "known gap: true when the last env var is removed from the desired template",
				desiredCapp: func(c *cappv1alpha1.Capp) {
					c.Spec.ConfigurationSpec.Template.Spec.Containers[0].Env = nil
				},
				want: true,
			},
			{
				name: "known gap: true when an annotation is removed from the desired template",
				desiredCapp: func(c *cappv1alpha1.Capp) {
					c.Annotations = nil
				},
				want: true,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				storedCapp := base.DeepCopy()
				if tc.storedCapp != nil {
					tc.storedCapp(storedCapp)
				}
				stored := storeLikeWebhook(ctx, t, km, *storedCapp, cfg)
				if tc.storedKsvc != nil {
					tc.storedKsvc(&stored)
				}

				desiredCapp := base.DeepCopy()
				if tc.desiredCapp != nil {
					tc.desiredCapp(desiredCapp)
				}
				require.Equal(t, tc.want, km.TemplateApplied(ctx, *desiredCapp, &stored))
			})
		}
	})
}

// newWebhookConfig returns a Knative config with non-default cluster values that the Knative webhook applies when it
// defaults a Service on create.
func newWebhookConfig(ctx context.Context, t *testing.T) *config.Config {
	t.Helper()
	cfg := config.FromContextOrDefaults(ctx)
	defaults, err := config.NewDefaultsConfigFromMap(map[string]string{"revision-cpu-request": "100m"})
	require.NoError(t, err)
	features, err := config.NewFeaturesConfigFromMap(map[string]string{"secure-pod-defaults": "enabled"})
	require.NoError(t, err)
	cfg.Defaults = defaults
	cfg.Features = features
	return cfg
}

// storeLikeWebhook returns the Knative Service the cluster would store for the Capp: the prepared resource is
// round-tripped through JSON and defaulted by the real Knative Service defaulting on create.
func storeLikeWebhook(ctx context.Context, t *testing.T, km KnativeServiceManager, capp cappv1alpha1.Capp, cfg *config.Config) knativev1.Service {
	t.Helper()
	desired := km.prepareResource(capp, ctx)
	raw, err := json.Marshal(desired)
	require.NoError(t, err)
	var stored knativev1.Service
	require.NoError(t, json.Unmarshal(raw, &stored))
	stored.SetDefaults(config.ToContext(apis.WithinCreate(ctx), cfg))
	return stored
}
