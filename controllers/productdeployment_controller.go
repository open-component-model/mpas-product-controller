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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

// ProductDeploymentReconciler reconciles a ProductDeployment object
type ProductDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeployments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Named return values: It's used for improving readability when dealing with the defer patch statement.
func (r *ProductDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	logger.V(4).Info("starting reconcile loop for product deployment")

	obj := &v1alpha1.ProductDeployment{}
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
		if err == nil {
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
			obj.IsDone() &&
			err == nil {
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

	// update the total number of pipelines this object needs to create and requeue.
	if obj.Status.Total == 0 {
		obj.Status.Total = len(obj.Spec.Pipelines)

		return ctrl.Result{Requeue: true}, nil
	}

	if obj.Status.Total == obj.Status.Active {
		logger.Info("all pipeline objects are already running, nothing to do...")
		return ctrl.Result{}, nil
	}

	// Remove any stale Ready condition, most likely False, set above. Its value
	// is derived from the overall result of the reconciliation in the deferred
	// block at the very end.
	conditions.Delete(obj, meta.ReadyCondition)

	// TODO: Figure out how to mark it as ready once all its pipeline objects are finished.
	// The owner should reconcile if it's updated by one of its children. But shouldn't be marked as done
	// until Success+Fail == Total.
	// Added IsDone. That might be enough to track progress.
	// Also need a way to requeue this object for further processing that is based on a ping instead of a requeue after.
	return r.reconcile(ctx, obj)
}

func (r *ProductDeploymentReconciler) reconcile(ctx context.Context, obj *v1alpha1.ProductDeployment) (result ctrl.Result, err error) {
	// TODO: if the reconciliation failed, we have to delete ALL created objects, otherwise they will mess up
	// the success / failed count of the parent object.
	logger := log.FromContext(ctx)

	logger.Info("preparing to create pipeline objects")

	var createdSoFar []*v1alpha1.ProductDeploymentPipeline

	defer func() {
		if err != nil {
			logger.V(4).Info("pipeline creation failed, cleaning up all potentially created pipelines so far")
			for _, pb := range createdSoFar {
				if cerr := r.Delete(ctx, pb); cerr != nil {
					logger.Error(err, "failed to cleanup pipeline item %s: %w", pb.Name, cerr)
				}
			}
		}
	}()

	active := 0
	// Create all the pipeline objects.
	for _, pipeline := range obj.Spec.Pipelines {
		pobj := &v1alpha1.ProductDeploymentPipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pipeline.Name,
				Namespace: obj.Namespace,
			},
			Spec: v1alpha1.ProductDeploymentPipelineSpec{
				Resource:      pipeline.Resource,
				Localization:  pipeline.Localization,
				Configuration: pipeline.Configuration,
				TargetRole:    pipeline.TargetRole,
				Interval:      metav1.Duration{Duration: 10 * time.Second},
			},
		}

		if err := controllerutil.SetOwnerReference(obj, pobj, r.Scheme); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set owner reference: %w", err)
		}

		if err := r.Create(ctx, pobj); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create pipeline object: %w", err)
		}

		logger.V(4).Info("pipeline object successfully created", "name", pobj.Name)
		createdSoFar = append(createdSoFar, pobj)
		active++
	}

	obj.Status.Active = active
	logger.Info("all pipeline objects successfully created")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProductDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ProductDeployment{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
