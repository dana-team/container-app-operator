package resourcemanagers

import (
	"context"
	"fmt"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	NfsPvc                    = "NfsPvc"
	eventNFSPVCCreationFailed = "NfsPvcCreationFailed"
	eventNFSPVCCreated        = "NfsPvcCreated"
)

type NFSPVCManager struct {
	rclient.ResourceManagerClient
	EventRecorder events.EventRecorder
}

// prepareResource prepares the NfsPvc resource based on the Capp object.
func (n NFSPVCManager) prepareResource(capp cappv1alpha1.Capp) []nfspvcv1alpha1.NfsPvc {
	//nolint:prealloc
	var nfsPvcs []nfspvcv1alpha1.NfsPvc

	for _, nfsVolume := range capp.Spec.VolumesSpec.NFSVolumes {
		nfsPvc := nfspvcv1alpha1.NfsPvc{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nfsVolume.Name,
				Namespace: capp.Namespace,
				Labels:    utils.ManagedResourceLabels(capp.Name),
			},
			Spec: nfspvcv1alpha1.NfsPvcSpec{
				Server: nfsVolume.Server,
				Path:   nfsVolume.Path,
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteMany, // We use ReadWriteMany as the access mode for NFS volumes, as they are shared across multiple pods(Knative revisions, autoscaler).
				},
				Capacity: nfsVolume.Capacity,
			},
		}
		nfsPvcs = append(nfsPvcs, nfsPvc)
	}

	return nfsPvcs

}

// getPreviousNFSPVCs returns a list of all NFSPVC objects that are related to the given Capp.
func (n NFSPVCManager) getPreviousNFSPVCs(ctx context.Context, capp cappv1alpha1.Capp) (nfspvcv1alpha1.NfsPvcList, error) {
	nfsPvcs := nfspvcv1alpha1.NfsPvcList{}
	if err := listManagedResources(ctx, n.K8sClient, capp, &nfsPvcs, NfsPvc, nil); err != nil {
		return nfsPvcs, err
	}
	return nfsPvcs, nil
}

// CleanUp attempts to delete the associated NFSPVCs for a given Capp resource.
func (n NFSPVCManager) CleanUp(ctx context.Context, capp cappv1alpha1.Capp) error {
	nfsPvcs, err := n.getPreviousNFSPVCs(ctx, capp)
	if err != nil {
		return err
	}
	resources := make([]*nfspvcv1alpha1.NfsPvc, len(nfsPvcs.Items))
	for i := range nfsPvcs.Items {
		resources[i] = &nfsPvcs.Items[i]
	}
	return deleteOwnedResources(ctx, n.K8sClient, &capp, resources)
}

// IsRequired is responsible to determine if resource NfsPvc is required.
func (n NFSPVCManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return len(capp.Spec.VolumesSpec.NFSVolumes) > 0
}

// Manage creates or updates a NFSPVC resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (n NFSPVCManager) Manage(ctx context.Context, capp cappv1alpha1.Capp) error {
	if n.IsRequired(capp) {
		return n.createOrUpdate(ctx, capp)
	}

	return n.CleanUp(ctx, capp)
}

// createOrUpdate creates or updates a NFSPVC resource.
func (n NFSPVCManager) createOrUpdate(ctx context.Context, capp cappv1alpha1.Capp) error {
	generatedNFSPVCs := n.prepareResource(capp)

	for i := range generatedNFSPVCs {
		nfspvc := &generatedNFSPVCs[i]
		existingNFSPVC := nfspvcv1alpha1.NfsPvc{}
		if err := n.K8sClient.Get(ctx, client.ObjectKey{Namespace: nfspvc.Namespace, Name: nfspvc.Name}, &existingNFSPVC); err != nil {
			if errors.IsNotFound(err) {
				if err := createManagedResource(ctx, n.K8sClient, n.CreateResource, n.EventRecorder, &capp, nfspvc,
					NfsPvc, eventNFSPVCCreated, eventNFSPVCCreationFailed); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("failed to get NFSPVC %q: %w", nfspvc.Name, err)
			}
		} else {
			orig := existingNFSPVC.DeepCopy()
			existingNFSPVC.Spec = *nfspvc.Spec.DeepCopy()
			if err := ensureOwnerReference(n.K8sClient, &capp, &existingNFSPVC, NfsPvc); err != nil {
				return err
			}
			if err := updateManagedResourceIfNeeded(ctx, n.UpdateResource, &existingNFSPVC, orig.Spec, existingNFSPVC.Spec, orig.OwnerReferences); err != nil {
				return err
			}
		}
	}

	return nil
}
