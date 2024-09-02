package controllers

import (
	"context"
	"fmt"
	"time"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	dnsrecordv1alpha1 "github.com/dana-team/provider-dns/apis/record/v1alpha1"

	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"

	"k8s.io/apimachinery/pkg/types"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/status"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/finalizer"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	rmanagers "github.com/dana-team/container-app-operator/internal/kinds/capp/resourcemanagers"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	cappControllerName = "CappController"
	RequeueTime        = 5 * time.Second
)

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
// +kubebuilder:rbac:groups=logging.banzaicloud.io,resources=syslogngflows,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups=logging.banzaicloud.io,resources=syslogngoutputs,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;update;create
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;update;create;patch;
// +kubebuilder:rbac:groups="events.k8s.io",resources=events,verbs=get;list;watch;update;create;patch
// +kubebuilder:rbac:groups="nfspvc.dana.io",resources=nfspvcs,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups="record.dns.crossplane.io",resources=cnamerecords,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups="cert-manager.io",resources=certificates,verbs=get;list;watch;update;create;delete

// SetupWithManager sets up the controller with the Manager.
func (r *CappReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cappv1alpha1.Capp{}).
		Named(cappControllerName).
		Watches(
			&knativev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromEvent),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&knativev1beta1.DomainMapping{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromHostname),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&cmapi.Certificate{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromHostname),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&dnsrecordv1alpha1.CNAMERecord{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromHostname),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&loggingv1beta1.SyslogNGOutput{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromEvent),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&loggingv1beta1.SyslogNGFlow{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromEvent),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

// findCappFromKnative maps reconciliation requests to Capp reconciliation requests.
func (r *CappReconciler) findCappFromEvent(ctx context.Context, object client.Object) []reconcile.Request {
	request := reconcile.Request{NamespacedName: types.NamespacedName{
		Namespace: object.GetNamespace(),
		Name:      object.GetName()}}

	return []reconcile.Request{request}
}

// findCappFromDomainMapping maps reconciliation requests to Capp reconciliation requests based on hostname.
func (r *CappReconciler) findCappFromHostname(ctx context.Context, object client.Object) []reconcile.Request {
	labels := object.GetLabels()

	request := reconcile.Request{NamespacedName: types.NamespacedName{
		Namespace: object.GetNamespace(),
		Name:      labels[utils.CappResourceKey]}}

	return []reconcile.Request{request}
}

func (r *CappReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("CappName", req.Name, "CappNamespace", req.Namespace)
	logger.Info("Starting Reconcile")
	capp := cappv1alpha1.Capp{}
	if err := r.Client.Get(ctx, req.NamespacedName, &capp); err != nil {
		if errors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("Didn't find Capp: %s, from the namespace: %s", capp.Name, capp.Namespace))
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get Capp: %s", err.Error())
	}

	resourceManagers := map[string]rmanagers.ResourceManager{
		rmanagers.KnativeServing: rmanagers.KnativeServiceManager{Ctx: ctx, Log: logger, K8sclient: r.Client, EventRecorder: r.EventRecorder},
		rmanagers.DNSRecord:      rmanagers.DNSRecordManager{Ctx: ctx, Log: logger, K8sclient: r.Client, EventRecorder: r.EventRecorder},
		rmanagers.Certificate:    rmanagers.CertificateManager{Ctx: ctx, Log: logger, K8sclient: r.Client, EventRecorder: r.EventRecorder},
		rmanagers.DomainMapping:  rmanagers.KnativeDomainMappingManager{Ctx: ctx, Log: logger, K8sclient: r.Client, EventRecorder: r.EventRecorder},
		rmanagers.SyslogNGFlow:   rmanagers.SyslogNGFlowManager{Ctx: ctx, Log: logger, K8sclient: r.Client, EventRecorder: r.EventRecorder},
		rmanagers.SyslogNGOutput: rmanagers.SyslogNGOutputManager{Ctx: ctx, Log: logger, K8sclient: r.Client, EventRecorder: r.EventRecorder},
		rmanagers.NfsPVC:         rmanagers.NFSPVCManager{Ctx: ctx, Log: logger, K8sclient: r.Client, EventRecorder: r.EventRecorder},
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
			logger.Info(fmt.Sprintf("Conflict detected, requeuing: %s", err.Error()))
			return ctrl.Result{RequeueAfter: RequeueTime}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to sync Capp: %s", err.Error())
	}
	return ctrl.Result{}, nil
}

// SyncApplication manages the lifecycle of Capp.
// It ensures all manifests are applied according to the specification and synchronizes the status accordingly.
func (r *CappReconciler) SyncApplication(ctx context.Context, capp cappv1alpha1.Capp, resourceManagers map[string]rmanagers.ResourceManager, logger logr.Logger) error {
	for _, manager := range resourceManagers {
		if err := manager.Manage(capp); err != nil {
			return err
		}
	}

	if err := status.SyncStatus(ctx, capp, logger, r.Client, r.OnOpenshift, resourceManagers); err != nil {
		return err
	}
	return nil
}
