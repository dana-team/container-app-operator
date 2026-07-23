package status

import (
	"context"
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"knative.dev/pkg/apis"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	hostname     = "app"
	zone         = "example.com."
	resourceName = "app.example.com"
)

func newRouteScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	utilruntime.Must(knativev1beta1.AddToScheme(s))
	utilruntime.Must(cmapi.AddToScheme(s))
	utilruntime.Must(dnsrecordv1alpha1.SchemeBuilder.AddToScheme(s))
	return s
}

func routeCapp() cappv1alpha1.Capp {
	capp := newCapp()
	capp.Spec.RouteSpec.Hostname = hostname
	return capp
}

func TestBuildDomainMappingStatus(t *testing.T) {
	ctx := context.Background()
	capp := routeCapp()

	t.Run("returns empty when not required", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newRouteScheme()).Build()

		result, err := buildDomainMappingStatus(ctx, fakeClient, capp, false, zone)
		require.NoError(t, err)
		assert.Empty(t, result.Conditions)
	})

	t.Run("returns empty when domain mapping not found", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newRouteScheme()).Build()

		result, err := buildDomainMappingStatus(ctx, fakeClient, capp, true, zone)
		require.NoError(t, err)
		assert.Empty(t, result.Conditions)
	})

	t.Run("returns status when domain mapping exists", func(t *testing.T) {
		dm := &knativev1beta1.DomainMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: cappNamespace,
			},
			Status: knativev1beta1.DomainMappingStatus{
				URL: apis.HTTPS(resourceName),
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(newRouteScheme()).
			WithObjects(dm).Build()

		result, err := buildDomainMappingStatus(ctx, fakeClient, capp, true, zone)
		require.NoError(t, err)
		require.NotNil(t, result.URL)
		assert.Equal(t, resourceName, result.URL.Host)
	})
}

func TestBuildCertificateStatus(t *testing.T) {
	ctx := context.Background()
	capp := routeCapp()

	t.Run("returns empty when not required", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newRouteScheme()).Build()

		result, err := buildCertificateStatus(ctx, fakeClient, capp, false, zone)
		require.NoError(t, err)
		assert.Empty(t, result.Conditions)
	})

	t.Run("returns empty when certificate not found", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newRouteScheme()).Build()

		result, err := buildCertificateStatus(ctx, fakeClient, capp, true, zone)
		require.NoError(t, err)
		assert.Empty(t, result.Conditions)
	})

	t.Run("returns status when certificate exists", func(t *testing.T) {
		cert := &cmapi.Certificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: cappNamespace,
			},
			Status: cmapi.CertificateStatus{
				Conditions: []cmapi.CertificateCondition{
					{Type: cmapi.CertificateConditionReady, Status: "True"},
				},
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(newRouteScheme()).
			WithObjects(cert).Build()

		result, err := buildCertificateStatus(ctx, fakeClient, capp, true, zone)
		require.NoError(t, err)
		require.Len(t, result.Conditions, 1)
		assert.Equal(t, cmapi.CertificateConditionReady, result.Conditions[0].Type)
	})
}

func TestBuildDNSRecordStatus(t *testing.T) {
	ctx := context.Background()
	capp := routeCapp()

	t.Run("returns empty when not required", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newRouteScheme()).Build()

		result, err := buildDNSRecordStatus(ctx, fakeClient, capp, false, zone)
		require.NoError(t, err)
		assert.Empty(t, result.CNAMERecordObjectStatus.Conditions)
	})
}

func TestBuildCNAMERecordStatus(t *testing.T) {
	ctx := context.Background()
	capp := routeCapp()

	t.Run("returns empty when cname record not found", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newRouteScheme()).Build()

		result, err := buildCNAMERecordStatus(ctx, fakeClient, capp, zone)
		require.NoError(t, err)
		assert.Empty(t, result.Conditions)
	})

	t.Run("returns status when cname record exists", func(t *testing.T) {
		target := "target.example.com."
		cname := &dnsrecordv1alpha1.CNAMERecord{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: cappNamespace,
			},
			Status: dnsrecordv1alpha1.CNAMERecordStatus{
				AtProvider: dnsrecordv1alpha1.CNAMERecordObservation{
					Cname: &target,
				},
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(newRouteScheme()).
			WithObjects(cname).Build()

		result, err := buildCNAMERecordStatus(ctx, fakeClient, capp, zone)
		require.NoError(t, err)
		require.NotNil(t, result.AtProvider.Cname)
		assert.Equal(t, target, *result.AtProvider.Cname)
	})
}
