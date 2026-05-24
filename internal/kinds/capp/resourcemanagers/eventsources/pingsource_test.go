package eventsources

import (
	"context"
	"fmt"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newPingSourceScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	utilruntime.Must(sourcesv1.AddToScheme(s))
	utilruntime.Must(servingv1.AddToScheme(s))
	return s
}

func newPingSourceRM() rclient.ResourceManagerClient {
	return rclient.ResourceManagerClient{
		K8sclient: fake.NewClientBuilder().WithScheme(newPingSourceScheme()).Build(),
	}
}

func newTestCapp(name, namespace string) cappv1alpha1.Capp {
	return cappv1alpha1.Capp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("test-uid"),
		},
	}
}

func newTestSource(name, schedule, data string) cappv1alpha1.SourceConfiguration {
	return cappv1alpha1.SourceConfiguration{
		Name: name,
		PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
			Schedule: schedule,
			Data:     data,
		},
	}
}

func TestGenerate(t *testing.T) {
	tests := []struct {
		name          string
		cappName      string
		cappNamespace string
		sourceName    string
		schedule      string
		data          string
	}{
		{
			name:          "generates PingSource with correct name and spec",
			cappName:      "my-capp",
			cappNamespace: "my-ns",
			sourceName:    "ping",
			schedule:      "* * * * *",
			data:          `{"key":"value"}`,
		},
		{
			name:          "generates PingSource with empty schedule and data",
			cappName:      "capp",
			cappNamespace: "default",
			sourceName:    "src",
			schedule:      "",
			data:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &PingSourceKind{}
			capp := newTestCapp(tt.cappName, tt.cappNamespace)
			source := newTestSource(tt.sourceName, tt.schedule, tt.data)

			obj := k.Generate(capp, source)
			ps, ok := obj.(*sourcesv1.PingSource)
			assert.True(t, ok)
			assert.Equal(t, fmt.Sprintf("%s-%s", tt.cappName, tt.sourceName), ps.Name)
			assert.Equal(t, tt.cappNamespace, ps.Namespace)
			assert.Equal(t, tt.schedule, ps.Spec.Schedule)
			assert.Equal(t, tt.data, ps.Spec.Data)
			assert.Equal(t, tt.cappName, ps.Spec.Sink.Ref.Name)
			assert.Equal(t, tt.cappNamespace, ps.Spec.Sink.Ref.Namespace)
			assert.Equal(t, "Service", ps.Spec.Sink.Ref.Kind)
			assert.Equal(t, servingv1.SchemeGroupVersion.String(), ps.Spec.Sink.Ref.APIVersion)
		})
	}
}

func TestCreateOrUpdate(t *testing.T) {
	tests := []struct {
		name         string
		preCreate    bool
		preData      string
		data         string
		expectedData string
	}{
		{
			name:         "creates PingSource when not found",
			data:         "data",
			expectedData: "data",
		},
		{
			name:         "updates PingSource when spec differs",
			preCreate:    true,
			preData:      "old-data",
			data:         "new-data",
			expectedData: "new-data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k := &PingSourceKind{}
			capp := newTestCapp("my-capp", "my-ns")
			rm := newPingSourceRM()

			if tt.preCreate {
				assert.NoError(t, k.CreateOrUpdate(ctx, rm, logr.Discard(), capp, newTestSource("ping", "* * * * *", tt.preData)))
			}

			assert.NoError(t, k.CreateOrUpdate(ctx, rm, logr.Discard(), capp, newTestSource("ping", "* * * * *", tt.data)))
			got := &sourcesv1.PingSource{}
			assert.NoError(t, rm.K8sclient.Get(ctx, types.NamespacedName{Name: "my-capp-ping", Namespace: "my-ns"}, got))
			assert.Equal(t, tt.expectedData, got.Spec.Data)
		})
	}
}

func TestCreateOrUpdate_ReconcilesOwnerRef(t *testing.T) {
	ctx := context.Background()
	k := &PingSourceKind{}
	capp := newTestCapp("my-capp", "my-ns")
	source := newTestSource("ping", "* * * * *", "data")
	rm := newPingSourceRM()

	// Pre-create without going through CreateOrUpdate (no owner ref)
	existing := &sourcesv1.PingSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-capp-ping",
			Namespace: "my-ns",
		},
	}
	assert.NoError(t, rm.K8sclient.Create(ctx, existing))

	// CreateOrUpdate should reconcile the missing owner ref
	assert.NoError(t, k.CreateOrUpdate(ctx, rm, logr.Discard(), capp, source))

	got := &sourcesv1.PingSource{}
	assert.NoError(t, rm.K8sclient.Get(ctx, types.NamespacedName{Name: "my-capp-ping", Namespace: "my-ns"}, got))
	assert.Len(t, got.OwnerReferences, 1)
	assert.Equal(t, capp.Name, got.OwnerReferences[0].Name)
}

func TestList(t *testing.T) {
	ctx := context.Background()
	k := &PingSourceKind{}
	capp := newTestCapp("my-capp", "my-ns")
	source := newTestSource("ping", "* * * * *", "data")
	rm := newPingSourceRM()

	assert.NoError(t, k.CreateOrUpdate(ctx, rm, logr.Discard(), capp, source))

	unrelated := &sourcesv1.PingSource{
		ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: "my-ns"},
	}
	assert.NoError(t, rm.K8sclient.Create(ctx, unrelated))

	result, err := k.List(ctx, rm, logr.Discard(), capp)
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "my-capp-ping", result[0].GetName())
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name      string
		preCreate bool
	}{
		{
			name:      "deletes existing PingSource",
			preCreate: true,
		},
		{
			name:      "returns no error when PingSource not found",
			preCreate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k := &PingSourceKind{}
			capp := newTestCapp("my-capp", "my-ns")
			source := newTestSource("ping", "* * * * *", "data")
			rm := newPingSourceRM()

			if tt.preCreate {
				assert.NoError(t, k.CreateOrUpdate(ctx, rm, logr.Discard(), capp, source))
			}

			assert.NoError(t, k.Delete(ctx, rm, logr.Discard(), capp, source))

			deleted := &sourcesv1.PingSource{}
			err := rm.K8sclient.Get(ctx, types.NamespacedName{Name: "my-capp-ping", Namespace: "my-ns"}, deleted)
			assert.True(t, errors.IsNotFound(err))
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		source    cappv1alpha1.SourceConfiguration
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "nil PingSourceConfiguration returns error",
			source:    cappv1alpha1.SourceConfiguration{Name: "ping"},
			wantErr:   true,
			errSubstr: "pingSourceConfiguration",
		},
		{
			name: "invalid cron schedule returns error",
			source: cappv1alpha1.SourceConfiguration{
				Name: "ping",
				PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
					Schedule: "not-a-cron",
				},
			},
			wantErr:   true,
			errSubstr: "schedule",
		},
		{
			name: "invalid JSON in data returns error",
			source: cappv1alpha1.SourceConfiguration{
				Name: "ping",
				PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
					Schedule: "* * * * *",
					Data:     "not-json{",
				},
			},
			wantErr:   true,
			errSubstr: "data",
		},
		{
			name: "valid schedule with valid JSON passes",
			source: cappv1alpha1.SourceConfiguration{
				Name: "ping",
				PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
					Schedule: "*/5 * * * *",
					Data:     `{"key":"value"}`,
				},
			},
			wantErr: false,
		},
		{
			name: "valid schedule with empty data passes",
			source: cappv1alpha1.SourceConfiguration{
				Name: "ping",
				PingSourceConfiguration: &cappv1alpha1.PingSourceConfiguration{
					Schedule: "0 12 * * *",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &PingSourceKind{}
			err := k.Validate(newTestCapp("my-capp", "my-ns"), tt.source)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetStatus(t *testing.T) {
	ctx := context.Background()
	k := &PingSourceKind{}
	capp := newTestCapp("my-capp", "my-ns")
	source := newTestSource("ping", "* * * * *", "data")
	rm := newPingSourceRM()

	assert.NoError(t, k.CreateOrUpdate(ctx, rm, logr.Discard(), capp, source))

	statuses, err := k.GetStatus(ctx, rm, logr.Discard(), capp)
	assert.NoError(t, err)
	assert.Len(t, statuses, 1)
	assert.Equal(t, "my-capp-ping", statuses[0].Name)
	assert.Equal(t, metav1.ConditionUnknown, statuses[0].Condition.Status)
	assert.Equal(t, "Source readiness not known", statuses[0].Condition.Message)
}
