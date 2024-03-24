package status

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	nfspvcv1alpha1 "github.com/dana-team/nfspvc-operator/api/v1alpha1"
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
