package resourceprepares

import (
	"context"
	"fmt"
	"reflect"

	rclient "github.com/dana-team/container-app-operator/internal/kinds/capp/wrappers"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NFSPVCManager struct {
	Ctx           context.Context
	K8sclient     client.Client
	Log           logr.Logger
	EventRecorder record.EventRecorder
}

// prepareResource prepares the NfsPvc resource based on the Capp object.
func (n NFSPVCManager) prepareResource(capp cappv1alpha1.Capp) []nfspvcv1alpha1.NfsPvc {
	nfsPvcs := []nfspvcv1alpha1.NfsPvc{}
	for _, nfsVolume := range capp.Spec.VolumesSpec.NFSVolumes {
		nfsPvc := nfspvcv1alpha1.NfsPvc{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nfsVolume.Name,
				Namespace: capp.Namespace,
				Labels: map[string]string{
					CappResourceKey: capp.Name,
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

// CleanUp deletes the NFSPVC resource associated with the Capp object.
// The NFSPVC resource is deleted by calling the DeleteResource method of the resourceManager object.
func (n NFSPVCManager) CleanUp(capp cappv1alpha1.Capp) error {
	resourceManager := rclient.ResourceBaseManagerClient{Ctx: n.Ctx, K8sclient: n.K8sclient, Log: n.Log}
	for _, NFSVolume := range capp.Status.VolumesStatus.NFSVolumesStatus {
		NFSVolumeObj := nfspvcv1alpha1.NfsPvc{}
		if err := resourceManager.DeleteResource(&NFSVolumeObj, NFSVolume.VolumeName, capp.Namespace); err != nil {
			return fmt.Errorf("unable to delete nfsVolume %q: %w ", NFSVolume.VolumeName, err)
		}
	}

	return nil
}

// IsRequired is responsible to determine if resource NfsPvc is required.
func (n NFSPVCManager) IsRequired(capp cappv1alpha1.Capp) bool {
	return len(capp.Spec.VolumesSpec.NFSVolumes) > 0
}

// CreateOrUpdateObject creates or updates a nfspvc object based on the provided capp.
// It returns an error if any operation fails.
func (n NFSPVCManager) CreateOrUpdateObject(capp cappv1alpha1.Capp) error {
	if n.IsRequired(capp) {
		generatedNFSPVCs := n.prepareResource(capp)
		resourceManager := rclient.ResourceBaseManagerClient{Ctx: n.Ctx, K8sclient: n.K8sclient, Log: n.Log}
		for _, NFSPVC := range generatedNFSPVCs {
			existingNFSPVC := nfspvcv1alpha1.NfsPvc{}
			if err := n.K8sclient.Get(n.Ctx, client.ObjectKey{Namespace: NFSPVC.Namespace, Name: NFSPVC.Name}, &existingNFSPVC); err != nil {
				if errors.IsNotFound(err) {
					if err := resourceManager.CreateResource(&NFSPVC); err != nil {
						n.EventRecorder.Event(&capp, corev1.EventTypeWarning, eventNFSPVCCreationFailed, fmt.Sprintf("nfsPVC %s creation failed", NFSPVC.Name))
						return fmt.Errorf("unable to create  nfsPVC %q: %w", NFSPVC.Name, err)
					} else {
						n.EventRecorder.Event(&capp, corev1.EventTypeNormal, eventNFSPVCCreated, fmt.Sprintf("nfsPVC %s created", NFSPVC.Name))
					}
				} else {
					return fmt.Errorf("unable to get nfsPvc %q: %w", NFSPVC.Name, err)
				}
			} else {
				if !reflect.DeepEqual(existingNFSPVC.Spec, NFSPVC.Spec) {
					existingNFSPVC.Spec = NFSPVC.Spec
					n.Log.Info("Trying to update the current nfsPvc")
					if err := resourceManager.UpdateResource(&existingNFSPVC); err != nil {
						return fmt.Errorf("unable to update nfsPVC %q: %w", NFSPVC.Name, err)
					}
				}

			}

		}
	}
	return nil
}
