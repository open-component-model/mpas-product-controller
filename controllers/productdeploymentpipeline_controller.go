// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"
	"time"

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

	// TODO: REMOVE THIS!!! THIS IS FOR DEMO PURPOSES!!!!!!!
	time.Sleep(10 * time.Second)

	objPatcher := patch.NewSerialPatcher(obj, r.Client)
	conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

	if err := objPatcher.Patch(ctx, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch pipeline object '%s': %w", obj.Name, err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProductDeploymentPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mpasv1alpha1.ProductDeploymentPipeline{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
