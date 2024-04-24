package actionmangers

import (
	"context"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/internal/kinds/capprevision/adapters"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HandleCappCreation creates the initial CappRevision.
func HandleCappCreation(ctx context.Context, k8sClient client.Client, capp cappv1alpha1.Capp, logger logr.Logger) error {
	return adapters.CreateCappRevision(ctx, k8sClient, logger, capp, 1)
}
