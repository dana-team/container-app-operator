package status

import (
	"context"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	volumeName = "vol-a"
	phaseBound = "Bound"
)

func newVolumesScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(cappv1alpha1.AddToScheme(s))
	utilruntime.Must(nfspvcv1alpha1.AddToScheme(s))
	return s
}

func newNfsPvc(name string, status nfspvcv1alpha1.NfsPvcStatus) *nfspvcv1alpha1.NfsPvc {
	return &nfspvcv1alpha1.NfsPvc{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cappNamespace,
		},
		Status: status,
	}
}

func TestBuildVolumesStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("returns empty when not required", func(t *testing.T) {
		capp := newCapp()
		fakeClient := fake.NewClientBuilder().WithScheme(newVolumesScheme()).Build()

		result, err := buildVolumesStatus(ctx, fakeClient, capp, false)
		require.NoError(t, err)
		assert.Empty(t, result.NFSVolumesStatus)
	})

	t.Run("appends empty status when nfspvc not found", func(t *testing.T) {
		capp := newCapp()
		capp.Spec.VolumesSpec.NFSVolumes = []cappv1alpha1.NFSVolume{
			{Name: volumeName},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(newVolumesScheme()).Build()

		result, err := buildVolumesStatus(ctx, fakeClient, capp, true)
		require.NoError(t, err)
		require.Len(t, result.NFSVolumesStatus, 1)
		assert.Equal(t, volumeName, result.NFSVolumesStatus[0].VolumeName)
		assert.Equal(t, nfspvcv1alpha1.NfsPvcStatus{}, result.NFSVolumesStatus[0].NFSPVCStatus)
	})

	t.Run("copies status from existing nfspvc", func(t *testing.T) {
		capp := newCapp()
		capp.Spec.VolumesSpec.NFSVolumes = []cappv1alpha1.NFSVolume{
			{Name: volumeName},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(newVolumesScheme()).
			WithObjects(newNfsPvc(volumeName, nfspvcv1alpha1.NfsPvcStatus{
				PvPhase:  phaseBound,
				PvcPhase: phaseBound,
			})).Build()

		result, err := buildVolumesStatus(ctx, fakeClient, capp, true)
		require.NoError(t, err)
		require.Len(t, result.NFSVolumesStatus, 1)
		assert.Equal(t, volumeName, result.NFSVolumesStatus[0].VolumeName)
		assert.Equal(t, phaseBound, result.NFSVolumesStatus[0].NFSPVCStatus.PvPhase)
		assert.Equal(t, phaseBound, result.NFSVolumesStatus[0].NFSPVCStatus.PvcPhase)
	})

	t.Run("handles multiple volumes with mixed existence", func(t *testing.T) {
		capp := newCapp()
		capp.Spec.VolumesSpec.NFSVolumes = []cappv1alpha1.NFSVolume{
			{Name: volumeName},
			{Name: "vol-b"},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(newVolumesScheme()).
			WithObjects(newNfsPvc(volumeName, nfspvcv1alpha1.NfsPvcStatus{
				PvPhase:  phaseBound,
				PvcPhase: phaseBound,
			})).Build()

		result, err := buildVolumesStatus(ctx, fakeClient, capp, true)
		require.NoError(t, err)
		require.Len(t, result.NFSVolumesStatus, 2)
		assert.Equal(t, volumeName, result.NFSVolumesStatus[0].VolumeName)
		assert.Equal(t, phaseBound, result.NFSVolumesStatus[0].NFSPVCStatus.PvPhase)
		assert.Equal(t, "vol-b", result.NFSVolumesStatus[1].VolumeName)
		assert.Equal(t, nfspvcv1alpha1.NfsPvcStatus{}, result.NFSVolumesStatus[1].NFSPVCStatus)
	})
}

func TestCheckVolumes(t *testing.T) {
	const otherVolume = "vol-b"

	readyStatus := nfspvcv1alpha1.NfsPvcStatus{
		Conditions: []metav1.Condition{{Type: nfspvcv1alpha1.ConditionReady, Status: metav1.ConditionTrue}},
	}

	stalePvc := newNfsPvc(volumeName, nfspvcv1alpha1.NfsPvcStatus{
		Conditions: []metav1.Condition{{Type: nfspvcv1alpha1.ConditionReady, Status: metav1.ConditionTrue, ObservedGeneration: 1}},
	})
	stalePvc.Generation = 2

	pendingVolume := &metav1.Condition{
		Status:  metav1.ConditionUnknown,
		Reason:  cappv1alpha1.CapabilityReasonPending,
		Message: "NfsPvc " + volumeName + " not observed yet",
	}

	tests := []struct {
		name    string
		objects []client.Object
		want    *metav1.Condition
	}{
		{
			name:    "pending when the Ready condition is stale",
			objects: []client.Object{stalePvc},
			want:    pendingVolume,
		},
		{
			name: "reports an NfsPvc that is not ready",
			objects: []client.Object{newNfsPvc(volumeName, nfspvcv1alpha1.NfsPvcStatus{
				Conditions: []metav1.Condition{{
					Type: nfspvcv1alpha1.ConditionReady, Status: metav1.ConditionFalse, Reason: reasonPvcPending, Message: "waiting for the PVC to bind",
				}},
			})},
			want: &metav1.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  reasonPvcPending,
				Message: "NfsPvc " + volumeName + ": waiting for the PVC to bind",
			},
		},
		{
			name:    "nil when all volumes are ready",
			objects: []client.Object{newNfsPvc(volumeName, readyStatus), newNfsPvc(otherVolume, readyStatus)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capp := newCapp()
			capp.Spec.VolumesSpec.NFSVolumes = []cappv1alpha1.NFSVolume{{Name: volumeName}, {Name: otherVolume}}
			fakeClient := fake.NewClientBuilder().WithScheme(newVolumesScheme()).WithObjects(tt.objects...).Build()

			got, err := checkVolumes(context.Background(), fakeClient, capp, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
