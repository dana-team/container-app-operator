package resourcemanagers

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func newDNSRecordScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(dnsrecordv1alpha1.AddToScheme(s))
	return s
}

func newDNSRecordManager(k8sClient client.Client) DNSRecordManager {
	return DNSRecordManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         events.NewFakeRecorder(10),
	}
}

func newDNSRecordClient(objects ...client.Object) client.Client {
	objs := append([]client.Object{newCappConfigWithDNS()}, objects...)
	return newFakeClient(newDNSRecordScheme(), objs...)
}

func newFakeClient(scheme *runtime.Scheme, objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()
}

func newCNAMERecord(resourceName string, mutate func(*dnsrecordv1alpha1.CNAMERecord)) *dnsrecordv1alpha1.CNAMERecord {
	recordName := hostnameBare
	zone := dnsZone
	cname := dnsCNAME
	rec := &dnsrecordv1alpha1.CNAMERecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: cappNamespace,
			Labels: utils.MergeMaps(utils.ManagedResourceLabels(cappName), map[string]string{
				utils.CappNamespaceKey: cappNamespace,
			}),
		},
		Spec: dnsrecordv1alpha1.CNAMERecordSpec{
			ForProvider: dnsrecordv1alpha1.CNAMERecordParameters{
				Name:  &recordName,
				Zone:  &zone,
				Cname: &cname,
			},
		},
	}
	rec.Spec.ProviderConfigReference = &xpv1.ProviderConfigReference{
		Name: dnsProvider,
		Kind: ClusterProviderConfigKind,
	}
	if mutate != nil {
		mutate(rec)
	}
	return rec
}

func TestDNSRecordCreateOrUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("creates when not found", func(t *testing.T) {
		dm := newDNSRecordManager(newDNSRecordClient())
		capp := newCappWithHostname(hostnameBare)

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
		require.Equal(t, hostnameFQDN, got.Name)
		require.Equal(t, dnsCNAME, *got.Spec.ForProvider.Cname)
	})

	t.Run("creates FQDN hostname", func(t *testing.T) {
		dm := newDNSRecordManager(newDNSRecordClient())
		capp := newCappWithHostname(hostnameFQDN)

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
		require.Equal(t, hostnameBare, *got.Spec.ForProvider.Name)
	})

	t.Run("updates when ForProvider differs", func(t *testing.T) {
		staleCname := "stale.ingress.capp-zone.com."
		existing := newCNAMERecord(hostnameFQDN, func(rec *dnsrecordv1alpha1.CNAMERecord) {
			rec.Spec.ForProvider.Cname = &staleCname
		})
		dm := newDNSRecordManager(newDNSRecordClient(existing))
		capp := newCappWithHostname(hostnameBare)

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
		require.Equal(t, dnsCNAME, *got.Spec.ForProvider.Cname)
	})

	t.Run("updates when ProviderConfigReference differs", func(t *testing.T) {
		wrongProvider := "wrong-provider"
		existing := newCNAMERecord(hostnameFQDN, func(rec *dnsrecordv1alpha1.CNAMERecord) {
			rec.Spec.ProviderConfigReference = &xpv1.ProviderConfigReference{
				Name: wrongProvider,
				Kind: ClusterProviderConfigKind,
			}
		})
		dm := newDNSRecordManager(newDNSRecordClient(existing))
		capp := newCappWithHostname(hostnameBare)

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
		require.Equal(t, dnsProvider, got.Spec.ProviderConfigReference.Name)
	})

	t.Run("adds owner reference when missing", func(t *testing.T) {
		existing := newCNAMERecord(hostnameFQDN, nil)
		dm := newDNSRecordManager(newDNSRecordClient(existing))
		capp := newCappWithHostname(hostnameBare)

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
	})

	t.Run("skips update when spec and owner match", func(t *testing.T) {
		dm := newDNSRecordManager(newDNSRecordClient())
		capp := newCappWithHostname(hostnameBare)
		require.NoError(t, dm.createOrUpdate(ctx, capp))

		before := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, before))
		beforeRV := before.ResourceVersion

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		after := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, after))
		require.Equal(t, beforeRV, after.ResourceVersion)
	})

	t.Run("returns error when CappConfig missing", func(t *testing.T) {
		dm := newDNSRecordManager(newFakeClient(newDNSRecordScheme()))
		capp := newCappWithHostname(hostnameBare)

		err := dm.createOrUpdate(ctx, capp)
		require.Error(t, err)
	})
}

func TestDNSRecordManage(t *testing.T) {
	ctx := context.Background()

	t.Run("reconciles when hostname is set", func(t *testing.T) {
		dm := newDNSRecordManager(newDNSRecordClient())
		capp := newCappWithHostname(hostnameBare)
		require.NoError(t, dm.Manage(ctx, capp))
	})

	t.Run("cleans up when hostname is empty", func(t *testing.T) {
		existing := newCNAMERecord(hostnameFQDN, nil)
		fakeClient := newDNSRecordClient(existing)
		dm := newDNSRecordManager(fakeClient)
		capp := newBaseCapp()

		require.NoError(t, dm.Manage(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got)
		require.True(t, errors.IsNotFound(getErr))
	})
}

func TestDNSRecordCleanUp(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes all owned DNS records", func(t *testing.T) {
		const otherRecordName = "other.capp-zone.com"
		fakeClient := newDNSRecordClient(
			newCNAMERecord(hostnameFQDN, nil),
			newCNAMERecord(otherRecordName, nil),
		)
		dm := newDNSRecordManager(fakeClient)

		require.NoError(t, dm.CleanUp(ctx, newBaseCapp()))

		for _, name := range []string{hostnameFQDN, otherRecordName} {
			got := &dnsrecordv1alpha1.CNAMERecord{}
			getErr := fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: cappNamespace}, got)
			require.True(t, errors.IsNotFound(getErr))
		}
	})

	t.Run("succeeds when no records exist", func(t *testing.T) {
		dm := newDNSRecordManager(newFakeClient(newDNSRecordScheme()))
		require.NoError(t, dm.CleanUp(ctx, newBaseCapp()))
	})

	t.Run("skips delete when capp deleting and record has owner reference", func(t *testing.T) {
		capp := newBaseCapp()
		now := metav1.Now()
		capp.DeletionTimestamp = &now

		record := newCNAMERecord(hostnameFQDN, nil)
		require.NoError(t, controllerutil.SetOwnerReference(&capp, record, newDNSRecordScheme()))

		dm := newDNSRecordManager(newDNSRecordClient(record))
		require.NoError(t, dm.CleanUp(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got))
	})

	t.Run("deletes when capp is deleting and record lacks owner reference", func(t *testing.T) {
		capp := newBaseCapp()
		now := metav1.Now()
		capp.DeletionTimestamp = &now

		record := newCNAMERecord(hostnameFQDN, nil)
		dm := newDNSRecordManager(newDNSRecordClient(record))
		require.NoError(t, dm.CleanUp(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		getErr := dm.K8sclient.Get(ctx, types.NamespacedName{Name: hostnameFQDN, Namespace: cappNamespace}, got)
		require.True(t, errors.IsNotFound(getErr))
	})
}
