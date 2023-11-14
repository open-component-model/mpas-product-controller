// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	rreconcile "github.com/fluxcd/pkg/runtime/reconcile"
	"github.com/open-component-model/ocm-controller/pkg/event"
	"github.com/open-component-model/ocm-controller/pkg/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/open-component-model/ocm-controller/api/v1alpha1"

	mpasv1alpha1 "github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/open-component-model/mpas-product-controller/pkg/deployers"
)

// ProductDeploymentPipelineScheduler reconciles a ProductDeploymentPipeline object and schedules them.
type ProductDeploymentPipelineScheduler struct {
	client.Client
	Scheme              *runtime.Scheme
	MpasSystemNamespace string

	Deployer      deployers.Deployer
	EventRecorder record.EventRecorder
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

	if obj.Status.SnapshotRef == nil || obj.Status.SnapshotRef.Name == "" {
		logger.Info("snapshot has not yet been set up, requeuing...")

		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	snapshot := &v1alpha1.Snapshot{}
	if err := r.Get(ctx, types.NamespacedName{Name: obj.Status.SnapshotRef.Name, Namespace: obj.Status.SnapshotRef.Namespace}, snapshot); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("snapshot not found yet, requeuing")

			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to retrieve snapshot object: %w", err)
	}

	if !conditions.IsTrue(snapshot, meta.ReadyCondition) {
		logger.Info("snapshot found but is not ready yet, requeuing...")

		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	patchHelper := patch.NewSerialPatcher(obj, r.Client)

	// Always attempt to patch the object and status after each reconciliation.
	defer func() {
		// Patching has not been set up, or the controller errored earlier.
		if patchHelper == nil {
			return
		}

		if derr := status.UpdateStatus(ctx, patchHelper, obj, r.EventRecorder, 0); derr != nil {
			err = errors.Join(err, derr)
		}
	}()

	// Starts the progression by setting ReconcilingCondition.
	// This will be checked in defer.
	// Should only be deleted on a success.
	rreconcile.ProgressiveStatus(false, obj, meta.ProgressingReason, "reconciliation in progress for resource: %s", obj.Name)

	target, err := r.SelectTarget(ctx, obj.Spec.TargetRole, obj.Namespace)
	if err != nil {
		conditions.MarkFalse(obj, mpasv1alpha1.DeployedCondition, mpasv1alpha1.PipelineDeploymentFailedReason, err.Error())
		event.New(r.EventRecorder, obj, eventv1.EventSeverityError, err.Error(), nil)

		return ctrl.Result{}, fmt.Errorf("failed to select a target with the given criteria: %w", err)
	}

	// Update the selected target.
	obj.Status.SelectedTargetRef = &meta.NamespacedObjectReference{
		Name:      target.Name,
		Namespace: target.Namespace,
	}

	if err := r.Deployer.Deploy(ctx, obj); err != nil {
		conditions.MarkFalse(obj, mpasv1alpha1.DeployedCondition, mpasv1alpha1.PipelineDeploymentFailedReason, err.Error())
		event.New(r.EventRecorder, obj, eventv1.EventSeverityError, err.Error(), nil)

		return ctrl.Result{}, fmt.Errorf("failed to deploy object to selected target: %w", err)
	}

	conditions.MarkTrue(obj, mpasv1alpha1.DeployedCondition, meta.SucceededReason, "Successfully deployed")
	event.New(r.EventRecorder, obj, eventv1.EventSeverityInfo, "Reconciliation success", nil)

	return ctrl.Result{}, nil
}
