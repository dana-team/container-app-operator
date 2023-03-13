package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	status_utils "github.com/dana-team/container-app-operator/internals/utils/status"
	"k8s.io/apimachinery/pkg/types"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	utils "github.com/dana-team/container-app-operator/internals/utils"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// CappReconciler reconciles a Capp object
type CappReconciler struct {
	Log logr.Logger
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=rcs.dana.io,resources=capps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rcs.dana.io,resources=capps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rcs.dana.io,resources=capps/finalizers,verbs=update

func (r *CappReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)
	capp := rcsv1alpha1.Capp{}
	if err := r.Client.Get(ctx, req.NamespacedName, &capp); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	err, deleted := utils.HandleResourceDeletion(ctx, capp, r.Log, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	if deleted {
		return ctrl.Result{}, nil
	}
	if err := utils.EnsureFinalizer(ctx, capp, r.Client); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.SyncApplication(ctx, capp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *CappReconciler) findCappFromKnative(knativeService client.Object) []reconcile.Request {
	cappList := &rcsv1alpha1.CappList{}
	listOps := &client.ListOptions{
		Namespace: knativeService.GetNamespace(),
	}
	err := r.List(context.TODO(), cappList, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(cappList.Items))
	for i, item := range cappList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *CappReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rcsv1alpha1.Capp{}).
		Watches(
			&source.Kind{Type: &knativev1.Service{}},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromKnative),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *CappReconciler) SyncApplication(ctx context.Context, capp rcsv1alpha1.Capp) error {
	if err := utils.CreateOrUpdateKnativeService(ctx, capp, r.Client, r.Log); err != nil {
		return err
	}
	if capp.Spec.RouteSpec.Hostname != "" {
		if err := utils.CreateOrUpdateKnativeDomainMapping(ctx, capp, r.Client, r.Log); err != nil {
			return err
		}
	}
	if err := status_utils.SyncStatus(ctx, capp, r.Log, r.Client); err != nil {
		return err
	}
	return nil
}
