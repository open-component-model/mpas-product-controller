package kubernetes

import (
	"context"
	"fmt"

	kustomizev1beta2 "github.com/fluxcd/kustomize-controller/api/v1beta2"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/open-component-model/mpas-product-controller/pkg/deployers"
	ocmv1alpha1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Deployer can deploy Kubernetes type Targets.
type Deployer struct {
	client client.Client
	next   deployers.Deployer
	scheme *runtime.Scheme
}

// NewDeployer creates a deployer for Kubernetes Targets.
func NewDeployer(client client.Client, scheme *runtime.Scheme, next deployers.Deployer) *Deployer {
	return &Deployer{
		scheme: scheme,
		client: client,
		next:   next,
	}
}

var _ deployers.Deployer = &Deployer{}

func (d *Deployer) Deploy(ctx context.Context, obj *v1alpha1.ProductDeploymentPipeline) error {
	target := obj.Status.SelectedTarget

	if target == nil {
		return fmt.Errorf("selected target on object cannot be empty")
	}

	if target.Spec.Type != v1alpha1.Kubernetes {
		if d.next == nil {
			return fmt.Errorf("cannot deploy target type %s and no next handler is configured", target.Spec.Type)
		}

		return d.next.Deploy(ctx, obj)
	}

	if target.Spec.Access == nil {
		return fmt.Errorf("access needs to be defined for the kubernetes target type")
	}

	snapshot := &ocmv1alpha1.Snapshot{}
	if err := d.client.Get(ctx, types.NamespacedName{
		Name:      obj.Status.SnapshotRef.Name,
		Namespace: obj.Status.SnapshotRef.Namespace,
	}, snapshot); err != nil {
		return fmt.Errorf("failed to find snapshot: %w", err)
	}

	owners := snapshot.GetOwnerReferences()
	if len(owners) != 1 {
		return fmt.Errorf("expected exactly one owner for snapshot but got %d", len(owners))
	}

	owner := owners[0]

	kustomization := &ocmv1alpha1.FluxDeployer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-kustomization",
			Namespace: obj.Namespace,
		},
		Spec: ocmv1alpha1.FluxDeployerSpec{
			SourceRef: ocmv1alpha1.ObjectReference{
				NamespacedObjectKindReference: meta.NamespacedObjectKindReference{
					Name:      owner.Name,
					Namespace: obj.Namespace,
					Kind:      owner.Kind,
				},
			},
			KustomizationTemplate: kustomizev1beta2.KustomizationSpec{
				KubeConfig: &meta.KubeConfigReference{
					SecretRef: meta.SecretKeyReference{
						Name: target.Spec.Access.SecretRef.Name,
					},
				},
				Prune:           true,
				TargetNamespace: obj.Namespace, //TODO: This needs to come from somewhere.
			},
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, d.client, kustomization, func() error {
		if kustomization.ObjectMeta.CreationTimestamp.IsZero() {
			if err := controllerutil.SetOwnerReference(obj, kustomization, d.scheme); err != nil {
				return fmt.Errorf("failed to set owner to kustomization object: %w", err)
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to create kustomization: %w", err)
	}

	return nil
}
