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
	"github.com/fluxcd/pkg/runtime/jitter"
	"github.com/fluxcd/pkg/runtime/patch"
	"github.com/fluxcd/pkg/runtime/predicates"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	kuberecorder "k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	rreconcile "github.com/fluxcd/pkg/runtime/reconcile"
	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

var targetOwnedConditions = []string{
	meta.ReadyCondition,
	meta.ReconcilingCondition,
	meta.StalledCondition,
}

// getPatchOptions composes patch options based on the given parameters.
// It is used as the options used when patching an object.
func getPatchOptions(ownedConditions []string, controllerName string) []patch.Option {
	return []patch.Option{
		patch.WithOwnedConditions{Conditions: ownedConditions},
		patch.WithFieldOwner(controllerName),
	}
}

// TargetReconciler reconciles a Target object
type TargetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	kuberecorder.EventRecorder
	ControllerName string
	patchOptions   []patch.Option
}

// SetupWithManager sets up the controller with the Manager.
func (r *TargetReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	r.patchOptions = getPatchOptions(targetOwnedConditions, r.ControllerName)
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Target{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicates.ReconcileRequestedPredicate{}),
		)).
		Complete(r)
}

//+kubebuilder:rbac:groups=mpas.ocm.software,resources=Targets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=Targets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=Targets/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Named return values: It's used for improving readability when dealing with the defer patch statement.
func (r *TargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	logger := log.FromContext(ctx)

	logger.V(4).Info("starting reconciliation", "target", req.NamespacedName)

	obj := &v1alpha1.Target{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	serialPatcher := patch.NewSerialPatcher(obj, r.Client)

	// Always attempt to patch the object and status after each reconciliation.
	defer func() {
		if obj == nil {
			return
		}

		patchOpts := []patch.Option{}
		patchOpts = append(patchOpts, r.patchOptions...)

		// Set status observed generation option if the object is stalled, or
		// if the object is ready.
		if conditions.IsStalled(obj) || conditions.IsReady(obj) {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}

		if err := serialPatcher.Patch(ctx, obj, patchOpts...); err != nil {
			// Ignore patch error "not found" when the object is being deleted.
			if !obj.GetDeletionTimestamp().IsZero() {
				err = kerrors.FilterOut(err, func(e error) bool { return apierrors.IsNotFound(e) })
			}
			retErr = kerrors.NewAggregate([]error{retErr, err})
		}

	}()

	result, retErr = r.reconcile(ctx, serialPatcher, obj)
	return
}

func (r *TargetReconciler) reconcile(ctx context.Context, sp *patch.SerialPatcher, obj *v1alpha1.Target) (result ctrl.Result, retErr error) {
	oldObj := obj.DeepCopy()

	defer func() {
		// If it's stalled, ensure reconciling is removed.
		if sc := conditions.Get(obj, meta.StalledCondition); sc != nil && sc.Status == metav1.ConditionTrue {
			conditions.Delete(obj, meta.ReconcilingCondition)
		}

		// Check if it's a successful reconciliation.
		if result.RequeueAfter == obj.GetRequeueAfter() && !result.Requeue && retErr == nil {
			conditions.Delete(obj, meta.ReconcilingCondition)
			if ready := conditions.Get(obj, meta.ReadyCondition); ready != nil &&
				ready.Status == metav1.ConditionFalse && !conditions.IsStalled(obj) {
				retErr = errors.New(conditions.GetMessage(obj, meta.ReadyCondition))
			}
		}

		if conditions.IsReconciling(obj) {
			reconciling := conditions.Get(obj, meta.ReconcilingCondition)
			reconciling.Reason = meta.ProgressingWithRetryReason
			conditions.Set(obj, reconciling)
		}

		// If it's still a successful reconciliation and it's not reconciling or
		// stalled, mark Ready=True.
		if !conditions.IsReconciling(obj) && !conditions.IsStalled(obj) &&
			retErr == nil && result.RequeueAfter == obj.GetRequeueAfter() {
			conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Target is ready")
		}

		// Emit events when object's state changes.
		ready := conditions.Get(obj, meta.ReadyCondition)
		// Became ready from not ready.
		if !conditions.IsReady(oldObj) && conditions.IsReady(obj) {
			r.eventLogf(ctx, obj, corev1.EventTypeNormal, ready.Reason, ready.Message)
		}
		// Became not ready from ready.
		if conditions.IsReady(oldObj) && !conditions.IsReady(obj) {
			r.eventLogf(ctx, obj, corev1.EventTypeWarning, ready.Reason, ready.Message)
		}

		// Apply jitter.
		if result.RequeueAfter == obj.GetRequeueAfter() {
			result.RequeueAfter = jitter.JitteredIntervalDuration(result.RequeueAfter)
		}
	}()

	// delete reconciling and stalled conditions if they exis
	rreconcile.ProgressiveStatus(false, obj, meta.ProgressingReason, "reconciliation in progress")

	if obj.Generation != obj.Status.ObservedGeneration {
		rreconcile.ProgressiveStatus(false, obj, meta.ProgressingReason,
			"processing object: new generation %d -> %d", obj.Status.ObservedGeneration, obj.Generation)
		if err := sp.Patch(ctx, obj, r.patchOptions...); err != nil {
			result, retErr = ctrl.Result{}, err
			return
		}
	}

	var kubernetesAccess v1alpha1.KubernetesAccess
	if err := yaml.Unmarshal(obj.Spec.Access.Raw, &kubernetesAccess); err != nil {
		conditions.MarkStalled(obj, v1alpha1.AccessInvalidReason, err.Error())
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.AccessInvalidReason, err.Error())
		result, retErr = ctrl.Result{}, err
		return
	}

	conditions.Delete(obj, meta.StalledCondition)

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: kubernetesAccess.TargetNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ns, func() error {
		return nil
	})

	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.NamespaceCreateOrUpdateFailedReason, err.Error())
		return ctrl.Result{}, fmt.Errorf("error reconciling namespace: %w", err)
	}

	// get secrets with the target label if it exists
	secrets := &corev1.SecretList{}
	if obj.Spec.SecretsSelector != nil {
		if err = r.List(ctx, secrets, client.MatchingLabels(obj.Spec.SecretsSelector.MatchLabels), client.InNamespace(ns.Name)); err != nil {
			conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.SecretRetrievalFailedReason, err.Error())
			return ctrl.Result{}, fmt.Errorf("error retrieving secrets: %w", err)
		}
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Spec.ServiceAccountName,
			Namespace: kubernetesAccess.TargetNamespace,
		},
	}

	// if the secrets exist, add it to the service account
	if len(secrets.Items) > 0 {
		for _, secret := range secrets.Items {
			sa.ImagePullSecrets = append(sa.ImagePullSecrets, corev1.LocalObjectReference{
				Name: secret.Name,
			})
		}
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		return nil
	})

	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ServiceAccountCreateOrUpdateFailedReason, err.Error())
		return ctrl.Result{}, fmt.Errorf("error reconciling service account: %w", err)
	}

	// Remove any stale Ready condition, most likely False, set above. Its value
	// is derived from the overall result of the reconciliation in the deferred
	// block at the very end.
	conditions.Delete(obj, meta.ReadyCondition)

	result, retErr = ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	return
}

func (r *TargetReconciler) eventLogf(ctx context.Context, obj runtime.Object, eventType string, reason string, messageFmt string, args ...interface{}) {
	msg := fmt.Sprintf(messageFmt, args...)
	// Log and emit event.
	if eventType == corev1.EventTypeWarning {
		ctrl.LoggerFrom(ctx).Error(errors.New(reason), msg)
	} else {
		ctrl.LoggerFrom(ctx).Info(msg)
	}
	r.Eventf(obj, eventType, reason, msg)
}
