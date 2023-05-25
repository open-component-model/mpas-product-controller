package deployers

import (
	"context"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

// Deployer takes a pipeline and a role and creates the necessary objects for it to be deployed.
type Deployer interface {
	Deploy(ctx context.Context, obj *v1alpha1.ProductDeploymentPipeline) error
}
