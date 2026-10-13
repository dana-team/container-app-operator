package status

import (
	"context"
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"knative.dev/pkg/apis"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func newCappConfig() *cappv1alpha1.CappConfig {
	return &cappv1alpha1.CappConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cappmeta.CappConfigName,
			Namespace: cappmeta.CappNS,
		},
		Spec: cappv1alpha1.CappConfigSpec{
			DNSConfig: cappv1alpha1.DNSConfig{
				Zone: zone,
			},
		},
	}
}

func TestBuildRouteStatus(t *testing.T) {
	ctx := context.Background()
	capp := routeCapp()

	t.Run("returns empty statuses when no sub-resources required", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(newRouteScheme()).Build()
		isRequired := map[string]bool{
			rmanagers.DomainMapping: false,
			rmanagers.DNSRecord:     false,
			rmanagers.Certificate:   false,
		}

		result, err := buildRouteStatus(ctx, fakeClient, capp, isRequired, newCappConfig())
		require.NoError(t, err)
		assert.Empty(t, result.DomainMappingObjectStatus.Conditions)
		assert.Empty(t, result.DNSRecordObjectStatus.CNAMERecordObjectStatus.Conditions)
		assert.Empty(t, result.CertificateObjectStatus.Conditions)
	})

	t.Run("populates all sub-statuses when resources exist", func(t *testing.T) {
		dm := &knativev1beta1.DomainMapping{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: cappNamespace,
			},
			Status: knativev1beta1.DomainMappingStatus{
				URL: apis.HTTPS(resourceName),
			},
		}
		cert := &cmapi.Certificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: cappNamespace,
			},
			Status: cmapi.CertificateStatus{
				Conditions: []cmapi.CertificateCondition{
					{Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionTrue},
				},
			},
		}
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
			WithObjects(dm, cert, cname).Build()
		isRequired := map[string]bool{
			rmanagers.DomainMapping: true,
			rmanagers.DNSRecord:     true,
			rmanagers.Certificate:   true,
		}

		result, err := buildRouteStatus(ctx, fakeClient, capp, isRequired, newCappConfig())
		require.NoError(t, err)
		require.NotNil(t, result.DomainMappingObjectStatus.URL)
		assert.Equal(t, resourceName, result.DomainMappingObjectStatus.URL.Host)
		require.Len(t, result.CertificateObjectStatus.Conditions, 1)
		assert.Equal(t, cmapi.CertificateConditionReady, result.CertificateObjectStatus.Conditions[0].Type)
		require.NotNil(t, result.DNSRecordObjectStatus.CNAMERecordObjectStatus.AtProvider.Cname)
		assert.Equal(t, target, *result.DNSRecordObjectStatus.CNAMERecordObjectStatus.AtProvider.Cname)
	})
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
					{Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionTrue},
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

func newCNAMERecord(conditions ...xpv2.Condition) *dnsrecordv1alpha1.CNAMERecord {
	record := &dnsrecordv1alpha1.CNAMERecord{ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: cappNamespace}}
	record.Status.Conditions = conditions
	return record
}

func newCertificate(conditions ...cmapi.CertificateCondition) *cmapi.Certificate {
	cert := &cmapi.Certificate{ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: cappNamespace}}
	cert.Status.Conditions = conditions
	return cert
}

func newDomainMapping(conditions ...apis.Condition) *knativev1beta1.DomainMapping {
	dm := &knativev1beta1.DomainMapping{ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: cappNamespace}}
	dm.Status.Conditions = conditions
	return dm
}

func TestCheckRoute(t *testing.T) {
	readyCNAME := newCNAMERecord(
		xpv2.Condition{Type: xpv2.TypeSynced, Status: corev1.ConditionTrue},
		xpv2.Condition{Type: xpv2.TypeReady, Status: corev1.ConditionTrue},
	)
	readyCert := newCertificate(cmapi.CertificateCondition{Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionTrue})
	readyDM := newDomainMapping(apis.Condition{Type: apis.ConditionReady, Status: corev1.ConditionTrue})

	staleCert := newCertificate(cmapi.CertificateCondition{Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionTrue, ObservedGeneration: 1})
	staleCert.Generation = 2

	pendingCert := &metav1.Condition{
		Status:  metav1.ConditionUnknown,
		Reason:  cappv1alpha1.CapabilityReasonPending,
		Message: "Certificate " + resourceName + " not observed yet",
	}

	tests := []struct {
		name        string
		tlsDisabled bool
		objects     []client.Object
		want        *metav1.Condition
	}{
		{
			name: "reports a CNAMERecord sync error",
			objects: []client.Object{newCNAMERecord(xpv2.Condition{
				Type: xpv2.TypeSynced, Status: corev1.ConditionFalse, Reason: xpv2.ReasonReconcileError, Message: "invalid provider credentials",
			})},
			want: &metav1.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  string(xpv2.ReasonReconcileError),
				Message: "CNAMERecord " + resourceName + ": invalid provider credentials",
			},
		},
		{
			name:    "unknown when the CNAMERecord has no Ready condition",
			objects: []client.Object{newCNAMERecord(xpv2.Condition{Type: xpv2.TypeSynced, Status: corev1.ConditionTrue})},
			want: &metav1.Condition{
				Status:  metav1.ConditionUnknown,
				Reason:  cappv1alpha1.CapabilityReasonPending,
				Message: "CNAMERecord " + resourceName,
			},
		},
		{
			name:    "pending when the Certificate status is stale",
			objects: []client.Object{readyCNAME, staleCert},
			want:    pendingCert,
		},
		{
			name: "reports a Certificate that is not ready",
			objects: []client.Object{readyCNAME, newCertificate(cmapi.CertificateCondition{
				Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionFalse, Reason: "DoesNotExist", Message: "Secret does not exist",
			})},
			want: &metav1.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  "DoesNotExist",
				Message: "Certificate " + resourceName + ": Secret does not exist",
			},
		},
		{
			name: "reports a DomainMapping that is not ready",
			objects: []client.Object{readyCNAME, readyCert, newDomainMapping(apis.Condition{
				Type: apis.ConditionReady, Status: corev1.ConditionFalse, Reason: reasonResolveFailed, Message: "addressable has no address",
			})},
			want: &metav1.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  reasonResolveFailed,
				Message: "DomainMapping " + resourceName + ": addressable has no address",
			},
		},
		{
			name:    "nil when all route resources are ready",
			objects: []client.Object{readyCNAME, readyCert, readyDM},
		},
		{
			name:        "skips the Certificate when TLS is disabled",
			tlsDisabled: true,
			objects:     []client.Object{readyCNAME, readyDM},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(newRouteScheme()).WithObjects(tt.objects...).Build()

			capp := routeCapp()
			capp.Spec.RouteSpec.TlsEnabled = !tt.tlsDisabled

			got, err := checkRoute(context.Background(), fakeClient, capp, newCappConfig())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
