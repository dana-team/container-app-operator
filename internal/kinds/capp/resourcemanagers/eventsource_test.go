package resourcemanagers

import (
	"context"
	"fmt"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/sources"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type mockKind struct {
	generateFunc func(cappv1alpha1.Capp, cappv1alpha1.SourceConfiguration) client.Object
	listResult   []client.Object
	listErr      error
	statusResult []cappv1alpha1.EventSourceStatus
	statusErr    error
}

func (m *mockKind) Generate(capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) client.Object {
	if m.generateFunc != nil {
		return m.generateFunc(capp, source)
	}
	return nil
}
func (m *mockKind) CreateOrUpdate(_ context.Context, _ rclient.ResourceManagerClient, _ logr.Logger, _ cappv1alpha1.Capp, _ cappv1alpha1.SourceConfiguration) error {
	return nil
}
func (m *mockKind) List(_ context.Context, _ rclient.ResourceManagerClient, _ logr.Logger, _ cappv1alpha1.Capp) ([]client.Object, error) {
	return m.listResult, m.listErr
}
func (m *mockKind) Delete(_ context.Context, _ rclient.ResourceManagerClient, _ logr.Logger, _ cappv1alpha1.Capp, _ cappv1alpha1.SourceConfiguration) error {
	return nil
}
func (m *mockKind) GetStatus(_ context.Context, _ rclient.ResourceManagerClient, _ logr.Logger, _ cappv1alpha1.Capp) ([]cappv1alpha1.EventSourceStatus, error) {
	return m.statusResult, m.statusErr
}
func (m *mockKind) Validate(_ cappv1alpha1.Capp, _ cappv1alpha1.SourceConfiguration) error {
	return nil
}

func newEventSourceManagerScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	utilruntime.Must(corev1.AddToScheme(s))
	return s
}

func newEventSourceManager(k8sClient client.Client) EventSourceManager {
	return EventSourceManager{
		Ctx:       context.Background(),
		K8sclient: k8sClient,
		Log:       logr.Discard(),
	}
}

func newManagedCapp(cappSources []cappv1alpha1.SourceConfiguration) cappv1alpha1.Capp {
	return cappv1alpha1.Capp{
		ObjectMeta: metav1.ObjectMeta{Name: "my-capp", Namespace: "my-ns"},
		Spec: cappv1alpha1.CappSpec{
			EventSourcesSpec: cappv1alpha1.EventSourcesSpec{Sources: cappSources},
		},
	}
}

func TestIsRequired(t *testing.T) {
	tests := []struct {
		name     string
		sources  []cappv1alpha1.SourceConfiguration
		expected bool
	}{
		{
			name:     "returns true when sources are declared",
			sources:  []cappv1alpha1.SourceConfiguration{{Name: "ping"}},
			expected: true,
		},
		{
			name:     "returns false when no sources are declared",
			sources:  nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			em := newEventSourceManager(fake.NewClientBuilder().WithScheme(newEventSourceManagerScheme()).Build())
			capp := newManagedCapp(tt.sources)
			assert.Equal(t, tt.expected, em.IsRequired(capp))
		})
	}
}

func TestManage(t *testing.T) {
	tests := []struct {
		name        string
		cappSources []cappv1alpha1.SourceConfiguration
		ownedObjs   []client.Object
		listErr     error
		expectError bool
	}{
		{
			name:        "returns nil when sources are declared",
			cappSources: []cappv1alpha1.SourceConfiguration{{Name: "ping"}},
		},
		{
			name: "cleans up owned resources when no sources declared",
			ownedObjs: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "owned-cm", Namespace: "my-ns"}},
			},
		},
		{
			name:        "returns error when cleanup fails",
			listErr:     fmt.Errorf("list error"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fakeClient := fake.NewClientBuilder().WithScheme(newEventSourceManagerScheme()).Build()

			for _, obj := range tt.ownedObjs {
				assert.NoError(t, fakeClient.Create(ctx, obj))
			}

			mock := &mockKind{listResult: tt.ownedObjs, listErr: tt.listErr}
			sources.Register("TestManage", mock)
			defer sources.Register("TestManage", nil)

			em := newEventSourceManager(fakeClient)
			err := em.Manage(newManagedCapp(tt.cappSources))

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				for _, obj := range tt.ownedObjs {
					got := &corev1.ConfigMap{}
					getErr := fakeClient.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, got)
					assert.True(t, apierrors.IsNotFound(getErr))
				}
			}
		})
	}
}

func TestCleanUp(t *testing.T) {
	tests := []struct {
		name        string
		ownedObjs   []client.Object
		listErr     error
		expectError bool
	}{
		{
			name: "deletes all owned resources",
			ownedObjs: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm-1", Namespace: "my-ns"}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm-2", Namespace: "my-ns"}},
			},
		},
		{
			name:      "returns no error when no owned resources",
			ownedObjs: nil,
		},
		{
			name:        "returns error when listing fails",
			listErr:     fmt.Errorf("list error"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fakeClient := fake.NewClientBuilder().WithScheme(newEventSourceManagerScheme()).Build()

			for _, obj := range tt.ownedObjs {
				assert.NoError(t, fakeClient.Create(ctx, obj))
			}

			mock := &mockKind{listResult: tt.ownedObjs, listErr: tt.listErr}
			sources.Register("TestCleanUp", mock)
			defer sources.Register("TestCleanUp", nil)

			em := newEventSourceManager(fakeClient)
			err := em.CleanUp(newManagedCapp(nil))

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				for _, obj := range tt.ownedObjs {
					got := &corev1.ConfigMap{}
					getErr := fakeClient.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, got)
					assert.True(t, apierrors.IsNotFound(getErr))
				}
			}
		})
	}
}

func TestCleanUpOrphans(t *testing.T) {
	pingConfig := &cappv1alpha1.PingSourceConfiguration{Schedule: "* * * * *"}
	generateBySourceName := func(capp cappv1alpha1.Capp, source cappv1alpha1.SourceConfiguration) client.Object {
		return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", capp.Name, source.Name),
			Namespace: capp.Namespace,
		}}
	}

	tests := []struct {
		name          string
		cappSources   []cappv1alpha1.SourceConfiguration
		ownedObjs     []client.Object
		listErr       error
		expectError   bool
		expectDeleted []string
		expectKept    []string
	}{
		{
			name: "deletes orphaned resource not present in spec",
			cappSources: []cappv1alpha1.SourceConfiguration{
				{Name: "ping-a", PingSourceConfiguration: pingConfig},
			},
			ownedObjs: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-capp-ping-a", Namespace: "my-ns"}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-capp-ping-b", Namespace: "my-ns"}},
			},
			expectKept:    []string{"my-capp-ping-a"},
			expectDeleted: []string{"my-capp-ping-b"},
		},
		{
			name: "keeps all owned when all are in spec",
			cappSources: []cappv1alpha1.SourceConfiguration{
				{Name: "ping-a", PingSourceConfiguration: pingConfig},
				{Name: "ping-b", PingSourceConfiguration: pingConfig},
			},
			ownedObjs: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-capp-ping-a", Namespace: "my-ns"}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-capp-ping-b", Namespace: "my-ns"}},
			},
			expectKept: []string{"my-capp-ping-a", "my-capp-ping-b"},
		},
		{
			name: "deletes all owned when none are in spec",
			cappSources: []cappv1alpha1.SourceConfiguration{
				{Name: "ping-a", PingSourceConfiguration: pingConfig},
			},
			ownedObjs: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-capp-ping-b", Namespace: "my-ns"}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-capp-ping-c", Namespace: "my-ns"}},
			},
			expectDeleted: []string{"my-capp-ping-b", "my-capp-ping-c"},
		},
		{
			name: "returns error when listing fails",
			cappSources: []cappv1alpha1.SourceConfiguration{
				{Name: "ping-a", PingSourceConfiguration: pingConfig},
			},
			listErr:     fmt.Errorf("list error"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fakeClient := fake.NewClientBuilder().WithScheme(newEventSourceManagerScheme()).Build()
			for _, obj := range tt.ownedObjs {
				assert.NoError(t, fakeClient.Create(ctx, obj))
			}

			mock := &mockKind{
				generateFunc: generateBySourceName,
				listResult:   tt.ownedObjs,
				listErr:      tt.listErr,
			}
			sources.Register("PingSource", mock)
			defer sources.Register("PingSource", nil)

			em := newEventSourceManager(fakeClient)
			err := em.cleanUpOrphans(newManagedCapp(tt.cappSources))

			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			for _, name := range tt.expectKept {
				got := &corev1.ConfigMap{}
				assert.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "my-ns"}, got))
			}
			for _, name := range tt.expectDeleted {
				got := &corev1.ConfigMap{}
				getErr := fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "my-ns"}, got)
				assert.True(t, apierrors.IsNotFound(getErr), "expected %q to be deleted", name)
			}
		})
	}
}

func TestGetStatus(t *testing.T) {
	tests := []struct {
		name          string
		statusResult  []cappv1alpha1.EventSourceStatus
		statusErr     error
		expectedNames []string
		expectError   bool
	}{
		{
			name: "returns statuses sorted by name",
			statusResult: []cappv1alpha1.EventSourceStatus{
				{Name: "c-source"},
				{Name: "a-source"},
				{Name: "b-source"},
			},
			expectedNames: []string{"a-source", "b-source", "c-source"},
		},
		{
			name:        "returns error when kind returns error",
			statusErr:   fmt.Errorf("status error"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(newEventSourceManagerScheme()).Build()

			mock := &mockKind{statusResult: tt.statusResult, statusErr: tt.statusErr}
			sources.Register("TestGetStatus", mock)
			defer sources.Register("TestGetStatus", nil)

			em := newEventSourceManager(fakeClient)
			result, err := em.GetStatus(newManagedCapp(nil))

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result.EventSources, len(tt.expectedNames))
				for i, name := range tt.expectedNames {
					assert.Equal(t, name, result.EventSources[i].Name)
				}
			}
		})
	}
}
