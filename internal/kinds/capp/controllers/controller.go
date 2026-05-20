package controllers

import (
	"context"
	"fmt"
	"time"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	dnsrecordv1alpha1 "github.com/dana-team/provider-dns-v2/apis/namespaced/record/v1alpha1"

	loggingv1beta1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"

	"k8s.io/apimachinery/pkg/types"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knativev1 "knative.dev/serving/pkg/apis/serving/v1"
	knativev1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
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
// +kubebuilder:rbac:groups="rcs.dana.io",resources=cappconfigs,verbs=get;list;watch;
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=domainmappings,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=revisions,verbs=get;list;watch;update;create
// +kubebuilder:rbac:groups=logging.banzaicloud.io,resources=syslogngflows,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups=logging.banzaicloud.io,resources=syslogngoutputs,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;update;create
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update;create;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;update;create;patch
// +kubebuilder:rbac:groups="events.k8s.io",resources=events,verbs=get;list;watch;update;create;patch;
// +kubebuilder:rbac:groups="nfspvc.dana.io",resources=nfspvcs,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups="record.dns-v2.m.crossplane.io",resources=cnamerecords,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups="cert-manager.io",resources=certificates,verbs=get;list;watch;update;create;delete

// SetupWithManager sets up the controller with the Manager.
func (r *CappReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cappv1alpha1.Capp{},
			builder.WithPredicates(
				predicate.Or(
					predicate.GenerationChangedPredicate{},
					predicate.AnnotationChangedPredicate{},
					predicate.LabelChangedPredicate{},
				),
			),
		).
		Named(cappControllerName).
		Watches(
			&knativev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromEvent),
			builder.WithPredicates(knativeServiceWatchPredicate()),
		).
		Watches(
			&knativev1beta1.DomainMapping{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromHostname),
			builder.WithPredicates(domainMappingWatchPredicate()),
		).
		Watches(
			&cmapi.Certificate{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromHostname),
			builder.WithPredicates(certificateWatchPredicate()),
		).
		Watches(
			&dnsrecordv1alpha1.CNAMERecord{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromHostname),
			builder.WithPredicates(cnameRecordWatchPredicate()),
		).
		Watches(
			&loggingv1beta1.SyslogNGOutput{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromEvent),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(
			&loggingv1beta1.SyslogNGFlow{},
			handler.EnqueueRequestsFromMapFunc(r.findCappFromEvent),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Complete(r)
}

// knativeServiceWatchPredicate: spec changes (generation) or revision lifecycle status on the Service.
func knativeServiceWatchPredicate() predicate.Predicate {
	return predicate.Or(
		predicate.GenerationChangedPredicate{},
		predicate.TypedFuncs[client.Object]{
			UpdateFunc: func(e event.TypedUpdateEvent[client.Object]) bool {
				oldObj, okOld := e.ObjectOld.(*knativev1.Service)
				newObj, okNew := e.ObjectNew.(*knativev1.Service)
				if !okOld || !okNew {
					return false
				}
				oldS, newS := oldObj.Status, newObj.Status
				return oldS.LatestReadyRevisionName != newS.LatestReadyRevisionName ||
					oldS.LatestCreatedRevisionName != newS.LatestCreatedRevisionName
			},
		},
	)
}

// cnameRecordWatchPredicate triggers on lifecycle changes that affect Capp flow.
func cnameRecordWatchPredicate() predicate.Predicate {
	return predicate.TypedFuncs[client.Object]{
		DeleteFunc: func(_ event.TypedDeleteEvent[client.Object]) bool { return true },
		UpdateFunc: func(e event.TypedUpdateEvent[client.Object]) bool {
			oldObj, okOld := e.ObjectOld.(*dnsrecordv1alpha1.CNAMERecord)
			newObj, okNew := e.ObjectNew.(*dnsrecordv1alpha1.CNAMERecord)
			if !okOld || !okNew {
				return false
			}
			return cnameRecordConditionChanged(oldObj, newObj, xpv1.TypeReady) ||
				cnameRecordConditionChanged(oldObj, newObj, xpv1.TypeSynced)
		},
	}
}

func cnameRecordConditionChanged(
	oldObj, newObj *dnsrecordv1alpha1.CNAMERecord,
	conditionType xpv1.ConditionType,
) bool {
	oldCond := oldObj.Status.GetCondition(conditionType)
	newCond := newObj.Status.GetCondition(conditionType)
	return oldCond.Status != newCond.Status
}

// conditionStatusChanged reports whether the status value of condType differs
// between oldConds and newConds. Both type and status are compared as strings
// so this works across knative, cert-manager, and any other condition schema.
func conditionStatusChanged(oldConds, newConds []conditionPair, condType string) bool {
	find := func(conds []conditionPair) string {
		for _, c := range conds {
			if c.condType == condType {
				return c.status
			}
		}
		return ""
	}
	return find(oldConds) != find(newConds)
}

type conditionPair struct {
	condType string
	status   string
}

// domainMappingWatchPredicate triggers on spec changes (generation) or Ready condition status changes.
func domainMappingWatchPredicate() predicate.Predicate {
	return predicate.Or(
		predicate.GenerationChangedPredicate{},
		predicate.TypedFuncs[client.Object]{
			UpdateFunc: func(e event.TypedUpdateEvent[client.Object]) bool {
				oldObj, okOld := e.ObjectOld.(*knativev1beta1.DomainMapping)
				newObj, okNew := e.ObjectNew.(*knativev1beta1.DomainMapping)
				if !okOld || !okNew {
					return false
				}
				return conditionStatusChanged(
					knativeConditions(oldObj.Status.Conditions),
					knativeConditions(newObj.Status.Conditions),
					string(knativev1beta1.DomainMappingConditionReady),
				)
			},
		},
	)
}

func knativeConditions(conds duckv1.Conditions) []conditionPair {
	out := make([]conditionPair, len(conds))
	for i, c := range conds {
		out[i] = conditionPair{condType: string(c.Type), status: string(c.Status)}
	}
	return out
}

// certificateWatchPredicate triggers on spec changes (generation) or Ready condition status changes.
func certificateWatchPredicate() predicate.Predicate {
	return predicate.Or(
		predicate.GenerationChangedPredicate{},
		predicate.TypedFuncs[client.Object]{
			UpdateFunc: func(e event.TypedUpdateEvent[client.Object]) bool {
				oldObj, okOld := e.ObjectOld.(*cmapi.Certificate)
				newObj, okNew := e.ObjectNew.(*cmapi.Certificate)
				if !okOld || !okNew {
					return false
				}
				return conditionStatusChanged(
					certificateConditions(oldObj.Status.Conditions),
					certificateConditions(newObj.Status.Conditions),
					string(cmapi.CertificateConditionReady),
				)
			},
		},
	)
}

func certificateConditions(conds []cmapi.CertificateCondition) []conditionPair {
	out := make([]conditionPair, len(conds))
	for i, c := range conds {
		out[i] = conditionPair{condType: string(c.Type), status: string(c.Status)}
	}
	return out
}

// findCappFromKnative maps reconciliation requests to Capp reconciliation requests.
func (r *CappReconciler) findCappFromEvent(ctx context.Context, object client.Object) []reconcile.Request {
	request := reconcile.Request{NamespacedName: types.NamespacedName{
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
	}}

	return []reconcile.Request{request}
}

// findCappFromDomainMapping maps reconciliation requests to Capp reconciliation requests based on hostname.
func (r *CappReconciler) findCappFromHostname(ctx context.Context, object client.Object) []reconcile.Request {
	labels := object.GetLabels()
	cappName := labels[utils.CappResourceKey]
	if cappName == "" {
		return nil
	}

	request := reconcile.Request{NamespacedName: types.NamespacedName{
		Namespace: object.GetNamespace(),
		Name:      cappName,
	}}

	return []reconcile.Request{request}
}

func (r *CappReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("CappName", req.Name, "CappNamespace", req.Namespace)
	logger.Info("Starting Reconcile")
	capp := cappv1alpha1.Capp{}
	if err := r.Get(ctx, req.NamespacedName, &capp); err != nil {
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
		rmanagers.EventSources:   rmanagers.EventSourceManager{Ctx: ctx, Log: logger, K8sclient: r.Client, EventRecorder: r.EventRecorder},
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
