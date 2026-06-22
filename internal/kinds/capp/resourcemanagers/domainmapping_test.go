package resourcemanagers

import (
	"context"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func newDomainMappingScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(knativev1beta1.AddToScheme(s))
	utilruntime.Must(knativev1.AddToScheme(s))
	return s
}

func newDomainMappingManager(k8sClient client.Client) (KnativeDomainMappingManager, *events.FakeRecorder) {
	recorder := events.NewFakeRecorder(10)
	return KnativeDomainMappingManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         recorder,
	}, recorder
}

func newDomainMappingClient(objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(newDomainMappingScheme()).
		WithObjects(objects...).
		Build()
}

func expectedDMResourceName() string {
	return utils.GenerateResourceName(testHostname, testZone)
}

func expectedDMSecretName() string {
	return utils.GenerateSecretName(expectedDMResourceName())
}

func newExistingDomainMapping(capp *cappv1alpha1.Capp) *knativev1beta1.DomainMapping {
	resourceName := expectedDMResourceName()
	return &knativev1beta1.DomainMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: capp.Namespace,
			Labels:    utils.ManagedResourceLabels(capp.Name),
		},
		Spec: knativev1beta1.DomainMappingSpec{
			Ref: newKnativeServiceRef(capp.Name),
		},
	}
}

func newKnativeServiceRef(name string) duckv1.KReference {
	return duckv1.KReference{
		APIVersion: knativev1.SchemeGroupVersion.String(),
		Name:       name,
		Kind:       knativeServiceKind,
	}
}

func newTLSSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      expectedDMSecretName(),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("cert"),
			"tls.key": []byte("key"),
		},
	}
}

func TestDomainMappingManagerPrepareResource(t *testing.T) {
	ctx := context.Background()

	t.Run("generates correct domain mapping without TLS", func(t *testing.T) {
		dm, _ := newDomainMappingManager(newDomainMappingClient(newCappConfigWithDNS()))
		capp := newCappWithHostname()

		got, err := dm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.Equal(t, expectedDMResourceName(), got.Name)
		require.Equal(t, cappNamespace, got.Namespace)
		require.Equal(t, cappName, got.Spec.Ref.Name)
		require.Equal(t, knativeServiceKind, got.Spec.Ref.Kind)
		require.Nil(t, got.Spec.TLS)
	})

	t.Run("sets TLS when secret exists", func(t *testing.T) {
		secret := newTLSSecret(cappNamespace)
		dm, _ := newDomainMappingManager(newDomainMappingClient(newCappConfigWithDNS(), secret))
		capp := newCappWithTLS()

		got, err := dm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.NotNil(t, got.Spec.TLS)
		require.Equal(t, expectedDMSecretName(), got.Spec.TLS.SecretName)
	})

	t.Run("proceeds without TLS when secret does not exist", func(t *testing.T) {
		dm, _ := newDomainMappingManager(newDomainMappingClient(newCappConfigWithDNS()))
		capp := newCappWithTLS()

		got, err := dm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.Nil(t, got.Spec.TLS)
	})

	t.Run("skips TLS when TLS is disabled", func(t *testing.T) {
		secret := newTLSSecret(cappNamespace)
		dm, _ := newDomainMappingManager(newDomainMappingClient(newCappConfigWithDNS(), secret))
		capp := newCappWithHostname()

		got, err := dm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.Nil(t, got.Spec.TLS)
	})

	t.Run("sets managed resource labels", func(t *testing.T) {
		dm, _ := newDomainMappingManager(newDomainMappingClient(newCappConfigWithDNS()))
		capp := newCappWithHostname()

		got, err := dm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.Equal(t, cappName, got.Labels[utils.CappResourceKey])
		require.Equal(t, utils.CappKey, got.Labels[utils.ManagedByLabelKey])
	})

	t.Run("returns error when capp config is missing", func(t *testing.T) {
		dm, _ := newDomainMappingManager(newDomainMappingClient())
		capp := newCappWithHostname()

		_, err := dm.prepareResource(ctx, capp)

		require.Error(t, err)
	})
}

func TestDomainMappingManagerIsRequired(t *testing.T) {
	dm, _ := newDomainMappingManager(newDomainMappingClient())

	t.Run("returns true when hostname is set", func(t *testing.T) {
		capp := newCappWithHostname()
		require.True(t, dm.IsRequired(capp))
	})

	t.Run("returns false when hostname is empty", func(t *testing.T) {
		capp := newBaseCapp()
		require.False(t, dm.IsRequired(capp))
	})
}

func TestDomainMappingManagerCreateOrUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("creates domain mapping when not found", func(t *testing.T) {
		dm, recorder := newDomainMappingManager(newDomainMappingClient(newCappConfigWithDNS()))
		capp := newCappWithHostname()

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDMResourceName(), Namespace: cappNamespace}, got))
		require.Len(t, got.OwnerReferences, 1)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
		require.Contains(t, <-recorder.Events, eventCappDomainMappingCreated)
	})

	t.Run("updates domain mapping when spec differs", func(t *testing.T) {
		capp := newCappWithHostname()
		existing := newExistingDomainMapping(&capp)
		existing.Spec.Ref.Name = "old-capp"
		require.NoError(t, controllerutil.SetOwnerReference(&capp, existing, newDomainMappingScheme()))

		dm, _ := newDomainMappingManager(newDomainMappingClient(newCappConfigWithDNS(), existing))

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDMResourceName(), Namespace: cappNamespace}, got))
		require.Equal(t, cappName, got.Spec.Ref.Name)
	})

	t.Run("skips update when spec is unchanged", func(t *testing.T) {
		capp := newCappWithHostname()
		existing := newExistingDomainMapping(&capp)
		require.NoError(t, controllerutil.SetOwnerReference(&capp, existing, newDomainMappingScheme()))

		dm, _ := newDomainMappingManager(newDomainMappingClient(newCappConfigWithDNS(), existing))

		before := &knativev1beta1.DomainMapping{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDMResourceName(), Namespace: cappNamespace}, before))
		beforeRV := before.ResourceVersion

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		after := &knativev1beta1.DomainMapping{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDMResourceName(), Namespace: cappNamespace}, after))
		require.Equal(t, beforeRV, after.ResourceVersion)
	})
}

func TestDomainMappingManagerManage(t *testing.T) {
	ctx := context.Background()

	t.Run("creates domain mapping when hostname is set", func(t *testing.T) {
		dm, _ := newDomainMappingManager(newDomainMappingClient(newCappConfigWithDNS()))
		capp := newCappWithHostname()

		require.NoError(t, dm.Manage(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDMResourceName(), Namespace: cappNamespace}, got))
	})

	t.Run("cleans up when hostname is empty", func(t *testing.T) {
		capp := newBaseCapp()
		existing := newExistingDomainMapping(&capp)

		dm, _ := newDomainMappingManager(newDomainMappingClient(existing))

		require.NoError(t, dm.Manage(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		err := dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDMResourceName(), Namespace: cappNamespace}, got)
		require.True(t, client.IgnoreNotFound(err) == nil && err != nil)
	})
}

func TestDomainMappingManagerCleanUp(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes domain mapping and associated TLS secret", func(t *testing.T) {
		capp := newCappWithHostname()
		existing := newExistingDomainMapping(&capp)
		secret := newTLSSecret(cappNamespace)

		dm, _ := newDomainMappingManager(newDomainMappingClient(existing, secret))

		require.NoError(t, dm.CleanUp(ctx, capp))

		gotDM := &knativev1beta1.DomainMapping{}
		err := dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDMResourceName(), Namespace: cappNamespace}, gotDM)
		require.True(t, client.IgnoreNotFound(err) == nil && err != nil)

		gotSecret := &corev1.Secret{}
		err = dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDMSecretName(), Namespace: cappNamespace}, gotSecret)
		require.True(t, client.IgnoreNotFound(err) == nil && err != nil)
	})

	t.Run("deletes domain mapping when no TLS secret exists", func(t *testing.T) {
		capp := newCappWithHostname()
		existing := newExistingDomainMapping(&capp)

		dm, _ := newDomainMappingManager(newDomainMappingClient(existing))

		require.NoError(t, dm.CleanUp(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		err := dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDMResourceName(), Namespace: cappNamespace}, got)
		require.True(t, client.IgnoreNotFound(err) == nil && err != nil)
	})

	t.Run("skips owned domain mapping when capp is deleting", func(t *testing.T) {
		capp := newCappWithHostname()
		now := metav1.Now()
		capp.DeletionTimestamp = &now

		existing := newExistingDomainMapping(&capp)
		require.NoError(t, controllerutil.SetOwnerReference(&capp, existing, newDomainMappingScheme()))

		dm, _ := newDomainMappingManager(newDomainMappingClient(existing))

		require.NoError(t, dm.CleanUp(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDMResourceName(), Namespace: cappNamespace}, got))
	})

	t.Run("succeeds when no domain mappings exist", func(t *testing.T) {
		dm, _ := newDomainMappingManager(newDomainMappingClient())
		capp := newCappWithHostname()

		require.NoError(t, dm.CleanUp(ctx, capp))
	})
}
