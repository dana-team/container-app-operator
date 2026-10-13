package status

import (
	"strings"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
)

func TestFormatChildCondition(t *testing.T) {
	tests := []struct {
		name   string
		status metav1.ConditionStatus
		reason string
		want   *metav1.Condition
	}{
		{
			name:   "uses NotReady when a False child has an invalid reason",
			status: metav1.ConditionFalse,
			reason: "not valid!",
			want:   &metav1.Condition{Status: metav1.ConditionFalse, Reason: cappv1alpha1.CapabilityReasonNotReady, Message: "Revision rev-1"},
		},
		{
			name:   "treats an empty status as Unknown",
			reason: knativev1.ReasonDeploying,
			want:   &metav1.Condition{Status: metav1.ConditionUnknown, Reason: knativev1.ReasonDeploying, Message: "Revision rev-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatChildCondition(kindRevision, "rev-1", tt.status, tt.reason, "")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTruncateConditionMessage(t *testing.T) {
	assert.Equal(t, "short", truncateConditionMessage("short"))

	got := truncateConditionMessage(strings.Repeat("x", maxConditionMessageLen+1))
	assert.Len(t, got, maxConditionMessageLen)
	assert.True(t, strings.HasSuffix(got, truncatedSuffix))
}
