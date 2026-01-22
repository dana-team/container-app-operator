package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	distref "github.com/distribution/reference"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

const annotationKeyLastBuildSpec = "rcs.dana.io/last-build-spec"

// buildInputs captures fields that trigger a new build when changed.
type buildInputs struct {
	Source    rcs.CappBuildSource `json:"source"`
	BuildFile rcs.CappBuildFile   `json:"buildFile"`
	Output    rcs.CappBuildOutput `json:"output"`
}

func buildRunNameFor(cb *rcs.CappBuild, counter int64) string {
	return fmt.Sprintf("%s-buildrun-%d", cb.Name, counter)
}

func newBuildRun(cb *rcs.CappBuild, counter int64) *shipwright.BuildRun {
	buildName := buildNameFor(cb)

	return &shipwright.BuildRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildRunNameFor(cb, counter),
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

func (r *CappBuildReconciler) reconcileBuildRun(
	ctx context.Context,
	cb *rcs.CappBuild,
) (*shipwright.BuildRun, error) {
	counter := nextBuildRunCounter(cb)
	desired := newBuildRun(cb, counter)

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

	orig := cb.DeepCopy()
	cb.Status.BuildRunCounter = counter
	if err := r.Status().Patch(ctx, cb, client.MergeFrom(orig)); err != nil {
		return nil, err
	}

	return desired, nil
}

func nextBuildRunCounter(cb *rcs.CappBuild) int64 {
	counter := cb.Status.BuildRunCounter
	if counter < 0 {
		counter = 0
	}
	return counter + 1
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
	desired := newBuildRun(cb, 0)
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

func (r *CappBuildReconciler) isNewBuildRequired(ctx context.Context, cb *rcs.CappBuild) bool {
	if cb.Status.LastBuildRunRef == "" {
		return true
	}

	lastSpecJson, ok := cb.Annotations[annotationKeyLastBuildSpec]
	if !ok {
		return true
	}

	var lastInputs buildInputs
	if err := json.Unmarshal([]byte(lastSpecJson), &lastInputs); err != nil {
		log.FromContext(ctx).Error(err, "Failed to unmarshal last build spec annotation", "CappBuild", cb.Name)
		return true
	}

	return !reflect.DeepEqual(cb.Spec.Source, lastInputs.Source) ||
		!reflect.DeepEqual(cb.Spec.BuildFile, lastInputs.BuildFile) ||
		!reflect.DeepEqual(cb.Spec.Output, lastInputs.Output)
}

func (r *CappBuildReconciler) recordBuildSpec(cb *rcs.CappBuild) error {
	if cb.Annotations == nil {
		cb.Annotations = make(map[string]string)
	}

	inputs := buildInputs{
		Source:    cb.Spec.Source,
		BuildFile: cb.Spec.BuildFile,
		Output:    cb.Spec.Output,
	}

	specJson, err := json.Marshal(inputs)
	if err != nil {
		return err
	}

	cb.Annotations[annotationKeyLastBuildSpec] = string(specJson)
	return nil
}
