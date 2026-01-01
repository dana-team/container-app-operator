package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rcs "github.com/dana-team/container-app-operator/api/v1alpha1"
	shipwright "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
)

func buildNameFor(cb *rcs.CappBuild) string {
	return cb.Name + "-build"
}

func (r *CappBuildReconciler) patchReadyCondition(
	ctx context.Context,
	cb *rcs.CappBuild,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	orig := cb.DeepCopy()

	cb.Status.ObservedGeneration = cb.Generation

	meta.SetStatusCondition(&cb.Status.Conditions, metav1.Condition{
		Type:               TypeReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cb.Generation,
	})

	return r.Status().Patch(ctx, cb, client.MergeFrom(orig))
}

func (r *CappBuildReconciler) newBuild(
	cb *rcs.CappBuild,
	selectedStrategyName string,
) *shipwright.Build {
	kind := shipwright.ClusterBuildStrategyKind

	build := &shipwright.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildNameFor(cb),
			Namespace: cb.Namespace,
			Labels: map[string]string{
				rcs.GroupVersion.Group + "/parent-cappbuild": cb.Name,
			},
		},
		Spec: shipwright.BuildSpec{
			Strategy: shipwright.Strategy{
				Name: selectedStrategyName,
				Kind: &kind,
			},
			Source: &shipwright.Source{
				Type: shipwright.GitType,
				Git: &shipwright.Git{
					URL: cb.Spec.Source.Git.URL,
				},
			},
			Output: shipwright.Image{
				Image: cb.Spec.Output.Image,
			},
		},
	}

	if cb.Spec.Source.Git.Revision != "" {
		rev := cb.Spec.Source.Git.Revision
		build.Spec.Source.Git.Revision = &rev
	}
	if cb.Spec.Source.ContextDir != "" {
		cd := cb.Spec.Source.ContextDir
		build.Spec.Source.ContextDir = &cd
	}
	if cb.Spec.Source.Git.CloneSecret != nil && cb.Spec.Source.Git.CloneSecret.Name != "" {
		sec := cb.Spec.Source.Git.CloneSecret.Name
		build.Spec.Source.Git.CloneSecret = &sec
	}
	if cb.Spec.Output.PushSecret != nil && cb.Spec.Output.PushSecret.Name != "" {
		ps := cb.Spec.Output.PushSecret.Name
		build.Spec.Output.PushSecret = &ps
	}

	return build
}

// reconcileBuild ensures the Shipwright Build exists and matches desired state.
func (r *CappBuildReconciler) reconcileBuild(
	ctx context.Context,
	cb *rcs.CappBuild,
	selectedStrategyName string,
) error {
	logger := log.FromContext(ctx)

	desired := r.newBuild(cb, selectedStrategyName)

	actual := &shipwright.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      desired.Name,
			Namespace: desired.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, actual, func() error {
		if err := controllerutil.SetControllerReference(cb, actual, r.Scheme); err != nil {
			return err
		}
		if actual.Labels == nil {
			actual.Labels = map[string]string{}
		}
		for k, v := range desired.Labels {
			actual.Labels[k] = v
		}
		actual.Spec = desired.Spec
		return nil
	})
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		logger.Info("Reconciled Shipwright Build", "name", actual.Name, "operation", string(op))
	}
	return nil
}
