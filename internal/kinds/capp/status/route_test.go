package status

import (
	"context"
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
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
	duckv1 "knative.dev/pkg/apis/duck/v1"
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

func TestDomainMappingNotReady(t *testing.T) {
	tests := []struct {
		name       string
		status     cappv1alpha1.RouteStatus
		wantReason string
		wantMsg    string
		wantOK     bool
	}{
		{
			name:   "ready when domain mapping has no conditions",
			status: cappv1alpha1.RouteStatus{},
			wantOK: true,
		},
		{
			name: "ready when domain mapping Ready condition is true",
			status: cappv1alpha1.RouteStatus{
				DomainMappingObjectStatus: knativev1beta1.DomainMappingStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							{Type: apis.ConditionReady, Status: corev1.ConditionTrue},
						},
					},
				},
			},
			wantOK: true,
		},
		{
			name: "not ready when domain mapping Ready condition is false",
			status: cappv1alpha1.RouteStatus{
				DomainMappingObjectStatus: knativev1beta1.DomainMappingStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{
							{Type: apis.ConditionReady, Status: corev1.ConditionFalse, Message: "DNS not propagated"},
						},
					},
				},
			},
			wantReason: cappv1alpha1.CappReadyReasonDomainMappingNotReady,
			wantMsg:    "DNS not propagated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, msg, ok := domainMappingNotReady(tt.status)
			assert.Equal(t, tt.wantOK, ok)
			if !ok {
				assert.Equal(t, tt.wantReason, reason)
				assert.Equal(t, tt.wantMsg, msg)
			}
		})
	}
}

func TestCertificateNotReady(t *testing.T) {
	tests := []struct {
		name       string
		status     cappv1alpha1.RouteStatus
		wantReason string
		wantMsg    string
		wantOK     bool
	}{
		{
			name:   "ready when certificate has no conditions",
			status: cappv1alpha1.RouteStatus{},
			wantOK: true,
		},
		{
			name: "ready when certificate Ready condition is true",
			status: cappv1alpha1.RouteStatus{
				CertificateObjectStatus: cmapi.CertificateStatus{
					Conditions: []cmapi.CertificateCondition{
						{Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionTrue},
					},
				},
			},
			wantOK: true,
		},
		{
			name: "not ready when certificate Ready condition is false",
			status: cappv1alpha1.RouteStatus{
				CertificateObjectStatus: cmapi.CertificateStatus{
					Conditions: []cmapi.CertificateCondition{
						{Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionFalse, Message: "issuer not found"},
					},
				},
			},
			wantReason: cappv1alpha1.CappReadyReasonCertificateNotReady,
			wantMsg:    "issuer not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, msg, ok := certificateNotReady(tt.status)
			assert.Equal(t, tt.wantOK, ok)
			if !ok {
				assert.Equal(t, tt.wantReason, reason)
				assert.Equal(t, tt.wantMsg, msg)
			}
		})
	}
}
