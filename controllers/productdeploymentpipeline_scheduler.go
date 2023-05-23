// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	"github.com/open-component-model/mpas-product-controller/pkg/deployers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	mpasv1alpha1 "github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

// ProductDeploymentPipelineScheduler reconciles a ProductDeploymentPipeline object and schedules them.
type ProductDeploymentPipelineScheduler struct {
	client.Client
	Scheme              *runtime.Scheme
	MpasSystemNamespace string

	Deployer deployers.Deployer
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProductDeploymentPipelineScheduler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mpasv1alpha1.ProductDeploymentPipeline{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ProductDeploymentPipelineScheduler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	logger := log.FromContext(ctx).WithName("pipeline-scheduler")

	logger.Info("scheduling pipeline object", "pipeline", req.NamespacedName)

	obj := &mpasv1alpha1.ProductDeploymentPipeline{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to find pipeline deployment object: %w", err)
	}

	if !conditions.IsReady(obj) || obj.Status.SnapshotRef == nil || obj.Status.SnapshotRef.Name == "" {
		logger.Info("pipeline not ready yet to be scheduler")

		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	objPatcher := patch.NewSerialPatcher(obj, r.Client)

	// Always attempt to patch the object and status after each reconciliation.
	defer func() {
		// Patching has not been set up, or the controller errored earlier.
		if objPatcher == nil {
			return
		}

		// Set status observed generation option if the object is stalled or ready.
		if conditions.IsStalled(obj) || conditions.IsReady(obj) {
			obj.Status.ObservedGeneration = obj.Generation
		}

		if perr := objPatcher.Patch(ctx, obj); perr != nil {
			err = errors.Join(err, perr)
		}
	}()

	target, err := r.FilterTarget(ctx, obj.Spec.TargetRole, r.MpasSystemNamespace)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, mpasv1alpha1.PipelineDeploymentFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to select a target with the given criteria: %w", err)
	}

	// Update the selected target.
	obj.Status.SelectedTargetRef = &meta.NamespacedObjectReference{
		Name:      target.Name,
		Namespace: target.Namespace,
	}

	if err := r.Deployer.Deploy(ctx, obj); err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, mpasv1alpha1.PipelineDeploymentFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to deploy object to selected target: %w", err)
	}

	conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

	return ctrl.Result{}, nil
}
