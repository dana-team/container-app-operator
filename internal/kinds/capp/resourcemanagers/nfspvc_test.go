package resourcemanagers

import (
	"context"
	"testing"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"
	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	nfsVolA   = "data-vol-a"
	nfsVolB   = "data-vol-b"
	nfsServer = "nfs.example.com"
	nfsPath   = "/export"
	nfsPathV2 = "/export/v2"
)

func newNFSPVCScheme() *runtime.Scheme {
	s := newScheme()
	utilruntime.Must(nfspvcv1alpha1.AddToScheme(s))
	return s
}

func newNFSPVCManager(k8sClient client.Client) NFSPVCManager {
	return NFSPVCManager{
		ResourceManagerClient: rclient.ResourceManagerClient{K8sclient: k8sClient, Log: logr.Discard()},
		EventRecorder:         events.NewFakeRecorder(10),
	}
}

func newNFSVolume(name, path string) cappv1alpha1.NFSVolume {
	return cappv1alpha1.NFSVolume{
		Name:     name,
		Server:   nfsServer,
		Path:     path,
		Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
	}
}

func cappWithVolumes(vols ...cappv1alpha1.NFSVolume) cappv1alpha1.Capp {
	capp := newBaseCapp()
	capp.Spec.VolumesSpec.NFSVolumes = vols
	return capp
}

func newNFSPVC(name string) *nfspvcv1alpha1.NfsPvc {
	return &nfspvcv1alpha1.NfsPvc{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cappNamespace,
			Labels:    utils.ManagedResourceLabels(cappName),
		},
		Spec: nfspvcv1alpha1.NfsPvcSpec{
			Server: nfsServer,
			Path:   nfsPath,
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
		},
	}
}

func TestNFSPVCManagerCreateOrUpdate(t *testing.T) {
	ctx := context.Background()
	keyA := types.NamespacedName{Name: nfsVolA, Namespace: cappNamespace}

	t.Run("creates when not found", func(t *testing.T) {
		nm := newNFSPVCManager(newFakeClient(newNFSPVCScheme()))
		capp := cappWithVolumes(newNFSVolume(nfsVolA, nfsPath))

		require.NoError(t, nm.createOrUpdate(ctx, capp))

		got := &nfspvcv1alpha1.NfsPvc{}
		require.NoError(t, nm.K8sclient.Get(ctx, keyA, got))
		require.Equal(t, nfsServer, got.Spec.Server)
		require.Equal(t, nfsPath, got.Spec.Path)
		require.Equal(t, cappName, got.OwnerReferences[0].Name)
	})

	t.Run("updates when spec differs", func(t *testing.T) {
		nm := newNFSPVCManager(newFakeClient(newNFSPVCScheme()))
		existing := newNFSPVC(nfsVolA)
		require.NoError(t, nm.K8sclient.Create(ctx, existing))

		capp := cappWithVolumes(newNFSVolume(nfsVolA, nfsPathV2))
		require.NoError(t, nm.createOrUpdate(ctx, capp))

		got := &nfspvcv1alpha1.NfsPvc{}
		require.NoError(t, nm.K8sclient.Get(ctx, keyA, got))
		require.Equal(t, nfsPathV2, got.Spec.Path)
	})

	t.Run("creates multiple NFSPVCs from spec", func(t *testing.T) {
		nm := newNFSPVCManager(newFakeClient(newNFSPVCScheme()))
		capp := cappWithVolumes(newNFSVolume(nfsVolA, nfsPath), newNFSVolume(nfsVolB, nfsPath))

		require.NoError(t, nm.createOrUpdate(ctx, capp))

		gotA := &nfspvcv1alpha1.NfsPvc{}
		require.NoError(t, nm.K8sclient.Get(ctx, keyA, gotA))

		gotB := &nfspvcv1alpha1.NfsPvc{}
		require.NoError(t, nm.K8sclient.Get(ctx, types.NamespacedName{Name: nfsVolB, Namespace: cappNamespace}, gotB))
	})
}

func TestNFSPVCManagerManage(t *testing.T) {
	ctx := context.Background()

	t.Run("reconciles when required", func(t *testing.T) {
		nm := newNFSPVCManager(newFakeClient(newNFSPVCScheme()))
		capp := cappWithVolumes(newNFSVolume(nfsVolA, nfsPath))

		require.NoError(t, nm.Manage(ctx, capp))
	})

	t.Run("cleans up when not required", func(t *testing.T) {
		fakeClient := newFakeClient(newNFSPVCScheme())
		require.NoError(t, fakeClient.Create(ctx, newNFSPVC(nfsVolA)))

		nm := newNFSPVCManager(fakeClient)
		require.NoError(t, nm.Manage(ctx, newBaseCapp()))

		got := &nfspvcv1alpha1.NfsPvc{}
		getErr := fakeClient.Get(ctx, types.NamespacedName{Name: nfsVolA, Namespace: cappNamespace}, got)
		require.True(t, errors.IsNotFound(getErr), "expected %q to not exist", nfsVolA)
	})
}

func TestNFSPVCManagerCleanUp(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes all owned resources", func(t *testing.T) {
		fakeClient := newFakeClient(newNFSPVCScheme())
		for _, vol := range []string{nfsVolA, nfsVolB} {
			require.NoError(t, fakeClient.Create(ctx, newNFSPVC(vol)))
		}

		require.NoError(t, newNFSPVCManager(fakeClient).CleanUp(ctx, newBaseCapp()))

		for _, vol := range []string{nfsVolA, nfsVolB} {
			got := &nfspvcv1alpha1.NfsPvc{}
			getErr := fakeClient.Get(ctx, types.NamespacedName{Name: vol, Namespace: cappNamespace}, got)
			require.True(t, errors.IsNotFound(getErr), "expected %q to not exist", vol)
		}
	})

	t.Run("skips delete when deleting and has owner reference", func(t *testing.T) {
		capp := newBaseCapp()
		now := metav1.Now()
		capp.DeletionTimestamp = &now

		nfspvc := newNFSPVC(nfsVolA)
		require.NoError(t, controllerutil.SetOwnerReference(&capp, nfspvc, newNFSPVCScheme()))

		nm := newNFSPVCManager(newFakeClient(newNFSPVCScheme(), nfspvc))
		require.NoError(t, nm.CleanUp(ctx, capp))

		got := &nfspvcv1alpha1.NfsPvc{}
		require.NoError(t, nm.K8sclient.Get(ctx, types.NamespacedName{Name: nfsVolA, Namespace: cappNamespace}, got))
	})
}
