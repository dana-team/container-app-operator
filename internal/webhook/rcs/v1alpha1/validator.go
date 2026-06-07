package webhooks

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"net/http"

	"github.com/cloudevents/sdk-go/v2/event"
	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/webhook/rcs/common"
	admissionv1 "k8s.io/api/admission/v1"
	sourcesv1 "knative.dev/eventing/pkg/apis/sources/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	eventSourcePath = "spec.eventSourcesSpec.sources"
)

type CappValidator struct {
	Client  client.Client
	Decoder admission.Decoder
	Log     logr.Logger
}

// +kubebuilder:webhook:path=/validate-capp,mutating=false,sideEffects=None,failurePolicy=fail,groups="rcs.dana.io",resources=capps,verbs=create;update,versions=v1alpha1,name=capp.validate.rcs.dana.io,admissionReviewVersions=v1;v1beta1

func (c *CappValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := log.FromContext(ctx).WithValues("webhook", "capp Webhook", "Name", req.Name)
	logger.Info("Webhook request received")

	capp := cappv1alpha1.Capp{}
	if err := c.Decoder.DecodeRaw(req.Object, &capp); err != nil {
		logger.Error(err, "could not decode capp object")
		return admission.Errored(http.StatusBadRequest, err)
	}

	var oldCapp *cappv1alpha1.Capp
	if req.Operation == admissionv1.Update {
		oldCapp = &cappv1alpha1.Capp{}
		err := c.Decoder.DecodeRaw(req.OldObject, oldCapp)
		if err != nil {
			logger.Error(err, "could not decode old capp object")
			return admission.Errored(http.StatusBadRequest, err)
		}
	}

	return c.handle(ctx, req.Operation, capp, oldCapp)
}

func (c *CappValidator) handle(ctx context.Context, operation admissionv1.Operation, capp cappv1alpha1.Capp, oldCapp *cappv1alpha1.Capp) admission.Response {
	config, err := common.GetCappConfig(ctx, c.Client)
	if err != nil {
		return admission.Denied("Failed to fetch CappConfig")
	}

	var allowedHostnamePatterns []cappv1alpha1.HostnamePattern
	if config.Spec.AllowedHostnamePatterns != nil {
		allowedHostnamePatterns = config.Spec.AllowedHostnamePatterns
	}

	if err := validateHostnameImmutability(operation, capp, oldCapp); err != nil {
		return admission.Denied(err.Error())
	}

	if operation == admissionv1.Create || capp.Spec.RouteSpec.Hostname != oldCapp.Spec.RouteSpec.Hostname {
		if errs := common.ValidateDomainName(capp.Spec.RouteSpec.Hostname, allowedHostnamePatterns); errs != nil {
			return admission.Denied(errs.Error())
		}
		taken, err := common.IsDomainNameTaken(ctx, capp.Spec.RouteSpec.Hostname)
		if err != nil {
			return admission.Denied(fmt.Sprintf("hostname check error: %v", err))
		}
		if taken {
			return admission.Denied(fmt.Sprintf("invalid name %q: hostname must be unique and not already taken", capp.Spec.RouteSpec.Hostname))
		}
	}

	if err := validateNFSVolumeMounts(capp); err != nil {
		return admission.Denied(err.Error())
	}

	if err := validateEventSources(ctx, capp); err != nil {
		return admission.Denied(err.Error())
	}

	minReplicas := capp.Spec.ScaleSpec.MinReplicas
	scaleDelay := capp.Spec.ScaleSpec.ScaleDelaySeconds

	if minReplicas > config.Spec.AutoscaleConfig.MinReplicasLimit {
		return admission.Denied(fmt.Sprintf("invalid minReplicas %d: must be less than or equal to global min scale %d", minReplicas, config.Spec.AutoscaleConfig.MinReplicasLimit))
	}

	if scaleDelay > config.Spec.AutoscaleConfig.MaxScaleDelay {
		return admission.Denied(fmt.Sprintf("invalid scaleDelaySeconds %d: must be less than or equal to global max scale delay %d", scaleDelay, config.Spec.AutoscaleConfig.MaxScaleDelay))
	}
	return admission.Allowed("")
}

func validateHostnameImmutability(operation admissionv1.Operation, capp cappv1alpha1.Capp, oldCapp *cappv1alpha1.Capp) error {
	if operation != admissionv1.Update {
		return nil
	}

	oldHostname := oldCapp.Spec.RouteSpec.Hostname
	newHostname := capp.Spec.RouteSpec.Hostname
	if oldHostname == newHostname || oldHostname == "" {
		return nil
	}

	return fmt.Errorf("spec.routeSpec.hostname is immutable once set")
}

func validateNFSVolumeMounts(capp cappv1alpha1.Capp) error {
	if len(capp.Spec.VolumesSpec.NFSVolumes) == 0 {
		return nil
	}

	mountedVolumes := make(map[string]struct{})
	for _, container := range capp.Spec.ConfigurationSpec.Template.Spec.Containers {
		for _, volumeMount := range container.VolumeMounts {
			mountedVolumes[volumeMount.Name] = struct{}{}
		}
	}

	missingVolumes := make(map[string]struct{})
	for _, nfsVolume := range capp.Spec.VolumesSpec.NFSVolumes {
		if _, ok := mountedVolumes[nfsVolume.Name]; ok {
			continue
		}
		missingVolumes[nfsVolume.Name] = struct{}{}
	}

	if len(missingVolumes) == 0 {
		return nil
	}

	missingVolumeNames := slices.Collect(maps.Keys(missingVolumes))
	slices.Sort(missingVolumeNames)

	return fmt.Errorf("invalid nfsVolumes: volumes [%s] must be mounted by at least one container", strings.Join(missingVolumeNames, ", "))
}

func validateEventSources(ctx context.Context, capp cappv1alpha1.Capp) error {
	seen := make(map[string]struct{})
	for i, src := range capp.Spec.EventSourcesSpec.Sources {
		if _, dup := seen[src.Name]; dup {
			return fmt.Errorf("%s[%d].name: duplicate value %q", eventSourcePath, i, src.Name)
		}
		seen[src.Name] = struct{}{}

		if src.PingSourceConfiguration != nil {
			if err := validatePingSourceConfiguration(ctx, src.PingSourceConfiguration); err != nil {
				return fmt.Errorf("%s[%d]: %w", eventSourcePath, i, err)
			}
		} else {
			return fmt.Errorf("%s[%d]: source %q must specify at least one source configuration (e.g. pingSourceConfiguration)", eventSourcePath, i, src.Name)
		}

		if src.URI != nil {
			if err := common.ValidateURI(src.URI); err != nil {
				return fmt.Errorf("%s[%d].uri: %w", eventSourcePath, i, err)
			}
		}
	}
	return nil
}

// validatePingSourceConfiguration makes sure a pingSource has a valid cron schedule and that the data field (if specified) is valid JSON.
func validatePingSourceConfiguration(ctx context.Context, cfg *cappv1alpha1.PingSourceConfiguration) error {
	schedule := cfg.Schedule
	if schedule == "" {
		schedule = "* * * * *"
	}
	ps := sourcesv1.PingSourceSpec{
		SourceSpec: duckv1.SourceSpec{
			Sink: duckv1.Destination{
				Ref: &duckv1.KReference{
					APIVersion: servingv1.SchemeGroupVersion.String(),
					Kind:       "Service",
					Name:       "placeholder",
				},
			},
		},
		Schedule:    schedule,
		Data:        cfg.Data,
		ContentType: event.ApplicationJSON,
	}

	if err := ps.Validate(ctx); err != nil {
		return err
	}
	return nil
}
