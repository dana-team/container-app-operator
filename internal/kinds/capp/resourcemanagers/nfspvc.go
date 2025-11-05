package resourcemanagers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/resourceclient"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	NfsPVC                    = "nfsPvc"
	eventNFSPVCCreationFailed = "NfsPvcCreationFailed"
	eventNFSPVCCreated        = "NfsPvcCreated"
)

type NFSPVCManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
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
				Labels: map[string]string{
					utils.CappResourceKey:   capp.Name,
					utils.ManagedByLabelKey: utils.CappKey,
				},
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

// CleanUp attempts to delete the associated NFSPVCs for a given Capp resource.
func (n NFSPVCManager) CleanUp(capp cappv1alpha1.Capp) error {
	resourceManager := rclient.ResourceManagerClient{Ctx: n.Ctx, K8sclient: n.K8sclient, Log: n.Log}

	for _, nfsVolume := range capp.Status.VolumesStatus.NFSVolumesStatus {
		nsfpvcVolume := rclient.GetBareNFSPVC(nfsVolume.VolumeName, capp.Namespace)

		if err := resourceManager.DeleteResource(&nsfpvcVolume); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
	}

	return nil
}

// IsRequired is responsible to determine if resource NfsPvc is required.
func (n NFSPVCManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return len(capp.Spec.VolumesSpec.NFSVolumes) > 0
}

// Manage creates or updates a NFSPVC resource based on the provided Capp if it's required.
// If it's not, then it cleans up the resource if it exists.
func (n NFSPVCManager) Manage(capp cappv1alpha1.Capp) error {
	if n.IsRequired(capp) {
		return n.createOrUpdate(capp)
	}

	return n.CleanUp(capp)
}

// createOrUpdate creates or updates a NFSPVC resource.
func (n NFSPVCManager) createOrUpdate(capp cappv1alpha1.Capp) error {
	generatedNFSPVCs := n.prepareResource(capp)
	resourceManager := rclient.ResourceManagerClient{Ctx: n.Ctx, K8sclient: n.K8sclient, Log: n.Log}

	for _, nfspvc := range generatedNFSPVCs {
		existingNFSPVC := nfspvcv1alpha1.NfsPvc{}
		if err := n.K8sclient.Get(n.Ctx, client.ObjectKey{Namespace: nfspvc.Namespace, Name: nfspvc.Name}, &existingNFSPVC); err != nil {
			if errors.IsNotFound(err) {
				if err := n.createNFSPVC(&capp, &nfspvc, resourceManager); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("failed to get NFSPVC %q: %w", nfspvc.Name, err)
			}
		} else if err := n.updateNFSPVC(existingNFSPVC, nfspvc, resourceManager); err != nil {
			return err
		}
	}

	return nil
}

// createKSVC creates a new NFSPVC and emits an event.
func (n NFSPVCManager) createNFSPVC(capp *cappv1alpha1.Capp, nfspvc *nfspvcv1alpha1.NfsPvc, resourceManager rclient.ResourceManagerClient) error {
	if err := resourceManager.CreateResource(nfspvc); err != nil {
		n.EventRecorder.Event(capp, corev1.EventTypeWarning, eventNFSPVCCreationFailed,
			fmt.Sprintf("Failed to create NFSPVC %s", nfspvc.Name))
		return err
	}

	n.EventRecorder.Event(capp, corev1.EventTypeNormal, eventNFSPVCCreated,
		fmt.Sprintf("Created NFSPVC %s", nfspvc.Name))

	return nil
}

// updateNFSPVC checks if an update to the NFSPVC is necessary and performs the update to match desired state.
func (n NFSPVCManager) updateNFSPVC(existingNFSPVC, nfspvc nfspvcv1alpha1.NfsPvc, resourceManager rclient.ResourceManagerClient) error {
	if !reflect.DeepEqual(existingNFSPVC.Spec, nfspvc.Spec) {
		existingNFSPVC.Spec = nfspvc.Spec
		return resourceManager.UpdateResource(&existingNFSPVC)
	}

	return nil
}
