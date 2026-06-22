package resourcemanagers

import (
	"context"
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func newCertificateScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(cmapi.AddToScheme(s))
	return s
}

func newCertificateManager(k8sClient client.Client) (CertificateManager, *events.FakeRecorder) {
	recorder := events.NewFakeRecorder(10)
	return CertificateManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         recorder,
	}, recorder
}

func newCertificateClient(objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(newCertificateScheme()).
		WithObjects(objects...).
		Build()
}

func newCappWithTLS() cappv1alpha1.Capp {
	capp := newCappWithHostname()
	capp.Spec.RouteSpec.TlsEnabled = true
	return capp
}

func expectedCertResourceName() string {
	return utils.GenerateResourceName(testHostname, testZone)
}

func expectedCertSecretName() string {
	return utils.GenerateSecretName(expectedCertResourceName())
}

func newExistingCertificate(capp *cappv1alpha1.Capp) *cmapi.Certificate {
	resourceName := expectedCertResourceName()
	return &cmapi.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: capp.Namespace,
			Labels:    utils.ManagedResourceLabels(capp.Name),
		},
		Spec: cmapi.CertificateSpec{
			CommonName: utils.TruncateCommonName(resourceName),
			DNSNames:   []string{resourceName},
			PrivateKey: &cmapi.CertificatePrivateKey{
				Algorithm: cmapi.RSAKeyAlgorithm,
				Encoding:  cmapi.PKCS1,
				Size:      PrivateKeySize,
			},
			IssuerRef: cmmeta.IssuerReference{
				Name:  testIssuerName,
				Kind:  testIssuerKind,
				Group: testIssuerGroup,
			},
			SecretName: expectedCertSecretName(),
			SecretTemplate: &cmapi.CertificateSecretTemplate{
				Labels: map[string]string{
					certificateUIDSecretLabelKey: "",
				},
			},
		},
	}
}

func TestCertificateManagerPrepareResource(t *testing.T) {
	ctx := context.Background()

	t.Run("generates correct certificate spec", func(t *testing.T) {
		cm, _ := newCertificateManager(newCertificateClient(newCappConfigWithDNS()))
		capp := newCappWithTLS()

		got, err := cm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.Equal(t, expectedCertResourceName(), got.Name)
		require.Equal(t, cappNamespace, got.Namespace)
		require.Equal(t, expectedCertSecretName(), got.Spec.SecretName)
		require.Equal(t, []string{expectedCertResourceName()}, got.Spec.DNSNames)
	})

	t.Run("sets RSA 4096 private key", func(t *testing.T) {
		cm, _ := newCertificateManager(newCertificateClient(newCappConfigWithDNS()))
		capp := newCappWithTLS()

		got, err := cm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.NotNil(t, got.Spec.PrivateKey)
		require.Equal(t, cmapi.RSAKeyAlgorithm, got.Spec.PrivateKey.Algorithm)
		require.Equal(t, PrivateKeySize, got.Spec.PrivateKey.Size)
	})

	t.Run("sets issuer reference from DNS config", func(t *testing.T) {
		cm, _ := newCertificateManager(newCertificateClient(newCappConfigWithDNS()))
		capp := newCappWithTLS()

		got, err := cm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.Equal(t, testIssuerName, got.Spec.IssuerRef.Name)
		require.Equal(t, testIssuerKind, got.Spec.IssuerRef.Kind)
		require.Equal(t, testIssuerGroup, got.Spec.IssuerRef.Group)
	})

	t.Run("adds knative secret label", func(t *testing.T) {
		cm, _ := newCertificateManager(newCertificateClient(newCappConfigWithDNS()))
		capp := newCappWithTLS()

		got, err := cm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.NotNil(t, got.Spec.SecretTemplate)
		require.Contains(t, got.Spec.SecretTemplate.Labels, certificateUIDSecretLabelKey)
	})

	t.Run("sets managed resource labels", func(t *testing.T) {
		cm, _ := newCertificateManager(newCertificateClient(newCappConfigWithDNS()))
		capp := newCappWithTLS()

		got, err := cm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.Equal(t, cappName, got.Labels[utils.CappResourceKey])
		require.Equal(t, utils.CappKey, got.Labels[utils.ManagedByLabelKey])
	})

	t.Run("returns error when capp config is missing", func(t *testing.T) {
		cm, _ := newCertificateManager(newCertificateClient())
		capp := newCappWithTLS()

		_, err := cm.prepareResource(ctx, capp)

		require.Error(t, err)
	})
}

func TestCertificateManagerIsRequired(t *testing.T) {
	cm, _ := newCertificateManager(newCertificateClient())

	t.Run("returns true when TLS enabled and hostname set", func(t *testing.T) {
		capp := newCappWithTLS()
		require.True(t, cm.IsRequired(capp))
	})

	t.Run("returns false when TLS disabled", func(t *testing.T) {
		capp := newCappWithHostname()
		require.False(t, cm.IsRequired(capp))
	})

	t.Run("returns false when hostname empty", func(t *testing.T) {
		capp := newBaseCapp()
		capp.Spec.RouteSpec.TlsEnabled = true
		require.False(t, cm.IsRequired(capp))
	})

	t.Run("returns false when both TLS disabled and hostname empty", func(t *testing.T) {
		capp := newBaseCapp()
		require.False(t, cm.IsRequired(capp))
	})
}

func TestCertificateManagerManage(t *testing.T) {
	ctx := context.Background()

	t.Run("creates certificate when required", func(t *testing.T) {
		cm, recorder := newCertificateManager(newCertificateClient(newCappConfigWithDNS()))
		capp := newCappWithTLS()

		require.NoError(t, cm.Manage(ctx, capp))

		got := &cmapi.Certificate{}
		require.NoError(t, cm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedCertResourceName(), Namespace: cappNamespace}, got))
		require.Len(t, got.OwnerReferences, 1)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
		require.Contains(t, <-recorder.Events, eventCappCertificateCreated)
	})

	t.Run("updates certificate when spec differs", func(t *testing.T) {
		capp := newCappWithTLS()
		existing := newExistingCertificate(&capp)
		existing.Spec.IssuerRef.Name = "old-issuer"
		require.NoError(t, controllerutil.SetOwnerReference(&capp, existing, newCertificateScheme()))

		cm, _ := newCertificateManager(newCertificateClient(newCappConfigWithDNS(), existing))

		require.NoError(t, cm.Manage(ctx, capp))

		got := &cmapi.Certificate{}
		require.NoError(t, cm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedCertResourceName(), Namespace: cappNamespace}, got))
		require.Equal(t, testIssuerName, got.Spec.IssuerRef.Name)
	})

	t.Run("skips update when spec is unchanged", func(t *testing.T) {
		capp := newCappWithTLS()
		existing := newExistingCertificate(&capp)
		require.NoError(t, controllerutil.SetOwnerReference(&capp, existing, newCertificateScheme()))

		cm, _ := newCertificateManager(newCertificateClient(newCappConfigWithDNS(), existing))

		before := &cmapi.Certificate{}
		require.NoError(t, cm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedCertResourceName(), Namespace: cappNamespace}, before))
		beforeRV := before.ResourceVersion

		require.NoError(t, cm.Manage(ctx, capp))

		after := &cmapi.Certificate{}
		require.NoError(t, cm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedCertResourceName(), Namespace: cappNamespace}, after))
		require.Equal(t, beforeRV, after.ResourceVersion)
	})

	t.Run("cleans up when not required", func(t *testing.T) {
		capp := newCappWithHostname()
		existing := newExistingCertificate(&capp)

		cm, _ := newCertificateManager(newCertificateClient(existing))

		require.NoError(t, cm.Manage(ctx, capp))

		got := &cmapi.Certificate{}
		err := cm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedCertResourceName(), Namespace: cappNamespace}, got)
		require.True(t, client.IgnoreNotFound(err) == nil && err != nil)
	})
}

func TestCertificateManagerCleanUp(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes certificates for non-deleting capp", func(t *testing.T) {
		capp := newCappWithTLS()
		existing := newExistingCertificate(&capp)

		cm, _ := newCertificateManager(newCertificateClient(existing))

		require.NoError(t, cm.CleanUp(ctx, capp))

		got := &cmapi.Certificate{}
		err := cm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedCertResourceName(), Namespace: cappNamespace}, got)
		require.True(t, client.IgnoreNotFound(err) == nil && err != nil)
	})

	t.Run("skips owned certificates when capp is deleting", func(t *testing.T) {
		capp := newCappWithTLS()
		now := metav1.Now()
		capp.DeletionTimestamp = &now

		existing := newExistingCertificate(&capp)
		require.NoError(t, controllerutil.SetOwnerReference(&capp, existing, newCertificateScheme()))

		cm, _ := newCertificateManager(newCertificateClient(existing))

		require.NoError(t, cm.CleanUp(ctx, capp))

		got := &cmapi.Certificate{}
		require.NoError(t, cm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedCertResourceName(), Namespace: cappNamespace}, got))
	})

	t.Run("succeeds when no certificates exist", func(t *testing.T) {
		cm, _ := newCertificateManager(newCertificateClient())
		capp := newCappWithTLS()

		require.NoError(t, cm.CleanUp(ctx, capp))
	})
}
