package webhooks

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dana-team/rcs-ocm-deployer/api/v1alpha1"

	"github.com/dana-team/rcs-ocm-deployer/internal/utils"
	corev1 "k8s.io/api/core/v1"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type CappMutator struct {
	Client  client.Client
	Decoder admission.Decoder
}

// +kubebuilder:webhook:path=/mutate-capp,mutating=true,sideEffects=NoneOnDryRun,failurePolicy=fail,groups=rcs.dana.io,resources=capps,verbs=create;update,versions=v1alpha1,name=capp.dana.io,admissionReviewVersions=v1;v1beta1

var (
	lastUpdatedByAnnotationKey = utils.RCSAPIGroup + "/last-updated-by"
)

const (
	MutatorServingPath              = "/mutate-capp"
	ExcludedServiceAccountNamespace = "rcs-deployer-system"
	usernamePartsCount              = 4 // Number of parts in the service account username
	namespaceIndex                  = 2 // Index for the namespace part in the username
)

// Handle implements the mutation webhook.
func (c *CappMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := log.FromContext(ctx).WithValues("mutation webhook", "capp mutation Webhook", "Name", req.Name)

	capp := cappv1alpha1.Capp{}
	if err := c.Decoder.DecodeRaw(req.Object, &capp); err != nil {
		logger.Error(err, "could not decode capp object")
		return admission.Errored(http.StatusBadRequest, err)
	}

	rcsConfig, err := getRCSConfig(ctx, c.Client)
	if err != nil {
		logger.Error(err, "failed to get RCS Config")
		admission.Errored(http.StatusInternalServerError, err)
	}

	c.handle(&capp, rcsConfig, req.UserInfo.Username)

	marshaledCapp, err := json.Marshal(capp)
	if err != nil {
		logger.Error(err, "could not marshal capp object")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledCapp)
}

// handle implements the main mutating logic. It modifies the annotations and resources of
// a Capp based on requester data and RCS Config.
func (c *CappMutator) handle(capp *cappv1alpha1.Capp, rcsConfig *v1alpha1.RCSConfig, username string) {
	mutateAnnotations(capp, username)
	mutateResources(capp, rcsConfig.Spec.DefaultResources)
}

// mutateAnnotations adds a last-updated-by annotation, indicating the username who last updated the Capp.
func mutateAnnotations(capp *cappv1alpha1.Capp, username string) {
	if capp.ObjectMeta.Annotations == nil {
		capp.ObjectMeta.Annotations = make(map[string]string)
	}

	if !isExcludedServiceAccount(username) {
		capp.ObjectMeta.Annotations[lastUpdatedByAnnotationKey] = username
	}
}

// mutateResources sets default values for the Capp container resources, if such do not already exist.
func mutateResources(capp *cappv1alpha1.Capp, defaultResources corev1.ResourceRequirements) {
	resources := []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory}

	for _, container := range capp.Spec.ConfigurationSpec.Template.Spec.Containers {
		setResourceQuantity(&container.Resources.Requests, defaultResources.Requests, resources)
		setResourceQuantity(&container.Resources.Limits, defaultResources.Limits, resources)
	}
}

// setResourceQuantity sets the resource requirement if it is not already set.
func setResourceQuantity(resourceList *corev1.ResourceList, defaultResources corev1.ResourceList, resourceNames []corev1.ResourceName) {
	for _, resourceName := range resourceNames {
		if defaultQuantity, ok := defaultResources[resourceName]; ok {
			if *resourceList == nil {
				*resourceList = corev1.ResourceList{}
			}

			if _, ok = (*resourceList)[resourceName]; !ok {
				(*resourceList)[resourceName] = defaultQuantity
			}
		}
	}
}

// isExcludedServiceAccount checks if the given username is a service account in the rcs-deployer-system namespace.
func isExcludedServiceAccount(username string) bool {
	parts := strings.Split(username, ":")
	if len(parts) == usernamePartsCount && parts[namespaceIndex] == ExcludedServiceAccountNamespace {
		return true
	}
	return false
}