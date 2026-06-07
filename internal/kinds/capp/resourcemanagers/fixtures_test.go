package resourcemanagers

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

const (
	cappName      = "my-capp"
	cappNamespace = "my-ns"
	testCappUID   = types.UID("test-uid")
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	return s
}

func newBaseCapp() cappv1alpha1.Capp {
	return cappv1alpha1.Capp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappName,
			Namespace: cappNamespace,
			UID:       testCappUID,
		},
	}
}

func newCappConfig() *cappv1alpha1.CappConfig {
	return &cappv1alpha1.CappConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.CappConfigName,
			Namespace: utils.CappNS,
		},
		Spec: cappv1alpha1.CappConfigSpec{
			AutoscaleConfig: cappv1alpha1.AutoscaleConfig{
				RPS:             200,
				CPU:             80,
				Memory:          70,
				Concurrency:     10,
				ActivationScale: 3,
			},
		},
	}
}
