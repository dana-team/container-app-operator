package status

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// buildVolumesStatus constructs the Volumes Status of the Capp object in accordance to the status of the corresponding nfsPVC object if such exists.
func buildVolumesStatus(ctx context.Context, kubeClient client.Client, capp cappv1alpha1.Capp, isRequired bool) (cappv1alpha1.VolumesStatus, error) {
	volumesStatus := cappv1alpha1.VolumesStatus{}

	if !isRequired {
		return volumesStatus, nil
	}

	for _, NFSPVC := range capp.Spec.VolumesSpec.NFSVolumes {
		NFSPVCObj := nfspvcv1alpha1.NfsPvc{}
		if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: capp.Namespace, Name: NFSPVC.Name}, &NFSPVCObj); err != nil {
			if apierrors.IsNotFound(err) {
				volumesStatus.NFSVolumesStatus = append(volumesStatus.NFSVolumesStatus, cappv1alpha1.NFSVolumeStatus{
					VolumeName:   NFSPVC.Name,
					NFSPVCStatus: nfspvcv1alpha1.NfsPvcStatus{},
				})
				continue
			}
			return volumesStatus, err
		}

		NFSPVCStatus := cappv1alpha1.NFSVolumeStatus{
			VolumeName:   NFSPVC.Name,
			NFSPVCStatus: NFSPVCObj.Status,
		}
		volumesStatus.NFSVolumesStatus = append(volumesStatus.NFSVolumesStatus, NFSPVCStatus)
	}

	return volumesStatus, nil
}

// checkVolumes returns why an NFS volume is not ready, or nil when all are ready.
func checkVolumes(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp, _ *cappv1alpha1.CappConfig) (*metav1.Condition, error) {
	for _, volume := range capp.Spec.VolumesSpec.NFSVolumes {
		pvc := &nfspvcv1alpha1.NfsPvc{}
		key := types.NamespacedName{Namespace: capp.Namespace, Name: volume.Name}
		if cond, err := getChildResource(ctx, k8sClient, rmanagers.NfsPvc, key, pvc); cond != nil || err != nil {
			return cond, err
		}

		ready := meta.FindStatusCondition(pvc.Status.Conditions, nfspvcv1alpha1.ConditionReady)
		if ready == nil || isConditionStale(ready.ObservedGeneration, pvc) {
			return newPendingCondition(rmanagers.NfsPvc, pvc.Name), nil
		}
		if cond := formatChildCondition(rmanagers.NfsPvc, pvc.Name, ready.Status, ready.Reason, ready.Message); cond != nil {
			return cond, nil
		}
	}
	return nil, nil
}
