package webhooks

import (
	"context"
	"encoding/json"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestCappValidator_Handle(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(cappv1alpha1.AddToScheme(scheme))

	decoder := admission.NewDecoder(scheme)

	tests := []struct {
		name        string
		capp        *cappv1alpha1.Capp
		expectAllow bool
		expectMsg   string
	}{
		{
			name: "Allow external scale metric with sources",
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-capp",
					Namespace: "test-ns",
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleMetric: "external",
					Sources: []cappv1alpha1.KedaSource{
						{Name: "test"},
					},
					RouteSpec: cappv1alpha1.RouteSpec{
						Hostname: "valid-hostname.com",
					},
					LogSpec: cappv1alpha1.LogSpec{},
				},
			},
			expectAllow: true,
		},
		{
			name: "Deny cpu scale metric with sources",
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-capp",
					Namespace: "test-ns",
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleMetric: "cpu",
					Sources: []cappv1alpha1.KedaSource{
						{Name: "test"},
					},
					RouteSpec: cappv1alpha1.RouteSpec{
						Hostname: "valid-hostname.com",
					},
					LogSpec: cappv1alpha1.LogSpec{},
				},
			},
			expectAllow: false,
			expectMsg:   "invalid scale metric \"cpu\": must be 'external' when sources are defined",
		},
		{
			name: "Deny external scale metric without sources",
			capp: &cappv1alpha1.Capp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-capp",
					Namespace: "test-ns",
				},
				Spec: cappv1alpha1.CappSpec{
					ScaleMetric: "external",
					Sources:     []cappv1alpha1.KedaSource{},
					RouteSpec: cappv1alpha1.RouteSpec{
						Hostname: "valid-hostname.com",
					},
					LogSpec: cappv1alpha1.LogSpec{},
				},
			},
			expectAllow: false,
			expectMsg:   "invalid scale metric 'external': must have at least one source defined",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Add default capp config
			cappConfig := &cappv1alpha1.CappConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.CappConfigName,
					Namespace: utils.CappNS,
				},
				Spec: cappv1alpha1.CappConfigSpec{
					AllowedHostnamePatterns: []string{".*"},
				},
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cappConfig).Build()

			validator := &CappValidator{
				Client:  fakeClient,
				Decoder: decoder,
			}

			raw, err := json.Marshal(tc.capp)
			if err != nil {
				t.Fatal(err)
			}

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: raw,
					},
					Name:      "test-capp",
					Namespace: "test-ns",
				},
			}

			resp := validator.Handle(context.Background(), req)
			assert.Equal(t, tc.expectAllow, resp.Allowed, "Expected allowed: %v, got: %v. Result: %v", tc.expectAllow, resp.Allowed, resp.Result)
			if !tc.expectAllow && tc.expectMsg != "" {
				assert.Contains(t, resp.Result.Message, tc.expectMsg)
			}
		})
	}
}
