package resourcemanagers

import (
	"context"
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func newCertificateScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(cmapi.AddToScheme(s))
	return s
}

func newCertificateManager(k8sClient client.Client) CertificateManager {
	return CertificateManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         events.NewFakeRecorder(10),
	}
}

func newCertificateClient(objects ...client.Object) client.Client {
	objs := append([]client.Object{newCappConfigWithDNS()}, objects...)
	return newFakeClient(newCertificateScheme(), objs...)
}

func newCertificate(name string, mutate func(*cmapi.Certificate)) *cmapi.Certificate {
	cert := &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cappNamespace,
			Labels:    utils.ManagedResourceLabels(cappName),
		},
	}
	if mutate != nil {
		mutate(cert)
	}
	return cert
}

func TestCertificateManagerPrepareResource(t *testing.T) {
	ctx := context.Background()

	t.Run("prepares certificate spec from capp and DNS config", func(t *testing.T) {
		mgr := newCertificateManager(newCertificateClient())
		capp := newCappWithTLS(hostnameBare, true)

		got, err := mgr.prepareResource(ctx, capp)
		require.NoError(t, err)
		require.Equal(t, hostnameFQDN, got.Name)
		require.Equal(t, cmmeta.IssuerReference{Name: issuerName, Kind: issuerKind, Group: issuerGroup}, got.Spec.IssuerRef)
		require.Equal(t, utils.GenerateSecretName(hostnameFQDN), got.Spec.SecretName)
		require.NotNil(t, got.Spec.SecretTemplate)
		require.Equal(t, "", got.Spec.SecretTemplate.Labels[certificateUIDSecretLabelKey])
		require.Equal(t, []string{hostnameFQDN}, got.Spec.DNSNames)
	})

	t.Run("returns error when CappConfig missing", func(t *testing.T) {
		mgr := newCertificateManager(newFakeClient(newCertificateScheme()))
		capp := newCappWithTLS(hostnameBare, true)

		_, err := mgr.prepareResource(ctx, capp)
		require.Error(t, err)
	})
}

func TestCertificateManagerManage(t *testing.T) {
	ctx := context.Background()

	t.Run("creates when not found", func(t *testing.T) {
		mgr := newCertificateManager(newCertificateClient())
		capp := newCappWithTLS(hostnameBare, true)

		require.NoError(t, mgr.Manage(ctx, capp))

		got := &cmapi.Certificate{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
	})

	t.Run("creates FQDN hostname", func(t *testing.T) {
		mgr := newCertificateManager(newCertificateClient())
		capp := newCappWithTLS(hostnameFQDN, true)

		require.NoError(t, mgr.Manage(ctx, capp))

		got := &cmapi.Certificate{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
	})

	t.Run("updates when spec differs", func(t *testing.T) {
		wrongIssuer := "wrong-issuer"
		existing := newCertificate(hostnameFQDN, func(cert *cmapi.Certificate) {
			cert.Spec.IssuerRef.Name = wrongIssuer
		})
		mgr := newCertificateManager(newCertificateClient(existing))
		capp := newCappWithTLS(hostnameBare, true)

		require.NoError(t, mgr.Manage(ctx, capp))

		got := &cmapi.Certificate{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
		require.Equal(t, issuerName, got.Spec.IssuerRef.Name)
	})

	t.Run("adds owner reference when missing", func(t *testing.T) {
		existing := newCertificate(hostnameFQDN, nil)
		mgr := newCertificateManager(newCertificateClient(existing))
		capp := newCappWithTLS(hostnameBare, true)

		require.NoError(t, mgr.Manage(ctx, capp))

		got := &cmapi.Certificate{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
	})

	t.Run("skips update when unchanged", func(t *testing.T) {
		mgr := newCertificateManager(newCertificateClient())
		capp := newCappWithTLS(hostnameBare, true)
		require.NoError(t, mgr.Manage(ctx, capp))

		before := &cmapi.Certificate{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, before))
		beforeRV := before.ResourceVersion

		require.NoError(t, mgr.Manage(ctx, capp))

		after := &cmapi.Certificate{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, after))
		require.Equal(t, beforeRV, after.ResourceVersion)
	})

	t.Run("cleans up when TLS is disabled", func(t *testing.T) {
		existing := newCertificate(hostnameFQDN, nil)
		fakeClient := newCertificateClient(existing)
		mgr := newCertificateManager(fakeClient)
		capp := newCappWithTLS(hostnameBare, false)

		require.NoError(t, mgr.Manage(ctx, capp))

		got := &cmapi.Certificate{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got)
		require.True(t, errors.IsNotFound(getErr))
	})

	t.Run("cleans up when hostname is empty", func(t *testing.T) {
		existing := newCertificate(hostnameFQDN, nil)
		fakeClient := newCertificateClient(existing)
		mgr := newCertificateManager(fakeClient)
		capp := newBaseCapp()

		require.NoError(t, mgr.Manage(ctx, capp))

		got := &cmapi.Certificate{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got)
		require.True(t, errors.IsNotFound(getErr))
	})
}

func TestCertificateManagerCleanUp(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes all owned resources", func(t *testing.T) {
		const otherCertName = "other.capp-zone.com"
		fakeClient := newFakeClient(newCertificateScheme(),
			newCertificate(hostnameFQDN, nil),
			newCertificate(otherCertName, nil),
		)
		mgr := newCertificateManager(fakeClient)

		require.NoError(t, mgr.CleanUp(ctx, newBaseCapp()))

		for _, name := range []string{hostnameFQDN, otherCertName} {
			got := &cmapi.Certificate{}
			getErr := fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: cappNamespace}, got)
			require.True(t, errors.IsNotFound(getErr))
		}
	})

	t.Run("succeeds when none exist", func(t *testing.T) {
		mgr := newCertificateManager(newFakeClient(newCertificateScheme()))
		require.NoError(t, mgr.CleanUp(ctx, newBaseCapp()))
	})

	t.Run("skips delete when deleting and has owner reference", func(t *testing.T) {
		capp := newBaseCapp()
		now := metav1.Now()
		capp.DeletionTimestamp = &now

		cert := newCertificate(hostnameFQDN, nil)
		require.NoError(t, controllerutil.SetOwnerReference(&capp, cert, newCertificateScheme()))

		mgr := newCertificateManager(newFakeClient(newCertificateScheme(), cert))
		require.NoError(t, mgr.CleanUp(ctx, capp))

		got := &cmapi.Certificate{}
		require.NoError(t, mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
	})

	t.Run("deletes when deleting and lacks owner reference", func(t *testing.T) {
		capp := newBaseCapp()
		now := metav1.Now()
		capp.DeletionTimestamp = &now

		cert := newCertificate(hostnameFQDN, nil)
		mgr := newCertificateManager(newFakeClient(newCertificateScheme(), cert))
		require.NoError(t, mgr.CleanUp(ctx, capp))

		got := &cmapi.Certificate{}
		getErr := mgr.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got)
		require.True(t, errors.IsNotFound(getErr))
	})
}
