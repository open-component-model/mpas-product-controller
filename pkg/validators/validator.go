package validators

import (
	"context"

	deliveryv1alpha1 "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
	gitv1alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
)

// Validator validates a pull request.
type Validator interface {
	FailValidation(ctx context.Context, repository gitv1alpha1.Repository, sync deliveryv1alpha1.Sync) error
	PassValidation(ctx context.Context, repository gitv1alpha1.Repository, sync deliveryv1alpha1.Sync) error
}
