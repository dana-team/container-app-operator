package controllers

import (
	"testing"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	knativeapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
)

func TestConditionStatusChanged(t *testing.T) {
	tests := []struct {
		name     string
		oldConds []conditionPair
		newConds []conditionPair
		condType string
		expected bool
	}{
		{
			name:     "no change when both have same status",
			oldConds: []conditionPair{{condType: "Ready", status: "True"}},
			newConds: []conditionPair{{condType: "Ready", status: "True"}},
			condType: "Ready",
			expected: false,
		},
		{
			name:     "no change when neither has the condition",
			oldConds: []conditionPair{},
			newConds: []conditionPair{},
			condType: "Ready",
			expected: false,
		},
		{
			name:     "changed when status transitions",
			oldConds: []conditionPair{{condType: "Ready", status: "False"}},
			newConds: []conditionPair{{condType: "Ready", status: "True"}},
			condType: "Ready",
			expected: true,
		},
		{
			name:     "changed when condition appears",
			oldConds: []conditionPair{},
			newConds: []conditionPair{{condType: "Ready", status: "True"}},
			condType: "Ready",
			expected: true,
		},
		{
			name:     "changed when condition disappears",
			oldConds: []conditionPair{{condType: "Ready", status: "True"}},
			newConds: []conditionPair{},
			condType: "Ready",
			expected: true,
		},
		{
			name:     "ignores other condition types",
			oldConds: []conditionPair{{condType: "Ready", status: "True"}, {condType: "Synced", status: "False"}},
			newConds: []conditionPair{{condType: "Ready", status: "True"}, {condType: "Synced", status: "True"}},
			condType: "Ready",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, conditionStatusChanged(tt.oldConds, tt.newConds, tt.condType))
		})
	}
}

func TestKnativeConditions(t *testing.T) {
	conds := duckv1.Conditions{
		{Type: knativev1beta1.DomainMappingConditionReady, Status: corev1.ConditionTrue},
		{Type: knativev1beta1.DomainMappingConditionIngressReady, Status: corev1.ConditionFalse},
	}
	pairs := knativeConditions(conds)
	assert.Equal(t, []conditionPair{
		{condType: "Ready", status: "True"},
		{condType: "IngressReady", status: "False"},
	}, pairs)
}

func TestCertificateConditions(t *testing.T) {
	conds := []cmapi.CertificateCondition{
		{Type: cmapi.CertificateConditionReady, Status: cmmeta.ConditionTrue},
		{Type: cmapi.CertificateConditionIssuing, Status: cmmeta.ConditionFalse},
	}
	pairs := certificateConditions(conds)
	assert.Equal(t, []conditionPair{
		{condType: "Ready", status: "True"},
		{condType: "Issuing", status: "False"},
	}, pairs)
}

func TestDomainMappingWatchPredicateIntegration(t *testing.T) {
	makeDM := func(condStatus corev1.ConditionStatus) *knativev1beta1.DomainMapping {
		dm := &knativev1beta1.DomainMapping{}
		if condStatus != "" {
			dm.Status.Conditions = duckv1.Conditions{
				{Type: knativev1beta1.DomainMappingConditionReady, Status: condStatus},
			}
		}
		return dm
	}

	tests := []struct {
		name     string
		oldObj   *knativev1beta1.DomainMapping
		newObj   *knativev1beta1.DomainMapping
		expected bool
	}{
		{
			name:     "no change when both Ready=True",
			oldObj:   makeDM(corev1.ConditionTrue),
			newObj:   makeDM(corev1.ConditionTrue),
			expected: false,
		},
		{
			name:     "changed when Ready transitions False to True",
			oldObj:   makeDM(corev1.ConditionFalse),
			newObj:   makeDM(corev1.ConditionTrue),
			expected: true,
		},
		{
			name:     "changed when Ready condition appears",
			oldObj:   makeDM(""),
			newObj:   makeDM(corev1.ConditionTrue),
			expected: true,
		},
		{
			name: "ignores non-Ready condition changes",
			oldObj: func() *knativev1beta1.DomainMapping {
				dm := makeDM(corev1.ConditionTrue)
				dm.Status.Conditions = append(dm.Status.Conditions, knativeapis.Condition{
					Type: knativev1beta1.DomainMappingConditionIngressReady, Status: corev1.ConditionFalse,
				})
				return dm
			}(),
			newObj: func() *knativev1beta1.DomainMapping {
				dm := makeDM(corev1.ConditionTrue)
				dm.Status.Conditions = append(dm.Status.Conditions, knativeapis.Condition{
					Type: knativev1beta1.DomainMappingConditionIngressReady, Status: corev1.ConditionTrue,
				})
				return dm
			}(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := conditionStatusChanged(
				knativeConditions(tt.oldObj.Status.Conditions),
				knativeConditions(tt.newObj.Status.Conditions),
				string(knativev1beta1.DomainMappingConditionReady),
			)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCertificateWatchPredicateIntegration(t *testing.T) {
	makeCert := func(condStatus cmmeta.ConditionStatus) *cmapi.Certificate {
		cert := &cmapi.Certificate{}
		if condStatus != "" {
			cert.Status.Conditions = []cmapi.CertificateCondition{
				{Type: cmapi.CertificateConditionReady, Status: condStatus},
			}
		}
		return cert
	}

	tests := []struct {
		name     string
		oldObj   *cmapi.Certificate
		newObj   *cmapi.Certificate
		expected bool
	}{
		{
			name:     "no change when both Ready=True",
			oldObj:   makeCert(cmmeta.ConditionTrue),
			newObj:   makeCert(cmmeta.ConditionTrue),
			expected: false,
		},
		{
			name:     "changed when Ready transitions False to True",
			oldObj:   makeCert(cmmeta.ConditionFalse),
			newObj:   makeCert(cmmeta.ConditionTrue),
			expected: true,
		},
		{
			name:     "changed when Ready condition appears",
			oldObj:   makeCert(""),
			newObj:   makeCert(cmmeta.ConditionTrue),
			expected: true,
		},
		{
			name: "ignores non-Ready condition changes",
			oldObj: func() *cmapi.Certificate {
				cert := makeCert(cmmeta.ConditionTrue)
				cert.Status.Conditions = append(cert.Status.Conditions, cmapi.CertificateCondition{
					Type: cmapi.CertificateConditionIssuing, Status: cmmeta.ConditionTrue,
				})
				return cert
			}(),
			newObj: func() *cmapi.Certificate {
				cert := makeCert(cmmeta.ConditionTrue)
				cert.Status.Conditions = append(cert.Status.Conditions, cmapi.CertificateCondition{
					Type: cmapi.CertificateConditionIssuing, Status: cmmeta.ConditionFalse,
				})
				return cert
			}(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := conditionStatusChanged(
				certificateConditions(tt.oldObj.Status.Conditions),
				certificateConditions(tt.newObj.Status.Conditions),
				string(cmapi.CertificateConditionReady),
			)
			assert.Equal(t, tt.expected, result)
		})
	}
}
