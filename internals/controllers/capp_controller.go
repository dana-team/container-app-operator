package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internals/resource-managers"
	finalizer_utils "github.com/dana-team/container-app-operator/internals/utils/finalizer"
	status_utils "github.com/dana-team/container-app-operator/internals/utils/status"
	"k8s.io/apimachinery/pkg/types"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// CappReconciler reconciles a Capp object
type CappReconciler struct {
	Log logr.Logger
	client.Client
	Scheme      *runtime.Scheme
	OnOpenshift bool
}

// +kubebuilder:rbac:groups=rcs.dana.io,resources=capps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rcs.dana.io,resources=capps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rcs.dana.io,resources=capps/finalizers,verbs=update
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;update;create
// +kubebuilder:rbac:groups=serving.knative.dev,resources=domainmappings,verbs=get;list;watch;update;create
// +kubebuilder:rbac:groups=serving.knative.dev,resources=revisions,verbs=get;list;watch;update;create
// +kubebuilder:rbac:groups=logging.banzaicloud.io,resources=flows,verbs=get;list;watch;update;create
// +kubebuilder:rbac:groups=logging.banzaicloud.io,resources=outputs,verbs=get;list;watch;update;create
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;update;create

func (r *CappReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)
	capp := rcsv1alpha1.Capp{}
	if err := r.Client.Get(ctx, req.NamespacedName, &capp); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	resourceManagers := []rmanagers.ResourceManager{
		rmanagers.KnativeDomainMappingManager{Ctx: ctx, Log: r.Log, K8sclient: r.Client},
		rmanagers.KnativeServiceManager{Ctx: ctx, Log: r.Log, K8sclient: r.Client},
		rmanagers.FlowManager{Ctx: ctx, Log: r.Log, K8sclient: r.Client},
		rmanagers.OutputManager{Ctx: ctx, Log: r.Log, K8sclient: r.Client}}
	err, deleted := finalizer_utils.HandleResourceDeletion(ctx, capp, r.Log, r.Client, resourceManagers)
	if err != nil {
		return ctrl.Result{}, err
	}
	if deleted {
		return ctrl.Result{}, nil
	}
	if err := finalizer_utils.EnsureFinalizer(ctx, capp, r.Client); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.SyncApplication(ctx, capp, resourceManagers); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *CappReconciler) findCappFromKnative(ctx context.Context, knativeService client.Object) []reconcile.Request {
	cappList := &rcsv1alpha1.CappList{}
	listOps := &client.ListOptions{
		Namespace: knativeService.GetNamespace(),
	}
	err := r.List(ctx, cappList, listOps)
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
			&knativev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromKnative),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *CappReconciler) SyncApplication(ctx context.Context, capp rcsv1alpha1.Capp, resourceManagers []rmanagers.ResourceManager) error {
	for _, manager := range resourceManagers {
		if err := manager.CreateOrUpdateObject(capp); err != nil {
			return err
		}
	}
	if err := status_utils.SyncStatus(ctx, capp, r.Log, r.Client, r.OnOpenshift); err != nil {
		return err
	}
	if capp.Spec.LogSpec != (rcsv1alpha1.LogSpec{}) {
		if err := status_utils.SyncLoggingStatus(ctx, capp, r.Log, r.Client); err != nil {
			return err
		}
	}
	return nil
}
