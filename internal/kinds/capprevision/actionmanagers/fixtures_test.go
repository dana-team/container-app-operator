package actionmanagers

import (
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	cappName      = "my-capp"
	cappNamespace = "my-ns"
	testCappUID   = types.UID("test-uid")

	stateDisabled      = "disabled"
	labelTeamKey       = "team"
	labelTeamPlatform  = "platform"
	labelTeamInfra     = "infra"
	annotationNoteKey  = "note"
	annotationNoteHi   = "hello"
	annotationUserA    = "user-a"
	annotationUserB    = "user-b"
	annotationKeyKey   = "key"
	annotationKeyVal   = "val"
	annotationKeyValA  = "val-a"
	annotationKeyValB  = "val-b"
	annotationOtherKey = "other"
	annotationSameVal  = "same"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	return s
}

func newFakeClient(objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(newScheme()).
		WithObjects(objects...).
		Build()
}

func newBaseCapp() cappv1alpha1.Capp {
	return cappv1alpha1.Capp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: cappNamespace,
			UID:       testCappUID,
		},
		Spec: cappv1alpha1.CappSpec{
			ConfigurationSpec: knativev1.ConfigurationSpec{
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
			},
		},
	}
}

func newCappConfig(revisionHistoryLimit int) *cappv1alpha1.CappConfig {
	return &cappv1alpha1.CappConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.CappConfigName,
			Namespace: utils.CappNS,
		},
		Spec: cappv1alpha1.CappConfigSpec{
			RevisionHistoryLimit: revisionHistoryLimit,
		},
	}
}

func newCappRevision(name string, revisionNumber int, capp cappv1alpha1.Capp, creationTime time.Time) *cappv1alpha1.CappRevision {
	return &cappv1alpha1.CappRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         capp.Namespace,
			CreationTimestamp: metav1.NewTime(creationTime),
			Labels: map[string]string{
				cappv1alpha1.GroupVersion.Group + "/cappName": capp.Name,
			},
		},
		Spec: cappv1alpha1.CappRevisionSpec{
			RevisionNumber: revisionNumber,
			CappTemplate: cappv1alpha1.CappTemplate{
				Spec:        *capp.Spec.DeepCopy(),
				Labels:      capp.Labels,
				Annotations: capp.Annotations,
			},
		},
	}
}
