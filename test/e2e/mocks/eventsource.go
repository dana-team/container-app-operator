package mocks

import (
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e/consts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
)

// CreatePingSourceObject returns a bare PingSource for existence/get checks.
func CreatePingSourceObject(name string) *sourcesv1.PingSource {
	return &sourcesv1.PingSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: consts.NSName,
		},
	}
}

// AddPingSource appends a PingSource entry to a Capp's EventSourcesSpec.
func AddPingSource(capp *cappv1alpha1.Capp, sourceName, schedule, data string) {
	capp.Spec.EventSourcesSpec.Sources = append(capp.Spec.EventSourcesSpec.Sources, cappv1alpha1.EventSource{
		Name: sourceName,
		PingSource: &cappv1alpha1.PingSourceSpec{
			Schedule: schedule,
			Data:     data,
		},
	})
}
