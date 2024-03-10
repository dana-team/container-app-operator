package controllers

import (
	"context"
	"fmt"
	"github.com/dana-team/container-app-operator/internals/status"
	"time"

	"github.com/dana-team/container-app-operator/internals/finalizer"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internals/resource-managers"
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

const RequeueTime = 5 * time.Second

// CappReconciler reconciles a Capp object
type CappReconciler struct {
	Log logr.Logger
	client.Client
	Scheme        *runtime.Scheme
	OnOpenshift   bool
	EventRecorder record.EventRecorder
}

// +kubebuilder:rbac:groups=rcs.dana.io,resources=capps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rcs.dana.io,resources=capps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rcs.dana.io,resources=capps/finalizers,verbs=update
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=domainmappings,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=revisions,verbs=get;list;watch;update;create
// +kubebuilder:rbac:groups=logging.banzaicloud.io,resources=flows,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups=logging.banzaicloud.io,resources=outputs,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;update;create
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;update;create;
// +kubebuilder:rbac:groups="events.k8s.io",resources=events,verbs=get;list;watch;update;create;

func (r *CappReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("CappName", req.Name, "CappNamespace", req.Namespace)
	logger.Info("Starting Reconcile")
	capp := rcsv1alpha1.Capp{}
	if err := r.Client.Get(ctx, req.NamespacedName, &capp); err != nil {
		if errors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("Didn't find Capp: %s, from the namespace: %s", capp.Name, capp.Namespace))
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get Capp: %s", err.Error())
	}

	resourceManagers := map[string]rmanagers.ResourceManager{
		rmanagers.DomainMapping:  rmanagers.KnativeDomainMappingManager{Ctx: ctx, Log: logger, K8sclient: r.Client, EventRecorder: r.EventRecorder},
		rmanagers.KnativeServing: rmanagers.KnativeServiceManager{Ctx: ctx, Log: logger, K8sclient: r.Client, EventRecorder: r.EventRecorder},
		rmanagers.Flow:           rmanagers.FlowManager{Ctx: ctx, Log: logger, K8sclient: r.Client, EventRecorder: r.EventRecorder},
		rmanagers.Output:         rmanagers.OutputManager{Ctx: ctx, Log: logger, K8sclient: r.Client, EventRecorder: r.EventRecorder},
	}

	err, deleted := finalizer.HandleResourceDeletion(ctx, capp, r.Client, resourceManagers)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to handle Capp deletion: %s", err.Error())
	}
	if deleted {
		return ctrl.Result{}, nil
	}
	if err := finalizer.EnsureFinalizer(ctx, capp, r.Client); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure finalizer in Capp: %s", err.Error())
	}
	if err := r.SyncApplication(ctx, capp, resourceManagers, logger); err != nil {
		if errors.IsConflict(err) {
			logger.Info(fmt.Sprintf("Conflict detected requeuing: %s", err.Error()))
			return ctrl.Result{RequeueAfter: RequeueTime}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to sync Capp: %s", err.Error())
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

// SyncApplication manages the lifecycle of Capp.
// It ensures all manifests are applied according to the specification and synchronizes the status accordingly.
func (r *CappReconciler) SyncApplication(ctx context.Context, capp rcsv1alpha1.Capp, resourceManagers map[string]rmanagers.ResourceManager, logger logr.Logger) error {
	for _, manager := range resourceManagers {
		if err := manager.CreateOrUpdateObject(capp); err != nil {
			return err
		}
	}
	if err := status.SyncStatus(ctx, capp, logger, r.Client, r.OnOpenshift, resourceManagers); err != nil {
		return err
	}
	return nil
}
