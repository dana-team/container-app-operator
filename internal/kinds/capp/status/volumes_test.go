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

func TestVolumesNotReady(t *testing.T) {
	tests := []struct {
		name       string
		status     cappv1alpha1.VolumesStatus
		wantReason string
		wantMsg    string
		wantOK     bool
	}{
		{
			name:   "ready when no volumes exist",
			status: cappv1alpha1.VolumesStatus{},
			wantOK: true,
		},
		{
			name:   "ready when all volumes are bound",
			status: nfsVolumesBound("vol-a", "vol-b"),
			wantOK: true,
		},
		{
			name:       "not ready when PV is not bound",
			status:     nfsVolumesUnbound("shared-data"),
			wantReason: cappv1alpha1.CappReadyReasonVolumesNotReady,
			wantMsg:    "NFS volume shared-data is not bound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, msg, ok := volumesNotReady(tt.status)
			assert.Equal(t, tt.wantOK, ok)
			if !ok {
				assert.Equal(t, tt.wantReason, reason)
				assert.Equal(t, tt.wantMsg, msg)
			}
		})
	}
}
