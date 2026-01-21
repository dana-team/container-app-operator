package controllers

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	distref "github.com/distribution/reference"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

func buildRunNameFor(cb *rcs.CappBuild) string {
	return fmt.Sprintf("%s-buildrun-%d", cb.Name, cb.Generation)
}

func newBuildRun(cb *rcs.CappBuild) *shipwright.BuildRun {
	buildName := buildNameFor(cb)

	return &shipwright.BuildRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildRunNameFor(cb),
			Namespace: cb.Namespace,
			Labels: map[string]string{
				rcs.GroupVersion.Group + "/parent-cappbuild": cb.Name,
			},
		},
		Spec: shipwright.BuildRunSpec{
			Build: shipwright.ReferencedBuild{
				Name: &buildName,
			},
		},
	}
}

// reconcileBuildRun ensures the expected BuildRun exists for this CappBuild generation.
func (r *CappBuildReconciler) reconcileBuildRun(
	ctx context.Context,
	cb *rcs.CappBuild,
) (*shipwright.BuildRun, error) {
	desired := newBuildRun(cb)

	existing := &shipwright.BuildRun{}
	key := client.ObjectKeyFromObject(desired)
	if err := r.Get(ctx, key, existing); err == nil {
		if !metav1.IsControlledBy(existing, cb) {
			return nil, &controllerutil.AlreadyOwnedError{Object: existing}
		}
		return existing, nil
	} else if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	if err := controllerutil.SetControllerReference(cb, desired, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.Create(ctx, desired); err != nil {
		return nil, err
	}
	return desired, nil
}

func deriveBuildSucceededStatus(br *shipwright.BuildRun) (metav1.ConditionStatus, string, string) {
	succeededCondition := br.Status.GetCondition(shipwright.Succeeded)
	if succeededCondition == nil {
		return metav1.ConditionUnknown, ReasonBuildRunPending, "BuildRun has not reported status yet"
	}

	switch succeededCondition.GetStatus() {
	case corev1.ConditionTrue:
		return metav1.ConditionTrue, ReasonBuildRunSucceeded, "BuildRun succeeded"
	case corev1.ConditionFalse:
		msg := "BuildRun failed"
		if buildRunMessage := strings.TrimSpace(succeededCondition.GetMessage()); buildRunMessage != "" {
			msg = fmt.Sprintf("BuildRun failed: %s", buildRunMessage)
		}
		return metav1.ConditionFalse, ReasonBuildRunFailed, strings.TrimSpace(msg)
	default:
		msg := "BuildRun is running"
		if buildRunMessage := strings.TrimSpace(succeededCondition.GetMessage()); buildRunMessage != "" {
			msg = fmt.Sprintf("BuildRun is running: %s", buildRunMessage)
		}
		return metav1.ConditionUnknown, ReasonBuildRunRunning, strings.TrimSpace(msg)
	}
}

func hasTagOrDigest(image string) bool {
	parsed, err := distref.ParseNormalizedNamed(image)
	if err != nil {
		return false
	}
	if _, ok := parsed.(distref.Digested); ok {
		return true
	}
	return !distref.IsNameOnly(parsed)
}

func computeLatestImage(cb *rcs.CappBuild, br *shipwright.BuildRun) string {
	if br.Status.Output != nil && br.Status.Output.Digest != "" {
		return cb.Spec.Output.Image + "@" + br.Status.Output.Digest
	}
	if hasTagOrDigest(cb.Spec.Output.Image) {
		return cb.Spec.Output.Image
	}
	return ""
}

func (r *CappBuildReconciler) patchBuildSucceededCondition(
	ctx context.Context,
	cb *rcs.CappBuild,
	br *shipwright.BuildRun,
) error {
	orig := cb.DeepCopy()

	cb.Status.ObservedGeneration = cb.Generation
	cb.Status.LastBuildRunRef = br.Name

	status, reason, message := deriveBuildSucceededStatus(br)
	meta.SetStatusCondition(&cb.Status.Conditions, metav1.Condition{
		Type:               TypeBuildSucceeded,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cb.Generation,
	})

	return r.Status().Patch(ctx, cb, client.MergeFrom(orig))
}

func (r *CappBuildReconciler) patchLatestImage(
	ctx context.Context,
	cb *rcs.CappBuild,
	latestImage string,
) error {
	if latestImage == "" {
		return nil
	}

	orig := cb.DeepCopy()
	cb.Status.ObservedGeneration = cb.Generation
	cb.Status.LatestImage = latestImage
	return r.Status().Patch(ctx, cb, client.MergeFrom(orig))
}

func (r *CappBuildReconciler) ensureBuildRunOnCommit(ctx context.Context, cb *rcs.CappBuild, counter int64) (*shipwright.BuildRun, error) {
	desired := newBuildRun(cb)
	desired.Name = fmt.Sprintf("%s-buildrun-oncommit-%d", cb.Name, counter)
	desired.Labels["rcs.dana.io/build-trigger"] = "oncommit"

	existing := &shipwright.BuildRun{}
	key := client.ObjectKeyFromObject(desired)
	if err := r.Get(ctx, key, existing); err == nil {
		if !metav1.IsControlledBy(existing, cb) {
			return nil, &controllerutil.AlreadyOwnedError{Object: existing}
		}
		return existing, nil
	} else if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	if err := controllerutil.SetControllerReference(cb, desired, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.Create(ctx, desired); err != nil {
		return nil, err
	}
	return desired, nil
}
