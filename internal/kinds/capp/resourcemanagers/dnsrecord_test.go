package resourcemanagers

import (
	"context"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"
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

func newDNSRecordScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(dnsrecordv1alpha1.AddToScheme(s))
	return s
}

func newDNSRecordManager(k8sClient client.Client) (DNSRecordManager, *events.FakeRecorder) {
	recorder := events.NewFakeRecorder(10)
	return DNSRecordManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         recorder,
	}, recorder
}

func newDNSRecordClient(objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(newDNSRecordScheme()).
		WithObjects(objects...).
		Build()
}

func expectedDNSResourceName() string {
	return utils.GenerateResourceName(testHostname, testZone)
}

func newExistingDNSRecord(capp *cappv1alpha1.Capp) *dnsrecordv1alpha1.CNAMERecord {
	resourceName := expectedDNSResourceName()
	recordName := utils.GenerateRecordName(testHostname, testZone)
	return &dnsrecordv1alpha1.CNAMERecord{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: capp.Namespace,
			Labels: utils.MergeMaps(utils.ManagedResourceLabels(capp.Name), map[string]string{
				utils.CappNamespaceKey: capp.Namespace,
			}),
		},
		Spec: dnsrecordv1alpha1.CNAMERecordSpec{
			ForProvider: dnsrecordv1alpha1.CNAMERecordParameters{
				Name:  &recordName,
				Zone:  strPtr(testZone),
				Cname: strPtr(testCNAME),
			},
		},
	}
}

func strPtr(s string) *string { return &s }

func TestDNSRecordManagerPrepareResource(t *testing.T) {
	ctx := context.Background()

	t.Run("generates correct resource name and record fields", func(t *testing.T) {
		dm, _ := newDNSRecordManager(newDNSRecordClient(newCappConfigWithDNS()))
		capp := newCappWithHostname()

		got, err := dm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.Equal(t, expectedDNSResourceName(), got.Name)
		require.Equal(t, cappNamespace, got.Namespace)
		require.Equal(t, testHostname, *got.Spec.ForProvider.Name)
		require.Equal(t, testZone, *got.Spec.ForProvider.Zone)
		require.Equal(t, testCNAME, *got.Spec.ForProvider.Cname)
	})

	t.Run("sets provider config reference", func(t *testing.T) {
		dm, _ := newDNSRecordManager(newDNSRecordClient(newCappConfigWithDNS()))
		capp := newCappWithHostname()

		got, err := dm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.NotNil(t, got.Spec.ProviderConfigReference)
		require.Equal(t, testProvider, got.Spec.ProviderConfigReference.Name)
		require.Equal(t, ClusterProviderConfigKind, got.Spec.ProviderConfigReference.Kind)
	})

	t.Run("sets managed resource labels", func(t *testing.T) {
		dm, _ := newDNSRecordManager(newDNSRecordClient(newCappConfigWithDNS()))
		capp := newCappWithHostname()

		got, err := dm.prepareResource(ctx, capp)

		require.NoError(t, err)
		require.Equal(t, cappName, got.Labels[utils.CappResourceKey])
		require.Equal(t, utils.CappKey, got.Labels[utils.ManagedByLabelKey])
		require.Equal(t, cappNamespace, got.Labels[utils.CappNamespaceKey])
	})

	t.Run("returns error when capp config is missing", func(t *testing.T) {
		dm, _ := newDNSRecordManager(newDNSRecordClient())
		capp := newCappWithHostname()

		_, err := dm.prepareResource(ctx, capp)

		require.Error(t, err)
	})
}

func TestDNSRecordManagerIsRequired(t *testing.T) {
	dm, _ := newDNSRecordManager(newDNSRecordClient())

	t.Run("returns true when hostname is set", func(t *testing.T) {
		capp := newCappWithHostname()
		require.True(t, dm.IsRequired(capp))
	})

	t.Run("returns false when hostname is empty", func(t *testing.T) {
		capp := newBaseCapp()
		require.False(t, dm.IsRequired(capp))
	})
}

func TestDNSRecordManagerCreateOrUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("creates DNS record when not found", func(t *testing.T) {
		dm, recorder := newDNSRecordManager(newDNSRecordClient(newCappConfigWithDNS()))
		capp := newCappWithHostname()

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDNSResourceName(), Namespace: cappNamespace}, got))
		require.Len(t, got.OwnerReferences, 1)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
		require.Contains(t, <-recorder.Events, eventCappDNSRecordCreated)
	})

	t.Run("updates DNS record when spec differs", func(t *testing.T) {
		capp := newCappWithHostname()
		existing := newExistingDNSRecord(&capp)
		oldCNAME := "old-cname.example.com"
		existing.Spec.ForProvider.Cname = &oldCNAME

		dm, _ := newDNSRecordManager(newDNSRecordClient(newCappConfigWithDNS(), existing))

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDNSResourceName(), Namespace: cappNamespace}, got))
		require.Equal(t, testCNAME, *got.Spec.ForProvider.Cname)
	})

	t.Run("skips update when spec is unchanged and owner ref present", func(t *testing.T) {
		capp := newCappWithHostname()
		existing := newExistingDNSRecord(&capp)
		require.NoError(t, controllerutil.SetOwnerReference(&capp, existing, newDNSRecordScheme()))

		dm, _ := newDNSRecordManager(newDNSRecordClient(newCappConfigWithDNS(), existing))

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDNSResourceName(), Namespace: cappNamespace}, got))
	})

	t.Run("updates when owner reference is missing", func(t *testing.T) {
		capp := newCappWithHostname()
		existing := newExistingDNSRecord(&capp)

		dm, _ := newDNSRecordManager(newDNSRecordClient(newCappConfigWithDNS(), existing))

		require.NoError(t, dm.createOrUpdate(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDNSResourceName(), Namespace: cappNamespace}, got))
		require.Len(t, got.OwnerReferences, 1)
	})
}

func TestDNSRecordManagerManage(t *testing.T) {
	ctx := context.Background()

	t.Run("creates DNS record when hostname is set", func(t *testing.T) {
		dm, _ := newDNSRecordManager(newDNSRecordClient(newCappConfigWithDNS()))
		capp := newCappWithHostname()

		require.NoError(t, dm.Manage(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDNSResourceName(), Namespace: cappNamespace}, got))
	})

	t.Run("cleans up when hostname is empty", func(t *testing.T) {
		capp := newBaseCapp()
		existing := newExistingDNSRecord(&capp)

		dm, _ := newDNSRecordManager(newDNSRecordClient(existing))

		require.NoError(t, dm.Manage(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		err := dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDNSResourceName(), Namespace: cappNamespace}, got)
		require.True(t, client.IgnoreNotFound(err) == nil && err != nil)
	})
}

func TestDNSRecordManagerCleanUp(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes DNS records for non-deleting capp", func(t *testing.T) {
		capp := newCappWithHostname()
		existing := newExistingDNSRecord(&capp)

		dm, _ := newDNSRecordManager(newDNSRecordClient(existing))

		require.NoError(t, dm.CleanUp(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		err := dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDNSResourceName(), Namespace: cappNamespace}, got)
		require.True(t, client.IgnoreNotFound(err) == nil && err != nil)
	})

	t.Run("skips owned DNS records when capp is deleting", func(t *testing.T) {
		capp := newCappWithHostname()
		now := metav1.Now()
		capp.DeletionTimestamp = &now

		existing := newExistingDNSRecord(&capp)
		require.NoError(t, controllerutil.SetOwnerReference(&capp, existing, newDNSRecordScheme()))

		dm, _ := newDNSRecordManager(newDNSRecordClient(existing))

		require.NoError(t, dm.CleanUp(ctx, capp))

		got := &dnsrecordv1alpha1.CNAMERecord{}
		require.NoError(t, dm.K8sclient.Get(ctx, types.NamespacedName{Name: expectedDNSResourceName(), Namespace: cappNamespace}, got))
	})

	t.Run("succeeds when no DNS records exist", func(t *testing.T) {
		dm, _ := newDNSRecordManager(newDNSRecordClient())
		capp := newCappWithHostname()

		require.NoError(t, dm.CleanUp(ctx, capp))
	})
}
