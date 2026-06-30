package resourcemanagers

import (
	"context"
	"testing"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func newDomainMappingScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(knativev1beta1.AddToScheme(s))
	return s
}

func newDomainMappingManager(k8sClient client.Client) DomainMappingManager {
	return DomainMappingManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         events.NewFakeRecorder(10),
	}
}

func newDomainMappingClient(objects ...client.Object) client.Client {
	objs := append([]client.Object{newCappConfigWithDNS()}, objects...)
	return newFakeClient(newDomainMappingScheme(), objs...)
}

func newDomainMapping(name string, mutate func(*knativev1beta1.DomainMapping)) *knativev1beta1.DomainMapping {
	dm := &knativev1beta1.DomainMapping{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cappNamespace,
			Labels:    utils.ManagedResourceLabels(cappName),
		},
		Spec: knativev1beta1.DomainMappingSpec{
			Ref: duckv1.KReference{
				APIVersion: knativev1.SchemeGroupVersion.String(),
				Name:       cappName,
				Kind:       knativeServiceKind,
			},
		},
	}
	if mutate != nil {
		mutate(dm)
	}
	return dm
}

func TestDomainMappingManagerPrepareResource(t *testing.T) {
	ctx := context.Background()

	t.Run("omits TLS when disabled", func(t *testing.T) {
		mgr := newDomainMappingManager(newDomainMappingClient(newSecret(utils.GenerateSecretName(hostnameFQDN), nil)))
		capp := newCappWithTLS(hostnameBare, false)

		got, err := mgr.prepareResource(ctx, capp)
		require.NoError(t, err)
		require.Nil(t, got.Spec.TLS)
	})

	t.Run("sets TLS when enabled and secret exists", func(t *testing.T) {
		mgr := newDomainMappingManager(newDomainMappingClient(newSecret(utils.GenerateSecretName(hostnameFQDN), nil)))
		capp := newCappWithTLS(hostnameBare, true)

		got, err := mgr.prepareResource(ctx, capp)
		require.NoError(t, err)
		require.NotNil(t, got.Spec.TLS)
		require.Equal(t, utils.GenerateSecretName(hostnameFQDN), got.Spec.TLS.SecretName)
	})

	t.Run("omits TLS when enabled and secret is missing", func(t *testing.T) {
		mgr := newDomainMappingManager(newDomainMappingClient())
		capp := newCappWithTLS(hostnameBare, true)

		got, err := mgr.prepareResource(ctx, capp)
		require.NoError(t, err)
		require.Nil(t, got.Spec.TLS)
	})
}

func TestDomainMappingManagerCreateOrUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("creates when not found", func(t *testing.T) {
		mgr := newDomainMappingManager(newDomainMappingClient())
		capp := newCappWithHostname(hostnameBare)

		require.NoError(t, mgr.createOrUpdate(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
		require.Equal(t, hostnameFQDN, got.Name)
		require.Equal(t, cappName, got.Spec.Ref.Name)
		require.Equal(t, knativeServiceKind, got.Spec.Ref.Kind)
		require.Equal(t, knativev1.SchemeGroupVersion.String(), got.Spec.Ref.APIVersion)
		require.Equal(t, cappName, got.Labels[utils.CappResourceKey])
	})

	t.Run("creates FQDN hostname", func(t *testing.T) {
		mgr := newDomainMappingManager(newDomainMappingClient())
		capp := newCappWithHostname(hostnameFQDN)

		require.NoError(t, mgr.createOrUpdate(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
		require.Equal(t, hostnameFQDN, got.Name)
	})

	t.Run("updates when spec differs", func(t *testing.T) {
		existing := newDomainMapping(hostnameFQDN, func(dm *knativev1beta1.DomainMapping) {
			dm.Spec.Ref.Name = "wrong-ksvc"
		})
		mgr := newDomainMappingManager(newDomainMappingClient(existing))
		capp := newCappWithHostname(hostnameBare)

		require.NoError(t, mgr.createOrUpdate(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
		require.Equal(t, cappName, got.Spec.Ref.Name)
	})

	t.Run("adds owner reference when missing", func(t *testing.T) {
		existing := newDomainMapping(hostnameFQDN, nil)
		mgr := newDomainMappingManager(newDomainMappingClient(existing))
		capp := newCappWithHostname(hostnameBare)

		require.NoError(t, mgr.createOrUpdate(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
	})

	t.Run("skips update when unchanged", func(t *testing.T) {
		mgr := newDomainMappingManager(newDomainMappingClient())
		capp := newCappWithHostname(hostnameBare)
		require.NoError(t, mgr.createOrUpdate(ctx, capp))

		before := &knativev1beta1.DomainMapping{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, before))
		beforeRV := before.ResourceVersion

		require.NoError(t, mgr.createOrUpdate(ctx, capp))

		after := &knativev1beta1.DomainMapping{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, after))
		require.Equal(t, beforeRV, after.ResourceVersion)
	})

	t.Run("returns error when CappConfig missing", func(t *testing.T) {
		mgr := newDomainMappingManager(newFakeClient(newDomainMappingScheme()))
		capp := newCappWithHostname(hostnameBare)

		err := mgr.createOrUpdate(ctx, capp)
		require.Error(t, err)
	})
}

func TestDomainMappingManagerManage(t *testing.T) {
	ctx := context.Background()

	t.Run("reconciles when hostname is set", func(t *testing.T) {
		mgr := newDomainMappingManager(newDomainMappingClient())
		capp := newCappWithHostname(hostnameBare)
		require.NoError(t, mgr.Manage(ctx, capp))
	})

	t.Run("cleans up when hostname is empty", func(t *testing.T) {
		existing := newDomainMapping(hostnameFQDN, nil)
		fakeClient := newDomainMappingClient(existing)
		mgr := newDomainMappingManager(fakeClient)
		capp := newBaseCapp()

		require.NoError(t, mgr.Manage(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got)
		require.True(t, errors.IsNotFound(getErr))
	})
}

func TestDomainMappingManagerCleanUp(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes all owned resources", func(t *testing.T) {
		const otherMappingName = "other.capp-zone.com"
		fakeClient := newFakeClient(newDomainMappingScheme(),
			newDomainMapping(hostnameFQDN, nil),
			newDomainMapping(otherMappingName, nil),
		)
		mgr := newDomainMappingManager(fakeClient)

		require.NoError(t, mgr.CleanUp(ctx, newBaseCapp()))

		for _, name := range []string{hostnameFQDN, otherMappingName} {
			got := &knativev1beta1.DomainMapping{}
			getErr := fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: cappNamespace}, got)
			require.True(t, errors.IsNotFound(getErr))
		}
	})

	t.Run("deletes associated TLS secret", func(t *testing.T) {
		fakeClient := newFakeClient(newDomainMappingScheme(),
			newDomainMapping(hostnameFQDN, nil),
			newSecret(utils.GenerateSecretName(hostnameFQDN), nil),
		)
		mgr := newDomainMappingManager(fakeClient)

		require.NoError(t, mgr.CleanUp(ctx, newBaseCapp()))

		got := &corev1.Secret{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: utils.GenerateSecretName(hostnameFQDN), Namespace: cappNamespace}, got)
		require.True(t, errors.IsNotFound(getErr))
	})

	t.Run("succeeds when none exist", func(t *testing.T) {
		mgr := newDomainMappingManager(newFakeClient(newDomainMappingScheme()))
		require.NoError(t, mgr.CleanUp(ctx, newBaseCapp()))
	})

	t.Run("skips delete when deleting and has owner reference", func(t *testing.T) {
		capp := cappWithDeletionTimestamp(newBaseCapp())

		mapping := newDomainMapping(hostnameFQDN, nil)
		require.NoError(t, controllerutil.SetOwnerReference(&capp, mapping, newDomainMappingScheme()))

		mgr := newDomainMappingManager(newFakeClient(newDomainMappingScheme(), mapping))
		require.NoError(t, mgr.CleanUp(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
	})

	t.Run("deletes when deleting and lacks owner reference", func(t *testing.T) {
		capp := cappWithDeletionTimestamp(newBaseCapp())

		mapping := newDomainMapping(hostnameFQDN, nil)
		mgr := newDomainMappingManager(newFakeClient(newDomainMappingScheme(), mapping))
		require.NoError(t, mgr.CleanUp(ctx, capp))

		got := &knativev1beta1.DomainMapping{}
		getErr := mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got)
		require.True(t, errors.IsNotFound(getErr))
	})
}
