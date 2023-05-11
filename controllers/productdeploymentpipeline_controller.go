// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	mpasv1alpha1 "github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

// ProductDeploymentPipelineReconciler reconciles a ProductDeploymentPipeline object
type ProductDeploymentPipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentpipelines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentpipelines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentpipelines/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ProductDeploymentPipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("reconciling pipeline object", "pipeline", req.NamespacedName)

	obj := &mpasv1alpha1.ProductDeploymentPipeline{}

	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to find pipeline deployment object: %w", err)
	}

	owners := obj.GetOwnerReferences()
	if len(owners) != 1 {
		return ctrl.Result{}, fmt.Errorf("expected exactly one owner, got: %d", len(owners))
	}

	owner := &mpasv1alpha1.ProductDeployment{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      owners[0].Name,
		Namespace: obj.Namespace,
	}, owner); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to find the owner: %w", err)
	}

	if owner.Status.Active != owner.Status.Total {
		logger.Info("owner isn't ready with creating all the pipelines yet")
		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	// TODO: Do this in a defer so the state of the owner is always updated if this pipeline fails.
	// If the pipeline fails, it shouldn't increase the failed count again.
	objPatcher := patch.NewSerialPatcher(obj, r.Client)
	conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

	if err := objPatcher.Patch(ctx, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch pipeline object '%s': %w", obj.Name, err)
	}

	if err := r.increaseOwnerSuccess(ctx, owner); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update owner object '%s', try again.. %w", owner.Name, err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProductDeploymentPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mpasv1alpha1.ProductDeploymentPipeline{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

func (r *ProductDeploymentPipelineReconciler) increaseOwnerSuccess(ctx context.Context, owner *mpasv1alpha1.ProductDeployment) error {
	// refresh owner state
	if err := r.Get(ctx, types.NamespacedName{
		Name:      owner.Name,
		Namespace: owner.Namespace,
	}, owner); err != nil {
		return fmt.Errorf("failed to find the owner: %w", err)
	}

	patcher := patch.NewSerialPatcher(owner, r.Client)

	owner.Status.Succeeded++
	if err := patcher.Patch(ctx, owner); err != nil {
		return fmt.Errorf("failed to patch owner object '%s': %w", owner.Name, err)
	}

	return nil
}
