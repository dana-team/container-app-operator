/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	rcsv1alpha1 "github.com/dana-team/container-app-operator/api/rcs/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var cappconfiglog = logf.Log.WithName("cappconfig-resource")

// SetupCappConfigWebhookWithManager registers the webhook for CappConfig in the manager.
func SetupCappConfigWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&rcsv1alpha1.CappConfig{}).
		WithValidator(&CappConfigCustomValidator{}).
		WithDefaulter(&CappConfigCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-rcs-rcs-dana-io-v1alpha1-cappconfig,mutating=true,failurePolicy=fail,sideEffects=None,groups=rcs.rcs.dana.io,resources=cappconfigs,verbs=create;update,versions=v1alpha1,name=mcappconfig-v1alpha1.kb.io,admissionReviewVersions=v1

// CappConfigCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind CappConfig when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type CappConfigCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &CappConfigCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind CappConfig.
func (d *CappConfigCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	cappconfig, ok := obj.(*rcsv1alpha1.CappConfig)

	if !ok {
		return fmt.Errorf("expected an CappConfig object but got %T", obj)
	}
	cappconfiglog.Info("Defaulting for CappConfig", "name", cappconfig.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-rcs-rcs-dana-io-v1alpha1-cappconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=rcs.rcs.dana.io,resources=cappconfigs,verbs=create;update,versions=v1alpha1,name=vcappconfig-v1alpha1.kb.io,admissionReviewVersions=v1

// CappConfigCustomValidator struct is responsible for validating the CappConfig resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type CappConfigCustomValidator struct {
	//TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &CappConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type CappConfig.
func (v *CappConfigCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cappconfig, ok := obj.(*rcsv1alpha1.CappConfig)
	if !ok {
		return nil, fmt.Errorf("expected a CappConfig object but got %T", obj)
	}
	cappconfiglog.Info("Validation for CappConfig upon creation", "name", cappconfig.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type CappConfig.
func (v *CappConfigCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	cappconfig, ok := newObj.(*rcsv1alpha1.CappConfig)
	if !ok {
		return nil, fmt.Errorf("expected a CappConfig object for the newObj but got %T", newObj)
	}
	cappconfiglog.Info("Validation for CappConfig upon update", "name", cappconfig.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type CappConfig.
func (v *CappConfigCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cappconfig, ok := obj.(*rcsv1alpha1.CappConfig)
	if !ok {
		return nil, fmt.Errorf("expected a CappConfig object but got %T", obj)
	}
	cappconfiglog.Info("Validation for CappConfig upon deletion", "name", cappconfig.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
