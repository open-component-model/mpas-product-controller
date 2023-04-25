// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"fmt"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	"github.com/open-component-model/mpas-product-controller/pkg/ocm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

const (
	// ProductDescriptionType defines the type of the ProductDescription resource in the component version.
	ProductDescriptionType = "productdescription.mpas.ocm.software"

	// MaximumNumberOfConcurrentPipelineRuns defines how many concurrent running pipelines there can be.
	MaximumNumberOfConcurrentPipelineRuns = 10
)

var productDescription = `apiVersion: meta.mpas.ocm.software/v1alpha1
kind: ProductDescription
metadata:
  name: sample-descriptor
spec:
  description: string
  pipelines:
  - name: backend
    source:
      name: deployment
      version: 0.0.1
    validation:
      name: validation-example
      version: 0.0.1
`

// ProductDeploymentGeneratorReconciler reconciles a ProductDeploymentGenerator object
type ProductDeploymentGeneratorReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	OCMClient ocm.Client
}

//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentgenerators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentgenerators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentgenerators/finalizers,verbs=update
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ProductDeploymentGeneratorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	logger.V(4).Info("starting product deployment generator loop")

	obj := &v1alpha1.ProductDeploymentGenerator{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
	}

	var patchHelper *patch.Helper
	if patchHelper, err = patch.NewHelper(obj, r.Client); err != nil {
		return
	}

	// Always attempt to patch the object and status after each reconciliation.
	defer func() {
		// Patching has not been set up, or the controller errored earlier.
		if patchHelper == nil {
			return
		}

		if condition := conditions.Get(obj, meta.StalledCondition); condition != nil && condition.Status == metav1.ConditionTrue {
			conditions.Delete(obj, meta.ReconcilingCondition)
		}

		// Check if it's a successful reconciliation.
		// We don't set Requeue in case of error, so we can safely check for Requeue.
		if result.RequeueAfter == obj.GetRequeueAfter() && !result.Requeue && err == nil {
			// Remove the reconciling condition if it's set.
			conditions.Delete(obj, meta.ReconcilingCondition)

			// Set the return err as the ready failure message if the resource is not ready, but also not reconciling or stalled.
			if ready := conditions.Get(obj, meta.ReadyCondition); ready != nil && ready.Status == metav1.ConditionFalse && !conditions.IsStalled(obj) {
				err = errors.New(conditions.GetMessage(obj, meta.ReadyCondition))
			}
		}

		// If still reconciling then reconciliation did not succeed, set to ProgressingWithRetry to
		// indicate that reconciliation will be retried.
		if conditions.IsReconciling(obj) {
			reconciling := conditions.Get(obj, meta.ReconcilingCondition)
			reconciling.Reason = meta.ProgressingWithRetryReason
			conditions.Set(obj, reconciling)
		}

		// If not reconciling or stalled than mark Ready=True
		if !conditions.IsReconciling(obj) &&
			!conditions.IsStalled(obj) &&
			err == nil &&
			result.RequeueAfter == obj.GetRequeueAfter() {
			conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")
		}
		// Set status observed generation option if the component is stalled or ready.
		if conditions.IsStalled(obj) || conditions.IsReady(obj) {
			obj.Status.ObservedGeneration = obj.Generation
		}

		// Update the object.
		if perr := patchHelper.Patch(ctx, obj); perr != nil {
			err = errors.Join(err, perr)
		}
	}()

	// Remove any stale Ready condition, most likely False, set above. Its value
	// is derived from the overall result of the reconciliation in the deferred
	// block at the very end.
	conditions.Delete(obj, meta.ReadyCondition)

	return r.reconcile(ctx, obj)
}

func (r *ProductDeploymentGeneratorReconciler) reconcile(ctx context.Context, obj *v1alpha1.ProductDeploymentGenerator) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	subscription := &replicationv1.ComponentSubscription{}

	if err := r.Get(ctx, types.NamespacedName{
		Name:      obj.Spec.SubscriptionRef.Name,
		Namespace: obj.Spec.SubscriptionRef.Namespace,
	}, subscription); err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ComponentSubscriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to find subscription object: %w", err)
	}

	if !conditions.IsReady(subscription) {
		logger.Info("referenced subscription isn't ready yet, requeuing")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	component := subscription.GetComponentVersion()
	logger.Info("fetching component", "component", component)

	octx, err := r.OCMClient.CreateAuthenticatedOCMContext(ctx, obj.Spec.ServiceAccountName, obj.Namespace)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.OCMAuthenticationFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to authenticate using service account: %w", err)
	}

	cv, err := r.OCMClient.GetComponentVersion(ctx, octx, component.Registry.URL, component.Name, component.Version)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ComponentVersionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to authenticate using service account: %w", err)
	}

	logger.Info("retrieved component version, fetching ProductDescription resource", "component", cv.GetName())

	// TODO: Insert OCM resource fetch here.
	prodDesc := &v1alpha1.ProductDescription{}
	if err := yaml.Unmarshal([]byte(productDescription), prodDesc); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to unmarshal product description: %w", err)
	}

	productDeployment := &v1alpha1.ProductDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prodDesc.Name,
			Namespace: obj.Namespace,
		},
	}

	spec := v1alpha1.ProductDeploymentSpec{}

	for _, p := range prodDesc.Spec.Pipelines {
		spec.Pipelines = append(spec.Pipelines, r.createProductPipeline(p))
	}

	productDeployment.Spec = spec

	if err := r.Create(ctx, productDeployment); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create product deployment object: %w", err)
	}

	logger.Info("successfully generated product deployment", "productDeployment", klog.KObj(productDeployment))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProductDeploymentGeneratorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ProductDeploymentGenerator{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

func (r *ProductDeploymentGeneratorReconciler) createProductPipeline(p v1alpha1.ProductDescriptionPipeline) v1alpha1.Pipeline {
	var result v1alpha1.Pipeline
	// Gather Source
	result.Name = p.Name

	// Gather Localization

	// Gather Configuration

	return result
}
